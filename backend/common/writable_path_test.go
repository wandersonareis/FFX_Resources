package common

import (
	"ffxresources/testData/tempdir"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withGameFilesRoot(t *testing.T, root string) {
	t.Helper()
	prev := GameFilesRoot
	GameFilesRoot = root
	t.Cleanup(func() { GameFilesRoot = prev })
}

func TestCheckWritablePathBloqueiaVbf(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	for _, target := range []string{
		filepath.Join(root, "data", "FFX_Data.vbf"),
		filepath.Join(root, "mods", "algum.vbf"),
		filepath.Join(tempdir.TempDir(t), "fora.vbf"),
		"relativo.qualquer.VBF",
	} {
		err := CheckWritablePath(target)
		if !errors.Is(err, ErrVbfImmutable) {
			t.Fatalf("alvo %q deveria ser recusado como .vbf, obtido: %v", target, err)
		}
	}
}

func TestCheckWritablePathAceitaModsEBloqueiaData(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	mods := filepath.Join(root, ModsFolder, "uspc", "menu", "macrodic.dcp")
	if err := CheckWritablePath(mods); err != nil {
		t.Fatalf("mods/ deveria ser gravável: %v", err)
	}
	modsEdits := filepath.Join(root, ModsFolder, "edits", "events.json")
	if err := CheckWritablePath(modsEdits); err != nil {
		t.Fatalf("mods/edits deveria ser gravável: %v", err)
	}

	data := filepath.Join(root, "ffx_data", "gamedata", "ps3data", "lmhiku0000.bin")
	if err := CheckWritablePath(data); err == nil {
		t.Fatal("data/ não deveria ser gravável")
	}
	if err := CheckWritablePath(root); err == nil {
		t.Fatal("a própria raiz do jogo não deveria ser gravável")
	}
}

func TestCheckWritablePathForaDaArvoreDoJogo(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	if err := CheckWritablePath(filepath.Join(tempdir.TempDir(t), "copia.png")); err != nil {
		t.Fatalf("destino fora da árvore do jogo deveria ser aceito: %v", err)
	}
	// Prefixo parecido com mods mas não é: "mods_backup" dentro de data/.
	quase := filepath.Join(root, "data", "mods_backup", "x.bin")
	if err := CheckWritablePath(quase); err == nil {
		t.Fatal("data/mods_backup não é mods/: deveria ser recusado")
	}
}

func TestWriteBytesToFileNaoCriaNadaParaAlvoBloqueado(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	vbf := filepath.Join(root, "data", "novo.vbf")
	if err := WriteBytesToFile(vbf, []byte("x")); err == nil {
		t.Fatal("WriteBytesToFile deveria recusar alvo .vbf")
	}
	if _, err := os.Stat(vbf); !os.IsNotExist(err) {
		t.Fatalf("arquivo .vbf não deveria existir, stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
		t.Fatal("nem o diretório do alvo bloqueado deveria ser criado")
	}

	data := filepath.Join(root, "ffx_data", "outro.bin")
	if err := WriteBytesToFile(data, []byte("x")); err == nil {
		t.Fatal("WriteBytesToFile deveria recusar alvo fora de mods/")
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("arquivo em data/ não deveria existir, stat=%v", err)
	}

	mods := filepath.Join(root, ModsFolder, "uspc", "novo.bin")
	if err := WriteBytesToFile(mods, []byte("conteudo")); err != nil {
		t.Fatalf("mods/ deveria aceitar gravação: %v", err)
	}
	got, err := os.ReadFile(mods)
	if err != nil || string(got) != "conteudo" {
		t.Fatalf("conteúdo gravado errado: %q err=%v", got, err)
	}
}

func TestWriteBytesToFileErroDeixaClaraARegra(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	err := WriteBytesToFile(filepath.Join(root, "data", "x.bin"), []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "mods/") {
		t.Fatalf("erro deveria citar mods/: %v", err)
	}
}

// A variante SEM a trava é o caminho do acesso externo (menu/CLI avulso):
// grava dentro da árvore do jogo, em qualquer pasta.
func TestWriteBytesToFileAnywhereGravaDentroDaArvore(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	target := filepath.Join(root, "data", "new_uspc", "event", "export_x.json")
	if err := WriteBytesToFileAnywhere(target, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("Anywhere deveria gravar em data/: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("arquivo não criado: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("conteúdo errado: %q", got)
	}

	// .vbf também é livre aqui — a regra de somente-leitura é dos escritores
	// do frontend (WriteBytesToFile).
	if err := WriteBytesToFileAnywhere(filepath.Join(root, "qualquer", "x.vbf"), []byte("x")); err != nil {
		t.Fatalf("Anywhere deveria gravar .vbf: %v", err)
	}
}
