//go:build !server

package appupdater

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"ffxresources/backend/interactions"
	"ffxresources/backend/loggingService"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// windowHTML é a janela de update em pt-BR (cópia adaptada do template
// assets/window.html do próprio Wails v3, MIT). Textos e cores mudam; o JS,
// os ids e o atributo data-state são contrato com o updater e ficam intactos.
//
//go:embed window.html
var windowHTML string

// startupDelay dá tempo da janela principal nascer e do frontend assinar os
// eventos updater:* antes do primeiro check silencioso.
const startupDelay = 5 * time.Second

// checkTimeout limita o check de startup (a API do GitHub pode engasgar e o
// usuário não deve esperar por isso).
const checkTimeout = 30 * time.Second

// themeToken é o placeholder de tema no window.html: o HTML nasce com
// `data-theme="__FFX_THEME__"` e o renderTheme troca por "dark"/"light"
// antes de a janela abrir (composeHTML roda em cada CheckAndInstall lendo
// este ponteiro — é por isso que a janela pinta já no tema certo).
const themeToken = "__FFX_THEME__"

// builtinWindow é o ponteiro entregue ao updater no Init. Mutá-lo aqui vale
// para a PRÓXIMA abertura: o Config do updater guarda a interface com o
// mesmo *BuiltinWindow.
var builtinWindow = &updater.BuiltinWindow{
	HTML: windowHTML,
	Options: updater.WindowOptions{
		Title: "Atualização de software",
	},
}

// renderTheme materializa o tema atual no HTML da janela. Roda no Init e
// antes de cada abertura — nunca deixa token não substituído (senão o
// seletor [data-theme] não casa e a janela abriria no tema errado de novo).
func renderTheme() {
	builtinWindow.HTML = strings.ReplaceAll(windowHTML, themeToken, CurrentTheme())
}

// emitTheme avisa TODAS as janelas (main e a de update, se aberta) sobre a
// troca de tema. A de update escuta `ffx:theme` e só troca o atributo
// data-theme — o CSS é dirigido por ele, então o restoyle é instantâneo.
func emitTheme(theme string) {
	if app := application.Get(); app != nil {
		app.Event.Emit("ffx:theme", theme)
	}
}

// Init configura o app.Updater com o provider de releases do GitHub e a
// janela de update própria. Deve rodar depois de application.New — é lá que o
// updater detecta o modo helper (WAILS_UPDATER_HELPER=1), troca o binário e
// encerra antes de qualquer código daqui — e antes de app.Run().
func Init(app *application.App, version string) error {
	setCurrent(version)

	gh, err := github.New(github.Config{
		Repository:    repository,
		ChecksumAsset: checksumAsset,
	})
	if err != nil {
		return fmt.Errorf("updater: provider github: %w", err)
	}

	SetTheme(interactions.NewInteractionService().FFXAppConfig().GetTheme())
	renderTheme()

	cfg := updater.Config{
		// Sem "v" na frente: o provider corta o prefixo das tags (v2.1.1).
		CurrentVersion: version,
		Providers:      []updater.Provider{gh},
		Window:         builtinWindow,
	}
	if err := app.Updater.Init(cfg); err != nil {
		return fmt.Errorf("updater: init: %w", err)
	}

	// Guarda o alvo quando a atualização entra em stage: o diálogo de saída
	// (App.shouldQuit) nomeia a versão que será aplicada no reinício.
	app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
		if e == nil {
			return
		}
		switch v := e.Data.(type) {
		case *updater.Release:
			setReadyVersion(v.Version)
		case updater.Release:
			setReadyVersion(v.Version)
		}
	})

	return nil
}

// StartupCheck faz UM check silencioso (sem janela) alguns segundos após o
// app subir e, se houver release mais nova, abre o fluxo completo — janela
// com as notas da release, download, verificação e stage. Sem release nova o
// usuário nem fica sabendo.
//
// No-op em build de desenvolvimento (currentVersion "dev", sem -ldflags):
// qualquer release real é "mais nova" que a string dev e a janela abriria em
// todo início de `wails3 dev`.
func StartupCheck(app *application.App) {
	if v := CurrentVersion(); v == "" || v == "dev" {
		return
	}

	go func() {
		time.Sleep(startupDelay)

		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		rel, err := app.Updater.Check(ctx)
		cancel()
		if err != nil {
			// Offline, rate limit, repo inacessível: não é o momento de
			// incomodar — o usuário ainda pode checar pelo menu Ajuda.
			loggingService.Debug("updater: check de startup ignorado: %s", err)
			return
		}
		if rel == nil {
			return // já é a última versão
		}

		loggingService.Info("updater: %s disponível (atual %s) — abrindo a janela de atualização", rel.Version, CurrentVersion())
		renderTheme()
		if err := app.Updater.CheckAndInstall(context.Background()); err != nil {
			loggingService.Error("updater: atualização do startup falhou: %s", err)
		}
	}()
}

// CheckNow abre a janela nativa de atualização e percorre o fluxo completo
// (check → download → verificação → stage). Bloqueia até o fim — menu e
// UpdateService já a chamam fora da thread de UI. Com tudo em dia a janela
// permanece aberta no estado "Você está atualizado" e o usuário a fecha.
func CheckNow() error {
	app := application.Get()
	if app == nil {
		return errors.New("updater: aplicativo ainda não inicializado")
	}
	renderTheme()
	return app.Updater.CheckAndInstall(context.Background())
}

// Restart aplica a atualização em stage: sobe o helper (o próprio binário,
// com as variáveis WAILS_UPDATER_HELPER_*), pede o Quit e, quando o processo
// sai, o helper troca o binário e relança a nova versão.
func Restart() error {
	app := application.Get()
	if app == nil {
		return errors.New("updater: aplicativo ainda não inicializado")
	}
	return app.Updater.Restart(context.Background())
}
