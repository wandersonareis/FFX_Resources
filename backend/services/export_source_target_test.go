package services

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters"
	jsonfmt "ffxresources/backend/formatters/json"
	"ffxresources/testData/tempdir"
)

// writeEventBinary grava um binário de evento na árvore indicada
// (data/ = origem; mods/ = tradução) com os textos do idioma 'us'.
func writeEventBinary(t *testing.T, version common.GameVersion, mods bool, id string, texts []string) {
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
	root := common.GameFilesRoot
	if mods {
		root = filepath.Join(root, common.ModsFolder)
	}
	dst := filepath.Join(root,
		common.GetLocalizationRootForVersion(version, common.DefaultLocalization),
		filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("criar diretório de %s: %v", id, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("gravar evento %s: %v", id, err)
	}
}

// TestExportSourceTargetAllFormats é o contrato do export para CAT:
// `text`/`source` vêm SEMPRE de data/ e `target` de mods/ — nos quatro
// formatos e independente do toggle EnableMods (common.DisableMods).
func TestExportSourceTargetAllFormats(t *testing.T) {
	seedEventsTempRoot(t)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)
	event.ClearAllEvents()
	t.Cleanup(event.ClearAllEvents)

	const id = "zev001"
	dataTexts := []string{"Primeira linha", "Segunda linha"}
	modsTexts := []string{"Primeira traduzida", "Segunda traduzida"}
	writeEventBinary(t, common.GameVersionFFX2, false, id, dataTexts)
	writeEventBinary(t, common.GameVersionFFX2, true, id, modsTexts)

	svc := NewMetadataService(nil)
	for _, disabled := range []bool{true, false} {
		common.DisableMods = disabled
		for _, format := range []string{"json", "strings", "csv", "xlsx"} {
			paths, err := svc.ExportArtifactBatch(
				[]ExportPlanItem{{Kind: KindEvents, IDs: []string{id}}},
				common.GameVersionFFX2, []string{common.DefaultLocalization}, format)
			if err != nil {
				t.Fatalf("[%s] export: %v", format, err)
			}
			if len(paths) != 1 {
				t.Fatalf("[%s] paths=%v", format, paths)
			}
			f, ok := formatters.Get(format)
			if !ok {
				t.Fatalf("formatter %s não registrado", format)
			}
			c, err := formatters.NewReader(f, paths[0]).Read()
			if err != nil {
				t.Fatalf("[%s] reler artefato: %v", format, err)
			}
			entry, ok := c[id]
			if !ok {
				t.Fatalf("[%s] entrada %s ausente (%v)", format, id, c.SortedKeys())
			}
			if len(entry.Rows) != len(dataTexts) {
				t.Fatalf("[%s] rows=%d, queria %d", format, len(entry.Rows), len(dataTexts))
			}
			for i, row := range entry.Rows {
				if got := row.Text[common.DefaultLocalization]; got != dataTexts[i] {
					t.Errorf("[%s] disableMods=%v row %d: text=%q, queria %q (data/)",
						format, disabled, i, got, dataTexts[i])
				}
				if row.Source != dataTexts[i] {
					t.Errorf("[%s] disableMods=%v row %d: source=%q, queria %q (data/)",
						format, disabled, i, row.Source, dataTexts[i])
				}
				if row.Target != modsTexts[i] {
					t.Errorf("[%s] disableMods=%v row %d: target=%q, queria %q (mods/)",
						format, disabled, i, row.Target, modsTexts[i])
				}
			}
		}
	}
}

// TestImportAppliesTargetIntoMods fecha o ciclo do formato CAT: o import lê
// o TARGET (não o text) e grava no slot 'us' do binário de mods/.
func TestImportAppliesTargetIntoMods(t *testing.T) {
	seedEventsTempRoot(t)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)
	event.ClearAllEvents()
	t.Cleanup(event.ClearAllEvents)

	const id = "zev001"
	writeEventBinary(t, common.GameVersionFFX2, false, id, []string{"Primeira linha", "Segunda linha"})
	writeEventBinary(t, common.GameVersionFFX2, true, id, []string{"Primeira traduzida", "Segunda traduzida"})

	svc := NewMetadataService(nil)
	paths, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindEvents, IDs: []string{id}}},
		common.GameVersionFFX2, nil, "json")
	if err != nil || len(paths) != 1 {
		t.Fatalf("export: %v (%v)", err, paths)
	}

	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	c, err := jsonfmt.JSONFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	entry := c[id]
	entry.Rows[0].Target = "Nova tradução"
	c[id] = entry

	editedRaw, err := jsonfmt.JSONFormatter{}.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(tempdir.TempDir(t), "edited.json")
	if err := os.WriteFile(edited, editedRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	// A store do import é mods-first: com o toggle ligado a row 1 já está
	// exatamente como o target pede (changed só na editada).
	common.DisableMods = false

	summary, err := svc.PreviewImport(edited, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("erros/avisos: %v / %v", summary.Errors, summary.Warnings)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1 (só a row com target novo)", summary.ChangedTexts)
	}

	changed, err := svc.ImportFile(edited, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if changed != 1 {
		t.Fatalf("import changed=%d, queria 1", changed)
	}

	strs, err := event.ReadLocalizedEventStringsFrom(id, common.GameVersionFFX2, common.SourceMods)
	if err != nil {
		t.Fatalf("reler mods/: %v", err)
	}
	if got := strs[0].GetLocalizedString(common.DefaultLocalization); got != "Nova tradução" {
		t.Fatalf("mods/ row 0 = %q, queria %q", got, "Nova tradução")
	}
	if got := strs[1].GetLocalizedString(common.DefaultLocalization); got != "Segunda traduzida" {
		t.Fatalf("mods/ row 1 = %q, queria a tradução anterior intacta", got)
	}
}

// TestPrepareImportRejectsOldFormat: artefato sem source/target (formato
// antigo) não é importável — erro de arquivo mandando re-exportar.
func TestPrepareImportRejectsOldFormat(t *testing.T) {
	seedEventsTempRoot(t)
	event.ClearAllEvents()
	t.Cleanup(event.ClearAllEvents)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}

	old := dto.Collection{
		"zev001": {
			Metadata: dto.Metadata{Key: "ffx-2/event/ze/zev001.bin", ID: "zev001"},
			Rows:     []dto.TextRow{hRow(0, "", "Texto antigo")},
		},
	}
	raw, err := jsonfmt.JSONFormatter{}.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tempdir.TempDir(t), "old.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetadataService(nil).prepareImport(path, common.GameVersionFFX2); err == nil {
		t.Fatal("artefato do formato antigo deveria ser rejeitado")
	}
}
