package builders_test

import (
	"testing"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
)

// refOf monta a row de REF dedupada do display: ponteiro do ORIGINAL da def
// (Hash[us]) e texto "$<ponteiro>".
func applyRefOf(index int, pointer string) dto.TextRow {
	return dto.TextRow{
		Index: index,
		Hash:  map[string]string{common.DefaultLocalization: pointer},
		Text:  map[string]string{common.DefaultLocalization: hash.Prefix(pointer)},
	}
}

// literalOf monta a row LITERAL (def): o hash é o do próprio texto.
func applyLiteralOf(index int, text string) dto.TextRow {
	return dto.TextRow{
		Index: index,
		Hash:  map[string]string{common.DefaultLocalization: hash.Sum64Hex(text)},
		Text:  map[string]string{common.DefaultLocalization: text},
	}
}

// ResolveApplyRefs expande a ref do lote para o literal da def e DESCARTA a
// ref órfã (sem def no lote): o applier nunca pode gravar "$hash" no binário.
func TestResolveApplyRefsExpandsAndDropsOrphans(t *testing.T) {
	defText := "Insight: o texto original compartilhado"
	defHash := hash.Sum64Hex(defText)
	const literal = "Texto próprio, não repetido"

	c := dto.Collection{
		"aaa": {
			Metadata: dto.Metadata{ID: "aaa"},
			Rows: []dto.TextRow{
				applyLiteralOf(0, defText),
				applyRefOf(1, defHash),
				applyLiteralOf(2, literal),
				applyRefOf(3, "deadbeefdeadbeef"), // órfã: ponteiro sem def no lote
			},
		},
		"bbb": {
			Metadata: dto.Metadata{ID: "bbb"},
			Rows:     []dto.TextRow{applyRefOf(0, "cafecafecafecafe")}, // entrada só com órfã
		},
	}

	out := builders.ResolveApplyRefs(c)

	entry, ok := out["aaa"]
	if !ok {
		t.Fatalf("entrada aaa sumiu: %v", out.SortedKeys())
	}
	if len(entry.Rows) != 3 {
		t.Fatalf("rows da aaa: %d, queria 3 (a órfã sai do lote)", len(entry.Rows))
	}
	if got := entry.Rows[1].Text[common.DefaultLocalization]; got != defText {
		t.Fatalf("ref não expandiu para o literal da def: %q", got)
	}
	if got := entry.Rows[1].Hash[common.DefaultLocalization]; got != defHash {
		t.Fatalf("o ponteiro da row não pode mudar no apply: %q", got)
	}
	if got := entry.Rows[2].Text[common.DefaultLocalization]; got != literal {
		t.Fatalf("literal alterado: %q", got)
	}
	if _, ok := out["bbb"]; ok {
		t.Fatal("entrada sem nada a aplicar não pode entrar no lote")
	}
}

// ResolveApplyRefs não muta a Collection de entrada (o DTO é do rascunho).
func TestResolveApplyRefsDoesNotMutateInput(t *testing.T) {
	defText := "Insight: o texto original compartilhado"
	defHash := hash.Sum64Hex(defText)
	c := dto.Collection{
		"aaa": {Rows: []dto.TextRow{applyLiteralOf(0, defText), applyRefOf(1, defHash)}},
	}

	_ = builders.ResolveApplyRefs(c)

	if got := c["aaa"].Rows[1].Text[common.DefaultLocalization]; got != hash.Prefix(defHash) {
		t.Fatalf("entrada mutada: %q", got)
	}
}

// A def DIVERGENTE não vira referência canônica: a ref continua órfã e sai do
// lote (mesma régua do ResolveDedupRefs).
func TestResolveApplyRefsDivergentDefIsNotCanonical(t *testing.T) {
	defText := "Insight: o texto original compartilhado"
	defHash := hash.Sum64Hex(defText)
	def := applyLiteralOf(0, defText)
	def.Divergent = true

	c := dto.Collection{
		"aaa": {Rows: []dto.TextRow{def, applyRefOf(1, defHash)}},
	}

	out := builders.ResolveApplyRefs(c)
	entry, ok := out["aaa"]
	if !ok {
		t.Fatalf("a def literal deveria ficar no lote: %v", out.SortedKeys())
	}
	if len(entry.Rows) != 1 {
		t.Fatalf("rows: %d, queria só a def (a ref órfã sai)", len(entry.Rows))
	}
}
