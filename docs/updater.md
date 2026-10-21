# Auto-update (Wails v3) — contrato para agentes

> Guia de referência para **agentes de IA** que forem mexer no auto-update,
> publicar release ou tocar no build. Os invariantes abaixo são o que o
> `pkg/updater` do Wails **exige** — violar qualquer um deles quebra o
> update em runtime, não no build (por isso há checklist no fim).

**Origem:** `github.com/wandersonareis/FFX_Resources` · releases em
`https://github.com/wandersonareis/FFX_Resources/releases`
**Versão do framework:** `github.com/wailsapp/wails/v3 v3.0.0-beta.33`
(`pkg/updater` presente — não há dependência extra a instalar)

---

## 1. Fluxo

```
check (API GitHub) ─► download do asset ─► SHA-256 vs SHA256SUMS ─► stage
     │                                                                 │
     └─ sem release nova: estado "Você está atualizado"               ▼
                                                              restart (binário)
                                                     o PRÓPRIO spira.exe renasce
                                                     como helper (WAILS_UPDATER_HELPER=1),
                                                     troca o binário e relança
```

- **Não existe executável auxiliar separado**: `application.New()` chama
  `updater.HandleHelperMode()` antes de qualquer coisa (`pkg/application/application.go`).
  Se o processo nascer com `WAILS_UPDATER_HELPER=1`, ele troca o binário e
  sai **sem chegar no restante do `main`**.
- O helper é esperado por `HelperReadyTimeout` (30 s default) — se o app não
  sair nesse tempo (ex.: diálogo de "alterações não salvas" cancelado), a
  atualização **fica em stage** e pode ser aplicada depois.

## 2. Contrato do asset de release (o mais frágil)

Cada release precisa publicar **exatamente** estes arquivos:

| Arquivo | Regra |
|---|---|
| `spira-<versão>-portable-<goos>-<goarch>.zip` | nome contém `windows` **E** `amd64` (matcher casa por substring); zip com **UM ÚNICO entry de topo** = `spira.exe` |
| `SHA256SUMS` | uma linha: `<hex-digest><2 espaços><nome-do-zip>` (formato `sha256sum`, minúsculas) |

Invariante por invariante:

1. **Um entry de topo só.** `maybeExtractInto` (`pkg/updater/extract.go`)
   rejeita archive com mais de uma entrada de topo → install falha. Não
   publique README/pasta junto do `spira.exe` no zip.
2. **`SHA256SUMS` é obrigatório.** O provider é configurado com
   `ChecksumAsset: "SHA256SUMS"`; sem o sidecar o `Check` falha (fail-closed).
   Publicar binários e checksum em momentos diferentes também derruba o
   install (o sidecar é buscado separado).
3. **Matcher pega o PRIMEIRO asset compatível.** Instaladores (`*-installer.*`)
   e checksums são pulados; **`.msix`/outros zips com `windows`+`amd64` no
   nome NÃO** — se publicar dois candidatos, a ordem decide. Hoje a release
   publica um zip só.
4. **Tag = `v` + a versão do arquivo `VERSION`** (`v2.1.1` ↔ `2.1.1`). O
   updater compara sem o `v`; divergência entre tag e `main.currentVersion`
   faz o app ou achar que está defasado (loop de update) ou que não há nada
   novo.
5. **Assets por plataforma** seguem `runtime.GOOS`/`GOARCH` do app rodando —
   o mesmo `RELEASE_ASSET` do Taskfile.

Referência de layout de release (repo oficial do framework):
<https://github.com/wailsapp/updater-demo>.

## 3. Versão — fonte única

```
VERSION  (arquivo na raiz, ex.: 2.1.1)
   ├─► -ldflags "-X main.currentVersion={{.VERSION}}"   (Taskfile; vira o
   │    CurrentVersion do updater; default "dev" sem ldflags)
   ├─► build/windows/info.json   (file_version / ProductVersion do .exe)
   ├─► frontend/package.json     ("version")
   └─► nome da tag git (v2.1.1) e do asset (…-2.1.1-…)
```

- `wails3 task version:sync` grava os dois arquivos (`scripts/version-sync.mjs`,
  idempotente); é dependência de `build` e `release`.
- **Nunca** edite `info.json`/`package.json` à mão: o sync sobrescreve.
- `main.currentVersion == "dev"` (build de dev, sem ldflags) **desliga o
  check de startup** — em `wails3 dev` a janela de update nunca abre sozinha.
  O menu *Ajuda ▸ Procurar atualizações…* continua funcionando em dev.

## 4. Comandos

| Comando | O que faz |
|---|---|
| `wails3 task build` | build de release da plataforma atual (injeta a versão nos ldflags) |
| `wails3 task version:sync` | só sincroniza `VERSION` → `info.json`/`package.json` |
| `wails3 task release` | `build` → `release:assets` (zip + `SHA256SUMS` em `dist/`) → `gh release create` |
| `node scripts/version-sync.mjs` | idem task, direto |

Fluxo de release completo:

1. Bump no arquivo `VERSION` (ex.: `2.2.0`) e commit.
2. (Opcional) `docs/releases/2.2.0.md` com o changelog — vira o **corpo da
   release** e é exatamente o texto que a janela de update mostra.
   Sem o arquivo, a task usa `gh release create --generate-notes`.
3. `wails3 task release` (requer `gh auth status` ok; `dist/` está no
   `.gitignore`).

`dist/` é apagado/reescrito a cada run; a task falha se `build/bin/spira.exe`
não existir (build antes).

## 5. Janela de atualização

Arquivo: **`backend/appupdater/window.html`** — cópia adaptada do template
`pkg/updater/assets/window.html` (MIT) com textos em pt-BR e tokens de cor do
`frontend/src/app/theme.css` (primary sky + superfícies slate).

> **Contrato — não mexer:** `data-state` no `<main>`, todos os `id` dos
> elementos, o JS inteiro (`setState`, `renderMarkdown`, `announce()`), os
> `Events.On("wails:updater:…")` e os `Events.Emit("wails:updater:user:…")`.
> Mover/renomear qualquer um desses faz os botões virarem mudo ou a janela
> travar no primeiro estado. Texto e CSS são livres.

- A janela segue o **esquema do SO** (`prefers-color-scheme`), não o toggle do
  app — é uma webview nativa separada, sem acesso ao `localStorage` do frontend.
- O renderer de notas é um **subconjunto** de Markdown: parágrafos, negrito,
  itálico, `code`, fences, listas, tabelas GFM e headings (até h3). **Sem
  imagens** — release notes com imagem aparecem como texto literal.
- Título/chrome: `updater.WindowOptions{Title: …}` em
  `backend/appupdater/appupdater.go` (não no HTML).

## 6. Eventos (para UI própria)

Publicados no **bus do app** (`app.Event`) — o frontend da janela principal
recebe todos (é como a banner em `frontend/src/components/update-banner.tsx`
acorda). Payloads em `pkg/updater/types.go`.

| Evento | Payload | Campos |
|---|---|---|
| `wails:updater:check-started` | — | |
| `wails:updater:update-available` | `Release` | `version`, `notes`, `artifact{filename,size,…}`, `publishedAt` |
| `wails:updater:no-update` | — | |
| `wails:updater:download-started` | `Release` | |
| `wails:updater:download-progress` | `Progress` | `written`, `total`, `rate`, `provider` |
| `wails:updater:download-complete` | `Release` | |
| `wails:updater:verifying` | — | |
| `wails:updater:installing` | — | |
| `wails:updater:update-ready` | `Release` | install em stage, falta reiniciar |
| `wails:updater:error` | `ErrorInfo` | `stage` ∈ `check\|download\|verify\|install`, `message` |
| `wails:updater:meta` | `Meta` | `currentVersion`, `skippedVersion` |
| `wails:updater:window:ready` | — | janela de update terminou de carregar |

Ações do usuário (emitidas pela janela; **não** emitir de outro lugar sem
abrir a janela antes): `wails:updater:user:{install,skip,remind,cancel,restart}`.

No frontend use `import { Events, Updater } from "@wailsio/runtime"` e as
constantes (`Updater.Events.UpdateAvailable`…) — o literal **tem o prefixo
`wails:`** (o repo `wailsapp/updater-demo` usa sem e os listeners não disparam).

## 7. Onde vive o código

| Arquivo | Papel |
|---|---|
| `backend/appupdater/version.go` | estado (`CurrentVersion`, `ReadyVersion`, `ReadyToApply`) + constantes `repository`/`checksumAsset` |
| `backend/appupdater/appupdater.go` | `//go:build !server` — `Init`, `StartupCheck`, `CheckNow`, `Restart` |
| `backend/appupdater/appupdater_stub.go` | `//go:build server` — no-ops, `CheckNow`/`Restart` devolvem erro |
| `backend/appupdater/window.html` | janela de update pt-BR (embed) |
| `main.go` | `currentVersion` (ldflags) + `appupdater.Init` após `application.New` + `StartupCheck` antes de `app.Run` |
| `backend/appmenu/menu.go` | item *Ajuda ▸ Procurar atualizações…* |
| `backend/appmenu/actions.go` | `onCheckUpdates` → `appupdater.CheckNow()` |
| `service_update.go` + `appServices()` | bindings `UpdateService` (`CheckNow`, `Restart`, `CurrentVersion`) |
| `app.go` | `shouldQuit` com diálogo de **atualização em stage** (`confirmUpdateRestart`) |
| `frontend/src/components/update-banner.tsx` | banner de estado no frontend |
| `scripts/version-sync.mjs` / `scripts/release-assets.ps1` | versão e assets |
| `Taskfile.yml` | vars `VERSION`, `RELEASE_ASSET`; tasks `version:sync`, `release`, `release:assets` |

### Integração com a saída (cuidado)

`App.shouldQuit` roda **durante** o `Restart` do updater (`Restart → Quit`).
Com edições não salvas, ele troca o diálogo normal por
`confirmUpdateRestart` — três saídas: *Salvar e reiniciar* (SaveRequested →
frontend salva → segunda passada libera), *Descartar e reiniciar*,
*Cancelar* (aborta o restart; a atualização **continua em stage**).

Ao mexer no `shouldQuit`, mantenha: **se `appupdater.ReadyToApply()` for
true, o restart não pode morrer num diálogo que o usuário não entende.**

## 8. Limitações conhecidas (não são bugs)

- **Só o executável é trocado.** Tudo que é `go:embed` (`frontend/out`,
  tabelas) atualiza junto; arquivo solto em `data/`/`config/` **não** viaja
  na release (o zip tem só o exe) — mantenha o app autocontido.
- **Windows sem Authenticode** → SmartScreen no exe novo (a task
  `windows:sign` existe; precisa de certificado).
- **Sem assinatura Ed25519**: `updater.Config.PublicKey` vazio = apenas
  SHA-256. Serviria de `updater-key.pub` embutido + `PublicKey:` no `Init`.
- **Sem CI**: release é manual pela task. Sem `CheckInterval` (só o check de
  startup de 1 vez + menu).
- **Modo servidor** não atualiza (stub) — sem janela nativa.

## 9. Checklist antes de fechar uma mudança

- [ ] `go build ./...` **e** `go build -tags server ./...` passam.
- [ ] `go test ./backend/...` verde (e `wails3 task test:e2e` se mexeu em server).
- [ ] Trocou a versão? Arquivo `VERSION` atualizado (nunca os derivados à mão).
- [ ] Mexeu no `window.html`? `data-state`, `id`s e o JS intactos; só texto/CSS.
- [ ] Mexeu em evento? Constantes Go **e** uso no banner seguem o literal
      `wails:updater:*`.
- [ ] Publicou release? zip de **um** entry de topo + `SHA256SUMS` juntos,
      nome do asset com `windows` e `amd64`, tag = `v` + `VERSION`.
- [ ] Mudou o fluxo de saída? `shouldQuit` continua tratando `ReadyToApply`.
