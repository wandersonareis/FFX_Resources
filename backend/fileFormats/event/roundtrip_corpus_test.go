// E2E do fluxo IO com o corpus LOCAL: busca em diretório pelos binários do
// codec events (stride 8) disponíveis em testData, por versão (FFX, FFX-2) e
// por localização (us, jp, ...) — o que estiver disponível, testa.
//
// Os binários do jogo são zona cinza e NÃO são commitados: neste ambiente sem
// a árvore, o teste pula (padrão do repo — "ambiente sem o jogo extraído").
// A garantia permanente do codec, independente de corpus, vem dos binários
// sintéticos (synthetic_roundtrip_test.go); este teste adiciona os casos
// reais — inclusive particularidades encontradas em traduções de terceiros
// que o dono do ambiente deixar na árvore.
//
// Por arquivo, a tríplice de asserção:
//
//  1. fidelidade por string — encode(decode(bytes)) == bytes nos dois lados
//     (regular e simplified), com o charset da localização do arquivo;
//  2. round-trip SEMÂNTICO — parse → EncodeLocalizedStrings (o writer real do
//     IO) → re-parse devolve as MESMAS strings, na mesma ordem;
//  3. ponto fixo — o arquivo que o writer gravou, relido e regravado, sai
//     byte a byte igual.
//
// NÃO se exige que o arquivo regravado seja idêntico ao original: o formato é
// uma gambiarra de ponteiros do PS2 e as otimizações de layout do writer
// (offsets compartilhados no dedupe de armazenamento) são aceitáveis. A
// garantia é sobre o CONTEÚDO — o resto se valida jogando.
package event_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/core/reader"
	"ffxresources/backend/fileFormats/event"
	testcommon "ffxresources/testData"
)

// codecDirs são os diretórios (sob a raiz de localização) lidos pelo codec
// events: eventos (event/), battle/btl (battletext, menumain, tuto0000),
// cloudsave e menu (menumain.bin, tutorial.msb). Fora deles — kernel,
// macrodic, encoding — é outro leitor, outro teste.
var codecDirs = []string{"event", filepath.Join("battle", "btl"), "cloudsave", "menu"}

type corpusArquivo struct {
	path    string
	loc     string
	charset string
	version common.GameVersion
	label   string
}

func TestCodecRoundTripCorpusDisponivel(t *testing.T) {
	repoRoot := filepath.Dir(testcommon.GetTestDataRootDirectory())
	arvores := []struct {
		version common.GameVersion
		dir     string
	}{
		{common.GameVersionFFX, "FFX"},
		{common.GameVersionFFX2, "FFX-2"},
	}

	testados := 0
	for _, arvore := range arvores {
		binRoot := filepath.Join(repoRoot, "testData", arvore.dir, "binary")
		if _, err := os.Stat(binRoot); err != nil {
			continue // árvore ausente: o que estiver disponível, testa
		}
		arquivos := coletarCorpus(t, binRoot, arvore.version)
		if len(arquivos) == 0 {
			continue
		}

		// Internals como o app: charsets embutidos + macros da própria
		// árvore (rows com {MCR} só re-codificam com a store carregada).
		prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
		common.GameFilesRoot, common.DisableMods = binRoot, true
		t.Cleanup(func() {
			common.GameFilesRoot, common.DisableMods = prevRoot, prevMods
		})
		if err := ffxencoding.PrepareVersionCharsets(arvore.version); err != nil {
			t.Fatalf("%s: charsets: %v", arvore.dir, err)
		}
		if err := reader.PrepareVersion(arvore.version); err != nil {
			t.Fatalf("%s: internals (macros): %v", arvore.dir, err)
		}

		for _, arq := range arquivos {
			arq := arq
			t.Run(arq.label, func(t *testing.T) {
				roundTripCorpusArquivo(t, arq)
			})
			testados++
		}
	}
	if testados == 0 {
		t.Skip("nenhum binário do codec events disponível em testData (binários do jogo não são commitados)")
	}
	t.Logf("corpus: %d arquivo(s) testado(s)", testados)
}

// coletarCorpus varre os diretórios do codec por localização presente na
// árvore, com o charset de cada uma (new_jppc lê com a tabela jp).
func coletarCorpus(t *testing.T, binRoot string, version common.GameVersion) []corpusArquivo {
	t.Helper()
	var out []corpusArquivo
	for _, loc := range common.SupportedLanguageCodes() {
		locRoot := filepath.Join(binRoot, filepath.FromSlash(common.GetLocalizationRootForVersion(version, loc)))
		if _, err := os.Stat(locRoot); err != nil {
			continue
		}
		charset := ffxencoding.GetCharsetForLanguage(loc)
		for _, dir := range codecDirs {
			root := filepath.Join(locRoot, filepath.FromSlash(dir))
			filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err // raiz ausente: WalkDir já devolve nil no primeiro erro de raiz inexistente
				}
				if d.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if ext != ".bin" && ext != ".msb" {
					return nil
				}
				rel, _ := filepath.Rel(binRoot, path)
				out = append(out, corpusArquivo{
					path:    path,
					loc:     loc,
					charset: charset,
					version: version,
					label:   filepath.ToSlash(rel),
				})
				return nil
			})
		}
	}
	return out
}

func roundTripCorpusArquivo(t *testing.T, arq corpusArquivo) {
	raw, err := os.ReadFile(arq.path)
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	rows, err := event.FromFieldStringData(raw, arq.charset, arq.version)
	if err != nil {
		t.Skipf("fora do codec events: %v", err)
	}
	if len(rows) == 0 {
		t.Skip("tabela sem rows")
	}

	// Os ponteiros do formato são uint16: arquivo cujo conteúdo não cabe no
	// espaço de endereçamento está fora do codec por construção — o jogo não
	// consegue endereçar strings além de 0xFFFF, e o round-trip aqui não
	// tem o que validar (ex.: crcr0000 do FFX-2, conhecidamente quebrado).
	if contentOffset := len(rows) * 8; len(raw)-contentOffset > 0xFFFF-contentOffset {
		t.Skipf("conteúdo além do limite uint16 dos ponteiros (%d bytes)", len(raw))
	}

	// (1) fidelidade por string: encode(decode(bytes)) == bytes.
	for i, f := range rows {
		if f == nil {
			t.Fatalf("row %d nil", i)
		}
		if enc, err := converter.StringToBytes(f.GetRegularString(), arq.charset, arq.version); err != nil {
			t.Errorf("row %d: encode regular: %v (%q)", i, err, f.GetRegularString())
		} else if !bytes.Equal(enc, f.RegularBytes) {
			t.Errorf("row %d: encode(decode(bytes)) != bytes (regular): %q", i, f.GetRegularString())
		}
		if enc, err := converter.StringToBytes(f.GetSimplifiedString(), arq.charset, arq.version); err != nil {
			t.Errorf("row %d: encode simplified: %v (%q)", i, err, f.GetSimplifiedString())
		} else if !bytes.Equal(enc, f.SimplifiedBytes) {
			t.Errorf("row %d: encode(decode(bytes)) != bytes (simplified): %q", i, f.GetSimplifiedString())
		}
	}

	// (2) round-trip semântico pelo writer real do IO: as strings que o
	// jogo lê não mudam.
	locals := make([]*event.LocalizedFieldStringObject, 0, len(rows))
	for _, f := range rows {
		locals = append(locals, event.NewLocalizedFieldStringObjectWithContent(arq.loc, f))
	}
	enc, err := event.EncodeLocalizedStrings(locals, arq.loc, arq.version)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	rows2, err := event.FromFieldStringData(enc, arq.charset, arq.version)
	if err != nil {
		t.Fatalf("releitura do regravado: %v", err)
	}
	if len(rows) != len(rows2) {
		t.Fatalf("contagem de rows mudou: %d -> %d", len(rows), len(rows2))
	}
	for i := range rows {
		if rows[i].GetRegularString() != rows2[i].GetRegularString() {
			t.Errorf("row %d: regular mudou no round-trip:\n antes: %q\n depois: %q",
				i, rows[i].GetRegularString(), rows2[i].GetRegularString())
		}
		if rows[i].GetSimplifiedString() != rows2[i].GetSimplifiedString() {
			t.Errorf("row %d: simplified mudou no round-trip:\n antes: %q\n depois: %q",
				i, rows[i].GetSimplifiedString(), rows2[i].GetSimplifiedString())
		}
	}

	// (3) ponto fixo: regravar o regravado sai byte a byte igual.
	locals2 := make([]*event.LocalizedFieldStringObject, 0, len(rows2))
	for _, f := range rows2 {
		locals2 = append(locals2, event.NewLocalizedFieldStringObjectWithContent(arq.loc, f))
	}
	enc2, err := event.EncodeLocalizedStrings(locals2, arq.loc, arq.version)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(enc, enc2) {
		i := 0
		for i < len(enc) && i < len(enc2) && enc[i] == enc2[i] {
			i++
		}
		t.Errorf("ponto fixo violado: 1º save=%d bytes, 2º save=%d, 1º desvio em %#x", len(enc), len(enc2), i)
	}
}
