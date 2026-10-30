package strings_test

import (
	"reflect"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	"ffxresources/backend/core/encoding"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
	fmtstrings "ffxresources/backend/formatters/strings"
)

func sampleCollection() dto.Collection {
	return dto.Collection{
		"azit0000": {
			Metadata: dto.NewEventMetadata("azit0000", common.GameVersionFFX).WithRowCount(2),
			Rows: []dto.TextRow{
				{
					Index: 0,
					Hash:  hash.Texts(map[string]string{"us": "Where are you?", "sp": "¿Dónde estás?"}),
					Text:  map[string]string{"us": "Where are you?", "sp": "¿Dónde estás?"},
				},
				{
					Index: 1,
					Hash:  hash.Texts(map[string]string{"us": "Where are you?"}),
					Text:  map[string]string{"us": "Where are you?"},
				},
			},
		},
		"command": {
			Metadata: dto.NewObjectMetadata(common.GameVersionFFX, "battle/kernel", "command.bin", "ffx/battle/kernel/command.bin").WithRowCount(2),
			Rows: []dto.TextRow{
				{
					Index: 0, Name: "name",
					Hash: hash.Texts(map[string]string{"us": "Potion"}),
					Text: map[string]string{"us": "Potion"},
				},
				{
					Index: 0, Name: "desc",
					Hash: hash.Texts(map[string]string{"us": "Restores HP"}),
					Text: map[string]string{"us": "Restores HP"},
				},
			},
		},
		"chunk_00": {
			Metadata: dto.NewMacroMetadata(common.GameVersionFFX, 0).WithRowCount(1),
			Rows: []dto.TextRow{
				{
					Index: 0, Name: "name",
					Hash: hash.Texts(map[string]string{"us": "Tom & Jerry"}),
					Text: map[string]string{"us": "Tom & Jerry"},
				},
			},
		},
	}
}

func TestGoldenExampleLines(t *testing.T) {
	// Linhas no estilo do exemplo do usuário: a versão NÃO vai na linha
	// (vive só no header) — parser aceita def, ref e chaves das 3 famílias
	// (hash mismatch aqui só loga).
	raw := "/*key=ffx/event/obj_ps3/az/azit0000/azit0000.bin row_count=309*/\n" +
		"azit0000:0:us║$ba7b73bfc15bb6ea = {PC:04:WAKKA}{TEXT_NEWLINE}{PC:01:YUNA}! Where are you?\n" +
		"azit0000:1:us║$ba7b73bfc15bb6ea = $ba7b73bfc15bb6ea\n" +
		"azit0042:17:us║$ba7b73bfc15bb6ea = $ba7b73bfc15bb6ea\n"
	c, err := fmtstrings.Unmarshal([]byte(raw))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ev, ok := c["azit0000"]
	if !ok || len(ev.Rows) != 2 {
		t.Fatalf("azit0000 rows: %+v", c.SortedKeys())
	}
	if ev.Rows[0].Index != 0 || ev.Rows[1].Index != 1 {
		t.Fatalf("indexes: %+v", ev.Rows)
	}
	if ev.Metadata.Key != "ffx/event/obj_ps3/az/azit0000/azit0000.bin" {
		t.Fatalf("key: %+v", ev.Metadata)
	}
	other, ok := c["azit0042"]
	if !ok || len(other.Rows) != 1 || other.Rows[0].Index != 17 {
		t.Fatalf("azit0042: %+v", c.SortedKeys())
	}
}

func TestMarshalShapesOutput(t *testing.T) {
	raw, err := fmtstrings.Marshal(sampleCollection())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"/*key=ffx/event/obj_ps3/az/azit0000/azit0000.bin row_count=2*/",
		"/*key=ffx/battle/kernel/command.bin row_count=2*/",
		"/*key=ffx/menu/macrodic.dcp row_count=1*/",
		"azit0000:0:us║$",
		"azit0000:0:sp║$",
		"command:name:0:us║$",
		"command:desc:0:us║$",
		"chunk_00:name:0:us║$",
		"Tom & Jerry",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, out)
		}
	}
	// "Where are you?" repete no default lang (>=5 runes) → segunda vira ref.
	h := hash.Sum64Hex("Where are you?")
	if !strings.Contains(out, "= $"+h+"\n") {
		t.Fatalf("expected dedup ref for repeated text:\n%s", out)
	}
}

func TestMarshalKeepsDivergentCopyLiteralAndCanonicalizesRefs(t *testing.T) {
	const original = "Same source sentence across files"
	const definition = "Translation A from definition"
	const divergent = "Translation B for one copy"
	bare := hash.Sum64Hex(original)
	c := dto.Collection{
		"aaa_divergent": {Metadata: dto.NewEventMetadata("aaa_divergent", common.GameVersionFFX), Rows: []dto.TextRow{{
			Index: 0, Hash: map[string]string{"us": bare},
			Text: map[string]string{"us": divergent}, Divergent: true,
		}}},
		"mmm_definition": {Metadata: dto.NewEventMetadata("mmm_definition", common.GameVersionFFX), Rows: []dto.TextRow{{
			Index: 0, Hash: map[string]string{"us": bare}, Text: map[string]string{"us": definition},
		}}},
		"zzz_reference": {Metadata: dto.NewEventMetadata("zzz_reference", common.GameVersionFFX), Rows: []dto.TextRow{{
			Index: 0, Hash: map[string]string{"us": bare}, Text: map[string]string{"us": original},
		}}},
	}
	raw, err := fmtstrings.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	if !strings.Contains(out, "aaa_divergent:0:us║$"+bare+" = "+divergent) {
		t.Fatalf("divergent text must remain literal:\n%s", out)
	}
	if !strings.Contains(out, "zzz_reference:0:us║$"+bare+" = $"+bare) {
		t.Fatalf("ordinary copy must remain a ref to canonical def:\n%s", out)
	}
}

func TestUnmarshalInfersExternalLiteralAsDivergentCopy(t *testing.T) {
	const original = "Canonical source text for external edit"
	const divergent = "External copy-specific translation"
	bare := hash.Sum64Hex(original)
	artifact := strings.Join([]string{
		"/*key=ffx/events/aaa_definition.bin row_count=1*/",
		"aaa_definition:0:us║$" + bare + " = " + original,
		"/*key=ffx/events/mmm_divergent.bin row_count=1*/",
		"mmm_divergent:0:us║$" + bare + " = " + divergent,
		"/*key=ffx/events/zzz_reference.bin row_count=1*/",
		"zzz_reference:0:us║$" + bare + " = $" + bare,
	}, "\n")
	parsed, err := fmtstrings.Unmarshal([]byte(artifact))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed["mmm_divergent"].Rows[0].Divergent {
		t.Fatal("literal text replacing $hash under the same hash should be inferred as divergent")
	}
	if got := parsed["zzz_reference"].Rows[0].Text["us"]; got != original {
		t.Fatalf("remaining $hash reference must resolve to the first canonical literal: %q", got)
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	in := sampleCollection()
	raw, err := fmtstrings.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, err := fmtstrings.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back) != len(in) {
		t.Fatalf("entry count: %d vs %d", len(back), len(in))
	}
	for _, k := range in.SortedKeys() {
		want, got := in[k], back[k]
		if got.Metadata.Key != want.Metadata.Key || got.Metadata.ID != want.Metadata.ID {
			t.Fatalf("%s metadata: %+v vs %+v", k, got.Metadata, want.Metadata)
		}
		if len(got.Rows) != len(want.Rows) {
			t.Fatalf("%s rows: %d vs %d", k, len(got.Rows), len(want.Rows))
		}
		wantRows := append([]dto.TextRow(nil), want.Rows...)
		dto.SortRows(wantRows)
		for i := range wantRows {
			wr, gr := wantRows[i], got.Rows[i]
			if wr.Index != gr.Index || wr.Name != gr.Name {
				t.Fatalf("%s row %d position: %+v vs %+v", k, i, gr, wr)
			}
			for lang, text := range wr.Text {
				if gr.Text[lang] != text {
					t.Fatalf("%s row %d lang %s: %q vs %q", k, i, lang, gr.Text[lang], text)
				}
				if gr.Hash[lang] != wr.Hash[lang] {
					t.Fatalf("%s row %d lang %s hash: %q vs %q", k, i, lang, gr.Hash[lang], wr.Hash[lang])
				}
			}
		}
	}
}

func TestMarshalLangsFilter(t *testing.T) {
	raw, err := fmtstrings.MarshalLangs(sampleCollection(), []string{"us"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	if strings.Contains(out, ":sp║$") {
		t.Fatalf("sp lines must be filtered:\n%s", out)
	}
	back, err := fmtstrings.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := back["azit0000"].Rows[0].Text["sp"]; ok {
		t.Fatalf("sp must not roundtrip: %+v", back["azit0000"].Rows[0])
	}
	if back["azit0000"].Rows[0].Text["us"] != "Where are you?" {
		t.Fatalf("us lost: %+v", back["azit0000"].Rows[0])
	}
}

func TestEscapesRoundTrip(t *testing.T) {
	tricky := "a=b\\c║d\nnewline here and  leading kept"
	c := dto.Collection{
		"ev001": {
			Metadata: dto.NewEventMetadata("ev001", common.GameVersionFFX).WithRowCount(1),
			Rows: []dto.TextRow{
				{Index: 0, Hash: hash.Texts(map[string]string{"us": tricky}), Text: map[string]string{"us": tricky}},
			},
		},
	}
	raw, err := fmtstrings.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "║d\n") {
		t.Fatalf("raw separator/newline leaked:\n%s", raw)
	}
	back, err := fmtstrings.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := back["ev001"].Rows[0].Text["us"]; got != tricky {
		t.Fatalf("escape roundtrip: %q vs %q", got, tricky)
	}
}

func TestEmptyCellsSkipped(t *testing.T) {
	c := dto.Collection{
		"ev001": {
			Metadata: dto.NewEventMetadata("ev001", common.GameVersionFFX),
			Rows: []dto.TextRow{
				{Index: 0, Hash: hash.Texts(map[string]string{"us": "Hi"}), Text: map[string]string{"us": "Hi", "sp": ""}},
			},
		},
	}
	raw, err := fmtstrings.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), ":sp║$") {
		t.Fatalf("empty sp cell must be skipped:\n%s", raw)
	}
	// Def vazio escrito à mão é preservado como "".
	hand := "/*key=ffx/event/obj_ps3/ev/ev001/ev001.bin row_count=1*/\n" +
		"ev001:0:us║$" + hash.Sum64Hex("Hi") + " = Hi\n" +
		"ev001:0:sp║$" + hash.Sum64Hex("") + " = \n"
	back, err := fmtstrings.Unmarshal([]byte(hand))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := back["ev001"].Rows[0].Text["sp"]; !ok || v != "" {
		t.Fatalf("empty def not preserved: %+v", back["ev001"].Rows[0])
	}
}

func TestMalformedLines(t *testing.T) {
	for _, line := range []string{
		"ev001:0:us $abc = Hi",               // sem ║
		"ev001:0:us║Hi",                      // sem =
		"ev001:0:us║abc = Hi",                // sem $ no hash
		"ev001:0:us║$xyz = Hi",               // hash não-hex
		"ev001:xx:us║$0000000000000000 = Hi", // index inválido
		"us║$0000000000000000 = Hi",          // sem index
	} {
		if _, err := fmtstrings.Unmarshal([]byte(line + "\n")); err == nil {
			t.Fatalf("expected error for %q", line)
		}
	}
}

// A versão NÃO é campo da linha — vive só no header /*key=*/. Linha com
// prefixo de versão é erro de formato normal (nada de strip silencioso,
// que parsearia errado: id="ffx", name=<id>).
func TestUnmarshalRejectsVersionPrefix(t *testing.T) {
	for _, line := range []string{
		"ffx:ev001:0:us║$" + hash.Sum64Hex("Hi") + " = Hi",
		"ffx2:ev001:0:us║$" + hash.Sum64Hex("Hi") + " = Hi",
		"lastmiss:ev001:0:us║$" + hash.Sum64Hex("Hi") + " = Hi",
	} {
		if _, err := fmtstrings.Unmarshal([]byte(line + "\n")); err == nil ||
			!strings.Contains(err.Error(), "version prefix") {
			t.Fatalf("esperava erro de version prefix para %q, veio: %v", line, err)
		}
	}
}

func TestRightAnchorGrowth(t *testing.T) {
	// Campos novos crescem pela esquerda: id/name continuam resolvidos.
	// (Versão NÃO é um desses campos — ver TestUnmarshalRejectsVersionPrefix.)
	raw := "/*key=ffx/battle/kernel/command.bin row_count=1*/\n" +
		"region:command:name:0:us║$" + hash.Sum64Hex("Potion") + " = Potion\n"
	back, err := fmtstrings.Unmarshal([]byte(raw))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := back["command"]
	if !ok {
		t.Fatalf("command not resolved: %+v", back.SortedKeys())
	}
	if entry.Rows[0].Name != "name" || entry.Rows[0].Text["us"] != "Potion" {
		t.Fatalf("wrong row: %+v", entry.Rows[0])
	}
}

// TestStringsFormatPUARoundTrip valida o token {PUA:XX:CHAR} no formato
// strings, que não dá uso especial a aspas ou caracteres de controle (só
// escapa \, CR, LF e ║): o token cruza marshal/unmarshal intacto e o texto
// volta aos bytes originais byte-a-byte.
//
// Com as correções de font na tabela us (comprovadas pela atlas
// font_0_0/font_0_1), aspas e apóstrofo deixam de duplicar:
//   - 0x3C desenha aspas RETAS → rune " (canônico); 0x95 desenha as CURVAS
//     (inclinadas) → rune ” único: nenhum dos dois gera token.
//   - 0x41 desenha apóstrofo reto → rune '; 0xD5 o curvo → rune ’ único
//     (o byte usado 181× nos diálogos originais).
//   - Duplicata genuína restante: espaço em 0xA0 (glifo vazio, igual a
//     0x3A) → decode emite {PUA:A0: }, com o mesmo rune da ocorrência
//     normal dentro do token.
func TestStringsFormatPUARoundTrip(t *testing.T) {
	// Tabela us real; snapshot para restaurar os mapas do pacote.
	prevB2C := ffxencoding.GetByteToCharMap(common.GameVersionFFX, "us")
	prevC2B := ffxencoding.GetCharToByteMap(common.GameVersionFFX, "us")
	defer func() {
		ffxencoding.SetCharMap(common.GameVersionFFX, "us", prevB2C, prevC2B)
	}()
	if err := ffxencoding.PrepareCharset(common.GameVersionFFX, "us"); err != nil {
		t.Fatalf("PrepareCharset: %v", err)
	}

	// Sem duplicata: 0x3C (aspas retas) e 0x95 (curvas) decodificam puros.
	decoded := converter.BytesToString([]byte{0x3C, 0x95, 0x00}, "us", common.GameVersionFFX)
	if want := "\"”"; decoded != want {
		t.Fatalf("decode de 0x3C+0x95 sem duplicata:\n got %q\nwant %q", decoded, want)
	}
	// Canônicos do encode — o byte "certo" de cada rune é o slot do seu glifo.
	for char, want := range map[rune]uint{'"': 0x3C, '”': 0x95, '\'': 0x41} {
		if b, err := ffxencoding.CharToByte(char, "us", common.GameVersionFFX); err != nil || b != want {
			t.Fatalf("byte canônico de %q: got 0x%02X (err %v), want 0x%02X", char, b, err, want)
		}
	}

	// Duplicata genuína: espaço normal (0x3A) + slot extra (0xA0). O decode
	// repete o caractere — o rune dentro do token é o mesmo espaço — e o
	// token preserva o byte exato para o re-import.
	spaceDecoded := converter.BytesToString([]byte{0x3A, 0xA0, 0x00}, "us", common.GameVersionFFX)
	if want := " {PUA:A0: }"; spaceDecoded != want {
		t.Fatalf("decode de 0x3A+0xA0:\n got %q\nwant %q", spaceDecoded, want)
	}
	reencoded, err := converter.StringToStoredBytes(spaceDecoded, "us", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !reflect.DeepEqual(reencoded, []byte{0x3A, 0xA0, 0x00}) {
		t.Fatalf("round-trip byte-exato:\n got % X\nwant % X", reencoded, []byte{0x3A, 0xA0, 0x00})
	}

	// Round-trip pelo formato strings: aspas retas/curvas e o token passam
	// sem escape e o re-encode pós-formato continua byte-exato.
	text := decoded + spaceDecoded
	collection := dto.Collection{
		"test0000": {
			Metadata: dto.NewEventMetadata("test0000", common.GameVersionFFX).WithRowCount(1),
			Rows: []dto.TextRow{{
				Index: 0,
				Hash:  hash.Texts(map[string]string{"us": text}),
				Text:  map[string]string{"us": text},
			}},
		},
	}
	data, err := fmtstrings.Marshal(collection)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	parsed, err := fmtstrings.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	entry, ok := parsed["test0000"]
	if !ok || len(entry.Rows) != 1 {
		t.Fatalf("Unmarshal perdeu a entrada: %+v", parsed)
	}
	if got := entry.Rows[0].Text["us"]; got != text {
		t.Fatalf("texto mudou no round-trip do formato strings:\n got %q\nwant %q", got, text)
	}
	reencodedFmt, err := converter.StringToStoredBytes(entry.Rows[0].Text["us"], "us", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("re-encode pós-formato: %v", err)
	}
	if want := []byte{0x3C, 0x95, 0x3A, 0xA0, 0x00}; !reflect.DeepEqual(reencodedFmt, want) {
		t.Fatalf("round-trip byte-exato pós-formato:\n got % X\nwant % X", reencodedFmt, want)
	}
}

// A projeção source/target do export sai na linha do idioma default e volta
// no parse — inclusive quando o TEXTO contém a sequência do marcador (o ║
// literal é escapado) e quando só há target (original em branco).
func TestSourceTargetProjectionRoundTrip(t *testing.T) {
	text := "A║source = B\nC"
	target := "Tradução║target = x\ny"
	collection := dto.Collection{
		"test0000": {
			Metadata: dto.NewEventMetadata("test0000", common.GameVersionFFX).WithRowCount(2),
			Rows: []dto.TextRow{
				{
					Index:  0,
					Hash:   hash.Texts(map[string]string{"us": text}),
					Text:   map[string]string{"us": text},
					Source: text,
					Target: target,
				},
				{
					// Original em branco, mas com tradução: a linha de us
					// precisa sair mesmo sem texto (o import depende dela).
					Index:  1,
					Hash:   hash.Texts(map[string]string{"us": ""}),
					Text:   map[string]string{"us": ""},
					Source: "",
					Target: "só tradução",
				},
			},
		},
	}
	data, err := fmtstrings.Marshal(collection)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	parsed, err := fmtstrings.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	entry, ok := parsed["test0000"]
	if !ok || len(entry.Rows) != 2 {
		t.Fatalf("rows=%d, queria 2:\n%s", len(entry.Rows), data)
	}
	first := entry.Rows[0]
	if got := first.Text["us"]; got != text {
		t.Fatalf("texto mudou: got %q want %q", got, text)
	}
	if first.Source != text {
		t.Fatalf("source=%q, queria %q", first.Source, text)
	}
	if first.Target != target {
		t.Fatalf("target=%q, queria %q", first.Target, target)
	}
	if got := entry.Rows[1].Target; got != "só tradução" {
		t.Fatalf("target da row sem texto=%q", got)
	}
}
