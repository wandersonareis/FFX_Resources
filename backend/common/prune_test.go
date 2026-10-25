package common

import (
	"ffxresources/testData/tempdir"
	"os"
	"path/filepath"
	"testing"
)

// O ramo vazio some inteiro e o piso fica de pé.
func TestPruneEmptyDirsPodaRamoVazioAteOPiso(t *testing.T) {
	floor := tempdir.TempDir(t)
	deep := filepath.Join(floor, "ffx_data", "gamedata", "ps3data", "dup")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("criando ramo: %v", err)
	}

	PruneEmptyDirs(deep, floor)

	if _, err := os.Stat(filepath.Join(floor, "ffx_data")); !os.IsNotExist(err) {
		t.Errorf("ramo esvazio sobreviveu até o piso (err=%v)", err)
	}
	if _, err := os.Stat(floor); err != nil {
		t.Errorf("piso foi apagado: %v", err)
	}
}

// Diretório com conteúdo não é podado — nem quando o invasor é invisível
// para o usuário (arquivo oculto), e o pai dele também fica.
func TestPruneEmptyDirsPreservaNaoVazio(t *testing.T) {
	floor := tempdir.TempDir(t)
	cheio := filepath.Join(floor, "mods", "ffx_data")
	oculto := filepath.Join(floor, "mods", "imagens")
	for dir, name := range map[string]string{cheio: "irmao.txt", oculto: ".DS_Store"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("criando %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("gravando %s: %v", name, err)
		}
	}

	PruneEmptyDirs(cheio, floor)
	PruneEmptyDirs(oculto, floor)

	for _, p := range []string{cheio, oculto, filepath.Join(floor, "mods")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("diretório com conteúdo foi apagado: %s (%v)", p, err)
		}
	}
}

// Diretório igual ao piso, inexistente ou fora dele não é tocado.
func TestPruneEmptyDirsRespeitaPisoEEstrangeiros(t *testing.T) {
	floor := tempdir.TempDir(t)
	fora := tempdir.TempDir(t)

	PruneEmptyDirs(floor, floor) // igual ao piso: no-op
	if _, err := os.Stat(floor); err != nil {
		t.Errorf("piso apagado ao podar a si mesmo: %v", err)
	}

	estranho := filepath.Join(fora, "vazio")
	if err := os.MkdirAll(estranho, 0o755); err != nil {
		t.Fatalf("criando %s: %v", estranho, err)
	}
	PruneEmptyDirs(estranho, floor) // outra árvore: fora do piso
	if _, err := os.Stat(estranho); err != nil {
		t.Errorf("diretório fora do piso foi apagado: %v", err)
	}

	PruneEmptyDirs(filepath.Join(floor, "nao_existe"), floor) // ilegível: silêncio
}
