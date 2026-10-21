# Documentação dos Formatos

Este diretório documenta os formatos de arquivo tratados pelo editor, um
arquivo por formato. Cada página descreve o **layout binário** e as
**funções principais** do pacote Go correspondente.

## Índice

| Formato | Arquivo | Conteúdo |
|---|---|---|
| Eventos (`event/obj_ps3/*.bin`) | [eventos.md](formatos/eventos.md) | Bloco de ponteiros + bloco de texto no mesmo arquivo |
| Textos de sistema (`battle/kernel/*.bin`) | [textos-objetos.md](formatos/textos-objetos.md) | Header + bloco de dados + bloco de texto separados |
| Tabelas de evento FFX-2 (btl/cloud/tutorial) | [tabelas-evento.md](formatos/tabelas-evento.md) | Mesma tabela de strings dos eventos |
| Painel de ajuda (`.sps2`) | [ajuda-sps2.md](formatos/ajuda-sps2.md) | Header + tabela de ponteiros u32 + segmentos |
| Dicionário de macros (`macrodic.dcp`) | [dicionario-macros.md](formatos/dicionario-macros.md) | Tabela de 16 offsets de chunk + chunks |
| Loc kit (`ffx_loc_kit_*.bin`) | [lockit.md](formatos/lockit.md) | Registros CRLF com charset do jogo e UTF-8 no mesmo arquivo |
| Container `.vbf` | [vbf.md](formatos/vbf.md) | Container leitura-only com índice e blocos comprimidos |
| Texturas `.dds.phyre` | [texturas-phyre.md](formatos/texturas-phyre.md) | Container PhyreEngine DX11 com payload DDS |
| Exportação JSON / Strings (`.json` / `.strings`) | [exportacao-json-strings.md](formatos/exportacao-json-strings.md) | Artefatos de texto do editor: estrutura do DTO, dedup `$hash`, round-trip |
| Exportação CSV / XLSX (`.csv` / `.xlsx`) | [exportacao-csv-xlsx.md](formatos/exportacao-csv-xlsx.md) | Tabela de tradução externa: colunas, dedup `$hash`, proteção de metadata |

## Motor compartilhado

O cálculo de ponteiros de string table é **um só motor** para todos os
formatos de objetos — a lógica é idêntica: um bloco de texto e o comprimento
do texto geram o valor dos ponteiros. Documentado em
[motor-strings.md](motor-strings.md) (`backend/core/strings`).

## Convenções gerais

- Todos os inteiros são **little-endian**.
- Textos do jogo usam o **charset customizado** por idioma
  (`backend/core/encoding`), não ASCII/UTF-8 direto — a conversão é feita em
  `backend/core/converter` (`StringToBytes` / `BytesToString`).
- O texto flui pela app como DTO (`backend/dto`): os pacotes de formato
  leem/escrevem binário; os builders (`backend/builders`) montam/aplicam o
  DTO; os formatters (`backend/formatters`) serializam JSON/.strings/CSV/XLSX.
- Edits são sempre salvos em `mods/<localização>/...`, nunca em `data/`.

## Outros guias

- [Auto-update (Wails v3)](updater.md) — contrato de release/build do
  updater (asset, `SHA256SUMS`, versão), eventos `wails:updater:*`, janela de
  update e checklist para **agentes de IA** mexerem no assunto.
