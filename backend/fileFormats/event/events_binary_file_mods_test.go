package event_test

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/fileFormats/event"
)

// TestLoadFromBinaryModsPerFile valida o fluxo de carga com mods:
//   - a descoberta de eventos acontece na árvore ORIGINAL (gamefiles) — um
//     diretório parcial em mods não pode esconder os demais eventos;
//   - o conteúdo de cada arquivo prefere o binário de mods quando existe.
func TestLoadFromBinaryModsPerFile(t *testing.T) {
	prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
	common.SetModsEnabled(true)
	defer func() {
		common.GameFilesRoot = prevRoot
		common.SetModsEnabled(prevMods)
	}()

	// Copia a árvore de eventos do FFX-2 para um temp.
	root := `F:\ffxWails\FFX_Resources\testData`
	game := tempdir.TempDir(t)
	src := filepath.Join(root, "FFX-2", "binary", "ffx_ps2", "ffx2", "master", "new_uspc")
	dst := filepath.Join(game, "ffx_ps2", "ffx2", "master", "new_uspc")
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	common.SetGameFilesRoot(game)

	version := common.GameVersionFFX2
	for _, cs := range common.Charsets {
		if err := ffxencoding.PrepareCharset(version, cs); err != nil {
			t.Fatal(err)
		}
	}

	// Carga baseline (sem mods): todos os eventos + contagem original do alvo.
	loadAll := func() []string {
		ev := event.NewEventsBinaryFile(version)
		if err := ev.LoadFromBinary(); err != nil {
			t.Fatalf("LoadFromBinary: %v", err)
		}
		return event.GetAllEventIDs(version)
	}
	ids := loadAll()
	if len(ids) == 0 {
		t.Fatal("eventos deveriam ter sido carregados da árvore original")
	}
	baseline := len(ids)

	targetID := "hiku2800"
	targetRows := len(event.GetEvent(version, targetID).Strings)

	// Donor: evento com contagem de rows diferente do alvo (e arquivo no disco).
	donorID, donorRows := donorFor(t, version, ids, targetID, targetRows)

	// Semeia mods com UM evento (alvo) cujo conteúdo é o do donor. Com a
	// descoberta por mods (bug), carregaria apenas 1 evento.
	modFile := filepath.Join(game, "mods", "ffx_ps2", "ffx2", "master", "new_uspc",
		"event", "obj_ps3", targetID[:2], targetID, targetID+".bin")
	if err := os.MkdirAll(filepath.Dir(modFile), 0755); err != nil {
		t.Fatal(err)
	}
	donorData, err := os.ReadFile(filepath.Join(game, "ffx_ps2", "ffx2", "master", "new_uspc",
		"event", "obj_ps3", donorID[:2], donorID, donorID+".bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modFile, donorData, 0644); err != nil {
		t.Fatal(err)
	}

	ids2 := loadAll()
	if len(ids2) != baseline {
		t.Fatalf("a descoberta deve enumerar a árvore original (mods parcial não esconde eventos): got %d, want %d",
			len(ids2), baseline)
	}

	// E o conteúdo do alvo deve vir do binário de mods (por arquivo).
	got := len(event.GetEvent(version, targetID).Strings)
	if got != donorRows {
		t.Fatalf("mods-first por arquivo: %s com %d rows, want %d (do donor %s)",
			targetID, got, donorRows, donorID)
	}
}

// donorFor escolhe um evento cujo binário tenha contagem de rows diferente do
// alvo (prova inequívoca de que o conteúdo lido veio do donor).
func donorFor(t *testing.T, version common.GameVersion, ids []string, targetID string, targetRows int) (string, int) {
	t.Helper()
	for _, id := range ids {
		if id == targetID {
			continue
		}
		rows := len(event.GetEvent(version, id).Strings)
		if rows != targetRows {
			return id, rows
		}
	}
	t.Fatal("nenhum evento com contagem de rows diferente do alvo")
	return "", 0
}
