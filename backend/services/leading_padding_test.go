package services

import (
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
)

// Espelho dos casos de leading-padding.test.ts (frontend) — as duas
// implementações devem concordar 1:1, é o mesmo "padding de tela".
func TestSplitLeadingPaddingMirrorsFrontend(t *testing.T) {
	cases := []struct {
		raw              string
		wantPad, wantBod string
	}{
		{"Hello", "", "Hello"},
		{"{TEXT_NEWLINE}Hello", "{TEXT_NEWLINE}", "Hello"},
		{"{TEXT_NEWLINE} {TEXT_NEWLINE}\n  Hello", "{TEXT_NEWLINE} {TEXT_NEWLINE}\n  ", "Hello"},
		{"{text_newline}Hi", "{text_newline}", "Hi"},
		{"{TIME:00}{TEXT_NEWLINE}Hi", "", "{TIME:00}{TEXT_NEWLINE}Hi"},
		{"", "", ""},
	}
	for _, c := range cases {
		pad, bod := splitLeadingPadding(c.raw)
		if pad != c.wantPad || bod != c.wantBod {
			t.Fatalf("splitLeadingPadding(%q) = (%q, %q), queria (%q, %q)",
				c.raw, pad, bod, c.wantPad, c.wantBod)
		}
	}
}

// A régua do import: o padding da STORE vence (com menos OU com mais padding
// no import) e o texto importado vence integral quando a store não tem
// padding nenhum. Nunca consulta o original.
func TestKeepStorePaddingStoreWins(t *testing.T) {
	cases := []struct {
		name, store, imported, want string
	}{
		{
			// Import com MENOS padding (nenhum): o da store se mantém.
			name:     "import com menos padding",
			store:    "{TEXT_NEWLINE}  Alpha line",
			imported: "Alfa linha",
			want:     "{TEXT_NEWLINE}  Alfa linha",
		},
		{
			// Import com MAIS padding: o da store se mantém, o excedente cai.
			name:     "import com mais padding",
			store:    "  Alpha",
			imported: "{TEXT_NEWLINE}{TEXT_NEWLINE}   Alfa",
			want:     "  Alfa",
		},
		{
			// Store SEM padding: o texto importado vence integral — não há
			// comparação com o original nem normalização.
			name:     "store sem padding: texto vence",
			store:    "Alpha line",
			imported: "{TEXT_NEWLINE}  Alfa linha",
			want:     "{TEXT_NEWLINE}  Alfa linha",
		},
		{
			// {TIME:00} não é padding de tela: store "com prefixo" mas sem
			// padding ⇒ texto importado vence.
			name:     "TIME não é padding",
			store:    "{TIME:00}Alpha",
			imported: "Alfa",
			want:     "Alfa",
		},
		{
			// Padding da store em caixa baixa também conta (case-insensitive).
			name:     "padding case-insensitive",
			store:    "{text_newline} Alpha",
			imported: "Alfa",
			want:     "{text_newline} Alfa",
		},
		{
			// Row real do now_help_page:76 — 12 espaços de recuo de tela.
			name:     "recuo de 12 espaços (now_help_page:76)",
			store:    "            When you want a character",
			imported: "Quando você quiser que o personagem",
			want:     "            Quando você quiser que o personagem",
		},
		{
			// Store vazia (célula sem us): não há padding, texto vence.
			name:     "store vazia",
			store:    "",
			imported: "{TEXT_NEWLINE} Alfa",
			want:     "{TEXT_NEWLINE} Alfa",
		},
	}
	for _, c := range cases {
		if got := keepStorePadding(c.store, c.imported); got != c.want {
			t.Fatalf("%s: keepStorePadding(%q, %q) = %q, queria %q",
				c.name, c.store, c.imported, got, c.want)
		}
	}
}

// O casamento por hash já devolve o par com o texto reconciliado: o merge
// grava o texto novo COM o padding da store, e diff só de padding não
// conta como texto alterado.
func TestImportMergeKeepsStorePadding(t *testing.T) {
	store := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", "            When you want a character"),
	}}
	// Artefato do tradutor externo: hash = texto vigente na store (staleness
	// token), texto novo SEM o recuo de tela.
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(0, "", "            When you want a character", "Quando você quiser que o personagem"),
	}}

	m := matchSimple(art, store)

	if len(m.pairs) != 1 || m.unmatched != nil {
		t.Fatalf("pairs=%d unmatched=%d, queria 1 e 0", len(m.pairs), len(m.unmatched))
	}
	if got := usText(m.pairs[0].imported); got != "            Quando você quiser que o personagem" {
		t.Fatalf("par sem o padding da store: %q", got)
	}
	if m.changed != 1 {
		t.Fatalf("changed=%d, queria 1 (o corpo mudou)", m.changed)
	}

	merged := mergeImportUs(dto.Collection{"btl": store}, map[string][]rowPair{"btl": m.pairs})
	if got := merged["btl"].Rows[0].Text[common.DefaultLocalization]; got != "            Quando você quiser que o personagem" {
		t.Fatalf("merge gravou sem padding: %q", got)
	}
}

// Diff SÓ de padding não é alteração: o import que perdeu o padding devolve
// exatamente o texto da store — changed 0 e merge no-op.
func TestImportMergePaddingOnlyDiffIsNotChanged(t *testing.T) {
	store := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", "{TEXT_NEWLINE}  Oi"),
	}}
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(0, "", "{TEXT_NEWLINE}  Oi", "Oi"),
	}}

	m := matchSimple(art, store)

	if m.changed != 0 {
		t.Fatalf("changed=%d, queria 0 (reconciliado == store)", m.changed)
	}
	merged := mergeImportUs(dto.Collection{"btl": store}, map[string][]rowPair{"btl": m.pairs})
	if got := merged["btl"].Rows[0].Text[common.DefaultLocalization]; got != "{TEXT_NEWLINE}  Oi" {
		t.Fatalf("merge mudou texto só de padding: %q", got)
	}
}

// Store sem padding ⇒ o texto importado vence integral (com o padding que
// ele trouxer): é assim que o recuo perdido se RECUPERA por re-import.
func TestImportMergeTextWinsWithoutStorePadding(t *testing.T) {
	store := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", "Quando você quiser que o personagem"),
	}}
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(0, "", "Quando você quiser que o personagem", "            Quando você quiser que o personagem"),
	}}

	m := matchSimple(art, store)

	if got := usText(m.pairs[0].imported); got != "            Quando você quiser que o personagem" {
		t.Fatalf("padding do import foi descartado sem padding na store: %q", got)
	}
	merged := mergeImportUs(dto.Collection{"btl": store}, map[string][]rowPair{"btl": m.pairs})
	if got := merged["btl"].Rows[0].Text[common.DefaultLocalization]; got != "            Quando você quiser que o personagem" {
		t.Fatalf("merge descartou o padding do import: %q", got)
	}
}

// A reconciliação não pode vazar: o map Text da row do artefato é
// compartilhado com o parse — mutar em lugar seria corrupção de dado.
func TestReconcileImportPaddingDoesNotMutateArtifact(t *testing.T) {
	storeRow := hRow(0, "", "{TEXT_NEWLINE}  Alpha")
	artifactRow := hEditRow(0, "", "{TEXT_NEWLINE}  Alpha", "Alfa")
	before := artifactRow.Text[common.DefaultLocalization]

	out := reconcileImportPadding(storeRow, artifactRow)

	if out.Text[common.DefaultLocalization] != "{TEXT_NEWLINE}  Alfa" {
		t.Fatalf("reconciliação errada: %q", out.Text[common.DefaultLocalization])
	}
	if got := artifactRow.Text[common.DefaultLocalization]; got != before {
		t.Fatalf("row do artefato foi mutada: %q (antes %q)", got, before)
	}
	if len(out.Text) != len(artifactRow.Text) {
		t.Fatalf("map copiado perdeu idiomas: %v vs %v", out.Text, artifactRow.Text)
	}
}
