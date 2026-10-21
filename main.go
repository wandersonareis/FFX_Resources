package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"

	"ffxresources/backend/appmenu"
	"ffxresources/backend/appwindows"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/out
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// logoUS é o logo usado na documentação (docs/imagens/logo_us.png) —
// embutido no package main (go:embed não aceita "..") e injetado no
// appwindows, que o serve em base64 na janela Sobre de HTML puro.
//
//go:embed docs/imagens/logo_us.png
var logoUS []byte

func main() {
	// Create an instance of the app structure
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				fmt.Println("Recovered from panic:", e.Error())
			} else {
				fmt.Printf("Recovered from panic: %v\n", r)
			}
		}
	}()

	appInstance := NewApp()

	// v2: wails.Run(&options.App{...}) combinava opções, janela e execução.
	// v3 separa em application.New (app + serviços), Window.NewWithOptions
	// (janela) e app.Run (event loop).
	app := application.New(application.Options{
		Name: "FINAL FANTASY X and X-2 HD Remaster Resources Editor",
		Icon: icon,
		// Modo servidor (`wails3 task build:server` com -tags server): o
		// mesmo binário sobe como HTTP puro, sem janela nativa, servindo o
		// frontend em http://localhost:9246 — bindings e eventos atravessam
		// o /wails/runtime do Wails no mesmo origem. No build desktop
		// (sem a tag) estas opções são ignoradas.
		//
		// 8080 é o default do Wails e a porta mais batida do dev web
		// (proxies, dev servers, Docker de exemplo): a 9246 segue a faixa
		// do dev server do projeto (a 9245 do `wails3 dev`). WAILS_SERVER_
		// HOST/PORT sobrepõem em runtime (Docker/CI) sem rebuild.
		Server: application.ServerOptions{
			Host: "localhost",
			Port: 9246,
		},
		// App (sessão/ciclo de vida) + serviços por domínio.
		Services: appServices(appInstance),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// v2: OnBeforeClose. shouldQuit devolve true para PERMITIR a saída,
		// o inverso do `prevent` do v2.
		ShouldQuit: appInstance.shouldQuit,
		// v2: LogLevel logger.TRACE — o v3 não tem TRACE; Debug é o nível
		// mais baixo disponível.
		LogLevel: slog.LevelDebug,
		Windows: application.WindowsOptions{
			// v2: WebviewUserDataPath: "" (default %APPDATA%\<binário>).
			WebviewUserDataPath: "",
		},
	})

	// Menu nativo do app (backend puro, zero JS): os handlers moram em
	// backend/appmenu e as operações em backend/appcore — o mesmo núcleo
	// que um CLI futuro usaria. Instalado após o app existir e antes do
	// event loop.
	menuController := appmenu.NewController()
	app.Menu.Set(appmenu.Build(app, menuController))

	// Logo da janela Sobre: o go:embed do appwindows não alcança docs/,
	// então quem embute é o main e injeta aqui (o Sobre é HTML puro — o
	// logo entra em base64 no documento).
	appwindows.SetAboutLogo(logoUS)

	// v2: Title/Width/Height/MinWidth/MinHeight/BackgroundColour/WindowStartState
	// moravam em options.App; no v3 viraram opções da janela. O menu e o menu
	// de contexto padrão continuam desligados/ligados por default
	// (EnableDefaultContextMenu: true vira DefaultContextMenuDisabled: false).
	//
	// UseApplicationMenu: no v3 o app.Menu.Set NÃO basta no Windows/Linux —
	// cada janela precisa ACEITAR o menu do app (o default é false, por
	// compatibilidade). No macOS não há efeito: lá o menu é sempre global.
	_ = app.Window.NewWithOptions(application.WebviewWindowOptions{
		// Name identifica a principal no registry (appwindows): as janelas
		// filhas nascem ligadas a ela (AttachModal) e fecham junto com ela.
		Name:               "main",
		Title:              "FINAL FANTASY X and X-2 HD Remaster Resources Editor",
		Width:              1280,
		Height:             800,
		MinWidth:           1024,
		MinHeight:          600,
		StartState:         application.WindowStateNormal,
		BackgroundColour:   application.NewRGB(255, 255, 255),
		URL:                "/",
		UseApplicationMenu: true,
		// v2: mac.Options{TitleBar, Appearance} — no v3 essas opções passaram
		// para a janela.
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBar{
				AppearsTransparent:   true,
				HideToolbarSeparator: true,
			},
			Appearance: application.NSAppearanceNameDarkAqua,
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
