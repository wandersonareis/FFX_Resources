package services

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/testData/tempdir"
)

// "Exportar para…" da árvore data/ grava na pasta escolhida pelo usuário
// preservando o caminho interno (raiz da versão + id) — o mesmo contrato do
// "Extrair para…" do .vbf — e não toca em mods/images.
func TestSaveImageToPreservesInnerPathUnderDestRoot(t *testing.T) {
	sample, _ := realSampleWithPayload(t)

	root := tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		ddsphyre.InvalidateIndexes()
	})

	const id = "gamedata/ps3data/zzz_export_to/tex"
	writePhyre(t, root, id, sample)

	destRoot := tempdir.TempDir(t)
	svc := NewMetadataService(nil)

	written, err := svc.SaveImageTo(KindImages, id, "png", destRoot, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("SaveImageTo: %v", err)
	}
	rel, rerr := filepath.Rel(destRoot, written)
	if rerr != nil {
		t.Fatalf("relativo ao destino: %v", rerr)
	}
	want := filepath.Join(
		ddsphyre.RootForVersion(common.GameVersionFFX),
		filepath.FromSlash(id)+".png",
	)
	if rel != want {
		t.Fatalf("destino = %s, esperado %s sob a raiz escolhida", rel, want)
	}
	if st, serr := os.Stat(written); serr != nil || st.Size() == 0 {
		t.Fatalf("arquivo não gravado: %v", serr)
	}
	if _, serr := os.Stat(filepath.Join(modsImagesRoot(), want)); !os.IsNotExist(serr) {
		t.Errorf("export para pasta customizada escreveu em mods/images: %v", serr)
	}
}

// Sem raiz escolhida (destino vazio) o export cai no padrão de mods/images,
// igual ao "Exportar" direto do menu.
func TestSaveImageToEmptyDestRootFallsBackToModsImages(t *testing.T) {
	sample, _ := realSampleWithPayload(t)

	root := tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		ddsphyre.InvalidateIndexes()
	})

	const id = "gamedata/ps3data/zzz_export_to_default/tex"
	writePhyre(t, root, id, sample)

	written, err := NewMetadataService(nil).SaveImageTo(KindImages, id, "dds", "", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("SaveImageTo: %v", err)
	}
	ddsPath, _ := ddsphyre.ExportPaths(common.GameVersionFFX, id)
	if written != ddsPath {
		t.Fatalf("destino = %s, esperado o padrão %s", written, ddsPath)
	}
	if _, serr := os.Stat(written); serr != nil {
		t.Fatalf("arquivo não gravado em mods/images: %v", serr)
	}
}
