package services

import (
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
)

// Matriz do casamento por hash (Passo 1 do plano):
// N7-M  hash nunca é reescrito no merge,
// N8    index trocado ainda casa (grupo de 1),
// N11   artefato velho ⇒ inserted 0,
// N12   1 traduzida + cópias ⇒ só a célula certa,
// N12b  célula de grupo N já reivindicada ⇒ unmatched,
// N13/N14 row em branco não é candidata nem loga,
// N15   filtro de export pelo padrão 'us' (pt tem texto, us vazio ⇒ sai),
// N16   RowCount recalculado no filtro,
// N17   import parcial (menos textos que o total),
// N18   ordem trocada,
// N-hf  hash não encontrado é log, não erro do resumo.

var hTestMeta = func() dto.Metadata { return dto.Metadata{Key: "ffx/battle/kernel/btl_txt.bin", ID: "btl"} }

func hRow(index int, name, text string) dto.TextRow {
	return dto.TextRow{
		Index: index,
		Name:  name,
		Hash:  map[string]string{common.DefaultLocalization: hash.Sum64Hex(text)},
		Text:  map[string]string{common.DefaultLocalization: text},
	}
}

// hEditRow simula a row editada por um tradutor: o TEXTO muda, o hash fica
// o do texto vigente no export (staleness token) — é o hash que casa com a
// store, nunca um derivado do texto editado.
func hEditRow(index int, name, exportText, newText string) dto.TextRow {
	r := hRow(index, name, exportText)
	r.Text[common.DefaultLocalization] = newText
	return r
}

// matchSimple casamento sem validação de tags (referência nil).
func matchSimple(imp, se dto.FileEntry) importMatch {
	return matchImportEntry(imp, se, nil)
}

// store de 5 rows: 0 e 2 em branco ("-", texto próprio do jogo), 1 e 3 com
// texto, sendo o texto de 1 duplicado na 3 (dedup ⇒ mesmo hash).
func hTestStore() dto.FileEntry {
	return dto.FileEntry{
		Metadata: hTestMeta(),
		Rows: []dto.TextRow{
			hRow(0, "", "-"),
			hRow(1, "a", "Alpha line"),
			hRow(2, "", ""),
			hRow(3, "b", "Alpha line"),
			hRow(4, "c", "Bravo line"),
		},
	}
}

func TestHashMatchSwappedOrder(t *testing.T) {
	store := hTestStore()
	// Artefato com ordem trocada: [4, 3, 1] (e blank espremido no meio).
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(4, "c", "Bravo line", "Bravo traduzido"),
		hRow(1, "a", "Alpha line"),
		{Index: 2, Text: map[string]string{common.DefaultLocalization: ""}},
		hEditRow(3, "b", "Alpha line", "Beta agora"),
	}}

	m := matchSimple(art, store)

	if len(m.unmatched) != 0 {
		t.Fatalf("ordem trocada não pode gerar unmatched: %d", len(m.unmatched))
	}
	if m.candidates != 3 || m.inserted != 3 {
		t.Fatalf("candidates=%d inserted=%d, queria 3 e 3", m.candidates, m.inserted)
	}
	if m.changed != 2 {
		t.Fatalf("changed=%d, queria 2 (rows 4 e 3 mudaram)", m.changed)
	}
}

func TestHashMatchPartialImport(t *testing.T) {
	store := hTestStore()
	// Import parcial: só 1 das 3 rows traduzíveis vai no arquivo.
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(4, "c", "Bravo line", "Bravo traduzido"),
	}}

	m := matchSimple(art, store)

	if len(m.unmatched) != 0 {
		t.Fatalf("parcial não pode gerar unmatched: %v", m.unmatched)
	}
	if m.candidates != 1 || m.inserted != 1 || m.changed != 1 {
		t.Fatalf("candidates=%d inserted=%d changed=%d, queria 1/1/1",
			m.candidates, m.inserted, m.changed)
	}
	// A posição vem sempre da store: par casa a row (4,"c") da store.
	if len(m.pairs) != 1 || m.pairs[0].store.Index != 4 || m.pairs[0].store.Name != "c" {
		t.Fatalf("par errado: %+v", m.pairs)
	}
}

func TestHashMatchNotFoundLogsNotErrors(t *testing.T) {
	store := hTestStore()
	// Hash que não existe na store (texto mudou lá desde o export): a row
	// traz hash estranho e texto estranho — nada casa.
	artifact := hRow(1, "a", "Texto que ninguém tem mais")
	artifact.Hash[common.DefaultLocalization] = "0123456789abcdef"

	m := matchSimple(dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{artifact}}, store)

	if len(m.unmatched) != 1 {
		t.Fatalf("hash estranho deve ser unmatched: %d", len(m.unmatched))
	}
	if m.inserted != 0 || m.changed != 0 {
		t.Fatalf("inserted=%d changed=%d, queria 0 e 0", m.inserted, m.changed)
	}
	// e NÃO é erro de resumo: quem bloqueia é o revisor, não o modal.
	// (a política de avisos/exclusão por entrada está em
	// TestImportE2EUnknownEntry e TestImportE2EOverLimitEntrySkipped.)
}

func TestHashMatchDedupGroupTiebreak(t *testing.T) {
	store := hTestStore()
	// "Alpha line" está nas rows 1 e 3 (mesmo hash). O artefato traduz
	// APENAS a 3: grupo N, desempate pela coordenada (3,b).
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(3, "b", "Alpha line", "Alpha TRADUZIDA"),
		hRow(1, "a", "Alpha line"),
	}}

	m := matchSimple(art, store)

	if len(m.unmatched) != 0 || m.changed != 1 {
		t.Fatalf("unmatched=%d changed=%d, queria 0 e 1", len(m.unmatched), m.changed)
	}
	if len(m.pairs) != 2 {
		t.Fatalf("pairs=%d, queria 2", len(m.pairs))
	}
	for _, p := range m.pairs {
		if p.store.Name != p.imported.Name {
			t.Fatalf("par cruzado: store %q vs imported %q", p.store.Name, p.imported.Name)
		}
	}
}

func TestHashMatchClaimedCellUnmatched(t *testing.T) {
	store := hTestStore()
	// Duas rows do artefato com o MESMO hash do grupo único (1,a): a segunda
	// não tem célula livre ⇒ unmatched.
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(1, "a", "Alpha line", "Alpha v1"),
		hEditRow(1, "a", "Alpha line", "Alpha v2"),
	}}

	m := matchSimple(art, store)

	if m.inserted != 1 || len(m.unmatched) != 1 {
		t.Fatalf("inserted=%d unmatched=%d, queria 1 e 1", m.inserted, len(m.unmatched))
	}
}

func TestMergeImportUsHashNeverRewritten(t *testing.T) {
	store := hTestStore()
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(4, "c", "Bravo line", "Bravo traduzido"),
	}}

	m := matchSimple(art, store)
	merged := mergeImportUs(dto.Collection{"btl": store},
		map[string][]rowPair{"btl": m.pairs})

	// A posição da store é preservada (todas as 5 rows, na ordem)...
	me := merged["btl"]
	if len(me.Rows) != 5 || me.Rows[4].Index != 4 || me.Rows[1].Index != 1 {
		t.Fatalf("store alterada no merge: %+v", me.Rows)
	}
	// ...o texto editado entrou SEM alterar idioma/hash de célula nenhuma.
	if me.Rows[4].Text[common.DefaultLocalization] != "Bravo traduzido" {
		t.Fatalf("texto novo não chegou: %q", me.Rows[4].Text[common.DefaultLocalization])
	}
	want := hash.Sum64Hex("Alpha line")
	if me.Rows[1].Hash[common.DefaultLocalization] != want {
		t.Fatalf("hash de célula não-editada reescrito: %q", me.Rows[1].Hash[common.DefaultLocalization])
	}
	if me.Rows[4].Hash[common.DefaultLocalization] != hash.Sum64Hex("Bravo line") {
		t.Fatalf("hash da célula editada reescrito — não devia")
	}
}

func TestMergeImportUsPreservesInferredDivergence(t *testing.T) {
	store := hTestStore()
	imported := hEditRow(3, "b", "Alpha line", "Beta for this copy")
	imported.Divergent = true
	merged := mergeImportUs(dto.Collection{"btl": store}, map[string][]rowPair{
		"btl": {{store: store.Rows[3], imported: imported}},
	})
	row := merged["btl"].Rows[3]
	if !row.Divergent {
		t.Fatal("merge import must preserve the parser's divergent-copy marker")
	}
	if got := row.Text[common.DefaultLocalization]; got != "Beta for this copy" {
		t.Fatalf("merged copy text: %q", got)
	}
}

func TestBlankImportRowNeverCandidate(t *testing.T) {
	store := hTestStore()
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		{Index: 9, Name: "zz", Hash: map[string]string{common.DefaultLocalization: "-"}, Text: map[string]string{common.DefaultLocalization: "-"}},
		hEditRow(4, "c", "Bravo line", "Bravo traduzido"),
	}}

	m := matchSimple(art, store)

	if m.candidates != 1 || len(m.unmatched) != 0 {
		t.Fatalf("candidates=%d unmatched=%d, queria 1 e 0 (blank nunca é candidato nem loga)",
			m.candidates, len(m.unmatched))
	}
}

func TestFilterExportRowsUsOnly(t *testing.T) {
	c := dto.Collection{
		"btl": dto.FileEntry{
			Metadata: dto.Metadata{Key: "ffx/battle/kernel/btl_txt.bin", ID: "btl", RowCount: 5},
			Rows: []dto.TextRow{
				// us vazio, pt preenchido ⇒ SAI (padrão 'us'; sem 'us' os
				// outros idiomas não interessam).
				{Index: 0, Text: map[string]string{"pt": "tradução", common.DefaultLocalization: ""}},
				hRow(1, "", "-"),
				{Index: 2, Text: map[string]string{common.DefaultLocalization: "   "}},
				hRow(3, "", "Texto real"),
				hRow(4, "", "  -  "), // "-" após trim ⇒ blank
			},
		},
		"vazia": dto.FileEntry{
			Metadata: dto.Metadata{Key: "ffx/x", ID: "vazia", RowCount: 1},
			Rows:     []dto.TextRow{hRow(0, "", "-")},
		},
	}

	out := filterExportRows(c)

	if _, ok := out["vazia"]; ok {
		t.Fatal("entrada só com row em branco deve sair do artefato")
	}
	e := out["btl"]
	if len(e.Rows) != 1 || e.Rows[0].Index != 3 {
		t.Fatalf("filtro errado: %+v", e.Rows)
	}
	if e.Metadata.RowCount != 1 {
		t.Fatalf("RowCount=%d, queria 1 (recalculado)", e.Metadata.RowCount)
	}
}

func TestMatchTagLostRejectedFromPairs(t *testing.T) {
	store := hTestStore()
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(1, "a", "Alpha line", "Alpha {PAUSE} line"),
		hEditRow(4, "c", "Bravo line", "Bravo {PAUSE} traduzido"),
	}}
	// original de data/: row 1 tem PAUSE no original; row 4 não.
	origByText := map[string]string{
		binaryRowKey(store.Rows[1]): "Alpha {PAUSE} line", // tags preservadas ⇒ ok
		binaryRowKey(store.Rows[4]): "Bravo line",         // tradução criou PAUSE ⇒ sobra
	}

	m := matchImportEntry(art, store, func() map[string]string { return origByText })

	if m.inserted != 1 || len(m.tagLost) != 1 {
		t.Fatalf("inserted=%d tagLost=%d, queria 1 e 1", m.inserted, len(m.tagLost))
	}
	if m.changed != 1 || len(m.pairs) != 1 || m.pairs[0].store.Index != 1 {
		t.Fatalf("changed=%d pairs=%+v, queria 1 e só a row 1", m.changed, m.pairs)
	}
	if got := m.tagLost[0].issues[0].Content; got != "PAUSE" {
		t.Fatalf("issue %q, queria PAUSE", got)
	}
	// text tags (cor/itálico/newline) NÃO são validadas.
	art2 := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(4, "c", "Bravo line", "Bravo {\\n} {CLR:0002} traduzido"),
	}}
	if m2 := matchImportEntry(art2, store, func() map[string]string { return origByText }); len(m2.tagLost) != 0 {
		t.Fatalf("tag de texto não pode ser validada: %+v", m2.tagLost)
	}
}

// Sem original (data/ ausente, nil) a validação de tags é pulada e a row
// idêntica à store (texto puro) casa direto — sem falso positivo.
func TestMatchWithoutOriginalSkipsTags(t *testing.T) {
	m := matchImportEntry(
		dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
			hEditRow(1, "a", "Alpha line", "Alpha {PAUSE}"),
		}},
		dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{hRow(1, "a", "Alpha line")}},
		nil,
	)
	if len(m.tagLost) != 0 || m.inserted != 1 || m.changed != 1 {
		t.Fatalf("sem original: tagLost=%d inserted=%d changed=%d, queria 0/1/1",
			len(m.tagLost), m.inserted, m.changed)
	}
}
