package appwindows

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Grupo de foco das janelas NÃO-MODAIS.
//
// Problema: cada janela do app é um top-level independente. Clicar na
// principal ativa só a principal; a sidebar de logs fica atrás (inclusive de
// janelas de OUTROS apps) e, se estiver minimizada, nem volta.
//
// Solução 100% Wails: DETECTAR o foco por janela (events.Common.WindowFocus)
// e PROPAGAR para as outras janelas abertas do grupo — Restore() nas
// minimizadas e Focus() em todas, terminando com Focus() na janela clicada.
// Assim o grupo inteiro vem para a frente, com a clicada ativa e por cima.
//
// Por que o debounce: foco é exclusivo e WindowFocus chega de forma
// assíncrona (canal → goroutine). Cada Focus() que a propagação dispara gera
// novos WindowFocus DEPOIS do handler retornar, então um booleano de
// reentrância não segura o ping-pong — uma janela curta de silêncio segura.
//
// Minimizar é SINCRONIZADO (minimisedGroup): minimizar uma janela ativa a
// outra, e sem esse estado a propagação restauraria a que o usuário acabou de
// minimizar (a janela nunca minimizava). A regra é: só restaura companheira
// minimizada quando a janela focada TAMBÉM estava minimizada — sinal de que o
// grupo está voltando do taskbar; se a clicada estava visível, a minimizada
// foi escolha do usuário e fica como está. Maximizar não propaga: Focus() não
// desmaximiza e só uma janela pode estar maximizada por vez na tela.
//
// Quem participa: a janela principal (criada no main.go, por isso entra
// explicitamente na enumeração) + toda spec sem `modal: true`. Uma janela
// não-bloqueante nova só precisa existir no mapa `specs` e ser criada pelo
// Open — o registro no grupo é automático (ver Open).

// focusPropagationQuiet é o silêncio exigido entre duas propagações. Precisa
// cobrir a ida e volta dos eventos que a própria propagação causa (os dois
// Focus() e o par WM_ACTIVATE/WM_SETFOCUS que o Windows manda por ativação).
const focusPropagationQuiet = 300 * time.Millisecond

// lastPropagation guarda o instante (UnixNano) da última propagação. É
// atomic porque os eventos de janela são despachados em goroutines.
var lastPropagation atomic.Int64

// minimisedGroup marca, por ID de janela, que ela foi minimizada e ainda não
// recebeu foco de volta. IDs do Wails são únicos por janela criada (nunca
// reusados), então não há colisão entre uma janela fechada e uma nova.
var minimisedGroup sync.Map

// markMinimised registra a minimização da janela (hook de WindowMinimise).
func markMinimised(id uint) { minimisedGroup.Store(id, true) }

// takeMinimised devolve se a janela estava minimizada e já limpa a marca: quem
// consome é o evento de foco, que é justamente o momento em que ela deixa de
// estar minimizada (restaurada pelo usuário ou por nós).
func takeMinimised(id uint) bool {
	_, was := minimisedGroup.LoadAndDelete(id)
	return was
}

// focusGroupOnce instala o hook da PRINCIPAL uma única vez. O hook enumera o
// grupo no momento do foco (e não na criação), então janelas companheiras
// abertas depois entram sem precisar de re-registro.
var focusGroupOnce sync.Once

// registerCompanion liga uma janela NÃO-MODAL ao grupo de foco. O hook é por
// janela porque o Wails entrega WindowFocus por janela (não há listener de
// "foco do app").
func registerCompanion(a *application.App, w *application.WebviewWindow) {
	registerWindowHooks(a, w)

	// A principal não é criada por este pacote (main.go), então o hook dela é
	// instalado na primeira companheira. Se a principal ainda não estiver no
	// registry, não consome o Once: a próxima abertura tenta de novo.
	if main, ok := a.Window.GetByName(MainWindowName); ok {
		focusGroupOnce.Do(func() {
			mainW, ok := main.(*application.WebviewWindow)
			if !ok {
				return
			}
			registerWindowHooks(a, mainW)
		})
	}
}

// registerWindowHooks liga a janela ao grupo: foco propaga, minimização é
// registrada (ver minimisedGroup).
func registerWindowHooks(a *application.App, w *application.WebviewWindow) {
	w.RegisterHook(events.Common.WindowFocus, func(*application.WindowEvent) {
		propagateFocus(a, w)
	})
	w.RegisterHook(events.Common.WindowMinimise, func(*application.WindowEvent) {
		markMinimised(w.ID())
	})
}

// propagateFocus leva o grupo inteiro para a frente quando uma janela do grupo
// recebe foco. A janela clicada (ref) termina ativa e por cima.
//
// Ordem: primeiro as companheiras (Restore nas minimizadas, Focus em todas) e
// por último ref — o último Focus() define quem fica ativo/na frente. Sem esse
// Focus() final a companheira ficaria ativa e a janela clicada atrás dela.
func propagateFocus(a *application.App, ref application.Window) {
	// Consumir SEMPRE o estado, antes de qualquer saída antecipada: este evento
	// de foco é o instante em que ref deixa de estar minimizada.
	wasMinimised := takeMinimised(ref.ID())

	if modalOpen(a) {
		return
	}

	others := groupOthers(a, ref, wasMinimised)
	if len(others) == 0 {
		return
	}

	if !beginPropagation() {
		return
	}

	for _, other := range others {
		if other.IsMinimised() {
			// Só chega aqui quando o grupo inteiro está voltando do taskbar
			// (wasMinimised): groupOthers já filtrou as minimizadas nos outros
			// casos. Restaura e limpa a marca — o foco dela que vem a seguir
			// não deve ser lido como "estava minimizada".
			other.Restore()
			takeMinimised(other.ID())
			continue
		}
		other.Focus()
	}
	ref.Focus()
}

// beginPropagation aplica o debounce e reserva a vez da propagação. Fica
// separado do resto para o estado de minimização ser consumido mesmo quando o
// evento é descartado (ver propagateFocus).
func beginPropagation() bool {
	now := time.Now().UnixNano()
	prev := lastPropagation.Load()
	if now-prev < int64(focusPropagationQuiet) {
		return false
	}
	return lastPropagation.CompareAndSwap(prev, now)
}

// groupOthers devolve as outras janelas ABERTAS do grupo, já filtrando as
// ocultas — a sidebar fechada no reuso (Hide) não deve ressurgir quando o
// usuário clica na principal.
//
// `includeMinimised` só é verdadeiro quando a janela focada também estava
// minimizada (grupo voltando do taskbar). Fora disso, uma companheira
// minimizada foi escolha do usuário e fica de fora: incluí-la faria a janela
// ressuscitar sozinha a cada clique na outra.
//
// A PRINCIPAL entra explicitamente: ela não nasce do `specs` (é criada no
// main.go), então enumerar só as specs deixaria o grupo vazio quando a janela
// focada fosse uma companheira — era o bug do "só funciona de main para logs".
func groupOthers(a *application.App, ref application.Window, includeMinimised bool) []application.Window {
	refID := ref.ID()
	others := make([]application.Window, 0, len(specs)+1)
	add := func(w application.Window) {
		if w.ID() == refID || !w.IsVisible() {
			return
		}
		if w.IsMinimised() && !includeMinimised {
			return
		}
		others = append(others, w)
	}

	if main, ok := a.Window.GetByName(MainWindowName); ok {
		add(main)
	}
	for name, s := range specs {
		if s.modal {
			continue
		}
		if other, found := a.Window.GetByName(name); found {
			add(other)
		}
	}
	return others
}

// modalOpen informa se alguma janela modal do pacote está aberta. Barato e
// sem estado extra: a fonte da verdade é a registry do Wails (coerente com o
// resto do pacote, que nunca guarda mapa próprio de janelas). Com modal
// aberta a principal está DESABILITADA (AttachModal) — propagar foco para ela
// não teria efeito e brigaria com o fluxo do modal.
func modalOpen(a *application.App) bool {
	for name, s := range specs {
		if !s.modal {
			continue
		}
		if _, found := a.Window.GetByName(name); found {
			return true
		}
	}
	return false
}
