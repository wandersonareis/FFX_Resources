package builders_test

import (
	"testing"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
)

// entryWith monta uma FileEntry com as rows dadas (Index/Name/Text us).
func entryWith(rows ...dto.TextRow) dto.FileEntry {
	return dto.FileEntry{
		Metadata: dto.NewMacroMetadata(common.GameVersionFFX2, 7).WithRowCount(len(rows)),
		Rows:     rows,
	}
}

func row(index int, name, text string) dto.TextRow {
	return dto.TextRow{Index: index, Name: name, Text: map[string]string{"us": text}}
}

func collection(id string, entry dto.FileEntry) dto.Collection {
	return dto.Collection{id: entry}
}

// O caso do dicionário: o mods tem rows "name_simplified" que o original não
// tem (ponteiro simplified errado no arquivo traduzido). A poda remove só
// essas linhas e mantém TODAS as do original.
func TestPruneNotInOriginalRemoveSoAsOrfas(t *testing.T) {
	original := collection("chunk_07", entryWith(
		row(108, "name", "Besaid Aurochs"),
		row(109, "name", "Zanarkand Abes"),
		row(110, "name", "--------"),
	))
	entrada := collection("chunk_07", entryWith(
		row(108, "name", "Besaid Aurochs"),
		row(108, "name_simplified", "esaid Aurochs"),
		row(109, "name", "Zanarkand Abes"),
		row(109, "name_simplified", "anarkand Abes"),
		row(110, "name", "--------"),
		row(110, "name_simplified", "-------"),
	))

	out, pruned, err := builders.PruneNotInOriginal(entrada, original)
	if err != nil {
		t.Fatalf("poda não deveria falhar: %v", err)
	}
	rows := out["chunk_07"].Rows
	if len(rows) != 3 {
		t.Fatalf("esperava 3 linhas (as do original), veio %d", len(rows))
	}
	for _, r := range rows {
		if r.Name == "name_simplified" {
			t.Fatalf("linha órfã sobreviveu: %+v", r)
		}
	}
	if len(pruned["chunk_07"]) != 3 {
		t.Fatalf("esperava 3 linhas podadas, veio %d", len(pruned["chunk_07"]))
	}
	if got := pruned["chunk_07"][1]; got.Index != 109 || got.Name != "name_simplified" || got.TextOf("us") != "anarkand Abes" {
		t.Fatalf("relatório da poda sem o texto completo: %+v", got)
	}
	if out["chunk_07"].Metadata.RowCount != 3 {
		t.Fatalf("metadata.rowCount devia acompanhar a poda: %d", out["chunk_07"].Metadata.RowCount)
	}
}

// Guarda: o binário não aceita menos linhas que o original. Se o rascunho
// perdeu uma linha do original, a poda aborta (nada é gravado).
func TestPruneNotInOriginalRecusaPerderLinhaDoOriginal(t *testing.T) {
	original := collection("chunk_07", entryWith(
		row(108, "name", "Besaid Aurochs"),
		row(109, "name", "Zanarkand Abes"),
	))
	entrada := collection("chunk_07", entryWith(
		row(108, "name", "Besaid Aurochs"),
		row(108, "name_simplified", "esaid Aurochs"),
	))

	if _, _, err := builders.PruneNotInOriginal(entrada, original); err == nil {
		t.Fatal("esperava erro: a linha 109 do original sumiu do rascunho")
	}
}

// Linha que existe no original SÓ em outro idioma: a linha fica, mas o texto
// do idioma sem par sai (o caso do "simplified" no us onde só o de o tem).
func TestPruneNotInOriginalPorIdioma(t *testing.T) {
	original := collection("chunk_07", entryWith(
		dto.TextRow{Index: 1, Name: "name_simplified", Text: map[string]string{"de": "Kurz"}},
	))
	entrada := collection("chunk_07", entryWith(
		dto.TextRow{
			Index: 1,
			Name:  "name_simplified",
			Text:  map[string]string{"de": "Kurz", "us": "anarkand Abes"},
		},
	))

	out, pruned, err := builders.PruneNotInOriginal(entrada, original)
	if err != nil {
		t.Fatalf("poda não deveria falhar: %v", err)
	}
	rows := out["chunk_07"].Rows
	if len(rows) != 1 {
		t.Fatalf("a linha do original não pode sair: %d", len(rows))
	}
	if _, ok := rows[0].Text["us"]; ok {
		t.Fatalf("texto us sem par devia sair: %+v", rows[0].Text)
	}
	if rows[0].Text["de"] != "Kurz" {
		t.Fatalf("texto de (existe no original) devia ficar: %+v", rows[0].Text)
	}
	if len(pruned["chunk_07"]) != 1 {
		t.Fatalf("esperava 1 idioma podado, veio %d", len(pruned["chunk_07"]))
	}
	got := pruned["chunk_07"][0]
	if got.TextOf("us") != "anarkand Abes" || len(got.Langs) != 1 || got.Langs[0] != "us" {
		t.Fatalf("relatório por idioma incorreto: %+v", got)
	}
}

// Sem original (arquivo só em mods ou fora de data/) nada é podado nem
// rejeitado: não há com o que comparar.
func TestPruneNotInOriginalSemOriginal(t *testing.T) {
	entrada := collection("chunk_07", entryWith(row(1, "name", "x"), row(1, "name_simplified", "y")))

	out, pruned, err := builders.PruneNotInOriginal(entrada, dto.Collection{})
	if err != nil {
		t.Fatalf("sem original não deve falhar: %v", err)
	}
	if len(out["chunk_07"].Rows) != 2 || len(pruned) != 0 {
		t.Fatalf("sem original a entrada deve passar intacta: rows=%d pruned=%d", len(out["chunk_07"].Rows), len(pruned))
	}
}
