// Testes da camada de ARQUIVO padronizada (Writer/Reader): o parágrafo de
// uso é o MESMO para os quatro formatos — construtor recebe os parâmetros,
// chamador decide quando Write/Read e trata os erros.
package formatters_test

import (
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters"
	_ "ffxresources/backend/formatters/csv"
	_ "ffxresources/backend/formatters/json"
	_ "ffxresources/backend/formatters/strings"
	_ "ffxresources/backend/formatters/xlsx"
)

// artifactSample monta uma collection mínima com os casos difíceis de
// serialização: vírgula + aspas (CSV), name por row, dois idiomas.
func artifactSample() dto.Collection {
	c := dto.Collection{}
	one := dto.FileEntry{
		Metadata: dto.Metadata{Key: "ffx/event/obj_ps3/az/azit0000/azit0000.bin", ID: "azit0000"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Primeira frase do evento"},
				Hash: map[string]string{"us": "aaaa0000aaaa0000"}},
			{Index: 1, Name: "game",
				Text: map[string]string{"us": "Frase com, vírgula e \"aspas\"", "jp": "テスト"},
				Hash: map[string]string{"us": "bbbb1111bbbb1111", "jp": "cccc2222cccc2222"}},
		},
	}
	one.Metadata = one.Metadata.WithRowCount(len(one.Rows))
	c["azit0000"] = one
	return c
}

// Round-trip Write→Read pelas OITO entradas do registry: textos (inclusive
// vírgula/aspas/CJK) e hash voltam íntegros — o hash do DTO volta bare.
func TestWriterReaderRoundTripAllFormats(t *testing.T) {
	names := []string{
		"json", "strings", "csv", "xlsx",
		"json-no-dedup", "strings-no-dedup", "csv-no-dedup", "xlsx-no-dedup",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			f, ok := formatters.Get(name)
			if !ok {
				t.Fatalf("formatter %s não registrado", name)
			}
			path := filepath.Join(tempdir.TempDir(t), "artifact"+f.Extension())

			w := formatters.NewWriter(f, artifactSample(), path, nil)
			written, err := w.Write()
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			if written != path {
				t.Fatalf("caminho escrito %q != %q", written, path)
			}

			r := formatters.NewReader(f, path)
			got, err := r.Read()
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			entry, ok := got["azit0000"]
			if !ok || len(entry.Rows) != 2 {
				t.Fatalf("entradas/rows: %+v", got)
			}
			if got := entry.Rows[0].Text["us"]; got != "Primeira frase do evento" {
				t.Errorf("row 0 us: %q", got)
			}
			if got := entry.Rows[1].Text["us"]; got != "Frase com, vírgula e \"aspas\"" {
				t.Errorf("row 1 us: %q", got)
			}
			if got := entry.Rows[1].Text["jp"]; got != "テスト" {
				t.Errorf("row 1 jp: %q", got)
			}
			// O hash do DTO é SEMPRE bare — o $ do artefato morre no parse.
			if got := entry.Rows[1].Hash["jp"]; got != "cccc2222cccc2222" {
				t.Errorf("row 1 jp hash: %q (deveria ser bare)", got)
			}
		})
	}
}

// O filtro de idiomas do construtor atravessa o round-trip: só o idioma
// pedido sai no artefato e volta.
func TestWriterLangsFilter(t *testing.T) {
	for _, name := range []string{"json", "strings", "csv", "xlsx"} {
		t.Run(name, func(t *testing.T) {
			f, ok := formatters.Get(name)
			if !ok {
				t.Fatalf("formatter %s não registrado", name)
			}
			path := filepath.Join(tempdir.TempDir(t), "artifact"+f.Extension())
			if _, err := formatters.NewWriter(f, artifactSample(), path, []string{"jp"}).Write(); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := formatters.NewReader(f, path).Read()
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			entry, ok := got["azit0000"]
			if !ok {
				t.Fatalf("entrada sumiu no round-trip filtrado")
			}
			// Formats linha-por-(row,lang) (strings/CSV/XLSX) não escrevem
			// row sem idioma no filtro — localiza a row do jp pelo Index.
			var jpRow *dto.TextRow
			for i := range entry.Rows {
				if entry.Rows[i].Index == 1 {
					jpRow = &entry.Rows[i]
					break
				}
			}
			if jpRow == nil {
				t.Fatalf("row jp sumiu no filtro: %+v", entry.Rows)
			}
			if got := jpRow.Text["us"]; got != "" {
				t.Errorf("us não filtrado: %q", got)
			}
			if got := jpRow.Text["jp"]; got != "テスト" {
				t.Errorf("jp sumiu no filtro: %q", got)
			}
		})
	}
}

// Coleção vazia nunca vira artefato — erro claro, sem arquivo no disco.
func TestNewWriterEmptyCollection(t *testing.T) {
	f, ok := formatters.Get("json")
	if !ok {
		t.Fatal("formatter json não registrado")
	}
	path := filepath.Join(tempdir.TempDir(t), "vazio.json")
	if _, err := formatters.NewWriter(f, dto.Collection{}, path, nil).Write(); err == nil ||
		!strings.Contains(err.Error(), "no text data") {
		t.Fatalf("esperava erro de coleção vazia, veio: %v", err)
	}
}

// SwapExt troca a extensão pelo formato — no-op para o caminho .json.
func TestSwapExt(t *testing.T) {
	stringsF, ok := formatters.Get("strings")
	if !ok {
		t.Fatal("formatter strings não registrado")
	}
	if got := formatters.SwapExt(stringsF, "mods/edits/event_all_ffx.json"); got != "mods/edits/event_all_ffx.strings" {
		t.Errorf("swap json→strings: %q", got)
	}
	jsonF, ok := formatters.Get("json")
	if !ok {
		t.Fatal("formatter json não registrado")
	}
	if got := formatters.SwapExt(jsonF, "mods/edits/event_all_ffx.json"); got != "mods/edits/event_all_ffx.json" {
		t.Errorf("swap json→json deveria ser no-op: %q", got)
	}
}
