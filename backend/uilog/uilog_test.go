package uilog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/loggingService"
)

// reset limpa o buffer entre testes (o pacote é estado de sessão).
func reset() {
	Clear()
}

// TestFileLevels confere o mapeamento status → nível (a cor da janela).
func TestFileLevels(t *testing.T) {
	reset()

	cases := map[string]string{
		StatusOk:       LevelSuccess,
		"gravado":      LevelSuccess,
		"sobrescrito":  LevelSuccess,
		StatusFailed:   LevelError,
		"incompatível": LevelError,
		StatusIgnored:  LevelWarn,
		StatusExists:   LevelWarn,
		"qualquer":     LevelInfo,
	}
	for status, want := range cases {
		File("Import", "caminho.bin", status, "detalhe")
		h := History()
		if len(h) == 0 {
			t.Fatalf("status %q: sem entradas", status)
		}
		if got := h[len(h)-1].Level; got != want {
			t.Errorf("status %q: nível %q, queria %q", status, got, want)
		}
		reset()
	}
}

// TestFileEntryShape confere a linha de arquivo: path e status presentes,
// detalhe na mensagem — o contrato do renderizador ("sucesso ou falha na
// mesma linha do caminho").
func TestFileEntryShape(t *testing.T) {
	reset()

	File("Import", `C:\jogo\command.bin`, "gravado", `C:\jogo\imported\command.bin`)

	h := History()
	if len(h) != 1 {
		t.Fatalf("queria 1 entrada, veio %d", len(h))
	}
	e := h[0]
	if e.Path != `C:\jogo\command.bin` {
		t.Errorf("path = %q", e.Path)
	}
	if e.Status != "gravado" {
		t.Errorf("status = %q", e.Status)
	}
	if e.Message != `C:\jogo\imported\command.bin` {
		t.Errorf("message = %q", e.Message)
	}
	if e.Action != "Import" {
		t.Errorf("action = %q", e.Action)
	}
	if e.Time <= 0 {
		t.Errorf("time não preenchido: %d", e.Time)
	}
}

// TestBufferCap confere o teto do histórico: além de bufferCap, as entradas
// mais antigas caem e a ordem se mantém.
func TestBufferCap(t *testing.T) {
	reset()

	for i := 0; i < bufferCap+50; i++ {
		Info("Teste", "linha %d", i)
	}
	h := History()
	if len(h) != bufferCap {
		t.Fatalf("histórico = %d entradas, queria %d", len(h), bufferCap)
	}
	if !strings.Contains(h[0].Message, "linha 50") {
		t.Errorf("a mais antiga deveria ser a linha 50, veio %q", h[0].Message)
	}
	if !strings.Contains(h[len(h)-1].Message, strconv.Itoa(bufferCap+49)) {
		t.Errorf("a mais nova deveria ser a linha %d, veio %q", bufferCap+49, h[len(h)-1].Message)
	}
}

// TestClear zera o histórico.
func TestClear(t *testing.T) {
	reset()

	Info("Teste", "uma linha")
	Success("Teste", "outra linha")
	if len(History()) != 2 {
		t.Fatalf("queria 2 entradas, veio %d", len(History()))
	}
	Clear()
	if got := History(); len(got) != 0 {
		t.Fatalf("depois do Clear queria 0 entradas, veio %d", len(got))
	}
}

// TestWriteThroughFile confere que a MESMA linha chega ao arquivo de log do
// app (JSON, campo message) — "o mesmo log da janela é o log em arquivo".
func TestWriteThroughFile(t *testing.T) {
	reset()

	dir := tempdir.TempDir(t)
	loggingService.ResetForTest(dir)
	defer func() { loggingService.ResetForTest(tempdir.TempDir(t)) }()

	File("Import", `C:\jogo\fscene.bin`, "falha", "reconstrução inválida")
	Success("Import", "lote concluído")

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil || len(files) == 0 {
		t.Fatalf("sem arquivo de log em %s (err=%v)", dir, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("lendo log: %v", err)
	}

	var sawFile, sawSuccess bool
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("linha não-JSON: %q (%v)", line, err)
		}
		if msg, _ := rec["message"].(string); msg == "Import: C:\\jogo\\fscene.bin — falha: reconstrução inválida" {
			sawFile = true
		}
		if msg, _ := rec["message"].(string); msg == "Import: lote concluído" {
			sawSuccess = true
		}
	}
	if !sawFile {
		t.Error("a linha de arquivo (path — status: detalhe) não chegou ao log")
	}
	if !sawSuccess {
		t.Error("a linha de sucesso não chegou ao log")
	}
}

// TestConcurrentPublish roda publicações paralelas: sem data race, sem
// perder entradas (go test -race é o juiz final).
func TestConcurrentPublish(t *testing.T) {
	reset()

	const goroutines, perG = 8, 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				Info("Teste", "g%d-%d", g, i)
			}
		}(g)
	}
	wg.Wait()
	if got := len(History()); got != goroutines*perG {
		t.Errorf("histórico = %d, queria %d", got, goroutines*perG)
	}
}
