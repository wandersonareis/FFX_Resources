// Montagem do menu nativo do aplicativo (Wails v3). Este arquivo só declara
// ESTRUTURA — rótulos, submenus e a ligação item→handler; os handlers vivem
// em actions.go e as regras no backend/appcore.
//
//	Aqui apenas o backend e o dialog do Wails — nada de frontend JS.
//
// Plataformas: no Windows/Linux o menu aparece na barra de título da
// janela; no macOS, no menu superior.
package appmenu

import (
	"ffxresources/backend/appcore"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Build monta a árvore do menu e devolve o menu pronto para
// app.Menu.Set(...). A ordem dos itens é a régua combinada: Texto por
// kind (árvore de data), Imagem, Dedupe, Importar por kind e Ajuda.
func Build(a *application.App, c *Controller) *application.Menu {
	menu := a.NewMenu()

	// ---- Arquivo ------------------------------------------------------
	fileMenu := menu.AddSubmenu("Arquivo")
	fileMenu.Add("Sair").
		SetAccelerator("Ctrl+Q").
		OnClick(func(*application.Context) { a.Quit() })

	// ---- Imagem --------------------------------------------------------
	imageMenu := menu.AddSubmenu("Imagem")
	imageMenu.Add("Extrair .dds").
		OnClick(func(*application.Context) { c.onExtractDDS() })
	imageMenu.Add("Extrair .png").
		OnClick(func(*application.Context) { c.onExtractPNG() })
	imageMenu.AddSeparator()
	imageMenu.Add("Importar .dds").
		OnClick(func(*application.Context) { c.onImportImage() })

	// ---- Dedupe ---------------------------------------------------------
	dedupeMenu := menu.AddSubmenu("Dedupe")
	dedupeMenu.Add("Substituir repetidos por hash").
		OnClick(func(*application.Context) { c.onDedupe() })
	dedupeMenu.Add("Resolver hashes").
		OnClick(func(*application.Context) { c.onDupe() })

	// ---- Exportar (export por kind da árvore de data) ---------------------
	textMenu := menu.AddSubmenu("Exportar")
	for _, k := range textKinds {
		kindMenu := textMenu.AddSubmenu(k.label)
		kindMenu.Add("Exportar JSON").
			OnClick(func(*application.Context) { c.onExportText(k.kind, appcore.FormatJSON) })
		kindMenu.Add("Exportar Strings").
			OnClick(func(*application.Context) { c.onExportText(k.kind, appcore.FormatStrings) })
		kindMenu.Add("Exportar CSV").
			OnClick(func(*application.Context) { c.onExportText(k.kind, appcore.FormatCSV) })
		kindMenu.Add("Exportar XLSX").
			OnClick(func(*application.Context) { c.onExportText(k.kind, appcore.FormatXLSX) })
		kindMenu.Add("Exportar PO (gettext)").
			OnClick(func(*application.Context) { c.onExportText(k.kind, appcore.FormatPO) })
	}

	// ---- Importar (mesmos kinds do Texto) -------------------------------
	importMenu := menu.AddSubmenu("Importar")
	for _, k := range textKinds {
		kindMenu := importMenu.AddSubmenu(k.label)
		kindMenu.Add("Importar JSON").
			OnClick(func(*application.Context) { c.onImportText(k.kind, appcore.FormatJSON) })
		kindMenu.Add("Importar Strings").
			OnClick(func(*application.Context) { c.onImportText(k.kind, appcore.FormatStrings) })
		kindMenu.Add("Importar CSV").
			OnClick(func(*application.Context) { c.onImportText(k.kind, appcore.FormatCSV) })
		kindMenu.Add("Importar XLSX").
			OnClick(func(*application.Context) { c.onImportText(k.kind, appcore.FormatXLSX) })
		kindMenu.Add("Importar PO (gettext)").
			OnClick(func(*application.Context) { c.onImportText(k.kind, appcore.FormatPO) })
	}

	// ---- Ajuda ----------------------------------------------------------
	// "Código-fonte" saiu do menu: os links de código-fonte E releases
	// vivem no rodapé da janela Sobre (janela filha de HTML embutido no
	// appwindows).
	helpMenu := menu.AddSubmenu("Ajuda")
	helpMenu.Add("Sobre").
		OnClick(func(*application.Context) { c.onAbout() })
	helpMenu.AddSeparator()
	// Auto-update (backend/appupdater): abre a janela nativa de atualização
	// e roda check → download → verificação → restart. O Wails despacha
	// OnClick fora da thread de UI, então o bloqueio do fluxo não congela
	// a janela.
	helpMenu.Add("Procurar atualizações…").
		OnClick(func(*application.Context) { c.onCheckUpdates() })

	return menu
}
