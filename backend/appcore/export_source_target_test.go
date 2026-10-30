package appcore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	jsonfmt "ffxresources/backend/formatters/json"
	"ffxresources/backend/services"
	"ffxresources/testData/tempdir"
)

// TestExportTextSourceTargetFromCounterparts: o export do menu projeta
// `text`/`source` do binário de data/ e `target` do par em mods/ — o mesmo
// artefato, apontando o binário original ou o traduzido.
func TestExportTextSourceTargetFromCounterparts(t *testing.T) {
	fixtureRoot := abs(t, filepath.Join("..", "..", "testData", "FFX", "binary"))
	rel := filepath.FromSlash("ffx_ps2/ffx/master/new_uspc/event/obj_ps3/az/azit0000/azit0000.bin")
	macrodic := filepath.Join(fixtureRoot,
		filepath.FromSlash("ffx_ps2/ffx/master/new_uspc/menu/macrodic.dcp"))
	orig, err := os.ReadFile(filepath.Join(fixtureRoot, rel))
	if err != nil {
		t.Skipf("fixture de evento ausente: %v", err)
	}
	if _, err := os.Stat(macrodic); err != nil {
		t.Skipf("macrodic de fixture ausente: %v", err)
	}

	root := tempdir.TempDir(t)
	prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = root
	common.DisableMods = true
	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		common.DisableMods = prevMods
	})

	version := common.GameVersionFFX
	if _, err := prepareTextRuntime(version, services.KindEvents, macrodic); err != nil {
		t.Fatalf("runtime: %v", err)
	}
	dataPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, orig, 0o644); err != nil {
		t.Fatal(err)
	}

	objs, err := parseFieldStringObjects(orig, version, common.DefaultLocalization)
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	origTexts := make([]string, len(objs))
	for i, obj := range objs {
		origTexts[i] = obj.GetLocalizedString(common.DefaultLocalization)
		obj.GetLocalizedContent(common.DefaultLocalization).SetRegularString("PT:" + origTexts[i])
	}
	modsBytes, err := event.EncodeLocalizedStrings(objs, common.DefaultLocalization, version)
	if err != nil {
		t.Fatalf("reconstruir mods/: %v", err)
	}
	modsPath := filepath.Join(root, common.ModsFolder, rel)
	if err := os.MkdirAll(filepath.Dir(modsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modsPath, modsBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	exportFrom := func(input, outDir string) dto.Collection {
		t.Helper()
		res, err := ExportText(context.Background(), ExportRequest{
			Inputs:       []string{input},
			Kind:         services.KindEvents,
			Format:       FormatJSON,
			Version:      VersionRequest{Known: true, V: version},
			OutDir:       outDir,
			Collision:    CollisionFail,
			MacrodicPath: macrodic,
		})
		if err != nil || len(res.Outcomes) != 1 {
			t.Fatalf("export %s: %v (%v)", filepath.Base(input), err, res.Outcomes)
		}
		o := res.Outcomes[0]
		if o.Status != StatusWritten {
			t.Fatalf("export %s: status=%v (%s)", filepath.Base(input), o.Status, o.Reason)
		}
		raw, err := os.ReadFile(o.Output)
		if err != nil {
			t.Fatal(err)
		}
		c, err := jsonfmt.JSONFormatter{}.Unmarshal(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(o.Output), err)
		}
		return c
	}

	fromData := exportFrom(dataPath, tempdir.TempDir(t))
	// O mesmo export apontando o binário de mods/ tem de produzir o MESMO
	// artefato (text/source de data/, target de mods/).
	fromMods := exportFrom(modsPath, tempdir.TempDir(t))

	id := fromData.SortedKeys()[0]
	if len(fromData[id].Rows) != len(fromMods[id].Rows) {
		t.Fatalf("row count divergiu entre data/ e mods/: %d vs %d",
			len(fromData[id].Rows), len(fromMods[id].Rows))
	}
	for i := range fromData[id].Rows {
		a, b := fromData[id].Rows[i], fromMods[id].Rows[i]
		if a.Text[common.DefaultLocalization] != b.Text[common.DefaultLocalization] ||
			a.Source != b.Source || a.Target != b.Target {
			t.Fatalf("row %d divergiu entre data/ e mods/: %+v vs %+v", i, a, b)
		}
	}

	for _, row := range fromData[id].Rows {
		if row.Index < 0 || row.Index >= len(origTexts) {
			continue
		}
		want := origTexts[row.Index]
		if got := row.Text[common.DefaultLocalization]; got != want {
			t.Errorf("row %d: text=%q, queria %q (data/)", row.Index, got, want)
		}
		if row.Source != want {
			t.Errorf("row %d: source=%q, queria %q (data/)", row.Index, row.Source, want)
		}
		if row.Target != "PT:"+want {
			t.Errorf("row %d: target=%q, queria %q (mods/)", row.Index, row.Target, "PT:"+want)
		}
	}
}
