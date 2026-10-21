//go:build server

package appupdater

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// errNoGUI é o que o frontend recebe ao chamar CheckNow/Restart no modo
// servidor: não há janela nativa para mostrar nem fluxo de troca de binário
// com o que o updater trabalha no desktop.
var errNoGUI = errors.New("atualização automática indisponível no modo servidor")

// Init, no modo servidor, só registra a versão: configurar o updater não
// teria efeito (o check abriria uma janela que não existe) e o startup check
// ficaria falhando a cada abertura.
func Init(_ *application.App, version string) error {
	setCurrent(version)
	return nil
}

// StartupCheck é no-op no modo servidor — nenhum evento updater:* é emitido
// e a banner do frontend nunca acorda.
func StartupCheck(_ *application.App) {}

// CheckNow não tem o que fazer sem GUI.
func CheckNow() error { return errNoGUI }

// Restart não tem o que fazer sem GUI.
func Restart() error { return errNoGUI }

// emitTheme é no-op no modo servidor: não há janela de update para avisar.
func emitTheme(string) {}
