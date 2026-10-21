// Package appwindows é o gerenciador das janelas FILHAS do app (frameless,
// conteúdo HTML/React dentro — o mesmo papel dos modais, mas como janela de
// verdade: arrastável, com botões de janela e foco próprio).
//
// Réguas:
//   - Toda janela filha nasce de um SPEC nomeado aqui: título, geometria e
//     conteúdo (URL) moram num lugar só — frontend e menu abrem pelo MESMO
//     nome, sem duplicar opções.
//   - URL "/w#<nome>": a rota /w é o host React (frontend/src/app/w) e o
//     hash escolhe o conteúdo montado ali dentro.
//   - Singleton por nome: GetByName → Show/Focus. O Wails remove a janela do
//     registry quando ela fecha, então a próxima abertura recria limpo — sem
//     mapa próprio e sem vazamento de referência.
package appwindows

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// MainWindowName é o Name da janela principal (main.go). Toda janela filha
// nasce ligada a ela e morre com ela.
const MainWindowName = "main"

// closeChildrenHook garante (uma única vez) que o fecho da principal fecha
// todas as filhas abertas — cobre Linux (AttachModal é no-op lá) e o caso de
// o SO destruir a principal sem passar pelo fluxo de owner do Windows.
var closeChildrenHook sync.Once

// URLs do repositório — fonte ÚNICA (o About exibe e o OpenExternal valida
// contra estes valores; renomear o remote é mudar aqui).
const (
	RepositoryURL = "https://github.com/wandersonareis/FFX_Resources"
	ReleasesURL   = RepositoryURL + "/releases"
)

// Eventos da janela Sobre — nomes SECOS, sem payload (o caminho
// postMessage do shim só leva o nome — ver inline_event_shim.go do
// framework). O Go mapeia cada nome na URL da whitelist acima.
const (
	aboutOpenSource   = "about:open-source"
	aboutOpenReleases = "about:open-releases"
)

//go:embed about.html
var aboutHTMLSrc string

// logoUS são os bytes de docs/imagens/logo_us.png — o go:embed não
// alcança fora do pacote (não sobe ../..), então o main.go embute e
// injeta via SetAboutLogo antes do app.Run.
var logoUS []byte

// SetAboutLogo registra os bytes do logo usado pela janela Sobre (o HTML
// injeta em base64 na montagem — ver aboutHTML).
func SetAboutLogo(png []byte) { logoUS = png }

// spec é a ficha de uma janela filha: chrome, geometria e conteúdo.
type spec struct {
	title             string
	width, height     int
	minWidth, minHght int
	// html: janela de conteúdo EMBUTIDO (WebviewWindowOptions.HTML), como
	// a do updater do framework — pinta no primeiro quadro, sem Next no
	// caminho. Sem a flag, a janela é frameless servindo a rota /w.
	html bool
}

var specs = map[string]spec{
	// Altura do config = titlebar do WindowChrome (h-9 = 36px) + conteúdo
	// (~389px): sem espaço morto embaixo. Abaixo disso o overflow-auto do
	// chrome rola (hints quebram mais linhas na largura mínima).
	"config": {title: "Configurações", width: 720, height: 425, minWidth: 560, minHght: 420},
	"about":  {title: "Sobre", width: 580, height: 480, minWidth: 480, minHght: 420, html: true},
}

// Open abre (ou foca, se já aberta) a janela filha pelo nome.
func Open(name string) error {
	s, ok := specs[name]
	if !ok {
		return fmt.Errorf("janela desconhecida: %q", name)
	}
	a := application.Get()
	if a == nil {
		return fmt.Errorf("app não inicializado")
	}
	registerAboutEvents(a)
	if w, found := a.Window.GetByName(name); found {
		w.Show()
		w.Focus()
		return nil
	}
	opts := application.WebviewWindowOptions{
		Name:      name,
		Title:     s.title,
		Width:     s.width,
		Height:    s.height,
		MinWidth:  s.minWidth,
		MinHeight: s.minHght,
		// Fundo escuro para não piscar branco antes do CSS (tema default dark).
		BackgroundColour: application.NewRGB(21, 22, 26),
	}
	if s.html {
		// HTML NA CONSTRUÇÃO — o updater_host.go do Wails adverte que
		// SetHTML pós-criação carrega em about:blank SEM o shim e os
		// emits fazem no-op silencioso. AllowSimpleEventEmit injeta
		// window.wails.Events (postMessage). Moldura NATIVA: sem o
		// runtime JS não existe --wails-draggable — X/arrasto/resize
		// vêm do SO, como na janela do updater.
		opts.HTML = aboutHTML()
		opts.AllowSimpleEventEmit = true
	} else {
		opts.Frameless = true
		opts.URL = "/w#" + name
	}
	w := a.Window.NewWithOptions(opts)
	w.Center()
	attachToMain(a, w)
	return nil
}

// registerAboutEvents liga os eventos da janela Sobre às URLs da
// whitelist. sync.Once: o bus do app é global — o registro vale para
// todas as aberturas (e janelas HTML recriam a cada ciclo). O Sender do
// CustomEvent carrega o nome da janela emissora — só o próprio Sobre é
// atendido.
var aboutEventsOnce sync.Once

func registerAboutEvents(a *application.App) {
	aboutEventsOnce.Do(func() {
		on := func(name, url string) {
			a.Event.On(name, func(e *application.CustomEvent) {
				if e.Sender != "about" {
					return
				}
				if err := OpenExternal(url); err != nil {
					a.Logger.Error("abrir navegador", "janela", "Sobre", "error", err)
				}
			})
		}
		on(aboutOpenSource, RepositoryURL)
		on(aboutOpenReleases, ReleasesURL)
	})
}

// aboutHTML monta o documento da janela Sobre: o logo em base64 entra no
// lugar do {{LOGO_IMG}} — o conteúdo inteiro já está pronto no primeiro
// quadro (zero fetch, binding ou hidratação).
func aboutHTML() string {
	img := ""
	if len(logoUS) > 0 {
		img = fmt.Sprintf(
			`<img class="logo" alt="FINAL FANTASY X | X-2 HD Remaster" src="data:image/png;base64,%s"/>`,
			base64.StdEncoding.EncodeToString(logoUS))
	}
	return strings.Replace(aboutHTMLSrc, "{{LOGO_IMG}}", img, 1)
}

// attachToMain liga a filha à principal (doc: Multiple Windows ▸
// Parent-Child Relationships): no Windows vira owner real
// (GWLP_HWNDPARENT) — a filha fica acima, acompanha a principal e a
// principal é desabilitada enquanto a filha está aberta (modal-like).
// Se a principal cair, o hook fecha as filhas junto.
func attachToMain(a *application.App, w *application.WebviewWindow) {
	parent, ok := a.Window.GetByName(MainWindowName)
	if !ok {
		return
	}
	closeChildrenHook.Do(func() {
		parent.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
			for name := range specs {
				if child, found := a.Window.GetByName(name); found {
					child.Close()
				}
			}
		})
	})
	parent.AttachModal(w)
	// Devolução do foco: o Wails só reabilita a owner DEPOIS do
	// DestroyWindow (defer do 2º WM_CLOSE) — no instante da destruição
	// a principal ainda está DESABILITADA e o Windows entrega a ativação
	// para a janela de OUTRO app. Este hook roda ANTES: HandleWindowEvent
	// processa hooks sincronos primeiro e só então os listeners — entre
	// eles o listener interno (WindowClosing) que dispara a destruição.
	// Reabilitar e focar a principal aqui deixa a ativação já nela
	// quando a filha morre (o defer do Wails vira re-enable inofensivo).
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		parent.SetEnabled(true)
		parent.Focus()
	})
}

// OpenExternal abre uma URL no navegador padrão — só URLs conhecidas
// (whitelist): o frontend jamais decide sozinho o que é aberto fora do app.
func OpenExternal(url string) error {
	switch url {
	case RepositoryURL, ReleasesURL:
	default:
		return fmt.Errorf("URL não permitida: %s", url)
	}
	a := application.Get()
	if a == nil {
		return fmt.Errorf("app não inicializado")
	}
	if err := a.Browser.OpenURL(url); err != nil {
		return fmt.Errorf("abrir navegador: %w", err)
	}
	return nil
}
