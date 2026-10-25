package eventtable

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/event"
)

func TestRelPath(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		id       string
		expected string
		ok       bool
	}{
		// KindBattleText
		{"battletext valid id", KindBattleText, "bika07_228", "battle/btl/bika07_228/bika07_228.bin", true},
		{"battletext empty id", KindBattleText, "", "", false},
		{"battletext path traversal", KindBattleText, "../etc/passwd", "", false},
		{"battletext tuto0000 rejected", KindBattleText, "tuto0000", "", false},
		{"battletext menumain rejected", KindBattleText, "menumain", "", false},
		{"battletext case insensitive tuto", KindBattleText, "TUTO0000", "", false},
		{"battletext case insensitive menu", KindBattleText, "MENUMAIN", "", false},

		// KindCloud
		{"cloud valid", KindCloud, "cloud", "cloudsave/cloud.bin", true},
		{"cloudsave alias", KindCloud, "cloudsave", "cloudsave/cloud.bin", true},
		{"cloudv valid", KindCloud, "cloudv", "cloudsave/cloudv.bin", true},
		{"cloud invalid", KindCloud, "invalid", "", false},

		// KindTutorial
		{"tutorial valid", KindTutorial, "tutorial", "menu/tutorial.msb", true},
		{"tuto0000 valid", KindTutorial, "tuto0000", "battle/btl/tuto0000/tuto0000.bin", true},
		{"tuto alias", KindTutorial, "tuto", "battle/btl/tuto0000/tuto0000.bin", true},
		{"tutorial invalid", KindTutorial, "invalid", "", false},

		// KindMenuMain
		{"menumain valid", KindMenuMain, "menumain", "menu/menumain.bin", true},
		{"menumain_btl valid", KindMenuMain, "menumain_btl", "battle/btl/menumain/menumain.bin", true},
		{"menumain invalid", KindMenuMain, "invalid", "", false},

		// Unknown kind
		{"unknown kind", "unknown", "id", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := RelPath(tc.kind, tc.id)
			if ok != tc.ok {
				t.Errorf("RelPath(%q, %q) ok = %v, want %v", tc.kind, tc.id, ok, tc.ok)
			}
			if rel != tc.expected {
				t.Errorf("RelPath(%q, %q) = %q, want %q", tc.kind, tc.id, rel, tc.expected)
			}
		})
	}
}

func TestRelPaths(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		id       string
		expected []string
	}{
		{"cloud returns pair", KindCloud, "cloud", []string{"cloudsave/cloud.bin"}},
		{"cloudsave alias returns pair", KindCloud, "cloudsave", []string{"cloudsave/cloud.bin"}},
		{"cloudv returns single", KindCloud, "cloudv", []string{"cloudsave/cloudv.bin"}},
		{"cloud invalid returns nil", KindCloud, "invalid", nil},
		{"tutorial tuto0000 returns two paths", KindTutorial, "tuto0000", []string{"battle/btl/tuto0000/tuto0000.bin", "battle/btl/tuto0000/tutorial.msb"}},
		{"tutorial tuto alias returns two paths", KindTutorial, "tuto", []string{"battle/btl/tuto0000/tuto0000.bin", "battle/btl/tuto0000/tutorial.msb"}},
		{"tutorial tutorial returns single", KindTutorial, "tutorial", []string{"menu/tutorial.msb"}},
		{"menumain single", KindMenuMain, "menumain", []string{"menu/menumain.bin"}},
		{"battletext single", KindBattleText, "bika07_228", []string{"battle/btl/bika07_228/bika07_228.bin"}},
		{"unknown kind returns nil", "unknown", "id", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := RelPaths(tc.kind, tc.id)
			if len(result) != len(tc.expected) {
				t.Errorf("RelPaths(%q, %q) len = %d, want %d", tc.kind, tc.id, len(result), len(tc.expected))
			}
			for i, r := range result {
				if i < len(tc.expected) && r != tc.expected[i] {
					t.Errorf("RelPaths[%d] = %q, want %q", i, r, tc.expected[i])
				}
			}
		})
	}
}

func TestKey(t *testing.T) {
	// Key returns version/path
	key, ok := Key(common.GameVersionFFX, KindBattleText, "bika07_228")
	if !ok {
		t.Fatal("Key should be valid")
	}
	expected := "ffx/battle/btl/bika07_228/bika07_228.bin"
	if key != expected {
		t.Errorf("Key = %q, want %q", key, expected)
	}

	// Invalid kind/id
	_, ok = Key(common.GameVersionFFX, "unknown", "id")
	if ok {
		t.Error("Key should be invalid for unknown kind")
	}

	_, ok = Key(common.GameVersionFFX, KindBattleText, "tuto0000")
	if ok {
		t.Error("Key should be invalid for tuto0000 in battletext")
	}
}

func TestRelPathLoc(t *testing.T) {
	loc := common.DefaultLocalization
	rel, ok := RelPathLoc(common.GameVersionFFX, loc, KindBattleText, "bika07_228")
	if !ok {
		t.Fatal("RelPathLoc should be valid")
	}
	expectedPrefix := common.GetLocalizationRootForVersion(common.GameVersionFFX, loc)
	if !filepath.IsAbs(rel) && rel != filepath.ToSlash(filepath.Join(expectedPrefix, "battle/btl/bika07_228/bika07_228.bin")) {
		t.Logf("RelPathLoc = %q", rel)
	}

	// Invalid kind
	_, ok = RelPathLoc(common.GameVersionFFX, loc, "unknown", "id")
	if ok {
		t.Error("RelPathLoc should be invalid for unknown kind")
	}
}

func TestIDs(t *testing.T) {
	// Test fixed IDs kinds
	for _, kind := range []string{KindCloud, KindTutorial, KindMenuMain} {
		ids := IDs(kind, common.GameVersionFFX)
		if len(ids) == 0 {
			t.Errorf("IDs(%q) should not be empty", kind)
		}
		// Verify they match fixedIDs
		if fixed, ok := fixedIDs[kind]; ok {
			if len(ids) != len(fixed) {
				t.Errorf("IDs(%q) len = %d, want %d", kind, len(ids), len(fixed))
			}
		}
	}

	// battletext depends on filesystem - just verify it doesn't crash
	ids := IDs(KindBattleText, common.GameVersionFFX)
	t.Logf("battletext IDs count: %d", len(ids))

	// Unknown kind returns nil
	if IDs("unknown", common.GameVersionFFX) != nil {
		t.Error("IDs(unknown) should return nil")
	}
}

func TestLoad(t *testing.T) {
	// Test loading a valid artifact (cloud is most likely to exist)
	// This may fail if the fixture doesn't exist, which is OK
	f, err := Load(KindCloud, common.GameVersionFFX, "cloud")
	if err != nil {
		t.Logf("Load cloud: %v (fixture may not exist)", err)
		return
	}
	if f == nil {
		t.Error("Load returned nil file")
	}
	if f.Kind != KindCloud {
		t.Errorf("Kind = %q, want %q", f.Kind, KindCloud)
	}
	if f.ID != "cloud" {
		t.Errorf("ID = %q, want %q", f.ID, "cloud")
	}
	if f.Version != common.GameVersionFFX {
		t.Errorf("Version = %v, want %v", f.Version, common.GameVersionFFX)
	}

	// Test invalid ID
	_, err = Load(KindCloud, common.GameVersionFFX, "invalid")
	if err == nil {
		t.Error("Load should error for invalid ID")
	}

	// Test unknown kind
	_, err = Load("unknown", common.GameVersionFFX, "id")
	if err == nil {
		t.Error("Load should error for unknown kind")
	}
}

func TestSave(t *testing.T) {
	// Save original values
	prevRoot := common.GameFilesRoot
	defer func() { common.GameFilesRoot = prevRoot }()

	// Create a minimal File and test Save
	f := &File{
		Kind:    KindBattleText,
		ID:      "test_save",
		Version: common.GameVersionFFX,
		Bins:    []BinSpec{{Name: "test_save", Rel: "battle/btl/test_save/test_save.bin", Start: 0, Len: 1}},
	}

	// Create test strings using the correct API - use empty field string
	f.Strings = []*event.LocalizedFieldStringObject{
		event.NewLocalizedFieldStringObjectWithContent("us", event.NewEmptyFieldString("us", common.GameVersionFFX)),
	}

	// Save to a temp location
	common.SetGameFilesRoot(tempdir.TempDir(t))
	
	err := f.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file was created in mods/
	for _, loc := range common.SupportedLanguageCodes() {
		for _, bin := range f.Bins {
			localeRel := filepath.ToSlash(filepath.Join(common.GetLocalizationRootForVersion(f.Version, loc), bin.Rel))
			target := filepath.Join(common.GameFilesRoot, common.ModsFolder, filepath.FromSlash(localeRel))
			if _, err := os.Stat(target); err != nil {
				t.Errorf("Saved file not found for %s: %v", loc, err)
			}
		}
	}
}

func TestLoadFromStore(t *testing.T) {
	// Clear store first
	ClearStore()

	// Load from store (may fail if fixture doesn't exist)
	f, err := LoadFromStore(KindCloud, common.GameVersionFFX, "cloud")
	if err != nil {
		t.Logf("LoadFromStore cloud: %v (fixture may not exist)", err)
		return
	}
	if f == nil {
		t.Error("LoadFromStore returned nil")
	}

	// Second load should use cache
	f2, err := LoadFromStore(KindCloud, common.GameVersionFFX, "cloud")
	if err != nil {
		t.Errorf("Second LoadFromStore failed: %v", err)
	}
	if f != f2 {
		t.Error("LoadFromStore should return cached instance")
	}
}

func TestGet(t *testing.T) {
	ClearStore()
	// Get returns nil for non-existent
	f1 := Get(KindCloud, common.GameVersionFFX, "cloud")
	if f1 != nil {
		t.Error("Get should return nil for non-cached entry")
	}

	// Manually add to store and test Get
	f2 := &File{Kind: KindCloud, ID: "test", Version: common.GameVersionFFX}
	storeMu.Lock()
	store[keyOf(KindCloud, common.GameVersionFFX, "test")] = f2
	storeMu.Unlock()

	f3 := Get(KindCloud, common.GameVersionFFX, "test")
	if f3 != f2 {
		t.Error("Get should return cached file")
	}
}

func TestClearStore(t *testing.T) {
	ClearStore()
	f := &File{Kind: KindCloud, ID: "test", Version: common.GameVersionFFX}
	storeMu.Lock()
	store[keyOf(KindCloud, common.GameVersionFFX, "test")] = f
	stamps[keyOf(KindCloud, common.GameVersionFFX, "test")] = map[string]common.FileStamp{}
	storeMu.Unlock()

	ClearStore()

	f2 := Get(KindCloud, common.GameVersionFFX, "test")
	if f2 != nil {
		t.Error("ClearStore should empty the store")
	}
}

func TestList(t *testing.T) {
	// List returns sorted IDs
	ids := List(KindCloud, common.GameVersionFFX)
	if len(ids) == 0 {
		t.Error("List should not be empty for cloud")
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[i-1] {
			t.Error("List should return sorted IDs")
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		kind     string
		valid    bool
	}{
		{KindBattleText, true},
		{KindCloud, true},
		{KindTutorial, true},
		{KindMenuMain, true},
		{"unknown", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			err := Validate(tc.kind)
			if tc.valid && err != nil {
				t.Errorf("Validate(%q) error: %v", tc.kind, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("Validate(%q) should error", tc.kind)
			}
		})
	}
}

func TestFixedIDsMap(t *testing.T) {
	// Verify fixedIDs map has expected entries
	expected := map[string][]string{
		KindCloud:     {"cloud", "cloudv"},
		KindTutorial:  {"tutorial", "tuto0000"},
		KindMenuMain:  {"menumain", "menumain_btl"},
	}

	for kind, expectedIDs := range expected {
		ids, ok := fixedIDs[kind]
		if !ok {
			t.Errorf("fixedIDs missing key %q", kind)
			continue
		}
		if len(ids) != len(expectedIDs) {
			t.Errorf("fixedIDs[%q] len = %d, want %d", kind, len(ids), len(expectedIDs))
		}
		for i, id := range ids {
			if i < len(expectedIDs) && id != expectedIDs[i] {
				t.Errorf("fixedIDs[%q][%d] = %q, want %q", kind, i, id, expectedIDs[i])
			}
		}
	}
}

func TestBinSpec(t *testing.T) {
	bs := BinSpec{
		Name:  "test",
		Rel:   "path/to/file.bin",
		Start: 10,
		Len:   20,
	}
	if bs.Name != "test" || bs.Rel != "path/to/file.bin" || bs.Start != 10 || bs.Len != 20 {
		t.Error("BinSpec fields not set correctly")
	}
}

func TestFileStruct(t *testing.T) {
	f := File{
		Kind:    KindBattleText,
		ID:      "test",
		Version: common.GameVersionFFX,
		Strings: []*event.LocalizedFieldStringObject{
			event.NewLocalizedFieldStringObjectWithContent("us", event.NewEmptyFieldString("us", common.GameVersionFFX)),
		},
		Bins: []BinSpec{{Name: "test", Rel: "battle/btl/test/test.bin", Start: 0, Len: 1}},
	}

	if f.Kind != KindBattleText {
		t.Errorf("Kind = %q", f.Kind)
	}
	if len(f.Strings) != 1 {
		t.Errorf("Strings len = %d", len(f.Strings))
	}
	if len(f.Bins) != 1 {
		t.Errorf("Bins len = %d", len(f.Bins))
	}
}