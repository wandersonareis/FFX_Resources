package builders_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/formatters"
	"ffxresources/backend/formatters/hash"
	jsonfmt "ffxresources/backend/formatters/json"
	_ "ffxresources/backend/formatters/strings"
)

// Copia a árvore help/ dos fixtures para um GameFilesRoot temporário: o e2e
// edita e regrava os binários sem tocar nos fixtures.
func seedTempHelpRoot(t *testing.T) string {
	t.Helper()
	fixtureRoot, err := filepath.Abs(filepath.Join("..", "..", "testData", "FFX", "binary"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	tempRoot := tempdir.TempDir(t)
	if err := os.CopyFS(tempRoot, os.DirFS(fixtureRoot)); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = tempRoot
	common.DisableMods = true
	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		common.DisableMods = prevMods
		helpfile.ClearHelp(common.GameVersionFFX)
	})
	return tempRoot
}

func helpFilePathIn(root, dir, name string) string {
	return filepath.Join(root, "ffx_ps2", "ffx", "master", "new_uspc", "help", dir, name+".sps2")
}

// modsHelpPathIn é o destino de gravação (árvore mods/, padrão dos formatos).
func modsHelpPathIn(root, dir, name string) string {
	return filepath.Join(root, "mods", "ffx_ps2", "ffx", "master", "new_uspc", "help", dir, name+".sps2")
}

func TestBuildHelpDTOKeysAndRows(t *testing.T) {
	root := seedTempHelpRoot(t)
	_ = root

	c, err := builders.BuildHelpDTO(common.GameVersionFFX, nil, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(c) != len(helpfile.HelpEntries) {
		t.Fatalf("entradas: got %d want %d", len(c), len(helpfile.HelpEntries))
	}

	entry, ok := c["now_help"]
	if !ok {
		t.Fatalf("missing now_help: %v", c.SortedKeys())
	}
	if len(entry.Rows) != 1208 {
		t.Fatalf("now_help rows: %d", len(entry.Rows))
	}
	if entry.Metadata.Key != "ffx/help/now_help/now_help.sps2" {
		t.Fatalf("key: %q", entry.Metadata.Key)
	}
	if entry.Metadata.ID != "now_help" {
		t.Fatalf("id: %q", entry.Metadata.ID)
	}
	if entry.Metadata.RowCount != 1208 {
		t.Fatalf("row_count: %d", entry.Metadata.RowCount)
	}
	// Rows SEM name: as linhas de help se diferenciam pelo arquivo
	// (metadata.key) + index — o campo legado text_XXXX saiu do formato.
	if entry.Rows[0].Name != "" || entry.Rows[3].Name != "" {
		t.Fatalf("rows com name inesperado: %q / %q", entry.Rows[0].Name, entry.Rows[3].Name)
	}
	if entry.Rows[3].Text["us"] != "Directional Button SE{ICON:F0:?}" {
		t.Fatalf("seg 3 us: %q", entry.Rows[3].Text["us"])
	}
	if _, ok := c["mon_boku"]; !ok {
		t.Fatal("missing mon_boku")
	}
	if len(c["mon_boku"].Rows) != 1578 {
		t.Fatalf("mon_boku rows: %d", len(c["mon_boku"].Rows))
	}
}

// e2e: build → edita row no DTO → apply → binário gravado em mods/ (padrão
// dos formatos) com ponteiros recalculados; original do gamefiles intacto;
// e a alteração PROPAGA para os painéis que compartilham o mesmo texto
// (anti-desalinhamento), sem tocar nos demais.
func TestApplyHelpDTORewritesBinary(t *testing.T) {
	root := seedTempHelpRoot(t)

	c, err := builders.BuildHelpDTO(common.GameVersionFFX, nil, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	const origText = "Directional Button SE{ICON:F0:?}"
	const newText = "Button"

	// Localiza as ocorrências do texto original em TODOS os painéis: a
	// propagação deve atualizá-las, mesmo fora do lote do apply.
	type occ struct {
		entry string
		index int
	}
	holders := map[string][]occ{}
	for _, k := range c.SortedKeys() {
		for _, r := range c[k].Rows {
			if r.Text["us"] == origText {
				holders[k] = append(holders[k], occ{entry: k, index: r.Index})
			}
		}
	}
	if len(holders) < 2 {
		t.Fatalf("fixtures deveriam repetir o texto em ≥2 painéis, achei em %d", len(holders))
	}

	before := map[string][]byte{}
	for name := range holders {
		data, err := os.ReadFile(helpFilePathIn(root, helpfile.HelpEntryDir(name), name))
		if err != nil {
			t.Fatalf("read before %s: %v", name, err)
		}
		before[name] = data
	}
	// Lote = só now_help (o antigo comportamento de "irmão intocado" vira
	// "cópias atualizadas").
	entry := c["now_help"]
	rows := make([]dto.TextRow, len(entry.Rows))
	copy(rows, entry.Rows)
	for i := range rows {
		if rows[i].Index == 3 {
			rows[i].Text = map[string]string{"us": newText}
		}
	}
	entry.Rows = rows

	if err := builders.ApplyHelpDTO(common.GameVersionFFX, dto.Collection{"now_help": entry}, []string{"now_help"}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// O binário compilado vai para mods/ (não regrava o gamefiles).
	modPath := modsHelpPathIn(root, "now_help", "now_help")
	after, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("read mod %s: %v", modPath, err)
	}
	if bytes.Equal(after, before["now_help"]) {
		t.Fatal("binário em mods/ deveria ter mudado após o apply")
	}

	// Original do gamefiles permanece intacto (sem .bak).
	originalAfter, err := os.ReadFile(helpFilePathIn(root, "now_help", "now_help"))
	if err != nil {
		t.Fatalf("read original after: %v", err)
	}
	if !bytes.Equal(originalAfter, before["now_help"]) {
		t.Fatal("o binário original do gamefiles não deveria ser alterado")
	}
	if _, err := os.Stat(helpFilePathIn(root, "now_help", "now_help") + ".bak"); !os.IsNotExist(err) {
		t.Fatal(".bak não é mais usado (o original fica no gamefiles)")
	}

	// Re-read confirma o novo texto e o encadeamento de ponteiros.
	f, err := helpfile.ReadHelpBinary(after, "now_help", common.DefaultLocalization, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got := f.Segments[3].Text; got != newText {
		t.Fatalf("seg 3 pós-import: %q", got)
	}
	if f.TextEnd != uint32(len(after)) {
		t.Fatalf("textEnd 0x%X != tamanho do arquivo 0x%X", f.TextEnd, len(after))
	}

	// Propagação: TODA cópia do texto, em TODOS os painéis, recebeu o novo
	// texto e foi gravada em mods/.
	for name, occs := range holders {
		if name == "now_help" {
			continue
		}
		data, err := os.ReadFile(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name))
		if err != nil {
			t.Fatalf("painel %s deveria ter sido gravado pela propagação: %v", name, err)
		}
		hf, err := helpfile.ReadHelpBinary(data, name, common.DefaultLocalization, common.GameVersionFFX)
		if err != nil {
			t.Fatalf("re-read %s: %v", name, err)
		}
		for _, o := range occs {
			if got := hf.Segments[o.index].Text; got != newText {
				t.Fatalf("%s[%d]: %q != %q (propagação falhou)", name, o.index, got, newText)
			}
		}
		// Originais dos cópias intactos.
		orig, err := os.ReadFile(helpFilePathIn(root, helpfile.HelpEntryDir(name), name))
		if err != nil {
			t.Fatalf("read original %s: %v", name, err)
		}
		if !bytes.Equal(orig, before[name]) {
			t.Fatalf("o binário original de %s não deveria ser alterado", name)
		}
	}
	// Painéis SEM ocorrência do texto não são gravados.
	for _, name := range helpfile.HelpEntryNames() {
		if _, ok := holders[name]; ok {
			continue
		}
		if _, err := os.Stat(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name)); !os.IsNotExist(err) {
			t.Fatalf("painel %s sem ocorrência não deveria ser gravado", name)
		}
	}
}

// Round-trip pelo artefato: export JSON+strings em mods/edits → read → apply
// sem mudanças → nenhum binário é gravado em mods/ (original preservado).
func TestHelpArtifactRoundTripUnchanged(t *testing.T) {
	root := seedTempHelpRoot(t)

	c, err := builders.BuildHelpDTO(common.GameVersionFFX, nil, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	jp, err := jsonfmt.HelpJSONPath(common.GameVersionFFX, "")
	if err != nil {
		t.Fatalf("json path: %v", err)
	}
	jf, ok := formatters.Get("json")
	if !ok {
		t.Fatal("formatter json não registrado")
	}
	if _, err := formatters.NewWriter(jf, c, jp, nil).Write(); err != nil {
		t.Fatalf("write json: %v", err)
	}
	sf, ok := formatters.Get("strings")
	if !ok {
		t.Fatal("formatter strings não registrado")
	}
	sp := formatters.SwapExt(sf, jp)
	if _, err := formatters.NewWriter(sf, c, sp, nil).Write(); err != nil {
		t.Fatalf("write strings: %v", err)
	}
	// Artefato ÚNICO por versão: 1 JSON + 1 .strings com os 6 painéis.
	editsDir := filepath.Join(root, "mods", "edits")
	for _, p := range []string{jp, sp} {
		if filepath.Dir(p) != editsDir {
			t.Fatalf("artefato fora de mods/edits: %s", p)
		}
	}
	// Nome único com sufixo de versão (padrão dos demais kinds); as entradas
	// se diferenciam pela metadata.key (nome dos .sps2), não por nome de arquivo.
	if filepath.Base(jp) != "help_all_localizations_ffx.json" {
		t.Fatalf("nome do artefato único: %s", filepath.Base(jp))
	}
	if filepath.Base(sp) != "help_all_localizations_ffx.strings" {
		t.Fatalf("nome do artefato strings: %s", filepath.Base(sp))
	}

	before, err := os.ReadFile(helpFilePathIn(root, "now_help", "now_help"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	read, err := formatters.NewReader(jf, jp).Read()
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	if len(read) != len(helpfile.HelpEntries) {
		t.Fatalf("entradas no artefato único: %d want %d", len(read), len(helpfile.HelpEntries))
	}
	if err := builders.ApplyHelpDTO(common.GameVersionFFX, read, read.SortedKeys()); err != nil {
		t.Fatalf("apply sem mudanças: %v", err)
	}

	after, err := os.ReadFile(helpFilePathIn(root, "now_help", "now_help"))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("binário original mudou sem edição")
	}
	if _, err := os.Stat(modsHelpPathIn(root, "now_help", "now_help")); !os.IsNotExist(err) {
		t.Fatal("nada deveria ser gravado em mods/ sem edição")
	}

	// O mesmo vale para o .strings.
	readS, err := formatters.NewReader(sf, sp).Read()
	if err != nil {
		t.Fatalf("read strings: %v", err)
	}
	if len(readS) != len(helpfile.HelpEntries) {
		t.Fatalf("entradas no strings único: %d want %d", len(readS), len(helpfile.HelpEntries))
	}
}

// helpIsRef espelha a regra do backend (helpRefBare/unmarshalCollection):
// ref = começa com "$" E o resto casa com o hash PRÓPRIO da row.
func helpIsRef(r dto.TextRow) bool {
	bare, ok := hash.Strip(r.Text[common.DefaultLocalization])
	return ok && bare == r.Hash[common.DefaultLocalization]
}

// Dedup do DTO entregue ao frontend: defs literais, refs "$hash" só em 'us',
// hash/idiomas/rows intocados e a entrada original não mutada.
func TestDedupHelpDTORefsAndDefs(t *testing.T) {
	seedTempHelpRoot(t)

	full, err := builders.BuildHelpDTO(common.GameVersionFFX, nil, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	dedup := builders.DedupHelpDTO(full)

	// A entrada NÃO é mutada: o DTO raw segue 100% literal (é ele que o
	// export/preview/validação de import usam).
	for _, k := range full.SortedKeys() {
		for _, r := range full[k].Rows {
			if helpIsRef(r) {
				t.Fatalf("DedupHelpDTO mutou a entrada: %s[%d] virou ref", k, r.Index)
			}
		}
	}

	refCount := 0
	literal := make(map[string]bool)
	for _, k := range dedup.SortedKeys() {
		fe, de := full[k], dedup[k]
		if len(fe.Rows) != len(de.Rows) {
			t.Fatalf("%s: rows %d != %d", k, len(de.Rows), len(fe.Rows))
		}
		for i := range de.Rows {
			fr, dr := fe.Rows[i], de.Rows[i]
			if fr.Index != dr.Index {
				t.Fatalf("%s[%d]: index mudou (%d != %d)", k, i, dr.Index, fr.Index)
			}
			// Hash intocado (é o ponteiro opaco das refs).
			if len(fr.Hash) != len(dr.Hash) {
				t.Fatalf("%s[%d]: hash map mudou", k, i)
			}
			for lang, h := range fr.Hash {
				if dr.Hash[lang] != h {
					t.Fatalf("%s[%d] hash[%s] mudou", k, i, lang)
				}
			}
			// Idiomas ≠ us nunca viram ref.
			for lang, txt := range fr.Text {
				if lang == common.DefaultLocalization {
					continue
				}
				if dr.Text[lang] != txt {
					t.Fatalf("%s[%d] lang %s alterado pelo dedup", k, i, lang)
				}
			}
			us := dr.Text[common.DefaultLocalization]
			if helpIsRef(dr) {
				refCount++
				bare, _ := hash.Strip(us)
				if !hash.IsDedupEligible(fr.Text[common.DefaultLocalization]) {
					t.Fatalf("%s[%d]: ref inelegível (≤ %d runes)", k, i, hash.MinDedupRunes)
				}
				if !literal[bare] {
					t.Fatalf("%s[%d]: ref %s sem def literal antes dela", k, i, bare)
				}
				continue
			}
			if us != fr.Text[common.DefaultLocalization] {
				t.Fatalf("%s[%d]: literal alterada sem virar ref", k, i)
			}
			if h := fr.Hash[common.DefaultLocalization]; h != "" {
				literal[h] = true
			}
		}
	}
	if refCount == 0 {
		t.Fatal("fixtures sem repetições: dedup não exercitado")
	}
}

// O contrato completo do usuário: o backend entrega a def editada + refs,
// o lote do frontend tem SÓ a entrada editada, e ao salvar o texto novo
// alcança todas as cópias em todos os painéis — que são gravadas em mods/ —
// enquanto painéis sem ocorrência e re-aplicações não gravam nada.
func TestHelpDedupApplyPropagatesToAllCopies(t *testing.T) {
	root := seedTempHelpRoot(t)

	full, err := builders.BuildHelpDTO(common.GameVersionFFX, nil, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	dedup := builders.DedupHelpDTO(full)

	// 1ª ref (ordem de chave) e a def literal com o mesmo hash.
	var refEntry string
	var refRow dto.TextRow
	for _, k := range dedup.SortedKeys() {
		for _, r := range dedup[k].Rows {
			if helpIsRef(r) {
				refEntry, refRow = k, r
				break
			}
		}
		if refEntry != "" {
			break
		}
	}
	if refEntry == "" {
		t.Fatal("fixtures sem refs de dedup")
	}
	bare, _ := hash.Strip(refRow.Text[common.DefaultLocalization])

	var defEntry string
	var defRow dto.TextRow
	for _, k := range full.SortedKeys() {
		for _, r := range full[k].Rows {
			if r.Hash[common.DefaultLocalization] == bare && r.Text[common.DefaultLocalization] != "" {
				defEntry, defRow = k, r
				break
			}
		}
		if defEntry != "" {
			break
		}
	}
	if defEntry == "" {
		t.Fatalf("def para a ref %s não encontrada", bare)
	}

	// Todas as ocorrências (painel + índice) do texto original.
	type occ struct {
		entry string
		index int
	}
	byEntry := map[string][]occ{}
	for _, k := range full.SortedKeys() {
		for _, r := range full[k].Rows {
			if r.Hash[common.DefaultLocalization] == bare {
				byEntry[k] = append(byEntry[k], occ{entry: k, index: r.Index})
			}
		}
	}
	if len(byEntry) < 2 {
		t.Fatalf("texto repetido deveria ocorrer em ≥2 painéis, achei em %d", len(byEntry))
	}

	// Sem edição: um lote contendo só a ref (def fora do lote) é identidade —
	// nada é gravado.
	if err := builders.ApplyHelpDTO(common.GameVersionFFX,
		dto.Collection{refEntry: dedup[refEntry]}, []string{refEntry}); err != nil {
		t.Fatalf("apply identidade: %v", err)
	}
	for _, name := range helpfile.HelpEntryNames() {
		if _, err := os.Stat(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name)); !os.IsNotExist(err) {
			t.Fatalf("lote sem edição gravou %s", name)
		}
	}

	// Simula o frontend: lote = SÓ a entrada da def, com a def editada e as
	// refs intactas como o backend entregou (hash mantido = ponteiro opaco).
	const newText = "TEXTO EDITADO PELO TRADUTOR (dedup)"
	edited := dedup[defEntry]
	rows := make([]dto.TextRow, len(edited.Rows))
	copy(rows, edited.Rows)
	for i := range rows {
		if rows[i].Index == defRow.Index {
			text := make(map[string]string, len(rows[i].Text))
			for lk, lv := range rows[i].Text {
				text[lk] = lv
			}
			text[common.DefaultLocalization] = newText
			rows[i].Text = text
		}
	}
	edited.Rows = rows

	if err := builders.ApplyHelpDTO(common.GameVersionFFX,
		dto.Collection{defEntry: edited}, []string{defEntry}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// TODAS as ocorrências, em TODOS os painéis, receberam o novo texto.
	for name, occs := range byEntry {
		data, err := os.ReadFile(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name))
		if err != nil {
			t.Fatalf("painel %s não gravado em mods: %v", name, err)
		}
		hf, err := helpfile.ReadHelpBinary(data, name, common.DefaultLocalization, common.GameVersionFFX)
		if err != nil {
			t.Fatalf("re-read %s: %v", name, err)
		}
		for _, o := range occs {
			if got := hf.Segments[o.index].Text; got != newText {
				t.Fatalf("%s[%d]: %q != %q", name, o.index, got, newText)
			}
		}
	}
	// Painéis SEM ocorrência não são gravados.
	for _, name := range helpfile.HelpEntryNames() {
		if _, ok := byEntry[name]; ok {
			continue
		}
		if _, err := os.Stat(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name)); !os.IsNotExist(err) {
			t.Fatalf("painel %s sem ocorrência não deveria ser gravado", name)
		}
	}

	// Idempotência: reaplicar o mesmo lote não muda os bytes de ninguém.
	snapshot := map[string][]byte{}
	for name := range byEntry {
		data, err := os.ReadFile(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name))
		if err != nil {
			t.Fatalf("snapshot %s: %v", name, err)
		}
		snapshot[name] = data
	}
	if err := builders.ApplyHelpDTO(common.GameVersionFFX,
		dto.Collection{defEntry: edited}, []string{defEntry}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	for name, want := range snapshot {
		data, err := os.ReadFile(modsHelpPathIn(root, helpfile.HelpEntryDir(name), name))
		if err != nil {
			t.Fatalf("re-read %s: %v", name, err)
		}
		if !bytes.Equal(data, want) {
			t.Fatalf("re-apply mudou os bytes de %s", name)
		}
	}
}

// Lote com DUAS entradas e textos literais (formato do import mesclado /
// rascunhos abertos ao mesmo tempo): a def editada manda, a cópia literal
// da outra entrada (texto de antes, não editada) NÃO reverte a alteração —
// e a edição própria daquela entrada aplica normal.
func TestHelpDedupMultiEntryBatchKeepsEdit(t *testing.T) {
	root := seedTempHelpRoot(t)

	full, err := builders.BuildHelpDTO(common.GameVersionFFX, nil, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	dedup := builders.DedupHelpDTO(full)

	// Primeira ref cuja def vive em OUTRO painel (cópia entre arquivos).
	var refEntry, defEntry string
	var refRow, defRow dto.TextRow
search:
	for _, k := range dedup.SortedKeys() {
		for _, r := range dedup[k].Rows {
			if !helpIsRef(r) {
				continue
			}
			bare, _ := hash.Strip(r.Text[common.DefaultLocalization])
			defEntry = ""
			for _, k2 := range dedup.SortedKeys() {
				for _, r2 := range dedup[k2].Rows {
					if r2.Hash[common.DefaultLocalization] == bare && !helpIsRef(r2) {
						defEntry, defRow = k2, r2
						break
					}
				}
				if defEntry != "" {
					break
				}
			}
			if defEntry != "" && defEntry != k {
				refEntry, refRow = k, r
				break search
			}
		}
	}
	if refEntry == "" {
		t.Fatal("fixtures sem ref de dedup entre painéis diferentes")
	}
	bare, _ := hash.Strip(refRow.Text[common.DefaultLocalization])

	// Ocorrências completas do texto original (para conferir a alavanca).
	type occ struct {
		entry string
		index int
	}
	var occurrences []occ
	for _, k := range full.SortedKeys() {
		for _, r := range full[k].Rows {
			if r.Hash[common.DefaultLocalization] == bare {
				occurrences = append(occurrences, occ{entry: k, index: r.Index})
			}
		}
	}
	if len(occurrences) < 2 {
		t.Fatalf("texto da def deveria ter ≥2 ocorrências, achei %d", len(occurrences))
	}

	// Lote bruto (como o merge do import devolve): AMBAS as entradas com os
	// textos literais do estado atual; só a def é editada.
	const newText = "TEXTO NOVO DA DEF"
	batch := dto.Collection{defEntry: full[defEntry], refEntry: full[refEntry]}
	rows := make([]dto.TextRow, len(batch[defEntry].Rows))
	copy(rows, batch[defEntry].Rows)
	for i := range rows {
		if rows[i].Index == defRow.Index {
			text := make(map[string]string, len(rows[i].Text))
			for lk, lv := range rows[i].Text {
				text[lk] = lv
			}
			text[common.DefaultLocalization] = newText
			rows[i].Text = text
		}
	}
	be := batch[defEntry]
	be.Rows = rows
	batch[defEntry] = be

	// Edição não relacionada na outra entrada (rascunho dela também aberto).
	const otherNew = "EDICAO DA OUTRA ENTRADA"
	refRows := make([]dto.TextRow, len(batch[refEntry].Rows))
	copy(refRows, batch[refEntry].Rows)
	editedOther := false
	for i := range refRows {
		if refRows[i].Hash[common.DefaultLocalization] == bare {
			continue // a própria cópia: permanece literal e de antes
		}
		if refRows[i].Text[common.DefaultLocalization] == "" {
			continue
		}
		text := make(map[string]string, len(refRows[i].Text))
		for lk, lv := range refRows[i].Text {
			text[lk] = lv
		}
		text[common.DefaultLocalization] = otherNew
		refRows[i].Text = text
		editedOther = true
		break
	}
	if !editedOther {
		t.Fatalf("%s sem row editável para o segundo rascunho", refEntry)
	}
	re := batch[refEntry]
	re.Rows = refRows
	batch[refEntry] = re

	if err := builders.ApplyHelpDTO(common.GameVersionFFX, batch, batch.SortedKeys()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// A cópia literal não reverteu a def: todas as ocorrências = novo texto.
	for _, o := range occurrences {
		data, err := os.ReadFile(modsHelpPathIn(root, helpfile.HelpEntryDir(o.entry), o.entry))
		if err != nil {
			t.Fatalf("painel %s não gravado: %v", o.entry, err)
		}
		hf, err := helpfile.ReadHelpBinary(data, o.entry, common.DefaultLocalization, common.GameVersionFFX)
		if err != nil {
			t.Fatalf("re-read %s: %v", o.entry, err)
		}
		if got := hf.Segments[o.index].Text; got != newText {
			t.Fatalf("%s[%d]: %q != %q (cópia literal reverteu a def)", o.entry, o.index, got, newText)
		}
	}

	// E a edição própria da outra entrada aplicou no seu segmento.
	data, err := os.ReadFile(modsHelpPathIn(root, helpfile.HelpEntryDir(refEntry), refEntry))
	if err != nil {
		t.Fatalf("read %s: %v", refEntry, err)
	}
	hf, err := helpfile.ReadHelpBinary(data, refEntry, common.DefaultLocalization, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("re-read %s: %v", refEntry, err)
	}
	for _, r := range refRows {
		if r.Text[common.DefaultLocalization] == otherNew {
			if got := hf.Segments[r.Index].Text; got != otherNew {
				t.Fatalf("%s[%d]: %q != %q", refEntry, r.Index, got, otherNew)
			}
			break
		}
	}
}
