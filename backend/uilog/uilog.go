// Package uilog é a ponte do log do app para a JANELA DE LOGS: cada entrada
// vai ao MESMO destino do loggingService (console colorido + arquivo JSON —
// "o mesmo log da janela é o log em arquivo e console") e, além disso, ao
// buffer da sessão (histórico: abrir a janela a qualquer momento mostra tudo
// que já foi feito) e ao bus de eventos do Wails (a exibição aberta — janela
// desktop ou drawer web — renderiza ao vivo).
//
// O log é feito DEPOIS da ação: as funções recebem o resultado em mãos
// (File recebe o status por arquivo — sucesso/falha na MESMA linha do
// caminho) e nunca interrompem o fluxo que as chama.
//
// Pacote separado (nem loggingService, nem services): loggingService é
// zerolog puro (sem dependência do Wails) e services é importado por
// builders. A granularidade por binário do apply NÃO passa por builders:
// quem observa as gravações é o gravador de common
// (common.RecordWrites — todo escritor de binário passa por
// common.WriteBytesToFile) e quem narra é o serviço de import
// (backend/services/import_log.go). Assim builders→uilog nunca existe.
package uilog

import (
	"fmt"
	"sync"
	"time"

	"ffxresources/backend/loggingService"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Níveis das entradas — o frontend colore por aqui: success verde, error
// vermelho, info azul, warn laranja (o padrão de terminal pedido).
const (
	LevelInfo    = "info"
	LevelSuccess = "success"
	LevelWarn    = "warn"
	LevelError   = "error"
)

// Status das linhas de arquivo (File). Os rótulos do appcore (Status.String)
// também são aceitos — o nível é derivado em levelForStatus.
const (
	StatusOk      = "ok"
	StatusFailed  = "falha"
	StatusIgnored = "ignorado"
	StatusExists  = "já existe"
)

// Eventos no bus do Wails — a exibição de logs assina o primeiro e se
// esvazia no segundo (botão "Limpar").
const (
	EventAppend  = "UILog"
	EventCleared = "UILogCleared"
)

// Entry é UMA linha da janela de logs. Path+Status (não vazios) caracterizam
// a linha de arquivo: "caminho — status: mensagem" na mesma linha; sem Path
// é linha de ação: "[ação] mensagem".
type Entry struct {
	// Time é o momento da entrada (Unix milliseconds).
	Time int64 `json:"time"`
	// Level decide a cor: info | success | warn | error.
	Level string `json:"level"`
	// Action rotula a operação ("Import", "Export", "VBF"…).
	Action string `json:"action,omitempty"`
	// Message é o texto da linha (ou o detalhe/motivo, nas linhas de arquivo).
	Message string `json:"message"`
	// Path é o arquivo envolvido (binário compilado, artefato importado).
	Path string `json:"path,omitempty"`
	// Status é o resultado na MESMA linha do path ("ok", "falha"…).
	Status string `json:"status,omitempty"`
}

// bufferCap é o teto de segurança do histórico da sessão: além disso, as
// entradas mais antigas caem (arquivo/console continuam com tudo — o buffer
// é só para "abrir a janela e ver o que já foi feito").
const bufferCap = 10000

var (
	mu sync.Mutex
	// entries é o histórico da sessão (mais antigas primeiro), fonte única
	// da verdade da exibição — webviews são contextos JS separados, então
	// guardar log no frontend não sobrevive a fechar/abrir a janela.
	entries []Entry
)

// Info registra uma linha de ação em nível info (azul).
func Info(action, format string, args ...any) {
	publish(LevelInfo, action, fmt.Sprintf(format, args...), "", "")
}

// Success registra uma linha de ação bem-sucedida (verde).
func Success(action, format string, args ...any) {
	publish(LevelSuccess, action, fmt.Sprintf(format, args...), "", "")
}

// Warn registra um aviso recuperável (laranja).
func Warn(action, format string, args ...any) {
	publish(LevelWarn, action, fmt.Sprintf(format, args...), "", "")
}

// Error registra uma falha (vermelho).
func Error(action, format string, args ...any) {
	publish(LevelError, action, fmt.Sprintf(format, args...), "", "")
}

// File registra o resultado de UM arquivo — o caminho e o status na MESMA
// linha ("command.bin — gravado: imported/command.bin"). status aceita as
// constantes Status* e os rótulos do appcore ("gravado", "sobrescrito",
// "incompatível"…); o nível (cor) é derivado. detail carrega o motivo
// (falha) ou o destino (gravado) e pode ser vazio.
func File(action, path, status, detail string) {
	publish(levelForStatus(status), action, detail, path, status)
}

// Clear zera o histórico da sessão e avisa as exibições abertas
// (EventCleared). O arquivo e o console de log NÃO são afetados: são o
// histórico permanente da execução.
func Clear() {
	mu.Lock()
	entries = nil
	mu.Unlock()
	if app := application.Get(); app != nil {
		app.Event.Emit(EventCleared)
	}
}

// History devolve uma cópia das entradas da sessão (mais antigas primeiro)
// — é a semeadura da exibição no momento em que abre.
func History() []Entry {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

// publish é o caminho único: (1) write-through no loggingService (console
// colorido + arquivo JSON), (2) buffer da sessão, (3) evento para as
// exibições abertas. Sem app Wails (testes/dev puro) o emit é no-op —
// arquivo e console seguem valendo.
func publish(level, action, message, path, status string) {
	e := Entry{
		Time:    time.Now().UnixMilli(),
		Level:   level,
		Action:  action,
		Message: message,
		Path:    path,
		Status:  status,
	}
	writeThrough(e)

	mu.Lock()
	entries = append(entries, e)
	if len(entries) > bufferCap {
		entries = entries[len(entries)-bufferCap:]
	}
	mu.Unlock()

	if app := application.Get(); app != nil {
		app.Event.Emit(EventAppend, e)
	}
}

// writeThrough grava a MESMA linha no sistema de log do app: o nível vira
// o nível do zerolog (success→info: é a mesma "notícia boa") e a linha leva
// ação, path e status juntos — a linha curta do console e o JSON do arquivo
// carregam o conteúdo da janela.
func writeThrough(e Entry) {
	action := e.Action
	if action == "" {
		action = "app"
	}
	switch e.Level {
	case LevelError:
		loggingService.Error("%s", consoleLine(e, action))
	case LevelWarn:
		loggingService.Warn("%s", consoleLine(e, action))
	default: // info e success
		loggingService.Info("%s", consoleLine(e, action))
	}
}

// consoleLine monta a linha única: "[ação] mensagem" ou, nas linhas de
// arquivo, "[ação] path — status: detalhe".
func consoleLine(e Entry, action string) string {
	if e.Path == "" {
		return fmt.Sprintf("%s: %s", action, e.Message)
	}
	line := fmt.Sprintf("%s: %s — %s", action, e.Path, e.Status)
	if e.Message != "" {
		line += ": " + e.Message
	}
	return line
}

// levelForStatus deriva o nível (cor) do status da linha de arquivo:
// gravado/sobrescrito são sucesso; falha/incompatível são erro; ignorado e
// colisão são aviso; desconhecido é info.
func levelForStatus(status string) string {
	switch status {
	case StatusOk, "gravado", "sobrescrito":
		return LevelSuccess
	case StatusFailed, "incompatível":
		return LevelError
	case StatusIgnored, StatusExists:
		return LevelWarn
	default:
		return LevelInfo
	}
}
