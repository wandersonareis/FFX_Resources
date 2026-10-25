package common

import (
	"ffxresources/testData/tempdir"
	"os"
	"path/filepath"
	"testing"
)

// TestResolvePathModsFirst valida a preferência mods-first da resolução de
// caminhos: com mods habilitado, um arquivo presente em mods/<rel> vence o
// de <gamefiles>/<rel>; sem mods (ou arquivo ausente em mods), cai para o
// original de data.
func TestResolvePathModsFirst(t *testing.T) {
	gameFiles := tempdir.TempDir(t)
	prevRoot, prevMods := GameFilesRoot, DisableMods
	GameFilesRoot = gameFiles
	defer func() {
		GameFilesRoot = prevRoot
		DisableMods = prevMods
	}()

	rel := filepath.Join("ffx_ps2", "ffx", "master", "new_uspc", "menu", "macrodic.dcp")
	modPath := filepath.Join(gameFiles, "mods", rel)
	dataPath := filepath.Join(gameFiles, rel)
	if err := os.MkdirAll(filepath.Dir(modPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath, []byte("MODS"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("DATA"), 0644); err != nil {
		t.Fatal(err)
	}

	SetModsEnabled(true)
	acc, err := NewFileAccessor(rel)
	if err != nil {
		t.Fatalf("NewFileAccessor: %v", err)
	}
	if got, err := os.ReadFile(acc.ResolvedPath); err != nil || string(got) != "MODS" {
		t.Fatalf("com mods habilitado, deve resolver para mods/: got %q (err %v)", got, err)
	}

	// Ausente em mods → cai para data.
	if err := os.Remove(modPath); err != nil {
		t.Fatal(err)
	}
	acc2, err := NewFileAccessor(rel)
	if err != nil {
		t.Fatalf("NewFileAccessor fallback: %v", err)
	}
	if got, err := os.ReadFile(acc2.ResolvedPath); err != nil || string(got) != "DATA" {
		t.Fatalf("fallback para data: got %q (err %v)", got, err)
	}

	// Mods desabilitado → sempre data, mesmo com o arquivo presente em mods.
	if err := os.WriteFile(modPath, []byte("MODS"), 0644); err != nil {
		t.Fatal(err)
	}
	SetModsEnabled(false)
	acc3, err := NewFileAccessor(rel)
	if err != nil {
		t.Fatalf("NewFileAccessor com mods off: %v", err)
	}
	if got, err := os.ReadFile(acc3.ResolvedPath); err != nil || string(got) != "DATA" {
		t.Fatalf("com mods desabilitado, deve ler data/: got %q (err %v)", got, err)
	}
}

func TestSetModsEnabled(t *testing.T) {
	prev := DisableMods
	defer func() { DisableMods = prev }()

	SetModsEnabled(true)
	if !AreModsEnabled() {
		t.Fatal("SetModsEnabled(true) deve habilitar AreModsEnabled")
	}
	SetModsEnabled(false)
	if AreModsEnabled() {
		t.Fatalf("SetModsEnabled(false) deve desabilitar AreModsEnabled")
	}
}
