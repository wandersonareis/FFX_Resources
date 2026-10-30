# Exportação JSON / Strings

Os artefatos de texto nativos do editor: o **JSON** — estrutura completa
(metadados + textos por idioma), para backup e ferramentas externas — e o
**.strings** — o mesmo conteúdo em texto-para-humanos, uma linha por
(row, idioma). Ambos serializam o MESMO DTO (`backend/dto`); o
[CSV / XLSX](exportacao-csv-xlsx.md) é a versão tabular desses mesmos
dados.

Pacotes: `backend/formatters/json` e `backend/formatters/strings`. Ao
contrário do CSV/XLSX, NÃO vivem no registry (`formatters.Register`): o
JSON é chamado direto, um tipo por kind (`JSONEventsFormatter`,
`JSONObjectFormatter`, `JSONMacroFormatter`, `JSONHelpFormatter` — todos
com a MESMA serialização, só os writers/caminhos diferem); o .strings tem
um `StringsFormatter` único. Os arquivos .strings nascem AO LADO dos
.json (mesmo diretório, mesmo basename, extensão trocada).

## A estrutura comum

O documento é a `Collection`: mapa raiz chaveado pelo **id** — nome do
arquivo sem extensão (events: eventID, ex. `azit0000`; objects: basename,
ex. `important`; macro: `chunk_07`; help: stem do `.sps2`). Cada entrada:

| Campo | Conteúdo |
|---|---|
| `metadata.key` | chave canônica (versão/padrão/arquivo), ex. `ffx/event/obj_ps3/az/azit0000/azit0000.bin` — a identidade da entrada: é dela que saem o kind, a versão e o casamento por basename |
| `metadata.id` | o id (chave do mapa raiz) |
| `metadata.row_count` | contagem de rows (conferência; o import reconta) |
| `rows[].index` | posição da row no binário (reconstrução posicional) |
| `rows[].name` | qualifica a row quando um índice carrega vários campos (objects: `name`, `desc`, `ability1`...; macro: `name`/`name_simplified`); vazio em events |
| `rows[].character` / `kind` | referência de EXIBIÇÃO dos `w_name.bin` (personagem dono do nome, tipo do equipamento) — nunca campo editável |
| `rows[].hash` | mapa `idioma → $<xxh64 hex 16>`: o PONTEIRO de conteúdo da row (identidade do segmento + staleness contra data/) |
| `rows[].text` | mapa `idioma → texto`: o conteúdo de referência, vindo de `data/` (ou do `.vbf`) |
| `rows[].source` | a referência do idioma fonte (`us`) de `data/`/`.vbf` (read-only, para CAT) |
| `rows[].target` | a tradução de `mods/` — **o único texto que o import aplica** |

Uma row por frase; um par (hash, text) POR IDIOMA dentro dela. Campos de
exibição (`original`, `refs`, `missing*`) nunca entram em export — o
formatter reconstrói a row campo a campo. A saída é determinística: chaves
ordenadas, rows por index, idiomas em ordem alfabética.

### source/target (projeção para CAT)

Ferramentas CAT exigem o par source/target. O export projeta:

- `text`/`source` SEMPRE de `data/` (ou do `.vbf`) — `source` é a referência
  do idioma fonte (`us`, inglês);
- `target` de `mods/` — a tradução (o PT-BR vive no slot `us`), e é o texto
  que o import grava de volta no binário;
- o toggle **Preferir binários traduzidos (mods)** NÃO muda o export:
  origem e tradução são lidas independentemente do toggle;
- nos formatos por idioma (CSV/XLSX/.strings) o par só sai na linha `us`;
  no JSON ele é um campo único da row (vazio quando o segmento ainda não tem
  tradução).

O JSON e o `.strings` NÃO carregam tag de idioma do alvo: só `source` e
`target` (conteúdo).

## JSON

- Indentação de 2 espaços, **sem escape HTML** (`&`, `<`, `>` saem
  literais).
- TODO hash sai com prefixo `$` — o marcador de ponteiro de conteúdo,
  uniforme nos QUATRO formatos de artefato (JSON, .strings, CSV e XLSX
  carregam `$` na coluna/campo hash; o prefixo vive no pacote `hash`).

Trecho de exemplo:

```json
{
  "azit0000": {
    "metadata": {
      "key": "ffx/event/obj_ps3/az/azit0000/azit0000.bin",
      "row_count": 2,
      "id": "azit0000"
    },
    "rows": [
      {
        "index": 0,
        "hash": { "us": "$aaaa0000aaaa0000" },
        "text": { "us": "Frase de teste comum ao arquivo" },
        "source": "Frase de teste comum ao arquivo",
        "target": "Frase traduzida comum ao arquivo"
      },
      {
        "index": 1,
        "name": "name_simplified",
        "hash": { "jp": "$cccc2222cccc2222", "us": "$bbbb1111bbbb1111" },
        "text": {
          "jp": "テスト",
          "us": "你好世界，这是一段很长的文字用来测试"
        },
        "source": "你好世界，这是一段很长的文字用来测试",
        "target": "Olá mundo, este é um texto longo para teste"
      }
    ]
  }
}
```

## Strings

Formato java-like, um bloco por entrada — header + uma linha por
(row, idioma) com texto não-vazio:

```
/*key=<key> row_count=<n>*/
<id>[:<name>]:<index>:<lang>║$<hash> = <literal | $hash>[║source = <esc>][║target = <esc>]
```

Exemplo:

```
/*key=ffx/event/obj_ps3/az/azit0000/azit0000.bin row_count=2*/
azit0000:0:us║$aaaa0000aaaa0000 = Frase de teste comum ao arquivo║source = Frase de teste comum ao arquivo║target = Frase traduzida comum ao arquivo
azit0000:name_simplified:1:jp║$cccc2222cccc2222 = テスト
azit0000:name_simplified:1:us║$bbbb1111bbbb1111 = 你好世界，这是一段很长的文字用来测试║source = 你好世界，这是一段很长的文字用来测试║target = Olá mundo, este é um texto longo para teste
```

- **A versão não vai na linha** — vive só no header `/*key=*/`, de onde
  `ParseKey` a recupera; a linha começa no arquivo (id) em diante, como a
  key dos demais formatos. Linha cujo primeiro campo é nome de versão
  (`ffx`, `ffx2`, …) é erro de formato.
- **Âncora semântica à direita** — último campo = lang, penúltimo =
  index; campos NOVOS (ex.: path/region) crescem pela esquerda sem
  quebrar o parser. `name` é omitido quando vazio (events/help);
  objects/macro emitem `<id>:<name>:<index>:<lang>`.
- **`║` (U+2566)** liga a chave ao hash de identidade; **` = `** liga o
  hash ao valor, com split no PRIMEIRO `=` (o texto pode conter `=` à
  vontade).
- **Projeção CAT no fim da linha** — depois do valor podem vir
  `║source = <escapado>` e `║target = <escapado>` (só na linha do idioma
  `us`; `target` é o texto que o import aplica). O `escapeValue` garante que
  um `║` literal nunca aparece no texto, então os marcadores são
  inconfundíveis — inclusive em linha de ref `$hash`.
- **def** — literal cujo `xxh64` bate com o hash da chave: verificação
  grátis na leitura (mismatch gera só log; o texto vence). **ref** —
  `$hash` resolvido pela tabela hash→texto do próprio arquivo.
- **Escapes no valor** — `\\` (backslash), `\r`, `\n`, `\║`; o parser é
  leniente: sequência desconhecida mantém backslash + caractere.
- Linhas em branco são puladas e `\r` de fim de linha é tolerado (CRLF).

## Dedup `$hash`

Mesma regra dos [formatos tabulares](exportacao-csv-xlsx.md): no idioma
default (`us`), textos repetidos DENTRO do mesmo artefato não são escritos
de novo — a PRIMEIRA ocorrência sai literal, as demais viram ref `$hash`.
A elegibilidade é pelo miolo VISÍVEL (`hash.IsDedupEligible`): tags
`{...}` e espaços/quebras das bordas NÃO contam, e o miolo precisa ter
mais de `MinDedupRunes` (10) runes. Só rows não-divergent participam.

`MarshalNoDedup` (json e strings) sai com TODO texto literal, sem refs —
a direção "dupe/resolve" do menu: expandir os hashes de conteúdo de volta
ao texto completo.

## Caminhos dos artefatos

O JSON é a referência: os mesmos nomes canônicos servem aos 4 formatos
(CSV/XLSX e .strings só trocam a extensão — ver
[CSV / XLSX](exportacao-csv-xlsx.md)):

- `events_all_localizations_ffx.json` — e as variantes por escopo
  (`event_<id>_...`, `events_sel_<n>_<id>_...`);
- um arquivo por entrada em objects: `<id>_all_localizations_ffx.json`;
- bulk das tabelas: `battletext_`, `cloudsave_`, `mainmenu_`, `tuto_`;
- `help_all_localizations_ffx.json` (um único artefato por versão);
- o dicionário mantém o canônico `macro_dictionary_all_localizations`.

A régua é a do `export_paths.go`: o dedup do marshal é por ARQUIVO, então
o NOME reflete o escopo — um subconjunto nunca sobrescreve o artefato
completo do tradutor.

## Idiomas

Todos os formatos aceitam o filtro de idiomas (`MarshalLangs`): vazio =
TODOS os idiomas carregados (regra do backend). Pelo seletor **Idiomas**
do header, o frontend manda `[default, ...extras marcados]` — o default
(`us`, `common.DefaultLocalization`) sai sempre e os extras vêm do
`ListLanguages`, com a escolha salva no `localStorage`
(`ffx.export-langs`). A estrutura não muda: só entram menos chaves no
`text`/`hash` (JSON) e menos linhas (.strings). O par `source`/`target` é
independente do filtro e só aparece na linha do idioma default (`us`), onde
vive a tradução.

## Importação (round-trip)

- Vias: **Importar de... arquivo** (aceita `.json`, `.strings`, `.csv`,
  `.xlsx` — JSON/strings chamados direto, CSV/XLSX pelo registry) e o menu
  nativo Importar ▸ <tipo> ▸ Importar JSON|Strings.
- **`target` é o texto importado**: a row é gravada no slot `us` do binário
  de `mods/` com o valor do target; target vazio = a row não pede alteração.
  `text`/`source` são referência de `data/` e o import não os aplica.
- **Casamento por coordenada (Index, Name)** — `data/` e `mods/` são o mesmo
  binário em estados diferentes. Se `data/` estiver disponível, o hash `us`
  do artefato é conferido contra o texto original do segmento: origem
  divergente (arquivo do jogo mudou desde o export) vira row pulada + log,
  nunca erro de arquivo.
- **Sem compatibilidade com o formato antigo** (sem `source`/`target`): o
  arquivo é rejeitado com "re-exporte no formato atual". Os textos vivem no
  binário — artefatos antigos podem ser importados com uma versão anterior
  do app e reexportados na nova.
- **O hash é ponteiro opaco na leitura**: refs `$hash` são expandidas pela
  tabela hash→texto do PRÓPRIO arquivo (inclusive texto editado sob hash
  antigo); ref órfã é mantida como está + log, para o problema aparecer na
  prévia. A verificação de def do .strings gera só log.
- **Divergência é inferida na leitura**: literal diferente sob o mesmo
  hash (um editor externo trocou o `$hash` de uma cópia por texto válido)
  marca a row como Divergent — a definição canônica continua sendo a
  primeira literal. É a diferença para o CSV/XLSX, onde a divergência vem
  explícita na coluna `divergent`.
- No .strings o header é informativo: a `key` ancora a metadata da
  entrada e o `row_count` é conferido via log (o import reconta).
- O import SEMPRE mostra prévia antes de aplicar; problema de entrada é
  aviso (a entrada é ignorada, o resto importa) e só bloqueador global
  desabilita o botão Importar, e nada é salvo no binário sem o "Salvar"
  explícito.
- Round-trip: export → import de um artefato com os mesmos targets não
  altera o binário (`changed = 0`). Em bytes o ciclo extrair → importar →
  exportar também é idempotente — ver [encoding.md](../encoding.md).
