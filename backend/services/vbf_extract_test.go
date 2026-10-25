package services

// Teste da extração/export da SELEÇÃO da árvore do .vbf: expansão de
// caminhos (arquivo, diretório = subárvore, raiz = tudo), preview com
// contagem de existentes, extração preservando a estrutura interna e
// export filtrando só os kinds de texto.
//
// O container é SINTÉTICO (montado aqui pelo mesmo layout do pacote vbf),
// em temp dir, com um FFX.exe fake para a descoberta de raízes achar —
// nada depende da instalação real do jogo.

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/fileFormats/vbf"
	"ffxresources/backend/interactions"
)

// montarVbfSintetico escreve um .vbf com os arquivos pedidos (blocos
// comprimidos quando rende). É o mesmo layout do montador de
// vbf_test.go — replicado aqui porque o original é interno do pacote.
func montarVbfSintetico(t *testing.T, path string, files map[string][]byte) {
	t.Helper()

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sortStringsLocal(names)

	var nameTab bytes.Buffer
	nameOffset := make(map[string]uint64, len(names))
	for _, n := range names {
		nameOffset[n] = uint64(nameTab.Len())
		nameTab.WriteString(n)
		nameTab.WriteByte(0)
	}

	const blockSize = 64 << 10
	type ent struct {
		name  string
		size  uint64
		start uint32
	}
	var (
		ents       []ent
		blockSizes []uint16
		chunks     [][]byte
		blockIx    uint32
	)
	for _, n := range names {
		data := files[n]
		e := ent{name: n, size: uint64(len(data)), start: blockIx}
		for pos := 0; pos < len(data); {
			end := pos + blockSize
			if end > len(data) {
				end = len(data)
			}
			chunk := data[pos:end]
			var stored []byte
			var sz uint16
			var buf bytes.Buffer
			zw := zlib.NewWriter(&buf)
			_, _ = zw.Write(chunk)
			_ = zw.Close()
			last := end == len(data)
			if buf.Len() <= 0xFFFF && buf.Len() < len(chunk) && !(last && int(buf.Len()) == len(chunk)) {
				stored, sz = buf.Bytes(), uint16(buf.Len())
			} else if len(chunk) == blockSize {
				stored, sz = chunk, 0
			} else {
				stored, sz = chunk, uint16(len(chunk))
			}
			blockSizes = append(blockSizes, sz)
			chunks = append(chunks, stored)
			blockIx++
			pos = end
		}
		ents = append(ents, e)
	}

	nameTableLen := 4 + nameTab.Len()
	headerLen := 16 + len(names)*48 + nameTableLen + len(blockSizes)*2

	payloadOff := make([]uint64, len(chunks))
	off := uint64(headerLen)
	for i, c := range chunks {
		payloadOff[i] = off
		off += uint64(len(c))
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, vbf.Magic)
	_ = binary.Write(&out, binary.LittleEndian, uint32(headerLen))
	_ = binary.Write(&out, binary.LittleEndian, uint64(len(names)))
	for _, n := range names {
		sum := md5.Sum([]byte(strings.ToLower(n)))
		out.Write(sum[:])
	}
	for _, e := range ents {
		_ = binary.Write(&out, binary.LittleEndian, e.start)
		_ = binary.Write(&out, binary.LittleEndian, uint32(0))
		_ = binary.Write(&out, binary.LittleEndian, e.size)
		dataOff := uint64(headerLen)
		if int(e.start) < len(payloadOff) {
			dataOff = payloadOff[e.start]
		}
		_ = binary.Write(&out, binary.LittleEndian, dataOff)
		_ = binary.Write(&out, binary.LittleEndian, nameOffset[e.name])
	}
	_ = binary.Write(&out, binary.LittleEndian, uint32(nameTableLen))
	out.Write(nameTab.Bytes())
	for _, sz := range blockSizes {
		out.Write([]byte{byte(sz), byte(sz >> 8)})
	}
	for _, c := range chunks {
		out.Write(c)
	}
	sum := md5.Sum(out.Bytes()[:headerLen])
	out.Write(sum[:])

	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sortStringsLocal(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// seedVbfExtractRoot monta um temp dir com FFX.exe fake + FFX_Data.vbf
// sintético e aponta o config para lá. Devolve o caminho do .vbf.
func seedVbfExtractRoot(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := tempdir.TempDir(t)
	exe := filepath.Join(dir, "FFX.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	vbfPath := filepath.Join(dir, "FFX_Data.vbf")
	montarVbfSintetico(t, vbfPath, files)

	_ = interactions.NewInteractionService()
	cfg := interactions.NewInteractionService().FFXAppConfig()
	prevExe := cfg.GetGameExeLocation()
	t.Cleanup(func() {
		cfg.SetGameExeLocation(prevExe)
		CloseVbfArchives()
	})
	cfg.SetGameExeLocation(exe)
	return vbfPath
}

func vbfExtractFixture() map[string][]byte {
	return map[string][]byte{
		// Diretório com dois arquivos (um deles fundo).
		"ffx_data/gamedata/ps3data/a_dir/deep/leaf.txt": []byte("leaf"),
		"ffx_data/gamedata/ps3data/a_dir/other.txt":     []byte("other"),
		// Texto comprimido (força o caminho deflate).
		"ffx_data/gamedata/ps3data/big.bin": bytes.Repeat([]byte("FFX Resources — repetido. "), 4000),
		// Fora do escopo do app (sem kind): só extrai, não exporta.
		"version_config/config.bin": []byte("cfg"),
	}
}

func TestVbfPreviewEExtracaoSelecao(t *testing.T) {
	vbfPath := seedVbfExtractRoot(t, vbfExtractFixture())
	svc := NewMetadataService(nil)

	dest := tempdir.TempDir(t)

	// Preview de um DIRETÓRIO: 2 arquivos, 0 existentes.
	prev, err := svc.PreviewVbfExtraction(vbfPath, []string{"ffx_data/gamedata/ps3data/a_dir"}, dest)
	if err != nil {
		t.Fatalf("PreviewVbfExtraction: %v", err)
	}
	if prev.Files != 2 || prev.Existing != 0 {
		t.Fatalf("preview = %+v, esperado {2 arquivos, 0 existentes}", prev)
	}

	// Extração do diretório: estrutura preservada, conteúdo igual.
	res, err := svc.ExtractVbfSelection(vbfPath, []string{"ffx_data/gamedata/ps3data/a_dir"}, dest)
	if err != nil {
		t.Fatalf("ExtractVbfSelection: %v", err)
	}
	if len(res.Done) != 2 || len(res.Failed) != 0 || res.Total != 2 {
		t.Fatalf("resultado = %+v", res)
	}
	for name, want := range vbfExtractFixture() {
		if !strings.HasPrefix(name, "ffx_data/gamedata/ps3data/a_dir/") {
			continue
		}
		got, rerr := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if rerr != nil {
			t.Errorf("lendo %s: %v", name, rerr)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: %d bytes, esperado %d", name, len(got), len(want))
		}
	}

	// Preview DEPOIS: os mesmos 2 agora existem (aviso de sobrescrita).
	prev2, err := svc.PreviewVbfExtraction(vbfPath, []string{"ffx_data/gamedata/ps3data/a_dir"}, dest)
	if err != nil {
		t.Fatalf("PreviewVbfExtraction(2): %v", err)
	}
	if prev2.Existing != 2 {
		t.Fatalf("preview pós-extração = %+v, esperado 2 existentes", prev2)
	}
}

func TestExtractVbfImagesSelectionPreservesInnerPathAndSkipsOtherKinds(t *testing.T) {
	phyre, _ := realSampleWithPayload(t)
	imagePath := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15040_19_0_0_128_128.dds.phyre"
	otherPath := "ffx_data/gamedata/ps3data/a_dir/not-an-image.bin"
	vbfPath := seedVbfExtractRoot(t, map[string][]byte{
		imagePath: phyre,
		otherPath: []byte("binary"),
	})
	dest := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = tempdir.TempDir(t)
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })

	res, err := NewMetadataService(nil).ExtractVbfImagesSelection(vbfPath, []string{"ffx_data/gamedata/ps3data"}, dest)
	if err != nil {
		t.Fatalf("ExtractVbfImagesSelection: %v", err)
	}
	if res.Total != 1 || len(res.Done) != 1 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado uma imagem e ignorar o binário", res)
	}
	stem := strings.TrimSuffix(imagePath, ddsphyre.Suffix)
	for _, ext := range []string{".dds", ".png"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(stem+ext))); err != nil {
			t.Errorf("artefato %s não preservou caminho interno: %v", ext, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(otherPath))); !os.IsNotExist(err) {
		t.Errorf("extração de imagens escreveu arquivo incompatível: %v", err)
	}
}

func TestVbfImageDuplicatesAreDiscoveredAsImagesAreOpened(t *testing.T) {
	phyre, _ := realSampleWithPayload(t)
	pathA := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15040_19_0_0_128_128.dds.phyre"
	pathB := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15041_19_0_0_128_128.dds.phyre"
	vbfPath := seedVbfExtractRoot(t, map[string][]byte{
		pathA: phyre,
		pathB: append([]byte(nil), phyre...),
	})
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = tempdir.TempDir(t)
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })
	vbfSessionResetAll()
	t.Cleanup(vbfSessionResetAll)

	svc := NewMetadataService(nil)
	first, err := svc.GetVbfImageEntry(vbfPath, pathA)
	if err != nil {
		t.Fatalf("abrindo primeira imagem: %v", err)
	}
	if len(first.Duplicates) != 0 {
		t.Fatalf("primeira imagem não deveria conhecer arquivos ainda não abertos: %+v", first.Duplicates)
	}
	second, err := svc.GetVbfImageEntry(vbfPath, pathB)
	if err != nil {
		t.Fatalf("abrindo segunda imagem: %v", err)
	}
	if len(second.Duplicates) != 1 || second.Duplicates[0].ID != first.Metadata.ID || second.Duplicates[0].VbfPath != pathA {
		t.Fatalf("segunda imagem não encontrou a primeira cópia aberta: %+v", second.Duplicates)
	}
	first, err = svc.GetVbfImageEntry(vbfPath, pathA)
	if err != nil {
		t.Fatalf("reabrindo primeira imagem: %v", err)
	}
	if len(first.Duplicates) != 1 || first.Duplicates[0].ID != second.Metadata.ID {
		t.Fatalf("primeira imagem não encontrou a segunda após reabertura: %+v", first.Duplicates)
	}
}

func TestVbfImageDedupMarksModsOverlayAsDivergent(t *testing.T) {
	phyre, _ := realSampleWithPayload(t)
	pathA := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15040_19_0_0_128_128.dds.phyre"
	pathB := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15041_19_0_0_128_128.dds.phyre"
	vbfPath := seedVbfExtractRoot(t, map[string][]byte{
		pathA: phyre,
		pathB: append([]byte(nil), phyre...),
	})
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = tempdir.TempDir(t)
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })
	vbfSessionResetAll()
	t.Cleanup(vbfSessionResetAll)

	texture, err := ddsphyre.Parse(phyre)
	if err != nil {
		t.Fatal(err)
	}
	dds, err := texture.ExtractToDDS()
	if err != nil {
		t.Fatal(err)
	}
	if len(dds) <= 128 {
		t.Fatalf("fixture DDS curto demais: %d", len(dds))
	}
	dds[len(dds)-1] ^= 0xff // mantém dimensões/formato, altera o payload efetivo.
	ddsPath := filepath.Join(tempdir.TempDir(t), "divergente.dds")
	if err := os.WriteFile(ddsPath, dds, 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewMetadataService(nil)
	if _, err := svc.GetVbfImageEntry(vbfPath, pathA); err != nil {
		t.Fatalf("abrindo imagem original: %v", err)
	}
	if err := svc.ImportVbfImage(vbfPath, pathB, ddsPath); err != nil {
		t.Fatalf("importando overlay em mods: %v", err)
	}
	if _, err := svc.GetVbfImageEntry(vbfPath, pathA); err != nil {
		t.Fatalf("reabrindo imagem original após invalidar a sessão: %v", err)
	}
	entry, err := svc.GetVbfImageEntry(vbfPath, pathB)
	if err != nil {
		t.Fatalf("abrindo imagem divergente: %v", err)
	}
	if len(entry.Duplicates) != 1 || entry.Duplicates[0].ID != matchImageID(t, vbfPath, pathA) || entry.Duplicates[0].Identical {
		t.Fatalf("overlay deveria aparecer como duplicata divergente: %+v", entry.Duplicates)
	}
}

func TestReplicateVbfImageWritesOnlyModsForOpenedDuplicates(t *testing.T) {
	phyre, _ := realSampleWithPayload(t)
	pathA := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15040_19_0_0_128_128.dds.phyre"
	pathB := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15041_19_0_0_128_128.dds.phyre"
	vbfPath := seedVbfExtractRoot(t, map[string][]byte{
		pathA: phyre,
		pathB: append([]byte(nil), phyre...),
	})
	gameRoot := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = gameRoot
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })
	vbfSessionResetAll()
	t.Cleanup(vbfSessionResetAll)

	svc := NewMetadataService(nil)
	if _, err := svc.GetVbfImageEntry(vbfPath, pathA); err != nil {
		t.Fatalf("abrindo fonte: %v", err)
	}
	if _, err := svc.GetVbfImageEntry(vbfPath, pathB); err != nil {
		t.Fatalf("abrindo cópia: %v", err)
	}
	res, err := svc.ReplicateVbfImage(vbfPath, pathA, []string{pathB})
	if err != nil {
		t.Fatalf("ReplicateVbfImage: %v", err)
	}
	if res.Total != 1 || len(res.Done) != 1 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v", res)
	}
	target, ok := matchVbfPath("FFX_Data.vbf", pathB)
	if !ok {
		t.Fatal("path de imagem não reconhecido")
	}
	modsPath := filepath.Join(gameRoot, common.ModsFolder, ddsphyre.RelPath(target.Version, target.ID))
	if _, err := os.Stat(modsPath); err != nil {
		t.Fatalf("réplica deveria ser gravada em mods/: %v", err)
	}
	archive, err := vbfArchiveFor(vbfPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := archive.Read(pathB)
	if err != nil || !bytes.Equal(got, phyre) {
		t.Fatalf("replicar modificou o .vbf: err=%v", err)
	}
}

func matchImageID(t *testing.T, vbfPath, innerPath string) string {
	t.Helper()
	_, target, err := NewMetadataService(nil).vbfTargetOf(vbfPath, innerPath)
	if err != nil {
		t.Fatal(err)
	}
	return target.ID
}

func TestVbfImageImportWritesModsAndSaveReadsVbfImage(t *testing.T) {
	phyre, _ := realSampleWithPayload(t)
	inner := "ffx_data/gamedata/ps3data/yonishi_data/dat_et/et_ffx/tex/d3d11/15040_19_0_0_128_128.dds.phyre"
	vbfPath := seedVbfExtractRoot(t, map[string][]byte{inner: phyre})
	gameRoot := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = gameRoot
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })

	texture, err := ddsphyre.Parse(phyre)
	if err != nil {
		t.Fatalf("parseando fixture: %v", err)
	}
	dds, err := texture.ExtractToDDS()
	if err != nil {
		t.Fatalf("extraindo DDS fixture: %v", err)
	}
	ddsPath := filepath.Join(tempdir.TempDir(t), "edited.dds")
	if err := os.WriteFile(ddsPath, dds, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewMetadataService(nil)
	if err := svc.ImportVbfImage(vbfPath, inner, ddsPath); err != nil {
		t.Fatalf("ImportVbfImage: %v", err)
	}
	_, target, err := svc.vbfTargetOf(vbfPath, inner)
	if err != nil {
		t.Fatal(err)
	}
	modsPath := filepath.Join(gameRoot, common.ModsFolder, ddsphyre.RelPath(target.Version, target.ID))
	if _, err := os.Stat(modsPath); err != nil {
		t.Fatalf("a importação deveria gravar em mods/: %v", err)
	}

	for _, format := range []string{"dds", "png"} {
		dest := filepath.Join(tempdir.TempDir(t), "saved."+format)
		if err := svc.SaveVbfImage(vbfPath, inner, format, dest); err != nil {
			t.Fatalf("SaveVbfImage(%s): %v", format, err)
		}
		if info, err := os.Stat(dest); err != nil || info.Size() == 0 {
			t.Errorf("arquivo salvo %s ausente/vazio: info=%v err=%v", format, info, err)
		}
	}
}

func TestVbfExtracaoRaizEAquivoSoltos(t *testing.T) {
	vbfPath := seedVbfExtractRoot(t, vbfExtractFixture())
	svc := NewMetadataService(nil)
	dest := tempdir.TempDir(t)

	// Arquivo solto + arquivo de outro ramo na MESMA seleção.
	res, err := svc.ExtractVbfSelection(vbfPath, []string{
		"ffx_data/gamedata/ps3data/big.bin",
		"version_config/config.bin",
	}, dest)
	if err != nil {
		t.Fatalf("ExtractVbfSelection: %v", err)
	}
	if len(res.Done) != 2 {
		t.Fatalf("done = %d, esperado 2 (%+v)", len(res.Done), res)
	}
	got, err := os.ReadFile(filepath.Join(dest, "version_config", "config.bin"))
	if err != nil || string(got) != "cfg" {
		t.Errorf("config.bin extraído errado: %q (%v)", got, err)
	}

	// RAIZ do container (path vazio) = tudo, sem duplicar com a seleção mista.
	all, err := svc.ExtractVbfSelection(vbfPath, []string{"", "ffx_data/gamedata/ps3data/a_dir"}, dest)
	if err != nil {
		t.Fatalf("ExtractVbfSelection(raiz): %v", err)
	}
	if len(all.Done) != len(vbfExtractFixture()) {
		t.Fatalf("raiz: done = %d, esperado %d", len(all.Done), len(vbfExtractFixture()))
	}
}

func TestVbfSelecaoComExcecoesDeCheckbox(t *testing.T) {
	vbfPath := seedVbfExtractRoot(t, vbfExtractFixture())
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		t.Fatalf("abrindo container: %v", err)
	}
	t.Cleanup(CloseVbfArchives)

	dir := "ffx_data/gamedata/ps3data/a_dir"
	child := dir + "/other.txt"

	// Diretório selecionado menos um arquivo explicitamente desmarcado.
	got, err := expandVbfSelection(a, []string{dir, "!" + child})
	if err != nil {
		t.Fatalf("expansão da seleção: %v", err)
	}
	if len(got) != 1 || got[0].Path != dir+"/deep/leaf.txt" {
		t.Fatalf("seleção com exceção = %+v, esperado só leaf.txt", got)
	}

	// Uma inclusão mais específica volta a marcar um arquivo sob uma exceção
	// de diretório (regra mais específica vence).
	got, err = expandVbfSelection(a, []string{"!" + dir, child})
	if err != nil {
		t.Fatalf("expansão da re-inclusão: %v", err)
	}
	if len(got) != 1 || got[0].Path != child {
		t.Fatalf("re-inclusão específica = %+v, esperado só other.txt", got)
	}
}

func TestVbfSelecaoRejectsTraversal(t *testing.T) {
	vbfPath := seedVbfExtractRoot(t, vbfExtractFixture())
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		t.Fatalf("abrindo container: %v", err)
	}
	t.Cleanup(CloseVbfArchives)
	for _, selection := range [][]string{
		{"../"},
		{"ffx_data/../../"},
		{`C:\outside`},
		{"!/"},
	} {
		if _, err := expandVbfSelection(a, selection); err == nil {
			t.Errorf("seleção insegura aceita: %q", selection)
		}
	}
}

func TestVbfExtractTargetRejectsTraversal(t *testing.T) {
	root := tempdir.TempDir(t)
	for _, inner := range []string{
		"../outside.bin",
		"ffx_data/../../outside.bin",
		`C:\outside.bin`,
		"/outside.bin",
	} {
		if _, err := vbfExtractTarget(root, inner, true); err == nil {
			t.Errorf("caminho inseguro aceito: %q", inner)
		}
	}
}

func TestVbfExtracaoDestinoDefaultEData(t *testing.T) {
	vbfPath := seedVbfExtractRoot(t, vbfExtractFixture())
	svc := NewMetadataService(nil)

	// destRoot "" = data/ do jogo (GameFilesRoot/data).
	gameRoot := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = gameRoot
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })

	res, err := svc.ExtractVbfSelection(vbfPath, []string{"ffx_data/gamedata/ps3data/big.bin"}, "")
	if err != nil {
		t.Fatalf("ExtractVbfSelection(default): %v", err)
	}
	if len(res.Done) != 1 {
		t.Fatalf("done = %d (%+v)", len(res.Done), res)
	}
	target := filepath.Join(gameRoot, common.DirData, "ffx_data", "gamedata", "ps3data", "big.bin")
	if _, serr := os.Stat(target); serr != nil {
		t.Errorf("arquivo não foi parar em data/: %s (%v)", target, serr)
	}

	// Preview sem destRoot devolve o mesmo default.
	prev, err := svc.PreviewVbfExtraction(vbfPath, []string{"ffx_data/gamedata/ps3data/big.bin"}, "")
	if err != nil {
		t.Fatalf("PreviewVbfExtraction(default): %v", err)
	}
	want := filepath.Join(gameRoot, common.DirData)
	if prev.DestRoot != want || prev.Existing != 1 {
		t.Errorf("preview default = %+v, esperado dest %s com 1 existente", prev, want)
	}
}

func TestVbfExportSelecaoIgnoraForaDeEscopo(t *testing.T) {
	vbfPath := seedVbfExtractRoot(t, vbfExtractFixture())
	svc := NewMetadataService(nil)

	// Formatos incompatíveis são ignorados silenciosamente numa seleção de texto.
	outputs, err := svc.ExportVbfSelection(vbfPath, "json", []string{"version_config/config.bin"}, nil)
	if err != nil {
		t.Fatalf("arquivos incompatíveis devem ser ignorados: %v", err)
	}
	if len(outputs) != 0 {
		t.Fatalf("não esperava outputs para arquivo incompatível: %v", outputs)
	}
}

func TestVbfExportSelecaoCaminhoDeTexto(t *testing.T) {
	// Integração real: exporta o arquivo de evento carregado DO CONTAINER
	// (não de data/) para o mesmo mods/edits dos exports normais.
	fixtureRoot, err := filepath.Abs(filepath.Join("..", "..", "testData", "FFX", "binary"))
	if err != nil {
		t.Fatal(err)
	}
	gameRoot := tempdir.TempDir(t)
	if err := os.CopyFS(gameRoot, os.DirFS(fixtureRoot)); err != nil {
		t.Fatalf("copiando fixtures: %v", err)
	}
	prevRoot, prevDisable := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = gameRoot
	common.DisableMods = true
	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		common.DisableMods = prevDisable
		vbfSessionResetAll()
	})

	inner := "ffx_ps2/ffx/master/new_uspc/event/obj_ps3/az/azit0000/azit0000.bin"
	eventBytes, err := os.ReadFile(filepath.Join(gameRoot, filepath.FromSlash(inner)))
	if err != nil {
		t.Fatalf("lendo fixture de evento: %v", err)
	}
	vbfPath := seedVbfExtractRoot(t, map[string][]byte{inner: eventBytes})
	svc := NewMetadataService(nil)

	outputs, err := svc.ExportVbfSelection(vbfPath, "json", []string{inner}, nil)
	if err != nil {
		t.Fatalf("ExportVbfSelection: %v", err)
	}
	if len(outputs) == 0 {
		t.Fatal("export não escreveu nenhum artefato")
	}
	for _, output := range outputs {
		if _, err := os.Stat(output); err != nil {
			t.Errorf("artefato não existe: %s (%v)", output, err)
		}
	}
}
