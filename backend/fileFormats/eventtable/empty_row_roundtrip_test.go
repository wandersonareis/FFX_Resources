package eventtable_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/event"
	testcommon "ffxresources/testData"
)

// monta o binario como o writer: cabecalhos + conteudo reconstruido.
func montaBinario(rows []*event.FieldString, content []byte) []byte {
	var buf bytes.Buffer
	for _, fs := range rows {
		buf.Write(fs.ToRegularHeaderBytes())
		buf.Write(fs.ToSimplifiedHeaderBytes())
	}
	buf.Write(content)
	return buf.Bytes()
}

// Linha vazia precisa de terminador proprio no conteudo (padrao do original:
// offsets consecutivos apontando para bytes nulos reais). Apontar para
// contentOffset faz a carga ler do offset ate o proximo nulo e a linha vazia
// volta com o texto da PRIMEIRA linha - conteudo de outra linha aparecendo na
// carga, sem ref e sem destaque.
func TestRoundTripComLinhasVazias(t *testing.T) {
	root := filepath.Dir(testcommon.GetTestDataRootDirectory())
	bin := filepath.Join(root, "testData", "FFX", "binary", "ffx_ps2", "ffx", "master",
		"new_uspc", "battle", "btl", "system_01", "system_01.bin")
	raw, err := os.ReadFile(bin)
	if err != nil {
		t.Skip("fixture ausente: ", err)
	}

	rows, err := event.FromFieldStringData(raw, "us", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("parse do original: %v", err)
	}
	rebuilt := montaBinario(rows, event.RebuildFieldStrings(rows, "us", common.GameVersionFFX))
	if !bytes.Equal(raw, rebuilt) {
		t.Fatalf("round-trip divergiu: original=%d bytes, reconstruido=%d bytes", len(raw), len(rebuilt))
	}

	relidos, err := event.FromFieldStringData(rebuilt, "us", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("releitura: %v", err)
	}
	contentOffset := len(rows) * 8
	aliasadas := 0
	for i, fs := range relidos {
		if fs.GetRegularString() != "" {
			continue
		}
		if rebuilt[fs.RegularOffset] != 0x00 {
			t.Fatalf("linha %d vazia nao aponta para terminador proprio (offset=%d)", i, fs.RegularOffset)
		}
		// contentOffset e o inicio do conteudo: aponta la so e alias quando o
		// byte ali NAO e terminador (ou seja, e o comeco de outra string).
		if fs.RegularOffset == contentOffset && rebuilt[contentOffset] != 0x00 {
			aliasadas++
		}
	}
	if aliasadas > 0 {
		t.Fatalf("%d linha(s) vazia(s) aliasando a primeira string", aliasadas)
	}
}
