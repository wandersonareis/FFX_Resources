package converter

import (
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/components"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/datastore"
)

// Testes simples (white-box) do decode binário: fixam o comportamento ATUAL de
// getStringAtLookupOffsetBinary antes de qualquer refatoração. O charset é
// sintético (mapa pequeno e determinístico) para que cada caso abaixo pinte
// exatamente um guard do switch — inclusive os casos em que a ORDEM dos guards
// define o resultado (flag de 5 seções antes dos comandos; buf.Len()==0 antes
// de 0x04..0x23). O teste com encoding/macrodic reais vive em
// backend/core/reader/decode_internals_test.go.

var (
	decodeTestCharset = "us"
	decodeTestVersion = common.GameVersionFFX
)

// seedDecodeMaps publica um charset sintético e restaura o anterior no fim.
//
//	0x30 → 'Z'   0x50 → 'A'   0x51 → 'B' (canônico de 'B')
//	0x99 → 'B'   (slot duplicado → {PUA:99:B})
//	0xD0 → 'W'   0x440 → 'X'  (slot da extensão de 5 seções)
func seedDecodeMaps(t *testing.T) {
	t.Helper()
	prevB2C := ffxencoding.GetByteToCharMap(decodeTestVersion, decodeTestCharset)
	prevC2B := ffxencoding.GetCharToByteMap(decodeTestVersion, decodeTestCharset)

	ffxencoding.SetCharMap(decodeTestVersion, decodeTestCharset,
		map[uint]rune{
			0x30:  'Z',
			0x50:  'A',
			0x51:  'B',
			0x99:  'B',
			0xD0:  'W',
			0x440: 'X',
		},
		map[rune]uint{
			'Z': 0x30,
			'A': 0x50,
			'B': 0x51,
			'W': 0xD0,
			'X': 0x440,
		},
	)
	t.Cleanup(func() {
		ffxencoding.SetCharMap(decodeTestVersion, decodeTestCharset, prevB2C, prevC2B)
	})
}

// clearMacrosAndKeyItems isola o decode de estado global (datastore de macros e
// KeyItems) e restaura no fim: sem isso, {MCR:...} e {KEY:...} dependeriam dos
// dados que outros testes carregaram antes.
func clearMacrosAndKeyItems(t *testing.T) {
	t.Helper()

	snapshot := components.NewEmptyMap[int, datastore.IGlobalLocalizedMacroStringObject]()
	datastore.GetMacros(decodeTestVersion).ForEach(func(idx int, macro datastore.IGlobalLocalizedMacroStringObject) {
		snapshot.Add(idx, macro)
	})
	datastore.Instance.ClearMacros(decodeTestVersion)

	prevKeyItems := datastore.KeyItems
	datastore.KeyItems = components.NewEmptyList[datastore.IGlobalLocalizedTextObject]()

	t.Cleanup(func() {
		datastore.SetMacros(decodeTestVersion, snapshot)
		datastore.KeyItems = prevKeyItems
	})
}

func TestGetStringAtLookupOffsetBinaryBounds(t *testing.T) {
	seedDecodeMaps(t)

	table := []byte{'x', 'x', 0x50, 0x00}

	cases := []struct {
		name   string
		offset int
		want   string
	}{
		{"offset negativo", -1, ""},
		{"offset igual ao tamanho", len(table), ""},
		{"offset além do fim", len(table) + 5, ""},
		{"offset no terminador", 3, ""},
		{"offset zero", 0, "{UNKCHR:78}{UNKCHR:78}A"}, // 'x' não está no charset sintético
		{"offset no meio", 2, "A"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getStringAtLookupOffsetBinary(table, tc.offset, decodeTestCharset, decodeTestVersion); got != tc.want {
				t.Fatalf("offset %d: got %q, want %q", tc.offset, got, tc.want)
			}
		})
	}
}

func TestGetStringAtLookupOffsetBinaryCharacterization(t *testing.T) {
	seedDecodeMaps(t)
	clearMacrosAndKeyItems(t)

	// Tabela de casos: um por guard, mais os casos de ordem/sentinela que o
	// refactor pode quebrar sem perceber.
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"vazio", []byte{}, ""},
		{"terminador imediato", []byte{0x00}, ""},
		{"texto simples", []byte{0x50, 0x51, 0x00}, "AB"},
		{"terminador no meio", []byte{0x50, 0x00, 0x51}, "A"},
		{"slot duplicado vira PUA", []byte{0x99}, "{PUA:99:B}"},
		{"byte texto fora do mapa", []byte{0x92}, "{UNKCHR:92}"},

		{"op desconhecido 0x06", []byte{0x06, 0x40}, "{UNKOP:06:40}"},
		{"op desconhecido 0x26", []byte{0x26, 0x30}, "{UNKOP:26:30}"},
		{"op truncado cai na sentinela", []byte{0x06}, "{HEX:06}"},

		{"op 0x2B (não é seção)", []byte{0x2B, 0x30}, "{UNKOP:2B:30}"},
		{"seção 0x2C", []byte{0x2C, 0x00}, "W"},
		{"op 0x2B fora do mapa", []byte{0x2B, 0x01}, "{UNKOP:2B:01}"},
		{"op 0x2B truncado cai na sentinela", []byte{0x2B}, "{HEX:2B}"},

		{"5 seções byte texto", []byte{0x04, 0x30}, "X"},
		{"5 seções com seção", []byte{0x04, 0x2B, 0x30}, "X"},
		{"5 seções byte texto fora do mapa", []byte{0x04, 0x31}, "{UNKDBLCHR:04:31}"},
		{"5 seções seção fora do mapa", []byte{0x04, 0x2B, 0x00}, "{UNKDBLCHR:04:2B:00}"},
		{"flag consumida uma vez", []byte{0x04, 0x04, 0x50}, "{UNKDBLCHR:04:04}A"},

		// Sentinela buf.Len()==0: o último byte sem terminador sai como HEX,
		// mesmo quando o byte teria um comando próprio. Consequência atual:
		// os fallbacks "{CMD:XX:??}", "{SPACE:??}", "{ICON:??}", etc. são
		// inalcançáveis (o read só falha quando o buffer está vazio, e aí a
		// sentinela já saiu antes). Os "??" de byte duplo (0x06/0x26..0x2A e
		// >=0x2B) continuam alcançáveis porque esses casos vêm ANTES da
		// sentinela.
		{"sentinela 0x04", []byte{0x04}, "{HEX:04}"},
		{"sentinela 0x05", []byte{0x05}, "{HEX:05}"},
		{"sentinela 0x0B", []byte{0x0B}, "{HEX:0B}"},
		{"sentinela 0x10", []byte{0x10}, "{HEX:10}"},
		{"sentinela 0x13", []byte{0x13}, "{HEX:13}"},
		{"sentinela 0x14", []byte{0x14}, "{HEX:14}"},
		{"op 0x24 truncado cai na sentinela", []byte{0x24}, "{HEX:24}"},

		// Estes vêm ANTES da sentinela: continuam comando no fim da tabela.
		{"PAUSE no fim", []byte{0x01}, "{PAUSE}"},
		{"BREAK no fim", []byte{0x02}, "{BREAK}"},

		{"BLANK05", []byte{0x05, 0x50}, "{BLANK05}A"},
		{"op 0x0C (2 bytes)", []byte{0x0C, 0x50}, "{UNKOP:0C:50}"},
		{"op 0x0F (2 bytes)", []byte{0x0F, 0x50}, "{UNKOP:0F:50}"},
		{"op 0x11 (2 bytes)", []byte{0x11, 0x50}, "{UNKOP:11:50}"},

		{"SPACE", []byte{0x07, 0x35}, "{SPACE:05}"},
		{"SPACE truncado cai na sentinela", []byte{0x07}, "{HEX:07}"},
		{"TIME", []byte{0x09, 0x3A}, "{TIME:0A}"},
		{"TIME truncado cai na sentinela", []byte{0x09}, "{HEX:09}"},

		{"cor conhecida", []byte{0x0A, 0x41}, "{CLR:WHITE}"},
		{"cor desconhecida", []byte{0x0A, 0x50}, "{CLR:50}"},
		{"cor truncada cai na sentinela", []byte{0x0A}, "{HEX:0A}"},

		{"botão", []byte{0x0B, 0x30}, "{BUTTON:30:TRIANGLE}"},
		{"ícone", []byte{0x0B, 0x80}, "{ICON:80:Red Gate}"},
		{"botão fora do mapa", []byte{0x0B, 0x2F}, "{BUTTON:2F:?}"},

		{"itálico", []byte{0x0E, 0x40}, "{TEXT_ITALIC}"},
		{"normal", []byte{0x0E, 0x41}, "{TEXT_NORMAL}"},
		{"estilo genérico", []byte{0x0E, 0x50}, "{CMD:0E:20}"},
		{"estilo underflow", []byte{0x0E, 0x00}, "{CMD:0E:D0}"},
		{"op 0x0E truncado cai na sentinela", []byte{0x0E}, "{HEX:0E}"},

		{"choice", []byte{0x10, 0x30}, "{CHOICE:00}"},
		{"choice end", []byte{0x10, 0xFF}, "{CHOICE-END}"},
		{"choice underflow", []byte{0x10, 0x00}, "{CHOICE:D0}"},

		{"var", []byte{0x12, 0x31}, "{VAR:01}"},
		{"var truncado cai na sentinela", []byte{0x12}, "{HEX:12}"},

		{"PC", []byte{0x13, 0x30}, "{PC:00:TIDUS}"},
		{"PC fora do mapa da versão", []byte{0x13, 0x46}, "{PC:16:?}"},
		{"PC fora da faixa", []byte{0x13, 0x47}, "{UNKDBLCHR:13:47}"},

		// Sem macros carregadas o MCR não tem o campo de conteúdo.
		{"MCR", []byte{0x14, 0x30}, "{MCR:s01:l00}"},
		{"MCR seção alta", []byte{0x20, 0x32}, "{MCR:s0D:l02}"},

		{"KEY sem catálogo", []byte{0x23, 0x31}, "{KEY:01}"},
		{"KEY truncado cai na sentinela", []byte{0x23}, "{HEX:23}"},

		{"op desconhecido 0x24", []byte{0x24, 0x31}, "{UNKOP:24:31}"},
		{"op 0x24 truncado cai na sentinela", []byte{0x24}, "{HEX:24}"},

		{"misto", []byte{0x50, 0x0A, 0x41, 0x51}, "A{CLR:WHITE}B"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getStringAtLookupOffsetBinary(tc.in, 0, decodeTestCharset, decodeTestVersion); got != tc.want {
				t.Fatalf("bytes % X: got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestGetStringAtLookupOffsetBinaryLinebreakModes(t *testing.T) {
	seedDecodeMaps(t)

	prev := WriteLinebreaksAsCommands
	t.Cleanup(func() { WriteLinebreaksAsCommands = prev })

	WriteLinebreaksAsCommands = true
	if got := getStringAtLookupOffsetBinary([]byte{0x03}, 0, decodeTestCharset, decodeTestVersion); got != "{TEXT_NEWLINE}" {
		t.Fatalf("modo comando: got %q, want %q", got, "{TEXT_NEWLINE}")
	}

	WriteLinebreaksAsCommands = false
	if got := getStringAtLookupOffsetBinary([]byte{0x03}, 0, decodeTestCharset, decodeTestVersion); got != "\n" {
		t.Fatalf("modo newline: got %q, want %q", got, "\n")
	}
}

// stubMacroString/stubMacro são o mínimo para exercitar o caminho de conteúdo
// do MCR sem carregar macrodic do disco.
type stubMacroString struct{ text string }

func (s stubMacroString) GetString() string { return s.text }
func (s stubMacroString) IsEmpty() bool     { return s.text == "" }
func (s stubMacroString) String() string    { return s.text }

type stubMacro struct{ text string }

func (m stubMacro) SetLocalizedContent(string, datastore.IGlobalMacroString) {}
func (m stubMacro) GetLocalizedContent(string) (datastore.IGlobalMacroString, bool) {
	return stubMacroString{text: m.text}, true
}
func (m stubMacro) GetLocalizedString(string) string { return m.text }
func (m stubMacro) GetDefaultContent() (datastore.IGlobalMacroString, bool) {
	return stubMacroString{text: m.text}, true
}
func (m stubMacro) CopyInto(datastore.IGlobalLocalizedMacroStringObject) {}
func (m stubMacro) String() string                                       { return m.text }

func TestGetStringAtLookupOffsetBinaryMacroContent(t *testing.T) {
	seedDecodeMaps(t)
	clearMacrosAndKeyItems(t)

	// s06:l40 → índice 6*0x100 + 0x40 = 0x640.
	datastore.SetMacro(decodeTestVersion, 0x640, stubMacro{text: "PROBE"})

	got := getStringAtLookupOffsetBinary([]byte{0x19, 0x70}, 0, decodeTestCharset, decodeTestVersion)
	if want := `{MCR:s06:l40:"PROBE"}`; got != want {
		t.Fatalf("MCR com conteúdo: got %q, want %q", got, want)
	}
}
