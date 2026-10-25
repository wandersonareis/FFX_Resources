package loggingService_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/loggingService"
)

// reset reinicializa o sistema de logs em um diretório temporário com o
// console silenciado: os testes asserem os ARQUIVOS.
func resetForTest(t *testing.T) string {
	t.Helper()
	dir := tempdir.TempDir(t)
	loggingService.ResetForTest(dir)
	t.Cleanup(func() { loggingService.ResetForTest(dir) })
	return dir
}

// readFile devolve o conteúdo de um arquivo de log.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler %s: %v", path, err)
	}
	return string(b)
}

// logFilesIn devolve o nome do ÚNICO arquivo de log do diretório ("" quando
// ausente): um arquivo por início de app, com o diagnóstico FUNDIDO nele
// (eventos DiagInfo/DiagWarn identificados pelo campo `key`).
func logFilesIn(dir string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ffx-") {
			return e.Name()
		}
	}
	return ""
}

// appLogContent devolve o conteúdo do arquivo único ("" quando ausente).
func appLogContent(t *testing.T, dir string) string {
	t.Helper()
	name := logFilesIn(dir)
	if name == "" {
		return ""
	}
	return readFile(t, filepath.Join(dir, name))
}

// Info/Warn vão para console+arquivo; o arquivo é JSON com os campos.
func TestInfoAndWarnPersistToJSONFile(t *testing.T) {
	dir := resetForTest(t)

	loggingService.Info("carregando %s com %d arquivos", "events", 7)
	loggingService.Warn("divergência em %s", "command.bin")

	content := appLogContent(t, dir)
	if content == "" {
		t.Fatal("arquivo de log geral ausente")
	}
	if !strings.Contains(content, `"level":"info"`) ||
		!strings.Contains(content, `"message":"carregando events com 7 arquivos"`) {
		t.Fatalf("info ausente no arquivo: %s", content)
	}
	if !strings.Contains(content, `"level":"warn"`) ||
		!strings.Contains(content, `"message":"divergência em command.bin"`) {
		t.Fatalf("warn ausente no arquivo: %s", content)
	}
}

// DiagWarn: console recebe a linha curta; o MESMO arquivo do log recebe o
// JSON completo com os detalhes (listas, contagens) e o campo `key`.
func TestDiagWarnWritesFullDetailsToDiagnosticsFile(t *testing.T) {
	dir := resetForTest(t)

	loggingService.DiagWarn("objects/command.bin (ffx2)",
		"original e tradução divergem — 424 de 429 rows casaram (5 rows só em mods, 0 rows só em data)",
		map[string]any{
			"kind":      "objects",
			"id":        "command.bin",
			"data_rows": 424,
			"mods_rows": 429,
			"rows_somente_mods": []map[string]any{
				{"index": 18, "name": "name", "snippet": "Berserker"},
			},
		})

	content := appLogContent(t, dir)
	if content == "" {
		t.Fatal("arquivo de log ausente")
	}
	for _, want := range []string{
		`"key":"objects/command.bin (ffx2)"`,
		`"level":"warn"`,
		`"data_rows":424`,
		`"rows_somente_mods"`,
		`"snippet":"Berserker"`,
		"original e tradução divergem",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("diagnóstico sem %q:\n%s", want, content)
		}
	}
}

// Verbose respeita VERBOSE_MODE: desligado não emite nada.
func TestVerboseRespectsEnv(t *testing.T) {
	dir := resetForTest(t)
	t.Setenv("VERBOSE_MODE", "0")

	loggingService.Verbose("não deveria emitir: %d", 42)

	if c := appLogContent(t, dir); strings.Contains(c, "não deveria emitir") {
		t.Fatalf("verbose deveria ser silenciado sem VERBOSE_MODE: %s", c)
	}

	t.Setenv("VERBOSE_MODE", "1")
	loggingService.Verbose("agora emitir: %d", 7)
	if c := appLogContent(t, dir); !strings.Contains(c, `"message":"agora emitir: 7"`) {
		t.Fatalf("verbose deveria emitir com VERBOSE_MODE=1: %q", c)
	}
}

// FromFrontend: nível validado, campos sanitizados/truncados e gravação no
// MESMO arquivo geral com source=frontend.
func TestFromFrontendValidationAndPersist(t *testing.T) {
	dir := resetForTest(t)

	if err := loggingService.FromFrontend("banana", "oi", nil); err == nil {
		t.Fatal("nível inválido deveria ser rejeitado")
	}
	if err := loggingService.FromFrontend("info", "", nil); err != nil {
		t.Fatalf("mensagem vazia deveria virar (vazio): %v", err)
	}

	if err := loggingService.FromFrontend("warn", "célula editada sem tag protegida",
		map[string]any{"arquivo": "command.bin", "chave@ruim": "valor"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	if logFilesIn(dir) == "" {
		t.Fatal("arquivo de log geral ausente")
	}
	content := appLogContent(t, dir)
	if !strings.Contains(content, `"source":"frontend"`) ||
		!strings.Contains(content, `"level":"warn"`) ||
		!strings.Contains(content, `"arquivo":"command.bin"`) {
		t.Fatalf("log do frontend ausente:\n%s", content)
	}
	// Campo com chave sanitizada gravado; chave inválida descartada.
	if strings.Contains(content, `"chave@ruim"`) {
		t.Fatalf("chave não sanitizada no arquivo:\n%s", content)
	}
}

// Um início de app = UM arquivo, com o timestamp de início no nome.
func TestSingleFilePerStart(t *testing.T) {
	dir := resetForTest(t)

	loggingService.Info("primeira mensagem")
	loggingService.DiagWarn("k/divergencia", "aviso", map[string]any{"n": 1})

	name := logFilesIn(dir)
	if name == "" {
		t.Fatal("nenhum arquivo de log gerado")
	}
	if !regexp.MustCompile(`^ffx-\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}\.log$`).MatchString(name) {
		t.Fatalf("nome fora do padrão ffx-<AAAA-MM-DD_HH-MM-SS>.log: %q", name)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("esperava 1 arquivo por início de app, achei %d: %v", len(entries), names)
	}

	// Diagnóstico FUNDIDO no mesmo arquivo: o campo `key` distingue.
	if content := appLogContent(t, dir); !strings.Contains(content, `"key":"k/divergencia"`) {
		t.Fatalf("diagnóstico fora do arquivo do app:\n%s", content)
	}
}

// Sem override de teste, `logs/` nasce AO LADO DO EXECUTÁVEL e em caminho
// absoluto — nunca relativo ao CWD (era o CWD que fazia a pasta nascer em
// pastas diferentes do repositório).
func TestLogDirAnchoredToExecutableDir(t *testing.T) {
	loggingService.ResetForTest("") // sem override → âncora no executável
	t.Cleanup(func() { loggingService.ResetForTest(tempdir.TempDir(t)) })

	dir := loggingService.LogDir()
	if dir == "" {
		t.Fatal("LogDir vazio")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("LogDir deveria ser absoluto: %q", dir)
	}
	if filepath.Base(dir) != "logs" {
		t.Fatalf("esperava a pasta 'logs': %q", dir)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if filepath.Dir(dir) != filepath.Dir(exe) {
		t.Fatalf("logs fora do diretório do executável: %q vs %q", dir, filepath.Dir(exe))
	}

	// Primeira escrita: o arquivo nasce nesse diretório (e não no CWD —
	// que em `go test` é o diretório do pacote, dentro do fonte).
	loggingService.Info("smoke: primeira escrita")
	loggingService.DiagWarn("smoke/chave", "evento", map[string]any{"n": 1})

	name := logFilesIn(dir)
	if name == "" {
		t.Fatalf("nenhum arquivo de log em %s", dir)
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("ler %s: %v", name, err)
	}
	if !strings.Contains(string(b), `"key":"smoke/chave"`) {
		t.Fatalf("diagnóstico ausente do arquivo %s:\n%s", name, b)
	}
	if _, err := os.Stat(filepath.Join(".", "logs")); err == nil {
		t.Fatal("pasta logs/ nasceu no CWD (diretório do fonte)")
	}
}

// O Init poda arquivos de execuções passadas além da retenção (30 dias);
// o arquivo corrente e os recentes sobrevivem.
func TestPruneOldLogsBeyondRetention(t *testing.T) {
	dir := resetForTest(t)

	stale := filepath.Join(dir, "ffx-2020-01-01_00-00-00.log")
	recent := filepath.Join(dir, "ffx-2020-02-02_00-00-00.log")
	for _, f := range []string{stale, recent} {
		if err := os.WriteFile(f, []byte("conteúdo"), 0o644); err != nil {
			t.Fatalf("criar %s: %v", f, err)
		}
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	fresh := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("aging stale: %v", err)
	}
	if err := os.Chtimes(recent, fresh, fresh); err != nil {
		t.Fatalf("aging recent: %v", err)
	}

	loggingService.ResetForTest(dir) // novo Init no MESMO dir → poda

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("arquivo além da retenção deveria ser podado (err=%v)", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("arquivo recente não deveria ser podado: %v", err)
	}
	if logFilesIn(dir) == "" {
		t.Fatal("arquivo da execução atual ausente após a poda")
	}
}
