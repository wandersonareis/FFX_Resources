package converter

import (
	"testing"

	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
)

// TestIndexBytesCJKRoundTrip confere todos os slots CJK: indexBytes(code)
// decodifica de volta ao mesmo slot (runes duplicados viram {PUA:...}).
func TestIndexBytesCJKRoundTrip(t *testing.T) {
	for _, cs := range []string{"jp", "ch", "kr"} {
		if err := ffxencoding.PrepareCharset(common.GameVersionFFX, cs); err != nil {
			t.Fatalf("PrepareCharset(%s): %v", cs, err)
		}
		for code := uint(0x30); code <= 0x800; code++ {
			chr, err := ffxencoding.ByteToChar(code, cs, common.GameVersionFFX)
			if err != nil {
				continue
			}
			want := charOrPUAToken(code, chr, cs, common.GameVersionFFX)
			got := getStringAtLookupOffsetBinary(indexBytesU(code), 0, cs, common.GameVersionFFX)
			if got != want {
				t.Fatalf("%s slot 0x%X: % X -> %q, want %q",
					cs, code, indexBytesU(code), got, want)
			}
		}
	}
}

func indexBytesU(code uint) []byte {
	out := indexBytes(code)
	buf := make([]byte, len(out))
	for i, v := range out {
		buf[i] = byte(v)
	}
	return buf
}
