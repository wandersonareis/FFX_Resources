package services

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters"
	jsonfmt "ffxresources/backend/formatters/json"
)

// seedEventFileOnDisk grava o binário do evento em data/: o export lê a
// origem SEMPRE da árvore original (nunca da store mods-first).
func seedEventFileOnDisk(t *testing.T, version common.GameVersion, id string, texts []string) {
	t.Helper()
	charset := ffxencoding.GetCharsetForLanguage(common.DefaultLocalization)
	objs := make([]*event.LocalizedFieldStringObject, 0, len(texts))
	for _, text := range texts {
		fs := event.NewEmptyFieldString(charset, version)
		fs.SetRegularString(text)
		objs = append(objs, event.NewLocalizedFieldStringObjectWithContent(common.DefaultLocalization, fs))
	}
	data, err := event.EncodeLocalizedStrings(objs, common.DefaultLocalization, version)
	if err != nil {
		t.Fatalf("codificar evento %s: %v", id, err)
	}
	rel, err := event.EventRelPath(id)
	if err != nil {
		t.Fatalf("caminho do evento %s: %v", id, err)
	}
	dst := filepath.Join(common.GameFilesRoot,
		common.GetLocalizationRootForVersion(version, common.DefaultLocalization),
		filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("criar diretório de %s: %v", id, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("gravar evento %s: %v", id, err)
	}
}

// TestExportEntryFiltersBlankRows: o export de ENTRADA ÚNICA (menu de
// contexto → ExportEntry) obedece ao mesmo contrato do export em lote —
// rows sem 'us' utilizável (vazio, só espaço ou "-") não chegam ao
// artefato, em JSON nem em .strings.
func TestExportEntryFiltersBlankRows(t *testing.T) {
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	defer event.ClearEvents(common.GameVersionFFX2)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira linha", // única traduzível
		"-",              // texto próprio do jogo: sai
		"  ",             // só espaço: sai
		"",               // vazio: sai
	})
	seedEventFileOnDisk(t, common.GameVersionFFX2, "zev001", []string{
		"Primeira linha",
		"-",
		"  ",
		"",
	})

	paths, err := NewMetadataService(nil).ExportEntry(KindEvents, common.GameVersionFFX2, "zev001", nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths=%v, queria JSON + .strings", paths)
	}
	for _, p := range paths {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Fatalf("artefato ausente: %s (%v)", p, statErr)
		}
	}

	// O JSON volta SEM a row em branco e com a contagem recalculada.
	out, err := formatters.NewReader(jsonfmt.JSONFormatter{}, paths[0]).Read()
	if err != nil {
		t.Fatalf("releitura: %v", err)
	}
	entry, ok := out["zev001"]
	if !ok {
		t.Fatal("entrada zev001 ausente no artefato")
	}
	if n := len(entry.Rows); n != 1 {
		for _, r := range entry.Rows {
			t.Logf("row %+v", r)
		}
		t.Fatalf("rows=%d, queria só a traduzível", n)
	}
	if got := entry.Rows[0].Text[common.DefaultLocalization]; got != "Primeira linha" {
		t.Fatalf("texto %q, queria %q", got, "Primeira linha")
	}
	if entry.Metadata.RowCount != 1 {
		t.Fatalf("row_count=%d, queria 1", entry.Metadata.RowCount)
	}
}

// TestExportEntryDropsFullyBlankEntry: entrada só com rows em branco não
// gera artefato — é omitida do lote inteiro (não escreve arquivo vazio).
func TestExportEntryDropsFullyBlankEntry(t *testing.T) {
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	defer event.ClearEvents(common.GameVersionFFX2)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"-", "  "})
	seedEventFileOnDisk(t, common.GameVersionFFX2, "zev001", []string{"-", "  "})

	_, err := NewMetadataService(nil).ExportEntry(KindEvents, common.GameVersionFFX2, "zev001", nil)
	if err == nil {
		t.Fatal("entrada só com rows em branco deveria falhar (nada a exportar)")
	}
}

// Guarda de contrato: isBlankText é o mesmo predicado do casamento de
// import — vazio, só espaço (e tab) ou o hífen do jogo.
func TestIsBlankTextContract(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"\t", true},
		{"-", true},
		{" - ", true},
	} {
		if got := isBlankText(c.in); got != c.want {
			t.Fatalf("isBlankText(%q)=%v, queria %v", c.in, got, c.want)
		}
	}
	if isBlankText("- ou não") {
		t.Fatal("texto com hífen e conteúdo não é blank")
	}
}
