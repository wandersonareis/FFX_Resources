// Package appupdater é o ÚNICO ponto do app que fala com o updater do
// Wails v3 (pkg/updater): configura a origem das releases no GitHub, cuida
// do check silencioso de startup e expõe as ações (verificar agora,
// reiniciar para aplicar) para o menu, para o UpdateService do frontend e
// para o diálogo de saída.
//
// O pacote é dividido por build tag porque o modo servidor (`-tags server`
// — binário HTTP puro, sem janela nativa) não tem o que atualizar da forma
// como o desktop atualiza:
//
//	appupdater.go          //go:build !server  — janela, check e fluxo reais
//	appupdater_stub.go     //go:build server   — no-ops e erro claro
//	version.go             (sem tag)           — estado compartilhado
//
// Origem das releases: github.com/wandersonareis/FFX_Resources, assets no
// formato "spira-<versão>-portable-<goos>-<goarch>.zip" com um único entry
// de topo (spira.exe) + sidecar SHA256SUMS — ambos publicados pela task
// `wails3 task release`.
package appupdater

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// repository é a origem das releases (owner/repo no GitHub).
const repository = "wandersonareis/FFX_Resources"

// checksumAsset é o sidecar SHA-256 publicado junto de cada release. Sem ele
// o updater instala sem verificar integridade — publicar release sem
// SHA256SUMS derruba o check com "checksum asset" ausente.
const checksumAsset = "SHA256SUMS"

var (
	stateMu sync.RWMutex

	// current é a versão em execução, gravada por Init (que recebe o
	// currentVersion do main, injetado por -ldflags no build de release).
	current string

	// ready é a versão já baixada e em stage, esperando o restart.
	ready string
)

// CurrentVersion devolve a versão em execução: "2.1.1" em build de release
// e "dev" em desenvolvimento (sem -ldflags). "" antes do Init.
func CurrentVersion() string {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return current
}

// setCurrent grava a versão em execução (chamado pelos dois Init).
func setCurrent(v string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	current = v
}

// setReadyVersion registra o alvo da atualização em stage (evento
// EventUpdateReady) — o diálogo de saída usa para nomear a versão.
func setReadyVersion(v string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	ready = v
}

// ReadyVersion devolve a versão que o próximo restart vai aplicar
// ("" quando não há nada em stage).
func ReadyVersion() string {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return ready
}

// ReadyToApply diz se já existe uma atualização baixada, verificada e em
// stage: o próximo restart troca o binário. É o sinal que o App.shouldQuit
// usa para trocar o diálogo normal de "alterações não salvas" por um que
// explica o reinício — sem isso o diálogo de saída cancelaria o restart do
// updater (o helper espera o pai sair e desiste após HelperReadyTimeout).
func ReadyToApply() bool {
	app := application.Get()
	if app == nil {
		return false
	}
	return app.Updater.State() == updater.StateReady
}
