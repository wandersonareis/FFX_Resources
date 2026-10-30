package csv

import (
	"strings"
	"testing"

	"ffxresources/backend/dto"
)

// Round-trip bulk: 2 entradas voltam com a chave do mapa = ID (como no
// JSON/DTO), key completa na Metadata e textos preservados.
func TestUnmarshalRoundTripBulk(t *testing.T) {
	c := dto.Collection{}
	one := dto.FileEntry{
		Metadata: dto.Metadata{Key: "ffx/event/obj_ps3/az/azit0000/azit0000.bin", ID: "azit0000"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Primeira frase do evento"},
				Hash: map[string]string{"us": "aaaa0000aaaa0000"}},
			{Index: 1, Name: "game",
				Text: map[string]string{"us": "Frase com, vírgula e \"aspas\""},
				Hash: map[string]string{"us": "bbbb1111bbbb1111"}},
		},
	}
	one.Metadata = one.Metadata.WithRowCount(len(one.Rows))
	two := dto.FileEntry{
		Metadata: dto.Metadata{Key: "ffx/menu/macrodic.dcp", ID: "chunk_07"},
		Rows: []dto.TextRow{
			{Index: 3, Name: "name_simplified",
				Text: map[string]string{"us": "你好世界"},
				Hash: map[string]string{"us": "cccc2222cccc2222"}},
		},
	}
	two.Metadata = two.Metadata.WithRowCount(len(two.Rows))
	c["azit0000"] = one
	c["chunk_07"] = two

	raw, err := NewCSVFormatter().Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), "ffx/event/obj_ps3/az/azit0000/azit0000.bin") {
		t.Fatal("coluna key deveria carregar a metadata.key completa")
	}
	// O hash de artefato sai no formato do pacote hash: "$"+hex.
	if !strings.Contains(string(raw), "$aaaa0000aaaa0000") {
		t.Fatal("coluna hash deveria carregar o hash com prefixo $")
	}
	// O mesmo no modo sem dedup (o hash de identidade nunca sai).
	noDedupRaw, err := CSVFormatterNoDedup{}.Marshal(c)
	if err != nil {
		t.Fatalf("marshal no-dedup: %v", err)
	}
	if !strings.Contains(string(noDedupRaw), "$cccc2222cccc2222") {
		t.Fatal("coluna hash (no-dedup) deveria carregar o hash com prefixo $")
	}

	got, err := NewCSVFormatter().Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entradas: %d, esperado 2", len(got))
	}
	for id, want := range c {
		entry, ok := got[id]
		if !ok {
			t.Fatalf("entrada %s sumiu (chave do mapa deveria ser o id)", id)
		}
		if entry.Metadata.Key != want.Metadata.Key {
			t.Errorf("%s: key %q != %q", id, entry.Metadata.Key, want.Metadata.Key)
		}
		for i, row := range want.Rows {
			for lang, text := range row.Text {
				if got := entry.Rows[i].Text[lang]; got != text {
					t.Errorf("%s row %d %s: %q != %q", id, i, lang, got, text)
				}
				// O DTO volta com hex PURO — o $ do artefato morre no parse.
				if got := entry.Rows[i].Hash[lang]; got != row.Hash[lang] {
					t.Errorf("%s row %d %s: hash %q != %q (DTO deveria ser bare)", id, i, lang, got, row.Hash[lang])
				}
			}
		}
	}
}

// Linha sem key é rejeitada com instrução clara (formato antigo não tem
// compat: nada de adapter).
func TestUnmarshalRejeitaLinhaSemKey(t *testing.T) {
	raw := "key,id,name,index,lang,hash,text,source,target,divergent\n" +
		",azit0000,,0,us,aaaa0000aaaa0000,Texto sem key,,,false\n"
	if _, err := NewCSVFormatter().Unmarshal([]byte(raw)); err == nil ||
		!strings.Contains(err.Error(), "metadata.key") {
		t.Fatalf("esperava erro citando metadata.key, veio: %v", err)
	}
}

// Política estrita do import: hash sem o prefixo $ é artefato do formato
// antigo — erro claro mandando re-exportar (a mesma régua do .strings).
func TestUnmarshalRejeitaHashSemPrefixo(t *testing.T) {
	raw := "key,id,name,index,lang,hash,text,source,target,divergent\n" +
		"ffx/event/obj_ps3/az/azit0000/azit0000.bin,azit0000,,0,us,aaaa0000aaaa0000,Texto,,,false\n"
	if _, err := NewCSVFormatter().Unmarshal([]byte(raw)); err == nil ||
		!strings.Contains(err.Error(), "prefixo $") {
		t.Fatalf("esperava erro citando prefixo $, veio: %v", err)
	}
}

// Célula de hash vazia é tolerada: artefato feito à mão — o import recalcula
// o hash pelo texto.
func TestUnmarshalToleraHashVazio(t *testing.T) {
	raw := "key,id,name,index,lang,hash,text,source,target,divergent\n" +
		"ffx/event/obj_ps3/az/azit0000/azit0000.bin,azit0000,,0,us,,Texto feito à mão,Texto feito à mão,Texto feito à mão,false\n"
	got, err := NewCSVFormatter().Unmarshal([]byte(raw))
	if err != nil {
		t.Fatalf("hash vazio deveria ser tolerado: %v", err)
	}
	entry, ok := got["azit0000"]
	if !ok || len(entry.Rows) != 1 {
		t.Fatalf("row sumiu no import de artefato manual: %+v", got)
	}
	if got := entry.Rows[0].Text["us"]; got != "Texto feito à mão" {
		t.Errorf("texto %q != %q", got, "Texto feito à mão")
	}
	if got := entry.Rows[0].Hash["us"]; got != "" {
		t.Errorf("hash deveria voltar vazio para o serviço recalcular, veio %q", got)
	}
	if got := entry.Rows[0].Source; got != "Texto feito à mão" {
		t.Errorf("source %q != %q", got, "Texto feito à mão")
	}
	if got := entry.Rows[0].Target; got != "Texto feito à mão" {
		t.Errorf("target %q != %q", got, "Texto feito à mão")
	}
}
