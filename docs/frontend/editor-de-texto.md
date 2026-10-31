# Editor de texto

O coração da app: como os textos do jogo são editados. Baseado em TipTap
(ProseMirror) — `frontend/src/components/editor/`.

## Como o texto do jogo aparece

Os binários do jogo não têm texto puro: têm **tags de controle** entre as
palavras — cores, botões do controle, macros, ícones, variáveis. O editor
renderiza cada tag como um **chip** no meio do texto:

```
{CLR:BLUE}Potion{/CLR} restores {BUTTON:31} HP.
   ↓ vira no editor:
[Potion] com texto azul, [▮A] com o glifo do botão, HP.
```

Cada chip é um nó atômico: o texto ao redor é editável normalmente, o chip
é um bloco.

## O que é editável e o que é protegido

| Tipo de tag | Exemplo | Pode editar? |
|---|---|---|
| Texto livre | qualquer coisa fora de `{...}` | sim |
| Tag de valor | `{BUTTON:31:X}` (botão), `{CLR:BLUE}` | sim — inserir/remover pela toolbar |
| Tag calculada | CMD, HEX, UNK, VAR (valores que o jogo calcula) | **não** — chip bloqueado |

- Chips bloqueados (CMD/HEX/UNK/VAR): Backspace/Delete sobre eles é
  vetado (`locked-tags.extension.ts`). Se um deles **sumir** do texto
  editado (recorte, colar por cima), o salvamento é **bloqueado** até
  restaurar — o gate avisa na hora e o diálogo de edição tem os botões
  **Restaurar/Corrigir**.
- Tags digitadas que não existem na gramática do jogo viram chip
  **inválido** (visível de imediato) e são sinalizadas no mesmo gate.

## Toolbar do editor

![Diálogo de tradução com toolbar, chips renderizados e idiomas de referência](../imagens/editor-dialogo.png)

No diálogo de tradução (Enter na linha):

- **Itálico** — itálico do jogo.
- **Botões** — menu com os glifos do controle (triângulo/círculo/X/quadrado
  na arte do jogo, a partir da sprite `public/pad_icon.png`).
- **Direcionais** — setas/D-pad/gates.
- **Cores** (`{CLR:...}`) — paleta das cores conhecidas do jogo +
  reset para branco. Qualquer valor de cor de fora da paleta (ex.: `rgb(...)`
  vindo de HTML colado) é aproximado para a cor do jogo mais próxima na
  serialização — nunca vira `{CLR:RGB(...)}`.
- **Desfazer/Refazer**.
- **Padding de tela** — espaços/quebras intencionais no início do texto são
  preservados automaticamente e exibidos como aviso (não são conteúdo).

O duplo clique seleciona a palavra **sem** o espaço final: a seleção nativa do
Chromium (palavra + espaço) é aparada para aplicar cor/itálico não alterar o
espaço seguinte.

## Autocomplete de tags

![Popup de autocomplete com PC, MCR, BUTTON e ICON](../imagens/editor-autocomplete.png)

Digitar `{` abre a sugestão em dois estágios:

1. **Estágio 1**: nomes de tag (PC, MCR, BUTTON, ICON...);
2. **Estágio 2**: após `TAG:`, os valores válidos do catálogo
   (`GetTagCatalog` do backend), com filtro pelo que foi digitado.

Concluir insere sempre o chip atômico — nunca texto solto que o jogo não
reconheceria.

## Colar de fora

Colar texto externo é higienizado: tags que sobreviveriam mal ao
round-trip são convertidas ou marcadas inválidas — o gate de tags
protegidas do salvamento é a rede final.

A colagem é **inline**: uma linha colada entra no parágrafo do cursor, sem
criar quebra de linha antes/depois. Quebras reais no meio do texto colado
continuam virando parágrafos, e `\n` nas pontas do clipboard (comum ao copiar
uma linha inteira) é descartado.

## Foco: o editor nunca é recriado

O editor (Tiptap) é criado **uma vez** e sobrevive às edições: o diálogo
guarda o rascunho a cada tecla/colagem, então o texto muda o tempo todo. Se o
editor fosse recriado a cada mudança, o `contenteditable` sairia do DOM e o
foco iria para o corpo da página — o tradutor teria que clicar de novo a
cada letra.

Por isso o efeito que cria o editor **não** depende do texto. Valor que vem
de fora (troca de linha, revisão) é aplicado pelo efeito de sincronia
(`setContent`), que preserva cursor e seleção.

Guards: `frontend/src/components/editor/game-text-editor.test.tsx` (o editor
não pode ser recriado quando o valor muda) e
`e2e/specs/editor-focus.spec.ts` (o foco fica no editor ao digitar e ao colar).

## Linhas órfãs (só em `mods/`) e a poda no save

A tabela é a UNIÃO de `data/` e `mods/`: uma linha que só existe na tradução
aparece com a coluna Original vazia (vermelha). O caso clássico é o
`macrodic.dcp` traduzido com ponteiros `simplified` apontando para dentro do
standard ("Zanarkand Abes" → "anarkand Abes"): o binário ganha uma linha
`name_simplified` que o original não tem.

- **Exclusão manual**: a linha órfã tem um botão de lixeira na célula
  Traduzido; o modal mostra índice, campo e o texto completo e avisa que a
  gravação é permanente no próximo Salvar.
- **Poda no save**: ao salvar, TODAS as linhas órfãs que sobraram são
  excluídas do binário. O frontend mostra a lista completa (com rolagem)
  antes de continuar; Cancelar aborta o save inteiro.
- **Guarda**: as linhas do original são intocáveis — o binário não aceita
  menos linhas que o original (`builders.PruneNotInOriginal` aborta se
  faltar alguma; o preenchimento da tradução parcial usa as originais).
- **Log**: até 5 linhas, o log mostra uma linha por exclusão com o texto
  completo (arquivo · bloco · índice); acima disso, um resumo por arquivo. O
  arquivo de diagnóstico (`logs/ffx-*.log`, chave `save-poda/…`) guarda
  sempre a lista completa.

Guards: `builders/prune_test.go`, `builders/prune_dictionary_test.go` (fixture
real do dicionário) e `e2e/specs/dictionary-prune.spec.ts`.

## Como o texto chega ao binário

1. Edição na célula ou no diálogo → **rascunho** (`edit-draft.ts`), nada
   é gravado ainda.
2. **Salvar** (`Ctrl+S` ou botão da aba) → o gate de tags protegidas roda
   (`save-all.ts`):
   - restaura tags que sumiram e avisa;
   - **bloqueia** o salvamento se ainda houver tag obrigatória ausente ou
     `{` aberto.
3. Rascunhos são agrupados por versão/kind (e por raiz `.vbf`, quando a
   tabela veio do container) e aplicados via `ApplyTextCollection` /
   `ApplyVbfTextCollection` — o backend recalcula os ponteiros
   ([motor de strings](../motor-strings.md)) e grava em `mods/`.
