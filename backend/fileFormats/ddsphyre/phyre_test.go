package ddsphyre

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
)

// samplePhyre varre uma árvore em busca de um .dds.phyre real (prioridade:
// build/bin/data, depois testData). skip=true quando não há arquivo.
func samplePhyre(t *testing.T) (string, []byte) {
	t.Helper()
	roots := []string{
		filepath.Join("..", "..", "..", "build", "bin", "data"),
		filepath.Join("..", "..", "testData", "FFX", "binary"),
		filepath.Join("..", "..", "examples", "testData"),
	}
	var found string
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() || found != "" {
				return nil
			}
			if strings.HasSuffix(d.Name(), Suffix) {
				found = path
			}
			return nil
		})
		if err != nil {
			continue
		}
		if found != "" {
			break
		}
	}
	if found == "" {
		t.Skip("nenhum .dds.phyre de amostra disponível")
	}
	raw, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("lendo %s: %v", found, err)
	}
	return found, raw
}

func TestParseExtractRoundTrip(t *testing.T) {
	_, raw := samplePhyre(t)

	tex, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !IsPhyre(raw) {
		t.Fatal("IsPhyre devolveu false para um arquivo válido")
	}
	switch tex.Format {
	case FormatDXT1, FormatDXT3, FormatDXT5, FormatBC5, FormatARGB8, FormatA8, FormatL8:
	default:
		t.Errorf("formato inesperado %q", tex.Format)
	}
	if tex.Width == 0 || tex.Height == 0 {
		t.Errorf("dimensões inválidas %dx%d", tex.Width, tex.Height)
	}
	if tex.DataOffset >= uint64(len(raw)) {
		t.Fatalf("dataOffset %d além do arquivo (%d)", tex.DataOffset, len(raw))
	}

	dds, err := tex.ExtractToDDS()
	if err != nil {
		t.Fatalf("ExtractToDDS: %v", err)
	}
	if string(dds[0:4]) != "DDS " {
		t.Fatalf("magia do DDS extraído = %q", dds[0:4])
	}
	hdr, err := parseDDSHeader(dds)
	if err != nil {
		t.Fatalf("parseDDSHeader: %v", err)
	}
	if hdr.width != tex.Width || hdr.height != tex.Height {
		t.Errorf("header DDS %dx%d, phyre %dx%d", hdr.width, hdr.height, tex.Width, tex.Height)
	}
	if got, err := ddsFormat(hdr); err != nil || got != tex.Format {
		t.Errorf("ddsFormat = %q/%v, esperado %q", got, err, tex.Format)
	}
	if hdr.mips != tex.MipmapCount+1 {
		t.Errorf("dwMipMapCount = %d, esperado mips+1 = %d", hdr.mips, tex.MipmapCount+1)
	}
	if !bytes.Equal(dds[ddsHeaderLen:], raw[int(tex.DataOffset):]) {
		t.Error("payload extraído difere do payload do phyre")
	}

	// Round-trip: reempacotar o DDS extraído devolve um container cuja
	// textura e payload são os mesmos do original.
	packed, err := PackDDS(raw, dds)
	if err != nil {
		t.Fatalf("PackDDS: %v", err)
	}
	again, err := Parse(packed)
	if err != nil {
		t.Fatalf("Parse(PackDDS): %v", err)
	}
	if again.Format != tex.Format || again.Width != tex.Width || again.Height != tex.Height {
		t.Errorf("repacote mudou a textura: %s", again)
	}
	if again.MipmapCount != tex.MipmapCount {
		t.Errorf("repacote mudou mips: %d → %d", tex.MipmapCount, again.MipmapCount)
	}
	if !bytes.Equal(again.Payload, tex.Payload) {
		t.Error("payload do repacote difere do original")
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		mut  func([]byte)
	}{
		{"magic", func(b []byte) { b[0], b[1], b[2], b[3] = 0x50, 0x48, 0x59, 0x52 }},
		{"size", func(b []byte) { put32(b, offSize, 80) }},
		{"platform", func(b []byte) { put32(b, offPlatformID, 0) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, raw := samplePhyre(t)
			b := append([]byte(nil), raw...)
			c.mut(b)
			if _, err := Parse(b); err == nil {
				t.Errorf("%s: esperava erro", c.name)
			}
			if IsPhyre(b) {
				t.Errorf("%s: IsPhyre deveria ser false", c.name)
			}
		})
	}
}

func TestPackDDSRejectsFormatChange(t *testing.T) {
	_, raw := samplePhyre(t)
	tex, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	other := FormatL8
	if tex.Format == FormatL8 {
		other = FormatA8
	}
	header, err := prepareDDSHeader(other, tex.Width, tex.Height, tex.MipmapCount)
	if err != nil {
		t.Fatalf("prepareDDSHeader: %v", err)
	}
	fake := append(header, tex.Payload...)
	if _, err := PackDDS(raw, fake); err == nil {
		t.Fatal("esperava erro de mudança de formato")
	}
}

func TestDDSToPNG(t *testing.T) {
	_, raw := samplePhyre(t)
	tex, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dds, err := tex.ExtractToDDS()
	if err != nil {
		t.Fatalf("ExtractToDDS: %v", err)
	}
	pngBytes, err := DDSToPNG(dds)
	if err != nil {
		t.Fatalf("DDSToPNG: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	if img.Bounds().Dx() != int(tex.Width) || img.Bounds().Dy() != int(tex.Height) {
		t.Errorf("png %v, phyre %dx%d", img.Bounds(), tex.Width, tex.Height)
	}
}

func TestScanFindsTextures(t *testing.T) {
	for _, version := range []common.GameVersion{common.GameVersionFFX, common.GameVersionFFX2} {
		ids, _, fallback, err := Scan(version)
		if err != nil {
			t.Fatalf("Scan(%s): %v", version, err)
		}
		if len(ids) == 0 {
			t.Logf("Scan(%s): nenhum .dds.phyre (árvore não extraída?)", version)
			continue
		}
		if fallback {
			t.Logf("Scan(%s): fallback para mods/ com %d texturas", version, len(ids))
		}
		for _, id := range ids {
			if !strings.HasPrefix(id, "gamedata/ps3data/") {
				t.Fatalf("id fora do esperado: %s", id)
			}
			if _, err := os.Stat(RelPath(version, id)); err != nil && !fallback {
				t.Errorf("RelPath não existe: %s", RelPath(version, id))
			}
		}
	}
}

func TestResolveOrders(t *testing.T) {
	ids, _, _, err := Scan(common.GameVersionFFX)
	if err != nil || len(ids) == 0 {
		t.Skipf("sem texturas para o teste (%v)", err)
	}
	id := ids[0]

	r, err := Resolve(common.GameVersionFFX, id)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Source != "phyre" {
		t.Errorf("sem artefatos em disco a fonte deveria ser phyre, veio %q", r.Source)
	}
	if len(r.PNG) == 0 || len(r.DDS) == 0 {
		t.Fatal("Resolve sem PNG/DDS preenchido")
	}
	if _, err := png.Decode(bytes.NewReader(r.PNG)); err != nil {
		t.Errorf("PNG inválido: %v", err)
	}
}

// withTempTree aponta GameFilesRoot para um diretório temporário com o
// sample copiado em ffx_data/gamedata/ps3data/teste.dds.phyre e devolve o
// id da textura. O root original é restaurado no fim do teste.
func withTempTree(t *testing.T) (root, id string, raw []byte) {
	t.Helper()
	_, sample := samplePhyre(t)
	root = tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() { common.GameFilesRoot = prev })

	id = "gamedata/ps3data/teste"
	dest := filepath.Join(root, "ffx_data", filepath.FromSlash(id)+Suffix)
	if err := common.EnsurePathExists(dest); err != nil {
		t.Fatalf("criando árvore: %v", err)
	}
	if err := os.WriteFile(dest, sample, 0o644); err != nil {
		t.Fatalf("copiando sample: %v", err)
	}
	return root, id, sample
}

func TestExtractResolveImportFlow(t *testing.T) {
	root, id, raw := withTempTree(t)

	// 1. nada extraído → fonte phyre (decode em memória, sem gravar).
	r, err := Resolve(common.GameVersionFFX, id)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Source != "phyre" {
		t.Errorf("fonte inicial = %q, esperado phyre", r.Source)
	}

	// 2. extrair grava .dds e .png em mods/images.
	paths, err := Extract(common.GameVersionFFX, id)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("Extract devolveu %d caminhos", len(paths))
	}
	for _, p := range paths {
		if !fileExists(p) {
			t.Errorf("Extract não gravou %s", p)
		}
	}
	if !strings.Contains(filepath.ToSlash(paths[0]), "/mods/images/ffx_data/") {
		t.Errorf("caminho do extract fora do padrão: %s", paths[0])
	}

	// 3. com artefatos em disco a prioridade vira .dds.
	r, err = Resolve(common.GameVersionFFX, id)
	if err != nil {
		t.Fatalf("Resolve após extract: %v", err)
	}
	if r.Source != "dds" {
		t.Errorf("fonte = %q, esperado dds", r.Source)
	}

	// 4. reimportar o mesmo DDS: em imagem o repack NÃO é byte a byte (o
	// header recalcula maxTextureBufferSize e os campos de textura podem
	// ter outro alinhamento/ordenamento) — o que importa é a equivalência
	// semântica: mesmos metadados e mesmo payload de textura.
	// E o reimport grava apenas em mods/.
	if err := Import(common.GameVersionFFX, id, paths[0]); err != nil {
		t.Fatalf("Import: %v", err)
	}
	modsRel := filepath.Join(root, common.ModsFolder, RelPath(common.GameVersionFFX, id))
	packed, err := os.ReadFile(modsRel)
	if err != nil {
		t.Fatalf("lendo mods: %v", err)
	}
	orig, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse do original: %v", err)
	}
	got, err := Parse(packed)
	if err != nil {
		t.Fatalf("Parse do import: %v", err)
	}
	if got.Format != orig.Format || got.Width != orig.Width || got.Height != orig.Height {
		t.Errorf("reimport mudou a textura: %s → %s", orig, got)
	}
	if got.MipmapCount != orig.MipmapCount {
		t.Errorf("reimport mudou mips: %d → %d", orig.MipmapCount, got.MipmapCount)
	}
	if !bytes.Equal(got.Payload, orig.Payload) {
		t.Error("payload da textura mudou no reimport")
	}

	// 5. o pristine nunca é tocado.
	still, err := os.ReadFile(filepath.Join(root, "ffx_data", filepath.FromSlash(id)+Suffix))
	if err != nil {
		t.Fatalf("lendo pristine: %v", err)
	}
	if !bytes.Equal(still, raw) {
		t.Error("Import alterou o arquivo de data/")
	}
}

func TestSaveDDSAndPNG(t *testing.T) {
	_, id, _ := withTempTree(t)
	dest := filepath.Join(tempdir.TempDir(t), "saida.dds")
	if err := Save(common.GameVersionFFX, id, "dds", dest); err != nil {
		t.Fatalf("Save dds: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("lendo saida: %v", err)
	}
	if _, err := parseDDSHeader(b); err != nil {
		t.Errorf("dds salvo inválido: %v", err)
	}
	if err := Save(common.GameVersionFFX, id, "gif", dest); err == nil {
		t.Error("formato inválido deveria dar erro")
	}
}
