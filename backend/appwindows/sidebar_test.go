package appwindows

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fullHD é a área útil de uma tela 1920x1080 com a barra de tarefas embaixo.
var fullHD = application.Rect{X: 0, Y: 0, Width: 1920, Height: 1040}

// TestSidebarUsable cobre a regra de restauração da sidebar de logs: a
// geometria salva (que sobrevive ao Hide do reuso) só vale se a titlebar
// ainda estiver agarrável em alguma tela — o resto cai fora e o chamador
// reancora na principal.
func TestSidebarUsable(t *testing.T) {
	cases := map[string]struct {
		saved application.Rect
		areas []application.Rect
		want  bool
	}{
		"ancorada na direita da principal (primeira abertura)": {
			saved: application.Rect{X: 1440, Y: 0, Width: 480, Height: 1040},
			areas: []application.Rect{fullHD},
			want:  true,
		},
		"inteira dentro da área útil": {
			saved: application.Rect{X: 100, Y: 100, Width: 480, Height: 800},
			areas: []application.Rect{fullHD},
			want:  true,
		},
		"estacionada rente à borda inferior (titlebar à mostra)": {
			saved: application.Rect{X: 1400, Y: 950, Width: 480, Height: 900},
			areas: []application.Rect{fullHD},
			want:  true,
		},
		"cortada à esquerda mas com 100px de titlebar na tela": {
			saved: application.Rect{X: -380, Y: 100, Width: 480, Height: 800},
			areas: []application.Rect{fullHD},
			want:  true,
		},
		"titlebar empurrada para fora do topo (sem como arrastar)": {
			saved: application.Rect{X: 100, Y: -40, Width: 480, Height: 800},
			areas: []application.Rect{fullHD},
			want:  false,
		},
		"só um fio à mostra (40px de titlebar < 80)": {
			saved: application.Rect{X: -440, Y: 100, Width: 480, Height: 800},
			areas: []application.Rect{fullHD},
			want:  false,
		},
		"fora de toda tela (monitor desconectado)": {
			saved: application.Rect{X: 2600, Y: 100, Width: 480, Height: 800},
			areas: []application.Rect{fullHD},
			want:  false,
		},
		"vivendo na segunda tela, ainda conectada": {
			saved: application.Rect{X: 2100, Y: 100, Width: 480, Height: 800},
			areas: []application.Rect{fullHD, {X: 1920, Y: 0, Width: 2560, Height: 1440}},
			want:  true,
		},
		"retângulo zerado (nunca posicionada)": {
			saved: application.Rect{},
			areas: []application.Rect{fullHD},
			want:  false,
		},
		"sem telas conhecidas (sem como julgar → reancora)": {
			saved: application.Rect{X: 1440, Y: 0, Width: 480, Height: 1040},
			areas: nil,
			want:  false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := sidebarUsable(tc.saved, tc.areas)
			if got != tc.want {
				t.Errorf("sidebarUsable(%+v) = %v, queria %v", tc.saved, got, tc.want)
			}
		})
	}
}
