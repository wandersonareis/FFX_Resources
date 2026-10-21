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
// para o catálogo (config, about).
func (w *WindowService) OpenWindow(name string) error {
	return appwindows.Open(name)
}

// OpenExternal abre uma URL no navegador padrão (whitelist: repo/releases).
func (w *WindowService) OpenExternal(url string) error {
	return appwindows.OpenExternal(url)
}
