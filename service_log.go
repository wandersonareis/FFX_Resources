package main

import (
	"ffxresources/backend/uilog"
)

// LogService expõe o histórico de logs da sessão às exibições de log (janela
// desktop /w#logs e drawer web): o buffer vive no backend/uilog — as
// webviews são contextos JS separados, então "abrir e ver tudo que já foi
// feito" só pode semear daqui.
type LogService struct{}

// History devolve as entradas da sessão (mais antigas primeiro): é a
// semeadura da exibição no momento em que abre.
func (l *LogService) History() []uilog.Entry {
	return uilog.History()
}

// ClearLog zera o histórico da sessão e avisa as exibições abertas
// (evento UILogCleared). O arquivo e o console de log NÃO mudam — são o
// registro permanente da execução.
func (l *LogService) ClearLog() error {
	uilog.Clear()
	return nil
}
