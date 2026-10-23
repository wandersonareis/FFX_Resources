package reader_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/components"
	"ffxresources/backend/core/converter"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/core/reader"
	"ffxresources/backend/datastore"
	"ffxresources/backend/dto"
	strfmt "ffxresources/backend/formatters/strings"
	testcommon "ffxresources/testData"
)

// Testes de decode com o encoding REAL do jogo: carregam os charsets embutidos
// e os macrodic.dcp via reader.InitializeInternals — o mesmo caminho da
// aplicação — e conferem o resultado de getStringAtLookupOffsetBinary sobre a
// tabela us/default/jp e sobre os macros publicados no datastore.
//
// Os testes simples (charset sintético, um guard por caso) ficam em
// backend/core/converter/string_bytes_test.go.

// setupGameInternals inicializa charsets + macrodic a partir de testData e
// restaura o estado global no fim (o suite Ginkgo do pacote faz o mesmo).
func setupGameInternals(t *testing.T) {
	t.Helper()

	root := testcommon.GetTestDataRootDirectory()
	gameRoot := filepath.Join(root, "FFX", "binary")
	if _, err := os.Stat(gameRoot); err != nil {
		t.Skipf("testData/FFX/binary indisponível: %v", err)
	}

	prevRoot := common.GameFilesRoot
	prevDisableMods := common.DisableMods
	prevKeyItems := datastore.KeyItems

	common.SetGameFilesRoot(gameRoot)
	common.DisableMods = true
	datastore.KeyItems = components.NewEmptyList[datastore.IGlobalLocalizedTextObject]()

	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		common.DisableMods = prevDisableMods
		datastore.KeyItems = prevKeyItems
		ffxencoding.ClearAllCharMaps()
		datastore.Instance.ClearAllMacros()
	})

	if err := reader.InitializeInternals(common.GameVersionFFX); err != nil {
		t.Fatalf("InitializeInternals: %v", err)
	}
}

// TestGameEncodingUsUsesTildeNotTrema fixa a regra da fonte us: os slots que
// no font original eram ä/ö/Ä/Ö foram trocados por ã/õ/Ã/Õ (o font us não tem
// espaço para o trema, que não é usado em português). O charset default
// (sp/fr/de/it) mantém o trema do font original — os mesmos bytes decodificam
// diferente por charset.
func TestGameEncodingUsUsesTildeNotTrema(t *testing.T) {
	setupGameInternals(t)

	us := ffxencoding.GetByteToCharMap(common.GameVersionFFX, "us")
	def := ffxencoding.GetByteToCharMap(common.GameVersionFFX, "default")
	if us == nil || def == nil {
		t.Fatal("charsets us/default não carregados")
	}

	for _, trema := range []rune{'ä', 'ö', 'Ä', 'Ö'} {
		for slot, got := range us {
			if got == trema {
				t.Fatalf("charset us não deve conter trema: rune %q no slot 0x%02X", trema, slot)
			}
		}
	}

	usSlots := map[uint]rune{0xA6: 'Ã', 0xBD: 'ã', 0xB4: 'Õ', 0xCB: 'õ'}
	for slot, want := range usSlots {
		if got := us[slot]; got != want {
			t.Errorf("us[0x%02X] = %q, want %q", slot, got, want)
		}
	}

	defSlots := map[uint]rune{0xA6: 'Ä', 0xBD: 'ä', 0xB4: 'Ö', 0xCB: 'ö'}
	for slot, want := range defSlots {
		if got := def[slot]; got != want {
			t.Errorf("default[0x%02X] = %q, want %q", slot, got, want)
		}
	}

	// Mesmos bytes, charsets diferentes: us → til, default/de → trema.
	raw := []byte{0xA6, 0xBD, 0xB4, 0xCB}
	if got, want := converter.BytesToString(raw, "us", common.GameVersionFFX), "ÃãÕõ"; got != want {
		t.Fatalf("decode us: got %q, want %q", got, want)
	}
	if got, want := converter.BytesToString(raw, "default", common.GameVersionFFX), "ÄäÖö"; got != want {
		t.Fatalf("decode default: got %q, want %q", got, want)
	}
	// "de" é resolvido para o bucket default (preserva o trema do font).
	if got, want := converter.BytesToString(raw, "de", common.GameVersionFFX), "ÄäÖö"; got != want {
		t.Fatalf("decode de: got %q, want %q", got, want)
	}

	// O encode é o inverso: ã/õ no us e ä/ö no default caem nos mesmos slots.
	for _, tc := range []struct {
		text    string
		charset string
	}{
		{"ÃãÕõ", "us"},
		{"ÄäÖö", "default"},
	} {
		encoded, err := converter.StringToBytes(tc.text, tc.charset, common.GameVersionFFX)
		if err != nil {
			t.Fatalf("encode %q [%s]: %v", tc.text, tc.charset, err)
		}
		if !bytes.Equal(encoded, raw) {
			t.Fatalf("encode %q [%s] = % X, want % X", tc.text, tc.charset, encoded, raw)
		}
	}
}

// TestGameEncodingRoundTripWithRealTables usa os slots reais para conferir que
// textos comuns (acentos + tags) sobrevivem ao par encode/decode.
func TestGameEncodingRoundTripWithRealTables(t *testing.T) {
	setupGameInternals(t)

	cases := []struct {
		name    string
		text    string
		charset string
	}{
		{"acentos us", "Cid's Ãã", "us"},
		{"português us", "Ação não é opção", "us"},
		{"trema default", "ÄäÖö", "default"},
		{"japonês", "こんにちは", "jp"},
		{"PC + newline", "{PC:00:TIDUS}{TEXT_NEWLINE}Hello", "us"},
		{"cores", "{TIME:00}{CLR:WHITE}Text{CLR:RED}Red{CLR:WHITE}", "us"},
		{"ícone", "{ICON:80:Red Gate}", "us"},
		{"op desconhecido", "{UNKOP:28:30}", "us"},
		{"hex", "{HEX:0D}", "us"},
		{"op 0x24", "{UNKOP:24:31}", "us"},
		{"key sem catálogo", "{KEY:01}", "us"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := converter.StringToBytes(tc.text, tc.charset, common.GameVersionFFX)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if got := converter.BytesToString(encoded, tc.charset, common.GameVersionFFX); got != tc.text {
				t.Fatalf("round-trip: got %q, want %q (bytes % X)", got, tc.text, encoded)
			}
		})
	}
}

// TestGameEncodingMacrodic carrega os macrodic.dcp de testData e confere o
// conteúdo real injetado nos tokens {MCR:sXX:lYY:...}. s06:l40 = índice 0x640
// e s06:l29 = 0x629 na tabela us do testData.
func TestGameEncodingMacrodic(t *testing.T) {
	setupGameInternals(t)

	if count := datastore.GetMacros(common.GameVersionFFX).Count(); count == 0 {
		t.Fatal("macros não publicados pelo InitializeInternals")
	}

	cases := []struct {
		name    string
		raw     []byte
		charset string
		want    string
	}{
		{"macro us s06:l40", []byte{0x19, 0x70}, "us", `{MCR:s06:l40:"Sin"}`},
		{"macro us s06:l29", []byte{0x19, 0x59}, "us", `{MCR:s06:l29:"Cid"}`},
		// O dicionário jp de testData não tem o índice 0x640; o token continua
		// com o marcador de ausência (HasMacros é global da versão).
		{"macro jp ausente", []byte{0x19, 0x70}, "jp", `{MCR:s06:l40:<Missing>}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := converter.BytesToString(tc.raw, tc.charset, common.GameVersionFFX); got != tc.want {
				t.Fatalf("decode % X [%s]: got %q, want %q", tc.raw, tc.charset, got, tc.want)
			}
		})
	}

	// O caminho inverso: o token com o conteúdo real re-codifica para os bytes
	// do MCR (ParseCommand usa só a seção/linha do prefixo).
	encoded, err := converter.StringToBytes(`{MCR:s06:l40:"Sin"}`, "us", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if want := []byte{0x19, 0x70}; !bytes.Equal(encoded, want) {
		t.Fatalf("encode MCR = % X, want % X", encoded, want)
	}
	if got := converter.BytesToString(encoded, "us", common.GameVersionFFX); got != `{MCR:s06:l40:"Sin"}` {
		t.Fatalf("round-trip MCR: got %q", got)
	}
}

// TestDecodeExtractedStringsCorpus roda o decode sobre os .strings extraídos
// com o algoritmo atual (build/bin/data/mods/edits/all), se existirem: é a
// verificação mais próxima de "o texto extraído continua o mesmo" sem os
// binários originais. O teste pula quando o diretório não existe (artefato
// gitignored) e não roda em -short.
func TestDecodeExtractedStringsCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("corpus de .strings pulado em -short")
	}
	setupGameInternals(t)

	repoRoot := filepath.Dir(testcommon.GetTestDataRootDirectory())
	dir := filepath.Join(repoRoot, "build", "bin", "data", "mods", "edits", "all")
	files, err := filepath.Glob(filepath.Join(dir, "*.strings"))
	if err != nil || len(files) == 0 {
		t.Skipf("corpus indisponível em %s", dir)
	}

	const maxSamples = 8
	var (
		checked  int
		mismatch int
		samples  []string
		skipped  = map[string]int{}
	)
	for _, file := range files {
		base := filepath.Base(file)
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ler %s: %v", base, err)
		}
		collection, err := strfmt.Unmarshal(data)
		if err != nil {
			t.Fatalf("parse %s: %v", base, err)
		}
		for _, entry := range collection {
			version, ok := versionFromKey(entry.Metadata.Key)
			if !ok || version != common.GameVersionFFX {
				continue // este corpus é ffx
			}
			for _, row := range entry.Rows {
				for lang, text := range row.Text {
					if text == "" || strings.HasPrefix(text, "$") {
						continue
					}
					charset := ffxencoding.GetCharsetForLanguage(lang)
					if reason := unstableForRoundTrip(text, charset, false); reason != "" {
						skipped[reason]++
						continue
					}

					checked++
					encoded, err := converter.StringToBytes(text, charset, common.GameVersionFFX)
					if err != nil {
						mismatch++
						if len(samples) < maxSamples {
							samples = append(samples, base+" encode: "+err.Error())
						}
						continue
					}
					got := converter.BytesToString(encoded, charset, common.GameVersionFFX)
					if normalizeMacros(got) != normalizeMacros(text) {
						mismatch++
						if len(samples) < maxSamples {
							samples = append(samples, base+" "+entry.Metadata.ID+"["+strconv.Itoa(row.Index)+"] "+lang+
								"\n  want="+clip(text)+"\n  got ="+clip(got))
						}
					}
				}
			}
		}
	}

	t.Logf("corpus: %d linhas conferidas, %d divergentes, ignoradas: %v", checked, mismatch, skipped)
	if checked == 0 {
		t.Fatal("corpus sem linhas conferidas (formato inesperado)")
	}
	if mismatch > 0 {
		t.Fatalf("%d linhas divergentes; amostras:\n%s", mismatch, strings.Join(samples, "\n"))
	}
}

// TestOriginalsExtractImportExtractRoundTrip confere o ciclo completo do
// usuário sobre os ORIGINAIS (build/bin/data/mods/edits, exportado pelo e2e
// com mods desativados e pelo decoder atual): extrair → importar
// (StringToBytes) → extrair de novo (BytesToString). Toda linha, de todo
// idioma, deve voltar texto idêntico — em particular jp/ch/kr, cujo
// re-encode multi-byte (indexBytes: seções 0x2B..0x2F e prefixo 0x04) já
// foi quebrado no passado.
//
// O payload do {MCR:...} é da fonte que gerou o dicionário (não do binário)
// e o ParseCommand o ignora no import; ele é removido antes da comparação
// para não depender do datastore carregado no teste.
func TestOriginalsExtractImportExtractRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("corpus de .strings pulado em -short")
	}
	setupGameInternals(t)

	repoRoot := filepath.Dir(testcommon.GetTestDataRootDirectory())
	dir := filepath.Join(repoRoot, "build", "bin", "data", "mods", "edits")
	files, err := filepath.Glob(filepath.Join(dir, "*.strings"))
	if err != nil || len(files) == 0 {
		t.Skipf("corpus indisponível em %s", dir)
	}

	const maxSamples = 8
	var (
		checked     int
		payloadCut  int
		mismatch    int
		misSamples  []string
		skipSamples = map[string][]string{}
		perLang     = map[string]map[string]int{} // lang -> {checked,mismatch,skip:<reason>}
	)
	noteSkip := func(reason, format string, args ...any) {
		if len(skipSamples[reason]) < 2 {
			skipSamples[reason] = append(skipSamples[reason], fmt.Sprintf(format, args...))
		}
	}
	noteMis := func(format string, args ...any) {
		if len(misSamples) < maxSamples {
			misSamples = append(misSamples, fmt.Sprintf(format, args...))
		}
	}
	for _, file := range files {
		base := filepath.Base(file)
		if strings.Contains(base, "loc_kit") {
			// loc kit é texto PLANO (registros CRLF, sem encoding do jogo):
			// o import aplica rec.SetText direto — o ciclo extrair→importar→
			// extrair é trivialmente idêntico e não passa por StringToBytes.
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ler %s: %v", base, err)
		}
		collection, err := strfmt.Unmarshal(data)
		if err != nil {
			t.Fatalf("parse %s: %v", base, err)
		}
		for _, entry := range collection {
			version, ok := versionFromKey(entry.Metadata.Key)
			if !ok || version != common.GameVersionFFX {
				continue // este corpus é ffx
			}
			for _, row := range entry.Rows {
				for lang, raw := range row.Text {
					if raw == "" || strings.HasPrefix(raw, "$") {
						continue
					}
					charset := ffxencoding.GetCharsetForLanguage(lang)
					text := mcrToken.ReplaceAllString(raw, "{MCR:s${1}:l${2}}")
					if text != raw {
						payloadCut++
					}
					stats := perLang[lang]
					if stats == nil {
						stats = map[string]int{}
						perLang[lang] = stats
					}
					if reason := unstableForRoundTrip(text, charset, true); reason != "" {
						stats["skip:"+reason]++
						noteSkip(reason, "%s %s[%d] %s: %s", base, entry.Metadata.ID, row.Index, lang, clip(text))
						continue
					}
					stats["checked"]++
					checked++
					encoded, err := converter.StringToBytes(text, charset, common.GameVersionFFX)
					if err != nil {
						stats["mismatch"]++
						mismatch++
						noteMis("%s %s[%d] %s encode: %v", base, entry.Metadata.ID, row.Index, lang, err)
						continue
					}
					got := converter.BytesToString(encoded, charset, common.GameVersionFFX)
					// O re-decode re-apênde o payload do MCR (dicionário do
					// datastore do teste); tira dos dois lados antes de comparar.
					if mcrToken.ReplaceAllString(got, "{MCR:s${1}:l${2}}") != text {
						stats["mismatch"]++
						mismatch++
						noteMis("%s %s[%d] %s\n  want=%s\n  got =%s", base, entry.Metadata.ID, row.Index, lang, clip(text), clip(got))
					}
				}
			}
		}
	}

	langs := make([]string, 0, len(perLang))
	for lang := range perLang {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		stats := perLang[lang]
		t.Logf("%s: conferidas=%d divergentes=%d ignoradas=%d %v",
			lang, stats["checked"], stats["mismatch"], stats["skip:unknown-tag"]+
				stats["skip:unknown-byte-token"]+stats["skip:char-not-in-charset"]+
				stats["skip:newline-literal"]+stats["skip:unterminated-tag"], stats)
	}
	t.Logf("total: %d conferidas, %d divergentes, %d payloads de MCR removidos", checked, mismatch, payloadCut)
	reasons := make([]string, 0, len(skipSamples))
	for reason := range skipSamples {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		t.Logf("amostras skip[%s]:\n%s", reason, strings.Join(skipSamples[reason], "\n"))
	}
	if len(misSamples) > 0 {
		t.Logf("amostras divergencia:\n%s", strings.Join(misSamples, "\n"))
	}
	if checked == 0 {
		t.Fatal("corpus sem linhas conferidas (formato inesperado)")
	}
	if mismatch > 0 {
		t.Fatalf("%d linhas divergentes no ciclo extrair->importar->extrair; amostras:\n%s", mismatch, strings.Join(misSamples, "\n"))
	}
}

var mcrToken = regexp.MustCompile(`\{MCR:s([0-9A-Fa-f]{1,2}):l([0-9A-Fa-f]{1,2})[^}]*\}`)
var cmdOpToken = regexp.MustCompile(`\{CMD:(24|25):([0-9A-Fa-f]{2})\}`)

// normalizeMacros apaga o payload do MCR: o conteúdo depende do dicionário
// carregado no datastore (aqui, o de testData; na extração, o do jogo).
// Também canoniza UNKOP -> UNKDBLCHR: o corpus foi extraído antes da
// reclassificação dos ops de 2 bytes; ambos preservam os mesmos bytes.
func normalizeMacros(s string) string {
	s = strings.ReplaceAll(s, "{UNKOP:", "{UNKDBLCHR:")
	s = cmdOpToken.ReplaceAllStringFunc(s, func(tok string) string {
		m := cmdOpToken.FindStringSubmatch(tok)
		op, _ := strconv.ParseUint(m[1], 16, 8)
		arg, _ := strconv.ParseUint(m[2], 16, 8)
		return fmt.Sprintf("{UNKDBLCHR:%02X:%02X}", op, (arg+0x30)&0xFF)
	})
	return mcrToken.ReplaceAllString(s, "{MCR:s${1}:l${2}}")
}

// unstableForRoundTrip explica por que uma linha extraída não pode ser
// comparada por encode→decode hoje:
//   - "\r"/"\n" crus: o decode emite {TEXT_NEWLINE}, não o newline literal;
//   - tokens "??" ({UNKDBLCHR:xx:??}) quando allowTruncated=false: são
//     artefatos do decoder antigo (op truncado) que o atual emite como
//     {HEX:xx}; com allowTruncated=true os "??" de seção truncada são
//     pontos fixos (parseHexBytes preserva os bytes presentes) e conferem;
//   - tags desconhecidas pelo ParseCommand;
//   - runes fora do charset da localização (texto vindo de outra fonte).
func unstableForRoundTrip(text, charset string, allowTruncated bool) string {
	if strings.ContainsAny(text, "\r\n") {
		return "newline-literal"
	}
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '{' {
			if _, err := converter.CharToBytes(runes[i], charset, common.GameVersionFFX); err != nil {
				return "char-not-in-charset"
			}
			continue
		}
		end := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == '}' {
				end = j
				break
			}
		}
		if end < 0 {
			return "unterminated-tag"
		}
		tag := string(runes[i : end+1])
		if !allowTruncated && strings.Contains(tag, "??") {
			return "unknown-byte-token"
		}
		if converter.ParseCommand(runes, i) == nil {
			return "unknown-tag"
		}
		i = end
	}
	return ""
}

// versionFromKey lê o primeiro segmento da key canônica (ffx/..., ffx2/...).
func versionFromKey(key string) (common.GameVersion, bool) {
	parsed, ok := dto.ParseKey(key)
	if !ok {
		return common.GameVersion{}, false
	}
	version, err := common.ParseGameVersionStrict(parsed.Version)
	if err != nil {
		return common.GameVersion{}, false
	}
	return version, true
}

func clip(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
