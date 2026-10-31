# E2E — Playwright contra o app em modo servidor

Testes de ponta a ponta do **FFX Resources Editor rodando como servidor
HTTP** (Wails v3 `-tags server`, ver [Server Build](https://v3.wails.io/guides/server-build/)):
o `spira-server.exe` serve o frontend em `http://localhost:9246` (porta
própria — 8080 é a default do Wails e a mais batida em dev web) e os
bindings/eventos atravessam `/wails/runtime` no mesmo origem. O navegador É
a janela: tudo que é nativo (menus do appcore, seletores de arquivo do SO)
não existe aqui — os fluxos que dependem de picker terminam em
cancelamento limpo, e é isso que os testes verificam.

## Como rodar

```sh
# uma vez: dependências do Playwright + Chromium
wails3 task install:e2e

# sempre: builda o servidor (DEV) e roda a suíte
wails3 task test:e2e
```

O Playwright sobe o `build/bin/spira-server.exe` sozinho (espera o
`/health` — a porta TCP aceita conexões antes de o app terminar de
carregar o config) e o **encerra no fim do run** — o servidor é
EXCLUSIVO da suíte. Se a porta já estiver ocupada (app em modo servidor,
`wails3 task run:server` manual), a suíte **aborta no início** e pede
para encerrar o processo e reexecutar: um servidor alheio valeria um
binário velho como alvo. A barreira é dupla: a precondition da task
`test:e2e` (mensagem em português, ANTES do build) e a probe no
`playwright.config.ts` (só no runner principal — loader/workers reavaliam
o config depois de o servidor da própria suíte já estar no ar), que
vale também para `npx playwright test` direto.

Direto, sem Task:

```sh
cd e2e
npm install
npx playwright test            # ou: --headed / --ui / --grep "editor"
```

## Regra dos fixtures: teste por EXISTÊNCIA

A suíte lê `build/bin/config/config.json` (a mesma fonte de verdade do
binário) e adapta os testes ao que existe na máquina:

| Fixture | Existe | Não existe |
| --- | --- | --- |
| `data/` (GameFilesLocation) | exibição de conteúdo: coluna **Original obrigatória** | specs de conteúdo/editor pulados |
| `mods/` | coluna **Traduzido** viva + prova do save em mods/ | só o teste de escrita/salvar em binário (que cria mods/ na hora e o remove na limpeza) |
| `.vbf` (ao lado do FFX.exe) | navegação do container: texto, textura, diálogo de extração | a árvore precisa mostrar o **erro pedindo para apontar o executável** |

O `.vbf` é o mínimo requerido (o jogo inteiro mora no container); ainda
assim os dois estados são sempre testados — o spec limpa o executável pelo
diálogo de Configurações, confere o erro e devolve o valor original.

## O que cada spec cobre

- `app-shell.spec` — casca: título, abas de versão (Ctrl+1..4), tema,
  barra de status, estado vazio e **página sem estouro horizontal**;
- `config-dialog.spec` — diálogo de Configurações: campos espelhando o
  config.json, pickers nativos virando cancelamento no modo servidor;
- `content.spec` — exibição: árvore de kinds, Original obrigatório,
  Traduzido quando mods/ existe (badge de progresso > 0);
- `editor-save.spec` — **o ciclo de escrita**: editar → Salvar (Ctrl+S) →
  binário regravado em mods/ (varredura de mtime) → UI recarregada
  exibindo o texto salvo → export pelo menu de contexto da árvore
  (artefatos em mods/edits com modal de progresso) → import do lote
  translated (bin regravado em mods/);
- `editor-focus.spec` — **o foco do editor**: digitar várias letras e colar
  NÃO podem tirar o foco do campo de edição (o editor nasce uma vez e nunca
  é recriado quando o rascunho muda); fecha com Cancelar (nada é salvo);
- `export-scopes.spec` — **os três tipos de export**: o popover Exportar
  ("todos os tipos" e "Exportar seleção" = os **tipos marcados no
  escopo**, cada um por inteiro — a marcação da árvore não entra) e o menu
  de contexto da árvore (a **seleção marcada**: parcial = um arquivo por
  entrada; kind inteiro marcado pela raiz = artefato bulk único com
  prefixo `tree_`, que não sobrescreve o canônico). Inclui a marcação
  tri-state das raízes de kind (`Texto` e `Imagens` ficam sem checkbox) e
  a extensão do artefato por formato (JSON/Strings/CSV/XLSX);
- `dialogs.spec` — todos os diálogos: busca global, busca no arquivo,
  menus Exportar/Importar, import translated, editor (cancelar não suja),
  painel de imagem (extrair/deletar/importar traduzidas/replicar) — cada
  um contido na viewport (**sem estouro de conteúdo**);
- `import-images-dialog.spec` — **o modal de importação de imagens
  traduzidas** (`mods/translated/images`): abre pelo dropdown Importar do
  painel, mostra a origem completa, o "Todas" em 3 estados e a legenda das
  cores; lista os caminhos relativos à raiz (com extensão, sem o caminho
  absoluto); as iguais ao binário nascem desmarcadas; desmarcar uma cópia
  do collapse deixa a linha indeterminada e o "Todas" consolida; confirma
  gravando **só a seleção** em `mods/` (com backup/restauração do fixture)
  — a seleção só é exercitada quando há `.dds.phyre` na pasta traduzida;
- `mods-toggle-navigation.spec` — **alternar o toggle "Preferir binários
  traduzidos (mods)" e seguir navegando**: com um texto já aberto (a
  seleção cai na recarga automática e a mesma entrada reabre, trocando
  depois para outro arquivo) e sem nada aberto (o primeiro clique
  carrega). É a guarda e2e do self-deadlock do view dedupado
  (`rawViewOf` × `clearDedupViewCache`) — ver
  `backend/services/rawview_reentrant_clear_test.go`;
- `mods-toggle-guard.spec` — **a guarda de edições não salvas do toggle
  de mods**: com rascunho sujo, o checkbox pergunta antes de aplicar —
  Cancelar devolve o checkbox e preserva o rascunho (nada recarrega);
  Descartar/Salvar aplicam o toggle e recarregam todo o conteúdo (o
  descartado morre, o salvo vai para o binário em mods/ e volta na
  recarga com a tradução ligada);
- `export-all-disabled-mods.spec` — **export bulk com o toggle de mods
  desligado**: "Exportar — todos os tipos" lê os ORIGINAIS de data/ e
  grava TUDO em `mods/edits` (artefatos `.strings`, nada fora de edits/; o
  mainmenu sai em inglês no `text`/`source` e com a tradução no `target`) —
  é a guarda e2e do carregamento completo dos eventos no export bulk
  (`ensureAllEventsLoaded`);
- `export-import-roundtrip.spec` — **o par source/target do formato CAT no
  ambiente real**: exporta um kind (Textos de batalha) em JSON, confere
  `source` (data/) e `target` (mods/) no artefato, isola o lote
  `mods/translated/text` com ele e importa SEM edição — a prévia tem 0
  textos alterados, nenhuma entrada/arquivo pulado e nenhum binário é
  regravado; o `PreviewImport` do mesmo artefato (binding pelo runtime do
  Wails) devolve `skipped` vazio (guarda do falso positivo de tag de
  controle em row no-op). O fixture `translated/text` é copiado antes e
  restaurado arquivo a arquivo no fim;
- `vbf.spec` — raízes do container, arquivo de texto e textura do
  container, diálogo de extração e o erro "aponte o executável".

## Fixture intocado

Os testes de escrita **gravam de verdade** (é o que validam) e depois
devolvem o fixture ao estado original:

- `mods/` é copiado antes e restaurado byte a byte no fim do spec de
  editor (criações quando mods/ não existia são removidas);
- `config.json` é restaurado no fim dos specs que o tocam (troca de aba
  persiste a versão; os testes de VBF apagam/devolvem o executável);
- nada é escrito em `data/` — extrações do .vbf são canceladas no diálogo
  de confirmação.

**Importante:** o fixture (`build/bin/config/config.json`, `data/`) é
compartilhado com o app desktop. Não rode a suíte com o `spira.exe`
aberto — o app regrava o config a qualquer setter e pode disputar o
arquivo com os testes (o mesmo vale para um `spira-server.exe` /
`wails3 task run:server` aberto: com a porta ocupada a suíte aborta no
início — encerre-o antes de reexecutar).

## Porta e overrides

`9246` (definida no `ServerOptions` do `main.go`, seguindo a faixa do dev
server `9245`). Variáveis do Wails valem em runtime sem rebuild:
`WAILS_SERVER_HOST` / `WAILS_SERVER_PORT` (e `E2E_PORT` para a suíte
acompanhar um servidor fora da porta padrão).
