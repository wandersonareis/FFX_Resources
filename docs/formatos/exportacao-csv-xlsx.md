# Exportação CSV / XLSX

Os formatos tabulares de tradução externa: a MESMA tabela em dois arquivos —
`.csv` (texto puro) e `.xlsx` (planilha estilizada, com as colunas de
metadata protegidas contra edição). Feitos para o trabalho em planilha
(Excel, LibreOffice, Google Sheets): o tradutor mexe nas colunas de texto e
devolve o arquivo pelo import. É a versão tabular dos mesmos dados do
[JSON / Strings](exportacao-json-strings.md).

Pacotes: `backend/formatters/csv` e `backend/formatters/xlsx`, registrados no
registry `backend/formatters/formatter.go` (interface `Formatter`:
`Extension` / `Marshal` / `MarshalLangs` / `Unmarshal`). Ambos recebem só o
DTO pronto (`backend/dto`) — sem conhecimento do domínio.

## A tabela

Uma linha por (row, idioma) com texto não-vazio — o mesmo texto em inglês e
japonês são DUAS linhas, a coluna `lang` distingue. A saída é determinística:
entradas por key, rows por index, idiomas em ordem alfabética.

Quais idiomas viram linha é decidido pelo filtro de idiomas do export
(`MarshalLangs`): vazio = todos; com seleção (seletor **Idiomas** do
header, salvo no `localStorage` `ffx.export-langs`) = o default `us`
sempre + os extras marcados — mesma tabela, menos idiomas.

| Coluna | Planilha | Conteúdo |
|---|---|---|
| `key` | A | metadata.key COMPLETA (versão/padrão/arquivo) — a identidade da entrada no app: é dela que saem o kind, a versão e o casamento por basename, então um artefato com várias entradas (`events_all`, `objects_all`) volta inteiro no import |
| `id` | B | metadata.ID (ex.: `chunk_07` do dicionário, que não vive no nome do arquivo); vira a chave da entrada no DTO |
| `name` | C | nome da row dentro do arquivo (ex.: `name_simplified`); vazio quando o formato não nomeia rows |
| `index` | D | posição da row no binário (reconstrução posicional) |
| `lang` | E | idioma do texto da linha (`us`, `jp`, ...) |
| `hash` | F | hash do texto no formato de artefato: `$` + `hash.Sum64Hex` (16 hex) — a identidade do texto no dedup. O `$` é o marcador do pacote `hash` (`RefPrefix`); no XLSX também garante que a célula fique tipada como texto (hash todo-numérico não vira número) |
| `text` | G | **o texto editável** — a coluna a mexer na tradução |
| `divergent` | H | `true` quando a row tem texto próprio, divergindo da tabela compartilhada |

## Dedup `$hash`

Textos repetidos DENTRO do mesmo artefato não são escritos de novo: no idioma
default (`us`), a PRIMEIRA ocorrência sai literal e as demais saem como
`"$<hash>"` — uma referência à definição. O import resolve as refs numa
primeira passada; ref órfã (sem definição no arquivo) entra como está, para o
problema aparecer na prévia.

A elegibilidade é pelo miolo VISÍVEL (`hash.IsDedupEligible`): tags `{...}` e
espaços/quebras das bordas NÃO contam, e o miolo precisa ter mais de
`MinDedupRunes` (10) runes — partículas curtas (um "Whoa!" atrás de três
tags) nunca colapsam. Só rows não-divergent participam.

As variantes `csv-no-dedup` e `xlsx-no-dedup` (registradas no registry) saem
com TODO texto literal, sem referências.

## CSV

- RFC 4180 (`encoding/csv`): vírgula separadora, aspas onde precisa, quebra
  LF (planilhas leem LF e CRLF).
- O import é tolerante (`LazyQuotes`, espaços à esquerda) mas exige o header
  EXATO: as 8 colunas da tabela acima, com esses nomes, nessa ordem.

## XLSX

Produzido com `github.com/xuri/excelize/v2`:

- **Layout** — `Sheet1`, header na linha 1 (negrito, fundo cinza,
  centralizado), dados da linha 2 em diante.
- **Larguras** — todas as colunas começam em 10 caracteres e crescem pelo
  maior texto observado (runes — o jogo tem CJK e PUA), teto 80. A coluna
  `text` tem `WrapText`: a altura da linha cresce sozinha ao abrir.
- **AutoFilter** em `A1:H{fim}` + congelamento da primeira linha.

### Proteção das colunas de metadata

A planilha sai PROTEGIDA, sem senha: as colunas de metadata (`A`–`F`,
`key`…`hash`) e o header (linha 1) são células travadas — só `text` (`G`) e
`divergent` (`H`) aceitam edição. Ainda permitidos: clicar/copiar células
travadas, usar o filtro, ordenar. Bloqueados: inserir/deletar linhas e
formatação.

É proteção contra EDIÇÃO ACIDENTAL, não segurança — quem insistir ainda
edita: desproteger é um clique (Revisão → Desproteger Planilha, sem senha) e
colar (Ctrl+V) bypassa a validação. O aviso customizado ("Edição não
permitida" / "Não altere os valores desta coluna.") viaja numa validação
sempre-inválida sobre `A:F` (estilo Stop): aparece ao tentar digitar em
metadata com a planilha desprotegida — o LibreOffice a expõe no menu de
contexto do botão direito. Com a planilha protegida, o aviso de edição é o
padrão do próprio aplicativo (o formato não permite customizá-lo).

O import lê apenas os VALORES — proteção e estilos não interferem em nada.

## Caminhos dos artefatos

Mesmo nome canônico do export JSON, só a extensão muda (`exportRegistry` /
`swapExt` em `backend/services/export_formats.go`):

- `events_all_localizations_ffx.csv` / `.xlsx` — e as variantes por escopo
  (`event_<id>_...`, `events_sel_<n>_<id>_...`);
- bulk das tabelas: `battletext_`, `cloudsave_`, `mainmenu_`, `tuto_`;
- `objects_all_localizations_ffx...` e `help_all_localizations_ffx...`;
- o dicionário mantém o canônico `macro_dictionary_all_localizations...`.

A régua é a do `export_paths.go`: o dedup do marshal é por ARQUIVO, então o
NOME reflete o escopo — um subconjunto nunca sobrescreve o artefato completo
do tradutor.

O menu nativo (Texto ▸ <tipo> ▸ Exportar CSV/XLSX) usa os MESMOS formatters
do registry (`appcore`), então a saída é idêntica à do export do app para o
mesmo escopo.

## Importação (round-trip)

- Vias: **Importar de... arquivo** (aceita `.json`, `.strings`, `.csv`,
  `.xlsx` — dispatch por extensão em `backend/services/import.service.go`) e
  o menu nativo Importar ▸ <tipo> ▸ Importar CSV/XLSX.
- Regras: exige `key` por linha (linha sem key aborta com erro — re-exporte
  o arquivo no formato atual); exige hash com prefixo `$` — célula não-vazia
  sem o `$` é artefato do formato antigo e aborta com o mesmo erro de
  re-export (célula vazia é tolerada: artefato feito à mão, o import
  recalcula pelo texto); resolve as refs `$hash`; o XLSX aceita
  colunas extras desde que as 8 primeiras existam.
- O import SEMPRE mostra prévia antes de aplicar; problema de entrada é
  aviso (a entrada é ignorada, o resto importa) e só bloqueador global
  desabilita o botão Importar, e nada é salvo no binário sem o "Salvar"
  explícito.
- Round-trip: export → import sem edição devolve a MESMA coleção (mesmos
  textos e metadados).
