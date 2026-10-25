// Gêmeos de ARQUIVO: o jogo rastreia os binários de texto pelo manifesto
// interno (proj/event/header/eventid.bin) — tuto0000.bin e tutorial.msb são
// o MESMO conteúdo gravado em dois caminhos, e nenhum pode deixar de existir
// por ser "cópia". Otimização é interna ao arquivo (offsets compartilhados);
// na camada de arquivos, toda cópia e todo gêmeo continuam existindo.
//
// O Save do KindTutorial modela os dois caminhos como Bins de um File único
// e grava os dois a partir das mesmas strings. Este teste garante, com
// fixture SINTÉTICA (temp, sem depender de build/bin/data):
//
//   - os dois gêmeos existem no mods após o save;
//   - são byte a byte idênticos entre si (cópia continua cópia);
//   - as strings que o jogo lê sobrevivem ao save (releitura confere).
package eventtable_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/fileFormats/eventtable"
)

// tabelaSinteticaTuto monta uma tabela mínima do codec events com as
// particularidades que interessam: tag, acento e linha vazia — cada row com
// offset próprio, como data/ grava.
func tabelaSinteticaTuto(t *testing.T) []byte {
	t.Helper()
	if err := ffxencoding.PrepareVersionCharsets(common.GameVersionFFX2); err != nil {
		t.Fatalf("charsets: %v", err)
	}
	textos := []string{
		"{TIME:00}Tutorial com {CLR:YELLOW}tag{CLR:WHITE} e {TEXT_NEWLINE}quebra.",
		"{TIME:00}Linha com acentos: você verá informações.",
		"", // linha vazia: terminador próprio
		"{TIME:00}Fim.",
	}
	contentOffset := len(textos) * 8
	var content bytes.Buffer
	headers := make([]byte, 0, contentOffset)
	for _, s := range textos {
		off := contentOffset + content.Len()
		enc, err := converter.StringToBytes(s, "us", common.GameVersionFFX2)
		if err != nil {
			t.Fatalf("codificar %q: %v", s, err)
		}
		content.Write(enc)
		content.WriteByte(0x00)
		h1 := make([]byte, 4)
		binary.LittleEndian.PutUint32(h1, uint32(off))
		h2 := make([]byte, 4)
		binary.LittleEndian.PutUint32(h2, uint32(off)) // simplified == regular
		headers = append(headers, h1...)
		headers = append(headers, h2...)
	}
	return append(headers, content.Bytes()...)
}

func stringsDaTabela(t *testing.T, raw []byte) []string {
	t.Helper()
	rows, err := event.FromFieldStringData(raw, "us", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := make([]string, len(rows))
	for i, fs := range rows {
		out[i] = fs.GetRegularString()
	}
	return out
}

// TestTuto0000SalvaOsDoisGemeos: save do tuto0000 grava tuto0000.bin E
// tutorial.msb — os dois existem, idênticos entre si, com o conteúdo intacto.
func TestTuto0000SalvaOsDoisGemeos(t *testing.T) {
	tabela := tabelaSinteticaTuto(t)
	esperadas := stringsDaTabela(t, tabela)

	tmp := tempdir.TempDir(t)
	usRel := filepath.FromSlash(common.GetLocalizationRootForVersion(common.GameVersionFFX2, common.DefaultLocalization))
	tutoDir := filepath.Join(tmp, usRel, "battle", "btl", "tuto0000")
	if err := os.MkdirAll(tutoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, nome := range []string{"tuto0000.bin", "tutorial.msb"} {
		if err := os.WriteFile(filepath.Join(tutoDir, nome), tabela, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot, common.DisableMods = tmp, false
	t.Cleanup(func() { common.GameFilesRoot, common.DisableMods = prevRoot, prevMods })

	f, err := eventtable.Load(eventtable.KindTutorial, common.GameVersionFFX2, "tuto0000")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(f.Bins) != 2 {
		t.Fatalf("tuto0000 modela %d bin(s); os gêmeos são 2 caminhos", len(f.Bins))
	}

	if err := f.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	modsDir := filepath.Join(tmp, common.ModsFolder, usRel, "battle", "btl", "tuto0000")
	bin1, err1 := os.ReadFile(filepath.Join(modsDir, "tuto0000.bin"))
	bin2, err2 := os.ReadFile(filepath.Join(modsDir, "tutorial.msb"))
	if err1 != nil || err2 != nil {
		t.Fatalf("gêmeo ausente após o save: tuto0000.bin=%v tutorial.msb=%v", err1, err2)
	}
	if !bytes.Equal(bin1, bin2) {
		t.Errorf("gêmeos divergiram: tuto0000.bin=%d bytes, tutorial.msb=%d bytes", len(bin1), len(bin2))
	}
	for _, nome := range []string{"tuto0000.bin", "tutorial.msb"} {
		raw, err := os.ReadFile(filepath.Join(modsDir, nome))
		if err != nil {
			t.Fatalf("reler %s: %v", nome, err)
		}
		gravadas := stringsDaTabela(t, raw)
		if len(gravadas) != len(esperadas) {
			t.Fatalf("%s: %d rows, esperadas %d", nome, len(gravadas), len(esperadas))
		}
		for i := range esperadas {
			if gravadas[i] != esperadas[i] {
				t.Errorf("%s: row %d mudou:\n esperada: %q\n gravada:  %q", nome, i, esperadas[i], gravadas[i])
			}
		}
	}
}
