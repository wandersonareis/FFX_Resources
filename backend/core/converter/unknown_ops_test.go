package converter

import (
	"reflect"
	"strings"
	"testing"

	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
)

// TestParseCommandRawByteTokens cobre os tokens de bytes crus emitidos pelo
// decode: UNKOP (op de 2 bytes), UNKDBLCHR (bytes exatos, inclusive a forma
// estendida de 3 bytes) e UNKTPLCHR (prefixo 0x04 + bytes).
func TestParseCommandRawByteTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []uint
	}{
		{"{UNKOP:2A:30}", []uint{0x2A, 0x30}},
		{"{UNKOP:06:40}", []uint{0x06, 0x40}},
		{"{UNKOP:2B:01}", []uint{0x2B, 0x01}},
		{"{UNKDBLCHR:04:31}", []uint{0x04, 0x31}},
		{"{UNKDBLCHR:04:2B:00}", []uint{0x04, 0x2B, 0x00}},
		{"{UNKTPLCHR:04:06:40}", []uint{0x04, 0x06, 0x40}},
	}
	for _, tc := range cases {
		got := ParseCommand([]rune(tc.in), 0)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseCommand(%q) = % X, want % X", tc.in, got, tc.want)
		}
	}
}

// TestUnknownOpsRoundTrip garante que os ops de 2 bytes preservam os bytes
// exatos na ida e volta (decode -> encode).
func TestUnknownOpsRoundTrip(t *testing.T) {
	seedDecodeMaps(t)
	clearMacrosAndKeyItems(t)

	inputs := [][]byte{
		{0x2A, 0x30},             // op 0x2A, arg 0x30 (JP "０" via op)
		{0x06, 0x31},             // op 0x06
		{0x24, 0x7F},             // op 0x24 (faixa antiga de CMD)
		{0x2B, 0x01},             // 0x2B é op sem o prefixo 0x04
		{0x28, 0x32, 0x3B, 0x00}, // op + texto + fim
	}
	for _, in := range inputs {
		text := getStringAtLookupOffsetBinary(in, 0, decodeTestCharset, decodeTestVersion)
		got, err := StringToBytes(text, decodeTestCharset, decodeTestVersion)
		if err != nil {
			t.Fatalf("StringToBytes(%q): %v", text, err)
		}
		// StringToBytes não inclui o terminador; compare o prefixo.
		want := in
		if len(want) > 0 && want[len(want)-1] == 0x00 {
			want = want[:len(want)-1]
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round-trip % X -> %q -> % X, want % X", in, text, got, want)
		}
	}
}

// TestParseCommandTruncatedTokens prende as semânticas que fecham o ciclo
// extrair→importar→extrair:
//   - "??" preserva os bytes presentes ({UNKDBLCHR:2C:??} → [0x2C]; o
//     re-decode do byte restante reemite o mesmo token — ponto fixo);
//   - {MCR:sXX:lYY} sem payload parseia (o payload é display opcional);
//   - linha de macro com underflow (byte cru 0x01 → lD1) volta com
//     wraparound: 0xD1+0x30 = 0x101, truncado para 0x01 na escrita.
func TestParseCommandTruncatedTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []uint
	}{
		{"{UNKDBLCHR:2C:??}", []uint{0x2C}},
		{"{UNKDBLCHR:04:2C:??}", []uint{0x04, 0x2C}},
		{"{UNKDBLCHR:??}", nil},
		{"{UNKOP:2B:??}", []uint{0x2B}},
		{"{MCR:s06:l40}", []uint{0x19, 0x70}},
		{"{MCR:s06:l40:\"Sin\"}", []uint{0x19, 0x70}},
		{"{MCR:s01:lD1}", []uint{0x14, 0x101}},
	}
	for _, tc := range cases {
		got := ParseCommand([]rune(tc.in), 0)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseCommand(%q) = % X, want % X", tc.in, got, tc.want)
		}
	}
}

// TestDecodeOpRealBinaryBytes prende os bytes reais de azit0300 (JP), onde a
// linha "？０！" do evento é 13 34 03 2D 9E 2A 30 3B 00: 0x2A 0x30 é op
// (0x2A) + argumento (0x30), NÃO um caractere CJK de 2 bytes, e o "!" final
// (0x3B) sobrevive depois do op. Referência: Fahrenheit DecodingGamecode*.
func TestDecodeOpRealBinaryBytes(t *testing.T) {
	if err := ffxencoding.PrepareCharset(common.GameVersionFFX, "jp"); err != nil {
		t.Fatalf("PrepareCharset(jp): %v", err)
	}
	raw := []byte{0x13, 0x34, 0x03, 0x2D, 0x9E, 0x2A, 0x30, 0x3B, 0x00}
	got := getStringAtLookupOffsetBinary(raw, 0, "jp", common.GameVersionFFX)
	if !strings.Contains(got, "{UNKOP:2A:30}") {
		t.Fatalf("0x2A 0x30 deve virar token de op, got %q", got)
	}
	// O byte de texto após o op (0x3B = "！" em jp) sobrevive ao op de 2 bytes.
	bang, err := ffxencoding.ByteToChar(0x3B, "jp", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ByteToChar(0x3B): %v", err)
	}
	if !strings.HasSuffix(got, string(bang)) {
		t.Fatalf("o 0x3B depois do op deve sobreviver, got %q", got)
	}
	// O caractere de seção 0x2D 0x9E (slot 0x23E) decodifica normal.
	chr, err := ffxencoding.ByteToChar(0x23E, "jp", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ByteToChar(0x23E): %v", err)
	}
	if want := charOrPUAToken(0x23E, chr, "jp", common.GameVersionFFX); !strings.Contains(got, want) {
		t.Fatalf("char de seção 0x2D 0x9E: got %q, want %q dentro", got, want)
	}
}
