package ddsphyre

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
)

// withToggleTree monta a árvore do toggle num root temporário: pristine em
// data/ e a "tradução" em mods/ (mesma imagem com um byte de payload virado
// — container que decoda igual, pixels diferentes). Sem artefatos em
// mods/images: quem os cria é o próprio teste.
func withToggleTree(t *testing.T) (id string, pristine, modded []byte) {
	t.Helper()
	_, pristine = samplePhyre(t)

	root := tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() { common.GameFilesRoot = prev })

	id = "gamedata/ps3data/toggle/a"
	modded = append([]byte(nil), pristine...)
	off, err := dataOffsetOf(modded)
	if err != nil {
		t.Fatalf("dataOffsetOf: %v", err)
	}
	modded[int(off)] ^= 0xFF

	writePhyre(t, root, id, pristine)
	writePhyre(t, filepath.Join(root, common.ModsFolder), id, modded)
	return id, pristine, modded
}

// withModsOff simula o checkbox desligado (common.DisableMods é o que
// SetModsEnabled escreve) e restaura o estado no fim do teste.
func withModsOff(t *testing.T) {
	t.Helper()
	prev := common.DisableMods
	common.DisableMods = true
	t.Cleanup(func() { common.DisableMods = prev })
}

// decodePNG devolve o PNG da decodificação completa do container — o
// mesmo caminho que resolveFrom usa quando não há artefato em disco.
func decodePNG(t *testing.T, raw []byte) []byte {
	t.Helper()
	png, err := DDSToPNG(extractTestDDS(t, raw))
	if err != nil {
		t.Fatalf("DDSToPNG: %v", err)
	}
	return png
}

// extractTestDDS devolve o DDS embutido no container.
func extractTestDDS(t *testing.T, raw []byte) []byte {
	t.Helper()
	tex, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dds, err := tex.ExtractToDDS()
	if err != nil {
		t.Fatalf("ExtractToDDS: %v", err)
	}
	return dds
}

// Container: mods-first clássico com EnableMods ligado; desligado, a árvore
// mods/ fica invisível e o pristine de data/ responde sozinho — o mesmo
// toggle que faz o texto voltar ao inglês.
func TestResolveRespectsDisableMods(t *testing.T) {
	id, pristine, modded := withToggleTree(t)
	version := common.GameVersionFFX

	wantModded := decodePNG(t, modded)
	wantPristine := decodePNG(t, pristine)

	on, err := Resolve(version, id)
	if err != nil {
		t.Fatalf("Resolve (mods ligado): %v", err)
	}
	if !on.InMods {
		t.Error("mods ligado: esperava InMods=true")
	}
	if !bytes.Equal(on.PNG, wantModded) {
		t.Error("mods ligado: preview não é o container traduzido")
	}

	withModsOff(t)
	off, err := Resolve(version, id)
	if err != nil {
		t.Fatalf("Resolve (mods desligado): %v", err)
	}
	if off.InMods {
		t.Error("mods desligado: esperava InMods=false")
	}
	if off.Source != "phyre" {
		t.Errorf("mods desligado: source = %q, esperado \"phyre\" (decode do container)", off.Source)
	}
	if !bytes.Equal(off.PNG, wantPristine) {
		t.Error("mods desligado: preview não é o pristine de data/")
	}
	if bytes.Equal(off.PNG, wantModded) {
		t.Error("mods desligado: preview é o container traduzido (toggle ignorado)")
	}
}

// Artefatos de mods/images: preferência com mods ligado, invisíveis
// desligado — desligados eles podem ser o export de uma tradução
// anterior e não podem decidir o preview.
func TestResolveSkipsArtifactsWithDisableMods(t *testing.T) {
	id, _, _ := withToggleTree(t)
	version := common.GameVersionFFX
	_, pngPath := ExportPaths(version, id)

	artifact := []byte("PNG-DE-EXPORT-ANTIGO")
	if err := common.EnsurePathExists(pngPath); err != nil {
		t.Fatalf("criando mods/images: %v", err)
	}
	if err := os.WriteFile(pngPath, artifact, 0o644); err != nil {
		t.Fatalf("gravando artefato: %v", err)
	}

	on, err := Resolve(version, id)
	if err != nil {
		t.Fatalf("Resolve (mods ligado): %v", err)
	}
	if on.Source != "png" || !bytes.Equal(on.PNG, artifact) {
		t.Errorf("mods ligado: esperava o artefato de disco (source=%q)", on.Source)
	}

	withModsOff(t)
	off, err := Resolve(version, id)
	if err != nil {
		t.Fatalf("Resolve (mods desligado): %v", err)
	}
	if off.Source != "phyre" {
		t.Errorf("mods desligado: source = %q, esperado \"phyre\"", off.Source)
	}
	if bytes.Equal(off.PNG, artifact) {
		t.Error("mods desligado: serviu o artefato de mods/images (toggle ignorado)")
	}
}

// Extract do usuário segue o que se vê (pristine com mods desligados);
// ExtractEffective — o refresh interno — espelha o container efetivo
// mesmo com mods desligados E mesmo com artefato velho em disco: é o par
// que impede o artefato pristine de mascarar uma tradução ao religar.
func TestExtractFollowsToggleAndEffectiveDoesNot(t *testing.T) {
	id, pristine, modded := withToggleTree(t)
	version := common.GameVersionFFX
	wantPristine := extractTestDDS(t, pristine)
	wantModded := extractTestDDS(t, modded)
	ddsPath, _ := ExportPaths(version, id)

	withModsOff(t)
	if _, err := Extract(version, id); err != nil {
		t.Fatalf("Extract (mods desligado): %v", err)
	}
	got, err := os.ReadFile(ddsPath)
	if err != nil {
		t.Fatalf("lendo artefato: %v", err)
	}
	if !bytes.Equal(got, wantPristine) {
		t.Error("Extract com mods desligado não exportou o pristine visível")
	}

	// O artefato pristine recém-escrito NÃO pode se auto-servir: o refresh
	// efetivo decodifica o container de mods/ e o sobrescreve.
	if _, err := ExtractEffective(version, id); err != nil {
		t.Fatalf("ExtractEffective (mods desligado): %v", err)
	}
	if got, err = os.ReadFile(ddsPath); err != nil {
		t.Fatalf("lendo artefato: %v", err)
	}
	if !bytes.Equal(got, wantModded) {
		t.Error("ExtractEffective não espelhou o container efetivo (toggle ou artefato velho vazaram no refresh)")
	}

	// Mods ligado: os dois voltam ao comportamento clássico (efetivo).
	common.DisableMods = false
	if _, err := Extract(version, id); err != nil {
		t.Fatalf("Extract (mods ligado): %v", err)
	}
	if got, err = os.ReadFile(ddsPath); err != nil {
		t.Fatalf("lendo artefato: %v", err)
	}
	if !bytes.Equal(got, wantModded) {
		t.Error("Extract com mods ligado deveria extrair o container efetivo")
	}
}

// Textura que SÓ existe em mods/: resolúvel com mods ligado; desligado, a
// árvore invisível não é consultada e o erro nomeia o toggle.
func TestResolveModsOnlyTextureWithToggleOff(t *testing.T) {
	_, pristine, _ := withToggleTree(t)
	root := common.GameFilesRoot
	id := "gamedata/ps3data/toggle/only"
	writePhyre(t, filepath.Join(root, common.ModsFolder), id, pristine)
	version := common.GameVersionFFX

	if _, err := Resolve(version, id); err != nil {
		t.Fatalf("Resolve (mods ligado) de textura só-mods: %v", err)
	}

	withModsOff(t)
	_, err := Resolve(version, id)
	if err == nil {
		t.Fatal("mods desligado: textura só em mods/ não deveria resolver")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("mods desligados")) {
		t.Errorf("erro não menciona o toggle: %v", err)
	}
}
