package vbf

import (
	"ffxresources/testData/tempdir"
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- montador de .vbf sintético (escrita só em temp dir de teste) ----

// encodeBlock aplica a MESMA regra do leitor: só comprime quando o resultado
// é estritamente menor e não colide com a detecção "último bloco cru".
func encodeBlock(chunk []byte, compress, last bool) ([]byte, uint16) {
	want := len(chunk)
	if compress {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(chunk); err != nil {
			panic(err)
		}
		if err := zw.Close(); err != nil {
			panic(err)
		}
		n := buf.Len()
		if n <= 0xFFFF && n < want && !(last && n == want) {
			return buf.Bytes(), uint16(n)
		}
	}
	if want == blockSize {
		return chunk, 0
	}
	return chunk, uint16(want)
}

// buildVBF monta um .vbf válido com os arquivos pedidos. compress=false
// força blocos crus (só então o caminho raw é exercitado).
func buildVBF(t *testing.T, files map[string][]byte, compress bool) string {
	t.Helper()

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sortStrings(names)

	// Tabela de nomes + offsets.
	var nameTab bytes.Buffer
	nameOffset := make(map[string]uint64, len(names))
	for _, n := range names {
		nameOffset[n] = uint64(nameTab.Len())
		nameTab.WriteString(n)
		nameTab.WriteByte(0)
	}

	// Blocos, na ordem em que serão gravados.
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
		e := ent{name: n, size: uint64(len(files[n])), start: blockIx}
		data := files[n]
		for pos := 0; pos < len(data); {
			end := pos + blockSize
			if end > len(data) {
				end = len(data)
			}
			stored, sz := encodeBlock(data[pos:end], compress, end == len(data))
			blockSizes = append(blockSizes, sz)
			chunks = append(chunks, stored)
			blockIx++
			pos = end
		}
		ents = append(ents, e)
	}

	nameTableLen := 4 + nameTab.Len()
	headerLen := 16 + len(names)*48 + nameTableLen + len(blockSizes)*2

	// Offsets absolutos do payload (vem logo após o cabeçalho).
	payloadOff := make([]uint64, len(chunks))
	off := uint64(headerLen)
	for i, c := range chunks {
		payloadOff[i] = off
		off += uint64(len(c))
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, Magic)
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
	if out.Len() != headerLen {
		t.Fatalf("montagem: headerLen calculado %d mas escrito %d", headerLen, out.Len())
	}
	// O MD5 do cabeçalho é o trailer dos ÚLTIMOS 16 bytes do arquivo
	// (payload antes dele) — é exatamente de onde o leitor de referência
	// o lê.
	for _, c := range chunks {
		out.Write(c)
	}
	sum := md5.Sum(out.Bytes()[:headerLen])
	out.Write(sum[:])

	path := filepath.Join(tempdir.TempDir(t), "test.vbf")
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---- testes ----

func fixture() map[string][]byte {
	big := bytes.Repeat([]byte("FFX Resources — texto repetido para garantir compressão. "), 6000)
	return map[string][]byte{
		"ffx_ps2/ffx/master/new_uspc/event/obj_ps3/az/azit0000/azit0000.bin": bytes.Repeat([]byte("A"), 100),
		"ffx_ps2/ffx/master/new_uspc/menu/macrodic.dcp":                      big[:200000],
		"ffx_data/gamedata/ps3data/zzz/full.bin":                             bytes.Repeat([]byte("F"), blockSize),
		"ffx_data/gamedata/ps3data/zzz/one_more.bin":                         bytes.Repeat([]byte("G"), blockSize+1),
		"ffx_data/gamedata/ps3data/empty.bin":                                {},
		"ffx_data/gamedata/ps3data/a_dir/deep/leaf.txt":                      []byte("leaf"),
		"ffx_data/gamedata/ps3data/a_dir/other.txt":                          []byte("other"),
	}
}

func openFixture(t *testing.T, compress bool) *Archive {
	t.Helper()
	a, err := Open(buildVBF(t, fixture(), compress))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func TestReadRoundTripRaw(t *testing.T) {
	assertRoundTrip(t, openFixture(t, false))
}

func TestReadRoundTripCompressed(t *testing.T) {
	assertRoundTrip(t, openFixture(t, true))
}

func assertRoundTrip(t *testing.T, a *Archive) {
	t.Helper()
	if a.Len() != len(fixture()) {
		t.Fatalf("Len = %d, esperado %d", a.Len(), len(fixture()))
	}
	for name, want := range fixture() {
		got, err := a.Read(name)
		if err != nil {
			t.Errorf("Read(%s): %v", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Read(%s): %d bytes, esperado %d (igual=%v)",
				name, len(got), len(want), bytes.Equal(got, want))
		}
	}
	// Lookup case-insensitive e com separador trocado.
	if _, ok := a.Entry("FFX_DATA/GAMEDATA/PS3DATA/EMPTY.BIN"); !ok {
		t.Error("lookup case-insensitive falhou")
	}
	if _, ok := a.Entry("nao/existe.bin"); ok {
		t.Error("entry inexistente devolveu ok")
	}
}

func TestListDiretoriosEDecompressao(t *testing.T) {
	a := openFixture(t, true)

	root := a.List("")
	if len(root) != 2 {
		t.Fatalf("raiz: %d filhos, esperado 2 (%+v)", len(root), root)
	}
	for i, want := range []string{"ffx_data", "ffx_ps2"} {
		if root[i].Name != want || !root[i].IsDir {
			t.Errorf("raiz[%d] = %+v, esperado dir %s", i, root[i], want)
		}
	}

	gamedata := a.List("ffx_data/gamedata/ps3data")
	if len(gamedata) != 3 {
		t.Fatalf("ps3data: %d filhos, esperado 3 (%+v)", len(gamedata), gamedata)
	}
	if !gamedata[0].IsDir || gamedata[0].Name != "a_dir" {
		t.Errorf("primeiro filho deveria ser o diretório a_dir: %+v", gamedata[0])
	}
	if !gamedata[1].IsDir || gamedata[1].Name != "zzz" {
		t.Errorf("segundo filho deveria ser o diretório zzz: %+v", gamedata[1])
	}
	if gamedata[2].IsDir || gamedata[2].Name != "empty.bin" {
		t.Errorf("terceiro filho deveria ser empty.bin: %+v", gamedata[2])
	}

	deep := a.List("ffx_data/gamedata/ps3data/a_dir")
	if len(deep) != 2 {
		t.Fatalf("a_dir: %d filhos, esperado 2 (%+v)", len(deep), deep)
	}
	if !deep[0].IsDir || deep[0].Name != "deep" {
		t.Errorf("deep deveria vir primeiro: %+v", deep[0])
	}
}

func TestErrNotVBF(t *testing.T) {
	p := filepath.Join(tempdir.TempDir(t), "nao.vbf")
	if err := os.WriteFile(p, bytes.Repeat([]byte("x"), 128), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(p); !errors.Is(err, ErrNotVBF) {
		t.Errorf("erro = %v, esperado ErrNotVBF", err)
	}
}

func TestErrHeaderMD5Invalido(t *testing.T) {
	p := buildVBF(t, fixture(), true)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Corrompe um byte do índice (o MD5 do trailer deixa de bater).
	raw[64] ^= 0xFF
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(p); !errors.Is(err, ErrCorrupt) {
		t.Errorf("erro = %v, esperado ErrCorrupt", err)
	}
}

func TestErrNotFound(t *testing.T) {
	a := openFixture(t, true)
	if _, err := a.Read("caminho/que/nao/existe.bin"); !errors.Is(err, ErrNotFound) {
		t.Errorf("erro = %v, esperado ErrNotFound", err)
	}
}

func TestFilesUnder(t *testing.T) {
	a := openFixture(t, true)
	if !a.IsDir("ffx_data/gamedata/ps3data/a_dir") {
		t.Error("IsDir não reconheceu diretório")
	}
	if a.IsDir("ffx_data/gamedata/ps3data/a_dir/other.txt") {
		t.Error("IsDir reconheceu arquivo como diretório")
	}

	// Raiz ("" = tudo).
	if got := len(a.FilesUnder("")); got != len(fixture()) {
		t.Errorf("FilesUnder(\"\") = %d, esperado %d", got, len(fixture()))
	}

	// Diretório: só o que está ABAIXO do prefixo (o próprio dir não conta).
	aDir := a.FilesUnder("ffx_data/gamedata/ps3data/a_dir")
	if len(aDir) != 2 {
		t.Fatalf("a_dir: %d entries, esperado 2", len(aDir))
	}
	want := map[string]bool{
		"ffx_data/gamedata/ps3data/a_dir/deep/leaf.txt": true,
		"ffx_data/gamedata/ps3data/a_dir/other.txt":     true,
	}
	for _, e := range aDir {
		if !want[e.Path] {
			t.Errorf("entry inesperada: %s", e.Path)
		}
	}

	// Case-insensitive e separador do Windows.
	if got := len(a.FilesUnder(`FFX_DATA\Gamedata\PS3DATA\A_DIR`)); got != 2 {
		t.Errorf("FilesUnder com caixa/separador misto = %d, esperado 2", got)
	}

	// Diretório inexistente: nada.
	if got := a.FilesUnder("nao/existe"); len(got) != 0 {
		t.Errorf("FilesUnder inexistente = %d entries, esperado 0", len(got))
	}
}

func TestExtractEntryRoundTrip(t *testing.T) {
	// Os DOIS caminhos de bloco (cru e compactado) — a extração tem as
	// mesmas regras de detecção do ReadEntry.
	for _, compress := range []bool{false, true} {
		a := openFixture(t, compress)
		dest := tempdir.TempDir(t)
		for name, want := range fixture() {
			e, ok := a.Entry(name)
			if !ok {
				t.Fatalf("Entry(%s): ausente no índice", name)
			}
			target := filepath.Join(dest, filepath.FromSlash(e.Path))
			if err := a.ExtractEntry(e, target); err != nil {
				t.Fatalf("ExtractEntry(%s): %v", name, err)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("lendo %s: %v", target, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("ExtractEntry(%s): %d bytes, esperado %d",
					name, len(got), len(want))
			}
		}
	}
}

func TestExtractEntryGrandeSemLimite(t *testing.T) {
	// Arquivo que DECLARA mais que MaxDecodeBytes: a extração NÃO recusa
	// (o cap é só do decode em memória) — mas o tamanho declarado é
	// irreal para montar aqui, então basta conferir que o erro NÃO é
	// ErrTooLarge (falha depois, no payload inexistente).
	files := map[string][]byte{"grande.bin": {}}
	p := buildVBFWithDeclaredSize(t, files, MaxDecodeBytes+1)
	a, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	e, ok := a.Entry("grande.bin")
	if !ok {
		t.Fatal("entry ausente")
	}
	err = a.ExtractEntry(e, filepath.Join(tempdir.TempDir(t), "grande.bin"))
	if errors.Is(err, ErrTooLarge) {
		t.Errorf("extração recusou com ErrTooLarge: %v", err)
	}
}

func TestExtractEntryNaoDeixaTmp(t *testing.T) {
	a := openFixture(t, true)
	// Entry apontando para bloco fora do índice: a extração falha no meio
	// e o .tmp tem que sumir (nada de arquivo meiado no destino).
	e, ok := a.Entry("ffx_ps2/ffx/master/new_uspc/menu/macrodic.dcp")
	if !ok {
		t.Fatal("entry ausente")
	}
	e.StartBlock = uint32(len(a.blocks)) + 10
	dest := filepath.Join(tempdir.TempDir(t), "fora.bin")
	if err := a.ExtractEntry(e, dest); err == nil {
		t.Fatal("esperado erro de bloco fora do índice")
	}
	if _, serr := os.Stat(dest); serr == nil {
		t.Error("destino existe apesar do erro")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".vbf-extract-*.tmp"))
	if len(matches) > 0 {
		t.Errorf("tmp deixado para trás: %v", matches)
	}
}

func TestErrTooLarge(t *testing.T) {
	// Entry que DECLARA mais que MaxDecodeBytes: o leitor recusa antes de
	// tocar no payload (nada de alocar 64 MiB num teste).
	files := map[string][]byte{"grande.bin": {}}
	p := buildVBFWithDeclaredSize(t, files, MaxDecodeBytes+1)
	a, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if _, err := a.Read("grande.bin"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("erro = %v, esperado ErrTooLarge", err)
	}
}

// TestOpenRealVBF abre os .vbf REAIS do jogo (quando presentes na máquina)
// para confirmar que índice, nomes, blocos e MD5 batem com o formato de
// verdade — um teste sintético sozinho não prova isso.
func TestOpenRealVBF(t *testing.T) {
	base := `D:\Steam\steamapps\common\FINAL FANTASY FFX&FFX-2 HD Remaster\data`
	for _, name := range []string{"FFX_Data.vbf", "FFX2_Data.vbf"} {
		p := filepath.Join(base, name)
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s ausente", p)
		}
		a, err := Open(p)
		if err != nil {
			t.Fatalf("%s: Open: %v", name, err)
		}
		if a.Len() == 0 {
			t.Errorf("%s: índice vazio", name)
		}
		// Paths conhecidos do remaster (batem com a árvore data/ extraída).
		var sample string
		for _, c := range a.Paths() {
			if strings.HasSuffix(c, ".dds.phyre") {
				sample = c
				break
			}
		}
		if sample == "" {
			t.Errorf("%s: nenhum .dds.phyre no índice", name)
		} else if raw, rerr := a.Read(sample); rerr != nil {
			t.Errorf("%s: Read(%s): %v", name, sample, rerr)
		} else if len(raw) == 0 {
			t.Errorf("%s: %s devolveu vazio", name, sample)
		}
		_ = a.Close()
	}
}

// buildVBFWithDeclaredSize monta um .vbf com UMA entry cujo tamanho declarado
// não corresponde aos bytes gravados (só a estrutura interessa ao teste).
func buildVBFWithDeclaredSize(t *testing.T, files map[string][]byte, declared uint64) string {
	t.Helper()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sortStrings(names)

	var nameTab bytes.Buffer
	for _, n := range names {
		nameTab.WriteString(n)
		nameTab.WriteByte(0)
	}
	blockCount := blocksFor(declared)
	nameTableLen := 4 + nameTab.Len()
	headerLen := uint64(16) + uint64(len(names))*48 + uint64(nameTableLen) + uint64(blockCount)*2

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, Magic)
	_ = binary.Write(&out, binary.LittleEndian, uint32(headerLen))
	_ = binary.Write(&out, binary.LittleEndian, uint64(len(names)))
	for i := 0; i < len(names); i++ {
		sum := md5.Sum([]byte(strings.ToLower(names[i])))
		out.Write(sum[:])
	}
	for i, n := range names {
		_ = binary.Write(&out, binary.LittleEndian, uint32(0)) // startBlock
		_ = binary.Write(&out, binary.LittleEndian, uint32(0))
		_ = binary.Write(&out, binary.LittleEndian, declared)
		_ = binary.Write(&out, binary.LittleEndian, headerLen) // offset fora do arquivo
		_ = binary.Write(&out, binary.LittleEndian, uint64(bytes.Index(nameTab.Bytes(), append([]byte(n), 0))))
		_ = i
	}
	_ = binary.Write(&out, binary.LittleEndian, uint32(nameTableLen))
	out.Write(nameTab.Bytes())
	for i := 0; i < blockCount; i++ {
		out.Write([]byte{0, 0})
	}
	sum := md5.Sum(out.Bytes())
	out.Write(sum[:])
	_ = sum

	p := filepath.Join(tempdir.TempDir(t), "declarado.vbf")
	if err := os.WriteFile(p, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
