package services

import (
	"testing"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
)

// Matriz do casamento do import (formato CAT: source/target):
//
//	N7-M  hash nunca é reescrito no merge,
//	N8    ordem trocada no artefato não muda a célula,
//	N11   coordenada ausente na store ⇒ unmatched (log, não erro),
//	N12   dedup: cópias do mesmo texto são células distintas (coordenada),
//	N12b  coordenada repetida no artefato ⇒ unmatched na segunda,
//	N13   row sem target não é candidata nem loga,
//	N15   filtro de export pelo padrão 'us' OU target preenchido,
//	N16   RowCount recalculado no filtro,
//	N17   import parcial (menos textos que o total),
//	N18   ordem trocada,
//	N-hf  origem divergente não é erro do resumo.

var hTestMeta = func() dto.Metadata { return dto.Metadata{Key: "ffx/battle/kernel/btl_txt.bin", ID: "btl"} }

// hRow é uma row da STORE: o texto atual do binário.
func hRow(index int, name, text string) dto.TextRow {
	return dto.TextRow{
		Index: index,
		Name:  name,
		Hash:  map[string]string{common.DefaultLocalization: hash.Sum64Hex(text)},
		Text:  map[string]string{common.DefaultLocalization: text},
	}
}

// hEditRow é uma row do ARTEFATO no formato CAT: text/source carregam a
// referência (o texto de data/ no export) e target é a tradução — o ÚNICO
// texto que o import aplica.
func hEditRow(index int, name, exportText, newText string) dto.TextRow {
	r := hRow(index, name, exportText)
	r.Source = exportText
	r.Target = newText
	return r
}

// matchSimple casa sem referência de data/ (a checagem de origem/tags é
// pulada — é a degradação quando data/ não está disponível).
func matchSimple(imp, se dto.FileEntry) importMatch {
	return matchImportEntry(imp, se, nil)
}

// matchWithOriginal casa validando a origem (hash do `us` de data/).
func matchWithOriginal(imp, se, orig dto.FileEntry) importMatch {
	return matchImportEntry(imp, se, &orig)
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

// hTestOrig é a referência de data/ da store acima (o `us` original).
func hTestOrig() dto.FileEntry {
	return hTestStore()
}

// catArtifact projeta uma collection da store para o formato de artefato
// (source/target preenchidos) — o export de uma árvore sem mods/.
func catArtifact(c dto.Collection) dto.Collection {
	return builders.ProjectSourceTarget(c, c)
}

func TestMatchRowsSwappedOrder(t *testing.T) {
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
	if m.candidates != 2 || m.inserted != 2 {
		t.Fatalf("candidates=%d inserted=%d, queria 2 e 2 (só rows com target)", m.candidates, m.inserted)
	}
	if m.changed != 2 {
		t.Fatalf("changed=%d, queria 2 (rows 4 e 3 mudaram)", m.changed)
	}
}

func TestMatchRowsPartialImport(t *testing.T) {
	store := hTestStore()
	// Import parcial: só 1 das rows traduzíveis vai no arquivo.
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

func TestMatchRowsMissingCoordinateUnmatched(t *testing.T) {
	store := hTestStore()
	// Coordenada que não existe na store (binário mudou desde o export).
	artifact := hEditRow(9, "zz", "Texto que ninguém tem mais", "Tradução órfã")

	m := matchSimple(dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{artifact}}, store)

	if len(m.unmatched) != 1 {
		t.Fatalf("coordenada ausente deve ser unmatched: %d", len(m.unmatched))
	}
	if m.inserted != 0 || m.changed != 0 {
		t.Fatalf("inserted=%d changed=%d, queria 0 e 0", m.inserted, m.changed)
	}
	// e NÃO é erro de resumo: quem bloqueia é o revisor, não o modal.
	// (a política de avisos/exclusão por entrada está em
	// TestImportE2EUnknownEntry e TestImportE2EOverLimitEntrySkipped.)
}

func TestMatchRowsSourceChangedUnmatched(t *testing.T) {
	store := hTestStore()
	orig := hTestOrig()
	// O segmento em data/ mudou depois do export: a row 1 agora é outro
	// texto; a 4 continua igual.
	orig.Rows[1] = hRow(1, "a", "Alpha line (revisada)")

	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(1, "a", "Alpha line", "Alpha traduzida"),
		hEditRow(4, "c", "Bravo line", "Bravo traduzido"),
	}}

	m := matchWithOriginal(art, store, orig)

	if m.inserted != 1 || len(m.unmatched) != 1 {
		t.Fatalf("inserted=%d unmatched=%d, queria 1 e 1", m.inserted, len(m.unmatched))
	}
	if m.pairs[0].store.Index != 4 {
		t.Fatalf("par errado: %+v", m.pairs)
	}
}

func TestMatchRowsDedupCopiesTargetOwnCoordinate(t *testing.T) {
	store := hTestStore()
	// "Alpha line" está nas rows 1 e 3 (mesmo hash). O artefato traduz
	// APENAS a 3: a coordenada diz exatamente qual célula recebe o texto.
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(3, "b", "Alpha line", "Alpha TRADUZIDA"),
		hEditRow(1, "a", "Alpha line", "Alpha TRADUZIDA"),
	}}

	m := matchSimple(art, store)

	if len(m.unmatched) != 0 || m.changed != 2 {
		t.Fatalf("unmatched=%d changed=%d, queria 0 e 2", len(m.unmatched), m.changed)
	}
	for _, p := range m.pairs {
		if p.store.Name != p.imported.Name {
			t.Fatalf("par cruzado: store %q vs imported %q", p.store.Name, p.imported.Name)
		}
	}
}

func TestMatchRowsClaimedCoordinateUnmatched(t *testing.T) {
	store := hTestStore()
	// Duas rows do artefato com a MESMA coordenada (1,a): a segunda não
	// sobrescreve a primeira.
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
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(3, "b", "Alpha line", "Beta for this copy"),
	}}
	art.Rows[0].Divergent = true

	m := matchSimple(art, store)
	merged := mergeImportUs(dto.Collection{"btl": store}, map[string][]rowPair{"btl": m.pairs})
	row := merged["btl"].Rows[3]
	if !row.Divergent {
		t.Fatal("merge import must preserve the parser's divergent-copy marker")
	}
	if got := row.Text[common.DefaultLocalization]; got != "Beta for this copy" {
		t.Fatalf("merged copy text: %q", got)
	}
}

func TestBlankTargetNeverCandidate(t *testing.T) {
	store := hTestStore()
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		// Sem target (segmento ainda não traduzido): não é candidata.
		{Index: 9, Name: "zz", Hash: map[string]string{common.DefaultLocalization: "-"}, Text: map[string]string{common.DefaultLocalization: "-"}},
		hEditRow(4, "c", "Bravo line", "Bravo traduzido"),
	}}

	m := matchSimple(art, store)

	if m.candidates != 1 || len(m.unmatched) != 0 {
		t.Fatalf("candidates=%d unmatched=%d, queria 1 e 0 (sem target nunca é candidata nem loga)",
			m.candidates, len(m.unmatched))
	}
}

func TestFilterExportRowsUsOrTarget(t *testing.T) {
	c := dto.Collection{
		"btl": dto.FileEntry{
			Metadata: dto.Metadata{Key: "ffx/battle/kernel/btl_txt.bin", ID: "btl", RowCount: 5},
			Rows: []dto.TextRow{
				// us vazio E target vazio ⇒ SAI (nada a importar).
				{Index: 0, Text: map[string]string{"pt": "tradução", common.DefaultLocalization: ""}},
				hRow(1, "", "-"),
				{Index: 2, Text: map[string]string{common.DefaultLocalization: "   "}},
				hRow(3, "", "Texto real"),
				hRow(4, "", "  -  "), // "-" após trim ⇒ blank
				// sem us, mas com target: FICA (tradução precisa voltar).
				{Index: 5, Target: "Tradução sem referência"},
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
	if len(e.Rows) != 2 || e.Rows[0].Index != 3 || e.Rows[1].Index != 5 {
		t.Fatalf("filtro errado: %+v", e.Rows)
	}
	if e.Metadata.RowCount != 2 {
		t.Fatalf("RowCount=%d, queria 2 (recalculado)", e.Metadata.RowCount)
	}
}

func TestMatchTagLostRejectedFromPairs(t *testing.T) {
	store := hTestStore()
	orig := hTestOrig()
	// row 1 tem PAUSE no original; row 4 não.
	orig.Rows[1] = hRow(1, "a", "Alpha {PAUSE} line")

	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hEditRow(1, "a", "Alpha {PAUSE} line", "Alpha {PAUSE} traduzida"),
		hEditRow(4, "c", "Bravo line", "Bravo {PAUSE} traduzido"),
	}}

	m := matchWithOriginal(art, store, orig)

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
	if m2 := matchWithOriginal(art2, store, orig); len(m2.tagLost) != 0 {
		t.Fatalf("tag de texto não pode ser validada: %+v", m2.tagLost)
	}
}

// Sem original (data/ ausente, nil) a validação é pulada e a row casa pela
// coordenada — sem falso positivo.
func TestMatchWithoutOriginalSkipsChecks(t *testing.T) {
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

// Re-import SEM edição: o target é o texto vigente da store, que tem uma tag
// de controle que o original EN não tem (MCR de speaker de uma tradução
// pré-existente). Como nada será gravado, a divergência não pode virar aviso
// de "tag de controle divergente" — é o caso do round-trip export→import.
func TestMatchNoOpRowSkipsTagValidation(t *testing.T) {
	original := "{TEXT_NEWLINE}Call any aeon you like. 44"
	translated := `{MCR:s06:l31:"Belgemine"}{TEXT_NEWLINE}Chame o aeon que você quiser. 44`

	store := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", translated),
	}}
	orig := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", original),
	}}
	row := hRow(0, "", original)
	row.Source = original
	row.Target = translated
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{row}}

	m := matchWithOriginal(art, store, orig)

	if len(m.tagLost) != 0 {
		t.Fatalf("row no-op não pode gerar tag divergente: %+v", m.tagLost)
	}
	if m.changed != 0 || m.inserted != 1 {
		t.Fatalf("changed=%d inserted=%d, queria 0 e 1", m.changed, m.inserted)
	}
}

// A mesma divergência numa row COM edição continua sendo validada (proteção
// contra sumir/sobrar tag de controle numa tradução nova).
func TestMatchChangedRowStillValidatesTags(t *testing.T) {
	original := "{TEXT_NEWLINE}Call any aeon you like. 44"
	translated := `{MCR:s06:l31:"Belgemine"}{TEXT_NEWLINE}Chame o aeon que você quiser. 44`
	edited := `{MCR:s06:l31:"Belgemine"}{MCR:s06:l31:"Belgemine"}{TEXT_NEWLINE}Chame o aeon. 44`

	store := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", translated),
	}}
	orig := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{
		hRow(0, "", original),
	}}
	row := hRow(0, "", original)
	row.Source = original
	row.Target = edited
	art := dto.FileEntry{Metadata: hTestMeta(), Rows: []dto.TextRow{row}}

	m := matchWithOriginal(art, store, orig)

	if len(m.tagLost) != 1 {
		t.Fatalf("tagLost=%+v, queria 1 (sobra de MCR na tradução editada)", m.tagLost)
	}
	if m.changed != 0 || m.inserted != 0 {
		t.Fatalf("changed=%d inserted=%d, queria 0 e 0", m.changed, m.inserted)
	}
}
