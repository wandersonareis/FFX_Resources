package services

// Testes da narração do import para a janela de logs (import_log.go):
// um binário gravado em 8 localizações vira UMA linha com o destino do 'us'
// e a contagem; falha mostra o motivo com o destino garantido.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/uilog"
	"ffxresources/testData/tempdir"
)

// modsPath monta o destino de um binário na raiz de idioma informada.
func modsPath(t *testing.T, version common.GameVersion, loc, pattern string) string {
	t.Helper()
	return filepath.Join(common.GameFilesRoot, common.ModsFolder,
		filepath.FromSlash(common.GetLocalizationRootForVersion(version, loc)),
		filepath.FromSlash(pattern))
}

// TestNarrateImportWritesUmaLinhaPorBinario: 8 gravações do MESMO binário
// (uma por localização) viram uma linha só, com o destino do 'us'.
func TestNarrateImportWritesUmaLinhaPorBinario(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	version := common.GameVersionFFX2
	const pattern = "event/obj_ps3/ev/ev001/ev001.bin"
	usDestino := modsPath(t, version, common.DefaultLocalization, pattern)

	var records []common.WriteRecord
	for _, loc := range common.SupportedLanguageCodes() {
		records = append(records, common.WriteRecord{
			Path:  modsPath(t, version, loc, pattern),
			Bytes: 1024,
		})
	}

	summary := narrateImportWrites(version, "strings", records)

	if summary.Written != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, queria 1 gravado e 0 falha", summary)
	}
	h := uilog.History()
	if len(h) != 1 {
		t.Fatalf("queria 1 linha de log, veio %d: %+v", len(h), h)
	}
	e := h[0]
	if e.Action != "Import" {
		t.Errorf("ação = %q, queria Import", e.Action)
	}
	if e.Path != usDestino {
		t.Errorf("path = %q, queria o destino do 'us' (%q)", e.Path, usDestino)
	}
	if e.Status != uilog.StatusOk {
		t.Errorf("status = %q, queria %s", e.Status, uilog.StatusOk)
	}
	// Detalhe agora é "importado de um strings" (padrão export)
	if !strings.Contains(e.Message, "importado de um strings") {
		t.Errorf("detalhe %q deveria ser 'importado de um strings'", e.Message)
	}
	if e.Level != uilog.LevelSuccess {
		t.Errorf("nível = %q, queria success (verde)", e.Level)
	}
}

// TestNarrateImportWritesVariosBinariosOrdenados: dois binários geram duas
// linhas, ordenadas pelo destino exibido (saída determinística).
func TestNarrateImportWritesVariosBinariosOrdenados(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	version := common.GameVersionFFX2
	records := []common.WriteRecord{
		{Path: modsPath(t, version, "us", "event/obj_ps3/ze/zev002/zev002.bin"), Bytes: 10},
		{Path: modsPath(t, version, "us", "event/obj_ps3/ev/ev001/ev001.bin"), Bytes: 20},
	}

	summary := narrateImportWrites(version, "strings", records)
	if summary.Written != 2 {
		t.Fatalf("summary = %+v, queria 2 gravados", summary)
	}
	h := uilog.History()
	if len(h) != 2 {
		t.Fatalf("queria 2 linhas, veio %d", len(h))
	}
	if h[0].Path > h[1].Path {
		t.Errorf("linhas fora de ordem: %q antes de %q", h[0].Path, h[1].Path)
	}
}

// TestNarrateImportWritesFalhaComDestino: falha de UMA localização marca o
// binário inteiro e a mensagem carrega o destino quando o erro não o tem
// (o usuário precisa saber ONDE tentou gravar).
func TestNarrateImportWritesFalhaComDestino(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	version := common.GameVersionFFX2
	const pattern = "menu/macrodic.dcp"
	destino := modsPath(t, version, common.DefaultLocalization, pattern)

	records := []common.WriteRecord{
		{Path: modsPath(t, version, common.DefaultLocalization, pattern), Err: errors.New("acesso negado")},
		{Path: modsPath(t, version, "jp", pattern), Bytes: 5},
	}

	summary := narrateImportWrites(version, "strings", records)
	if summary.Written != 0 || summary.Failed != 1 {
		t.Fatalf("summary = %+v, queria 0 gravado e 1 falha", summary)
	}
	h := uilog.History()
	if len(h) != 1 {
		t.Fatalf("queria 1 linha, veio %d", len(h))
	}
	e := h[0]
	if e.Status != "falha" {
		t.Errorf("status = %q, queria falha", e.Status)
	}
	if e.Level != uilog.LevelError {
		t.Errorf("nível = %q, queria error (vermelho)", e.Level)
	}
	if !strings.Contains(e.Message, "acesso negado") {
		t.Errorf("detalhe %q deveria conter o motivo", e.Message)
	}
	if !strings.Contains(e.Message, "1 de 2 localizações") {
		t.Errorf("detalhe %q deveria dizer quantas localizações falharam", e.Message)
	}
	// O destino que falhou é o do 'us' — o mesmo que a linha já mostra, então
	// ele NÃO precisa se repetir no detalhe.
	if strings.Count(e.Message, destino) > 0 && e.Path == destino && strings.Contains(e.Message, "destino "+destino) {
		t.Errorf("detalhe %q repete o destino já exibido na linha", e.Message)
	}
}

// TestNarrateImportWritesFalhaTotalDeTodasAsLocalizacoes: todas as
// localizações falharam — a linha é a falha do binário (motivo + destino).
func TestNarrateImportWritesFalhaTotalDeTodasAsLocalizacoes(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	version := common.GameVersionFFX2
	const pattern = "battle/btl/btl001/btl001.bin"
	records := []common.WriteRecord{
		{Path: modsPath(t, version, common.DefaultLocalization, pattern), Err: errors.New("disco cheio")},
		{Path: modsPath(t, version, "de", pattern), Err: errors.New("disco cheio")},
	}

	summary := narrateImportWrites(version, "strings", records)
	if summary.Failed != 1 || summary.Written != 0 {
		t.Fatalf("summary = %+v, queria 1 falha", summary)
	}
	h := uilog.History()
	if len(h) != 1 || !strings.Contains(h[0].Message, "disco cheio") {
		t.Fatalf("linha de falha total = %+v", h)
	}
	// Falha da própria localização exibida: o destino não precisa se repetir.
	if strings.Contains(h[0].Message, "destino ") {
		t.Errorf("detalhe %q não deveria repetir o destino da linha", h[0].Message)
	}
}

// TestGroupWritesByBinaryForaDasRaizesDeIdioma: destino fora das raízes de
// localização (caminho avulso) não é agrupado com nada — cada um na sua.
func TestGroupWritesByBinaryForaDasRaizesDeIdioma(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRootTest(t, root)

	records := []common.WriteRecord{
		{Path: filepath.Join(root, "avulso", "a.bin"), Bytes: 1},
		{Path: filepath.Join(root, "avulso", "b.bin"), Bytes: 2},
	}
	groups := groupWritesByBinary(common.GameVersionFFX, records)
	if len(groups) != 2 {
		t.Fatalf("grupos = %d, queria 2 (sem agrupar caminho avulso)", len(groups))
	}
	for _, g := range groups {
		if len(g.entries) != 1 {
			t.Errorf("grupo %q com %d entradas, queria 1", g.path, len(g.entries))
		}
	}
}

// TestApplyRecordedDevolveErroDoApply: o erro do apply volta intacto (o
// fluxo de import decide o que fazer com ele) e a narração acontece.
func TestApplyRecordedDevolveErroDoApply(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	root := tempdir.TempDir(t)
	withGameFilesRootTest(t, root)

	boom := errors.New("apply quebrou")
	summary, err := applyRecorded(common.GameVersionFFX, "Import", "importado de um json", func() error {
		if werr := common.WriteBytesToFile(
			filepath.Join(root, common.ModsFolder, "menu", "macrodic.dcp"), []byte("x"),
		); werr != nil {
			t.Fatalf("gravação em mods/: %v", werr)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("erro devolvido = %v, queria o erro do apply", err)
	}
	if summary.Written != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, queria 1 gravado", summary)
	}
	h := uilog.History()
	if len(h) != 1 || h[0].Status != uilog.StatusOk {
		t.Fatalf("histórico = %+v, queria 1 linha com status %s", h, uilog.StatusOk)
	}
	// Detalhe deve ser "importado de um json"
	if !strings.Contains(h[0].Message, "importado de um json") {
		t.Errorf("detalhe %q deveria conter 'importado de um json'", h[0].Message)
	}
	if _, serr := os.Stat(filepath.Join(root, common.ModsFolder, "menu", "macrodic.dcp")); serr != nil {
		t.Fatalf("arquivo deveria existir: %v", serr)
	}
}

// withGameFilesRootTest aponta o GameFilesRoot para um diretório temporário
// (os gravadores narram destinos dentro dele).
func withGameFilesRootTest(t *testing.T, root string) {
	t.Helper()
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() { common.GameFilesRoot = prev })
}
