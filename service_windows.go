package main

import (
	"ffxresources/backend/appwindows"
)

// WindowService expõe a abertura de janelas filhas e de URLs externas ao
// frontend: no desktop o diálogo vira JANELA frameless (arrastável, com
// botões de janela); no browser/server mode o mesmo fluxo cai no modal —
// quem decide é o isDesktop() do frontend, não o backend.
//
// O Sobre NÃO passa por aqui: é janela de HTML embutido no appwindows
// (pinta no primeiro quadro, sem Next no caminho) e os links do rodapé
// viajam como eventos nomeados, mapeados na whitelist do Go.
type WindowService struct{}

// OpenWindow abre (ou foca) a janela filha nomeada — ver backend/appwindows
// para o catálogo (config, about, logs).
func (w *WindowService) OpenWindow(name string) error {
	return appwindows.Open(name)
}

// CloseWindow fecha a janela filha nomeada (inexistente não é erro): é o
// caminho de desfazer do checkbox de logs — o X da própria janela emite
// LogWindowClosed e o checkbox espelha; desmarcar chama isto aqui.
func (w *WindowService) CloseWindow(name string) error {
	return appwindows.Close(name)
}

// OpenExternal abre uma URL no navegador padrão (whitelist: repo/releases).
func (w *WindowService) OpenExternal(url string) error {
	return appwindows.OpenExternal(url)
}
