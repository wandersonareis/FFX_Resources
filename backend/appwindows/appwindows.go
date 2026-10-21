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
//   - Spec sidebar (logs): janela companheira NÃO-MODAL na lateral direita
//     da principal — o app segue usável com ela aberta, e o fechamento dela
//     é anunciado (LogWindowClosed) para o checkbox da principal desmarcar.
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

// logsClosedEvent é emitido quando a janela de LOGS fecha (X/Esc/Alt+F4):
// o checkbox da janela principal escuta e desmarca — o estado dos dois
// lados da ponte fica sincronizado nos dois sentidos.
const logsClosedEvent = "LogWindowClosed"

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
	// sidebar: janela COMPANHEIRA na lateral direita da principal (logs).
	// Diferente das modal: NÃO AttachModal (o app segue usável com ela
	// aberta), nasce posicionada à direita da principal COM A ALTURA DELA
	// (a altura do spec é só o tamanho de criação) e avisa o próprio
	// fechamento (logsClosedEvent) para o checkbox da principal desmarcar.
	sidebar bool
}

var specs = map[string]spec{
	// Altura do config = titlebar do WindowChrome (h-9 = 36px) + conteúdo
	// (~389px): sem espaço morto embaixo. Abaixo disso o overflow-auto do
	// chrome rola (hints quebram mais linhas na largura mínima).
	"config": {title: "Configurações", width: 720, height: 425, minWidth: 560, minHght: 420},
	"about":  {title: "Sobre", width: 580, height: 480, minWidth: 480, minHght: 420, html: true},
	// logs: a altura 600 é provisória — positionRightOfMain sobrescreve com
	// a altura da principal no momento da abertura.
	"logs": {title: "Logs", width: 480, height: 600, minWidth: 360, minHght: 280, sidebar: true},
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
	if s.sidebar {
		// Janela companheira: sem modal, sem Center — nasce à direita da
		// principal, com a altura dela (positionRightOfMain), e avisa o
		// próprio fechamento (checkbox desmarca).
		ensureCloseChildrenHook(a)
		positionRightOfMain(a, w, s.width)
		w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
			a.Event.Emit(logsClosedEvent)
		})
		return nil
	}
	w.Center()
	attachToMain(a, w)
	return nil
}

// Close fecha a janela filha nomeada. Janela ausente (nunca aberta, ou já
// fechada pelo X) não é erro: quem chama está sincronizando estado de UI
// (o checkbox de logs desmarcando), não gerenciando ciclo de vida.
func Close(name string) error {
	if _, ok := specs[name]; !ok {
		return fmt.Errorf("janela desconhecida: %q", name)
	}
	a := application.Get()
	if a == nil {
		return fmt.Errorf("app não inicializado")
	}
	if w, found := a.Window.GetByName(name); found {
		w.Close()
	}
	return nil
}

// sidebarGap é a folga entre a borda direita da principal e a janela de logs.
const sidebarGap = 8

// positionRightOfMain ancora a janela à direita da principal com a MESMA
// ALTURA dela (efeito sidebar: nasce e termina junto com a principal — a
// altura do spec é só o tamanho de criação, sobrescrito aqui). Depois é
// janela livre: arrastável e redimensionável. Position/Size/SetSize/
// SetPosition do v3 são DIPs, então a conta e o clamp usam a área útil em
// DIPs (Screen.WorkArea) — a física ficaria em outra escala de coordenadas.
//
// Sem espaço ao lado (principal maximizada, ou larga demais): borda direita
// da área útil, colada no topo e com a altura da área útil (a principal
// maximizada tem a workarea inteira como altura) — cobre a margem morta do
// conteúdo (layout limitado a 1440px), não a faixa da caption da principal
// (senão as duas barras de controles de janela empilham uma sobre a outra).
func positionRightOfMain(a *application.App, w *application.WebviewWindow, width int) {
	main, ok := a.Window.GetByName(MainWindowName)
	if !ok {
		w.Center()
		return
	}
	mx, my := main.Position()
	mw, mh := main.Size()
	h := mh
	x, y := mx+mw+sidebarGap, my

	wa := workAreaAt(a, x, y)
	if wa.Width > 0 {
		if h > wa.Height {
			h = wa.Height
		}
		if x+width > wa.X+wa.Width {
			x = wa.X + wa.Width - width
			y = wa.Y
		}
		if x < wa.X {
			x = wa.X
		}
		if y < wa.Y {
			y = wa.Y
		}
		if y+h > wa.Y+wa.Height {
			y = wa.Y + wa.Height - h
		}
	}
	w.SetSize(width, h)
	w.SetPosition(x, y)
}

// workAreaAt devolve a área útil (DIPs) da tela que contém o ponto — ou a
// primária quando nenhuma contém (ponto fora de todas). Rect zerado = sem
// telas conhecidas: o chamador posiciona sem clamp.
func workAreaAt(a *application.App, x, y int) application.Rect {
	screens := a.Screen.GetAll()
	if len(screens) == 0 {
		return application.Rect{}
	}
	target := screens[0]
	for _, s := range screens {
		if s.IsPrimary {
			target = s
		}
		b := s.Bounds
		if x >= b.X && x < b.X+b.Width && y >= b.Y && y < b.Y+b.Height {
			target = s
			break
		}
	}
	wa := target.WorkArea
	if wa.Width == 0 || wa.Height == 0 {
		wa = target.Bounds
	}
	return wa
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

// attachToMain liga a filha MODAL à principal (doc: Multiple Windows ▸
// Parent-Child Relationships): no Windows vira owner real
// (GWLP_HWNDPARENT) — a filha fica acima, acompanha a principal e a
// principal é desabilitada enquanto a filha está aberta (modal-like).
// Se a principal cair, o hook fecha as filhas junto.
//
// A janela de LOGS não passa por aqui (sidebar: sem AttachModal) — mas
// usa o mesmo hook de fechamento em bloco (ensureCloseChildrenHook).
func attachToMain(a *application.App, w *application.WebviewWindow) {
	parent, ok := a.Window.GetByName(MainWindowName)
	if !ok {
		return
	}
	ensureCloseChildrenHook(a)
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

// ensureCloseChildrenHook garante (uma única vez) que o fecho da principal
// fecha TODAS as filhas abertas — cobre Linux (AttachModal é no-op lá) e o
// caso de o SO destruir a principal sem passar pelo fluxo de owner do
// Windows. Vale para modais (via attachToMain) e para a lateral de logs.
func ensureCloseChildrenHook(a *application.App) {
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
