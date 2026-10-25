# Janela de logs

A janela de logs (`/w/#logs` no desktop, drawer na web) mostra a saída das
operações do app — import, export, aplicação de edições — **em tempo real**,
com o mesmo conteúdo do console colorido e do arquivo JSON de log. Este
documento descreve o contrato da janela: o que cada tipo de linha significa
e o que um agente precisa saber para mexer nela.

## Arquitetura

| Camada | Arquivo | Papel |
|---|---|---|
| Buffer + evento | `backend/uilog/uilog.go` | histórico da sessão, evento `UILog` (append) e `UILogCleared` (limpar), *write-through* para o `loggingService` (console + arquivo) |
| Serviço exposto | `service_log.go` | `History()` (semente ao abrir a janela) e `Clear()` |
| Gravador de gravações | `backend/common/write_record.go` | `RecordWrites(fn)` observa tudo que passa por `common.WriteBytesToFile` — o gargalo único de escrita de binários |
| Narração do import | `backend/services/import_log.go` | agrupa as gravações por binário e emite a linha do log |
| Render | `frontend/src/components/log-panel.tsx` | lista monoespaçada: `hora [ação] path — status: detalhe` |

A escolha de desenho: **o apply não devolve os destinos** (devolve erro ou
nada) e a propagação por hash recompila *todos* os binários que carregam um
texto editado. Em vez de mudar a assinatura do motor (`builders`), a
operação liga o gravador em `common` no ponto em que os binários são
escritos — nenhum escritor de formato muda, e o `.vbf` pode usar o mesmo
gancho depois.

## Linhas

### Linha de ação (`Info`, `Success`, `Warn`, `Error`)

```
hh:mm:ss [Import] traduzido.json — aplicando 12 texto(s) em events (ffx2)
hh:mm:ss [Import] traduzido.json — 12 texto(s) em 3 binário(s) gravado(s) em mods/
```

Sem `Path`: é o fluxo da operação. Nível → cor: `info` azul, `success`
verde, `warn` laranja, `error` vermelho.

### Linha de arquivo (`File(action, path, status, detail)`)

```
hh:mm:ss [Import] mods/ffx_ps2/ffx2/master/new_uspc/event/obj_ps3/ev/ev001/ev001.bin — gravado: 8 localizações · 8.0 KB
hh:mm:ss [Import] mods/ffx_ps2/ffx2/master/new_uspc/menu/macrodic.dcp — falha: disco cheio
```

Com `Path`: o resultado de **um** arquivo. Contrato do renderizador:
`path — status: detalhe`, tudo na mesma linha.

- **Sucesso** → `status` `gravado` (verde) e o caminho é o destino real
  dentro de `mods/`. O detalhe conta as localizações gravadas (`8
  localizações`) — cada binário existe em uma linha por idioma, e a linha é
  **uma por binário** (o destino exibido é o do idioma padrão, `us`).
- **Falha** → `status` `falha` (vermelho) e o detalhe carrega o motivo com
  o **destino garantido**: o erro de permissão do Windows já nomeia o
  arquivo; quando a mensagem embrulhada não o tem, o destino entra antes do
  motivo. Se só parte das localizações falhou, o detalhe diz quantas
  (`1 de 8 localizações falhou: …`).

## O que cada import registra

| Fluxo | Onde | Linhas |
|---|---|---|
| Importar (toolbar, 1 artefato) | `MetadataService.ImportFile` | ação (azul) → uma linha por binário → resumo (verde) |
| Importar em lote (`mods/translated`) | `MetadataService.ImportTranslated` | ação (azul) por artefato → uma linha por binário → resumo do lote |
| Importar edição da entrada | `MetadataService.ImportEntry` | ação (azul) → uma linha por binário |
| Menu nativo ▸ Importar | `appmenu/actions.go` | uma linha por binário apontado (`path — status: destino/motivo`) |

## Regras ao mexer no assunto

1. **Nunca narrar antes da ação.** O log é escrito *depois*: `File` recebe o
   resultado em mãos (`import_log.go` só roda quando o apply voltou).
2. **Nada de `builders`/`fileFormats` importando `uilog`.** `uilog` depende
   do runtime do Wails e `builders` é importado por serviços — o gancho de
   gravação fica em `common` (dependência de baixo nível) e a narração, no
   serviço.
3. **O gravador é opt-in e sem custo.** Sem `RecordWrites` ativo,
   `recordWrite` só checa a pilha vazia. Ele nunca muda o comportamento da
   escrita.
4. **Status é rótulo, não número.** `uilog.levelForStatus` deriva a cor:
   `gravado`/`sobrescrito` → sucesso, `falha`/`incompatível` → erro,
   `ignorado`/`já existe` → aviso.
5. **Um binário = uma linha.** Agrupar por idioma detalharia demais: um
   import completo de FFX-2 recompila centenas de binários em 8
   localizações.

## Limitação conhecida

Erros da *fase de coleta* do apply (ex.: `string index out of range`)
chegam como **erro agregado** com os ids dos arquivos afetados
(`failed to update N event(s): [...]`) — a granularidade por binário aí
exige um gancho no motor (`backend/builders/tableapply.go`), que não foi
feito. As gravações em disco, inclusive as que falharam, são sempre
narrativas uma a uma.
