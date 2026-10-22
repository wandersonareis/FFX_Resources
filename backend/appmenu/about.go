// Janela Sobre: janela FILHA frameless de HTML embutido — o documento
// about.html vive no appwindows (WebviewWindowOptions.HTML) e o logo entra
// em base64 na montagem (SetAboutLogo, main.go). Logo, descrição e os
// links de código-fonte e releases no rodapé, abertos no navegador pelo
// backend (OpenExternal, whitelist).
//
// A versão anterior em rota React (/w#about) saiu de cena: conteúdo
// estático pinta no primeiro quadro sem Next no caminho (mesma decisão
// da janela do updater do framework).
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
