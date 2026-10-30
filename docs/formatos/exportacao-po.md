# Exportação / importação PO (gettext)

O `.po` é o artefato de intercâmbio com ferramentas CAT de verdade — Poedit,
OmegaT, Crowdin, Weblate — sem o XML do XLIFF (que acusa erro de schema em
boa parte delas). O formato é texto puro: qualquer editor de PO abre, e o
que o tradutor preenche (`msgstr`) volta para o binário em `mods/`.

Pacote: `backend/formatters/po` (registrado como `po`).

## Onde aparece

- **Exportar**: seletor de formato do header (Exportar ▸ …) e menu nativo
  (Exportar ▸ `<kind>` ▸ “Exportar PO (gettext)”).
- **Importar**: “Importar ▸ … ▸ Importar de…” (seletor de arquivo aceita
  `*.po`) e menu nativo (Importar ▸ `<kind>` ▸ “Importar PO (gettext)”).
- O nome do arquivo é o canônico de sempre, com a extensão trocada:
  `battletext_all_localizations_ffx.po`, `event_azit0000_all_localizations_ffx.po`,
  `macro_dictionary_all_localizations_ffx.po` (em `mods/edits/`).

## Estrutura

```po
msgid ""
msgstr ""
"Project-Id-Version: FFX Resources\n"
"MIME-Version: 1.0\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Content-Transfer-Encoding: 8bit\n"
"Language: pt-BR\n"

#. import: {"file":"ffx/battle/btl/bika00_10/bika00_10.bin","name":"bika00_10","index":0,"lang":"us","hash":"$45c331fabe140a27"}
#. ref[fr]: Ouf !
#. ref[sp]: ¡Ufa!
#: ffx/battle/btl/bika00_10/bika00_10.bin:0
msgctxt "bika00_10|0"
msgid "{PC:00:TIDUS}{TEXT_NEWLINE}Whew!"
msgstr ""
```

- O **cabeçalho** é a entrada `msgid ""`/`msgstr ""` padrão do gettext. O
  idioma alvo é `pt-BR` — a tradução vive no slot `us` do binário, como em
  todo o app (`common.XLiffTargetLang`).
- O **par traduzível** é o do slot `us`: `msgid` = o original de `data/`
  (`Source`) e `msgstr` = a tradução que vive no slot `us` de `mods/`
  (`Target`). É o mesmo `source`/`target` que os demais formatos carregam.
- As **referências** (`#. ref[<idioma>]`) são os textos originais do jogo
  nos outros idiomas (`ch`, `de`, `fr`, `it`, `jp`, `kr`, `sp`) — read-only,
  o import nunca escreve nelas. O filtro é o mesmo dos outros formatos: os
  idiomas marcados no seletor do header; `todos` (menu nativo) traz os 7.
- O **msgctxt** (`<entrada>|<índice>[|<campo>]`) e a **referência `#:`**
  (`<arquivo>:<índice>`) são a cópia legível: o tradutor vê onde a string
  está, e o import ainda funciona se o editor bagunçar o JSON.

## Metadados (`#. import:`)

O canal principal do import é o JSON do comentário extraído — o que o editor
PO preserva, mas não depende:

| campo | origem | uso no import |
|---|---|---|
| `file` | `metadata.Key` | versão + kind + identidade do arquivo (`ffx/battle/btl/bika00_10/bika00_10.bin`) |
| `name` | `metadata.ID` | id da entrada (chave da Collection) |
| `index` | `row.Index` | 1ª metade da coordenada da row |
| `field` | `row.Name` (só quando não é vazio) | 2ª metade da coordenada (`name`/`desc`/`ability1` em objects; `game`/`utf8` em lockit) |
| `lang` | `us` | slot do par traduzível |
| `hash` | hash do original de `data/` (`$`+hex) | stale-check: se `data/` mudou depois do export, a row é pulada |

Sem o JSON, o import reconstrói a mesma coordenada pelo `msgctxt` e pelo
`#:` — a redundância é proposital (o tradutor sempre vê o contexto; o import
não depende de um único canal).

## Regras do import

O `.po` vira a mesma `dto.Collection` dos outros formatos e entra no mesmo
pipeline (`backend/services/import.service.go`):

1. O kind e a versão saem da `metadata.key` (`file`).
2. O casamento é por **coordenada** `(Index, Name)` — a posição no binário,
   estável entre `data/` e `mods/`.
3. O **stale-check** compara o hash do original de `data/` com o `hash` do
   artefato: original mudou ⇒ a row é pulada (“origem não encontrada” no
   resumo do import) em vez de sobrescrever a tradução de outro segmento.
4. As **tags de controle** do original e da tradução são comparadas: tag
   divergente ⇒ row pulada (a linha inteira não entra).
5. `msgstr` vazio = **não traduzido**: a row não pede alteração nenhuma.
6. Só o `us` é gravado no binário (em `mods/`), como em todos os formatos.

O apply reconstrói o binário e grava em `mods/<localização>/…`; a `data/`
fica intacta.

## Limites

- **Sem plurais**: `msgid_plural`/`msgstr[n]` são ignorados (o jogo não tem) —
  a entrada sai do artefato, mas não derruba o resto do arquivo.
- **Sem dedup**: o `msgid` é sempre o texto literal — um PO com `msgid`
  repetido em contextos diferentes é normal, mas o `msgid` nunca vira
  referência `$hash` (ao contrário do JSON).
- **Só o hash do `us` viaja**: o `hash` do artefato é o do original do slot
  traduzível (o que o stale-check usa). Os hashes de identidade dos outros
  idiomas não são carregados — eles não são usados no import.
- **Sem outros idiomas como traduções**: os idiomas além do `us` são
  referência do original do jogo, não um segundo par traduzível.
