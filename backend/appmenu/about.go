// Janela Sobre: agora é uma janela FILHA frameless servida pela rota React
// /w#about (frontend/src/components/window/about-window.tsx) — logo,
// descrição e os links de código-fonte e releases no rodapé, abertos no
// navegador pelo backend (OpenExternal, whitelist).
//
// Substitui a versão anterior em HTML/CSS embutido no Go
// (WebviewWindowOptions.HTML): o conteúdo virou frontend para ter o mesmo
// tratamento dos demais diálogos.
package appmenu

import (
	"ffxresources/backend/appwindows"
)

// onAbout abre (ou foca) a janela Sobre — mesma janela singleton que o
// frontend abre pelo binding WindowService.OpenWindow("about").
func (c *Controller) onAbout() {
	if err := appwindows.Open("about"); err != nil {
		c.errBox("Sobre", "Não foi possível abrir a janela:\n"+classifyError(err))
	}
}
