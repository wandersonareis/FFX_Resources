# Encoding do texto (`backend/core/converter`)

O texto do jogo NÃO é ASCII/UTF-8: cada idioma usa um **charset
customizado** (tabelas em `backend/core/encoding`) e os bytes carregam
também **comandos de controle** (cor, pausa, escolha, macro...). O pacote
`converter` é a única fronteira texto ↔ bytes:

- **Decode** — `BytesToString` (`string_bytes.go`): bytes → texto com
  tokens `{...}`. O núcleo é `getStringAtLookupOffsetBinary`, agora um
  `binaryDecoder` com estado (cursor, saída, charset/versão/idioma).
- **Encode** — `StringToBytes` (`string_to_bytes.go`): texto → bytes.
  `ParseCommand` resolve os tokens; `CharToBytes` → `indexBytes`.

O contrato central é o **round-trip estável**: extrair → importar →
exportar devolve o MESMO texto, em qualquer idioma — inclusive CJK
(`jp`/`ch`/`kr`), cujos caracteres são multi-byte. `indexBytes` é a
inversa **exata** do decode: o que um emite, o outro reconstroi.

## Charset por idioma

| Charset | Idiomas | Observação |
|---|---|---|
| `us` | inglês (default) | o font trocou ä/ö/Ä/Ö por ã/õ/Ã/Õ — trema não existe no font us |
| `default` | sp, fr, de, it | preserva o trema do font original |
| `jp`, `ch`, `kr` | japonês, chinês, coreano | caracteres multi-byte (seções + extensão de 5 seções) |

`ffxencoding.ResolveCharset` aceita charset OU código de idioma; o MESMO
byte decodifica diferente por charset (0xA6 é `Ã` no us e `Ä` no default),
por isso o idioma viaja junto ao texto em todo o fluxo.

## Anatomia dos bytes de texto

| Seqüência | Significado | Índice flat (slot) |
|---|---|---|
| `30..FF` | byte de texto simples | o próprio byte |
| `2C..2F xx` | seção (2 bytes) | `(byte1 − 0x2B) × 0xD0 + byte2` |
| `04 30..FF` | extensão de 5 seções (prefixo + byte) | `0x410 + byte` |
| `04 2B..2F xx` | extensão + seção (3 bytes) | `0x410 + (byte1 − 0x2B) × 0xD0 + byte2` |

O prefixo `0x04` liga uma flag que vale para **um** byte: o próximo sai
deslocado em `0x410`. `indexBytes` (encode) reconstrói essas formas — é a
inversa da tabela acima.

## Ops de 2 bytes (a correção CJK)

`0x06`, `0x08`, `0x0C..0x0F`, `0x11` e `0x24..0x2B` são **operadores de
1 argumento** (referência: `DecodingGamecode*` da Fahrenheit) — NÃO
caracteres de 2 bytes. O decoder antigo tratava esses pares como
caracteres de seção CJK (`>= 0x2B`), o que corrompia texto `jp`/`ch`/`kr`
e quebrava o round-trip: importar um texto extraído regravava bytes
diferentes dos originais.

Prova real (azit0300, JP): a linha `？０！` é
`13 34 03 2D 9E 2A 30 3B 00` — `0x2A 0x30` é o **op** `0x2A` com
**argumento** `0x30`, não um caractere CJK; o `0x3B` (`！`) após o par
sobrevive.

- `0x2B` só é **seção** quando vem depois do prefixo `0x04` — sozinho é op.
- Ops saem como `{UNKOP:xx:yy}` com os dois bytes crus; `0x0E` é o op de
  estilo (`{TEXT_ITALIC}` / `{TEXT_NORMAL}` / `{CMD:0E:xx}`).

## Ordem dos guards do decode (contrato do formato)

```
1. byte de texto (>= 0x30);
2. prefixo 0x04 (extensão de 5 seções: 2 ou 3 bytes);
3. seção 0x2C..0x2F (2 bytes);
4. ops de 2 bytes (0x06, 0x08, 0x0C..0x0F, 0x11, 0x24..0x2B);
5. comandos sem argumento que valem no fim da tabela (0x01..0x03);
6. sentinela de fim (último byte sem terminador);
7. comandos (com argumento).
```

A ordem É o formato: 0x2B vira seção ou op dependendo de onde o guard o
pega, e a sentinela antes dos comandos faz um último byte sozinho sair
como `{HEX:xx}` mesmo quando teria comando próprio. Trocar a ordem muda
o resultado.

## Tokens

**Controle** (encode/decode simétricos):

| Token | Bytes | Token | Bytes |
|---|---|---|---|
| `{PAUSE}` | `01` | `{VAR:xx}` | `12 xx` |
| `{BREAK}` | `02` | `{PC:xx:nome}` | `13 xx` |
| `{TEXT_NEWLINE}` ou `\n` | `03` | `{MCR:sxx:lyy[:payload]}` | `14..22 xx` |
| `{BLANK05}` / `{BLANK0C}` / `{BLANK0F}` / `{BLANK11}` | `05` / `0C` / `0F` / `11` | `{KEY:xx[:nome]}` | `23 xx` |
| `{SPACE:xx}` | `07 xx` | `{CMD:xx:yy}` | `xx yy` (genérico) |
| `{TIME:xx}` | `09 xx` | `{CHOICE:xx}` / `{CHOICE-END}` | `10 xx` / `10 FF` |
| `{CLR:nome}` | `0A xx` | `{TEXT_ITALIC}` / `{TEXT_NORMAL}` | `0E 40` / `0E 41` |
| `{BUTTON:xx:nome}` / `{ICON:xx:nome}` | `0B xx` | | |

O `nome`/`payload` é **display**: o import usa só o índice (speaker fora
do mapa da versão vira `?`, macro ausente `<Missing>` — o round-trip
continua byte-exato). `{MCR}` sem payload (macros não carregadas) também
importa.

**Bytes crus** (round-trip exato de dado desconhecido):

- `{UNKOP:xx:yy}` — op de 2 bytes;
- `{UNKCHR:xx}` — byte de texto fora do mapa;
- `{UNKDBLCHR[:sec]:xx[:yy]}` — caractere de seção fora do mapa (2 ou 3
  bytes, com a forma estendida `04`);
- `{UNKTPLCHR:04:xx:yy}` — seção estendida fora do mapa;
- `{HEX:xx(:yy...)}` — bytes literais;
- `??` — byte ausente (dado truncado no binário): o token preserva os
  bytes que existem e o re-decode do que sobrou reemite o mesmo token
  (ponto fixo).

**`{PUA:código:char}`** — slot duplicado da tabela: o byte decodifica a um
rune cujo slot canônico é OUTRO. O `código` é autoritativo (re-emite os
bytes exatos via `indexBytes`); o `char` é informativo, para o texto
extraído ficar legível/pesquisável.

## Casos de borda

- **Último byte sem terminador** — sai `{HEX:xx}`, sem consumir argumento
  (`0x01..0x03` são a exceção: valem mesmo no fim).
- **Dado truncado** — seção sem byte baixo emite `{UNKDBLCHR:xx:??}` (o
  caso vem ANTES da sentinela); op truncado cai na sentinela `{HEX:xx}`.
- **Slot duplicado** — ver `{PUA}` acima (ex.: aspas duplas em `0x3C` e
  `0x95` da tabela us).

## Garantias (testes)

- `converter/string_bytes_test.go` — caracterização do decode com charset
  sintético: um guard por caso, ordem dos guards e sentinela.
- `converter/unknown_ops_test.go` — ops de 2 bytes: tokens de bytes crus
  no `ParseCommand`, round-trip byte-exato e os bytes reais de azit0300.
- `converter/index_bytes_test.go` — TODOS os slots `0x30..0x800` de
  `jp`/`ch`/`kr` voltam ao mesmo slot via `indexBytes` (duplicados viram
  `{PUA}`).
- `core/reader/decode_internals_test.go` — charsets REAIS + macrodic
  (`reader.InitializeInternals`): til×trema us/default, round-trip de
  textos comuns (acentos, tags, `{PC}`, `{UNKOP}`), macros do `macrodic.dcp`
  e o ciclo extrair → importar → extrair sobre o corpus `.strings`
  exportado — toda linha, de todo idioma, volta idêntica.
