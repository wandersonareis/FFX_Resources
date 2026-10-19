package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	jsonfmt "ffxresources/backend/formatters/json"
	strfmt "ffxresources/backend/formatters/strings"
)

// Testes fim-a-fim do prepareImport com events sintéticos (sem fixtures de
// disco): artefato escrito em disco → PreviewImport/ImportFile.
//
// Cobre os três cenários pedidos:
//   - ordem dos textos trocada no arquivo,
//   - import parcial (menos textos que o total),
//   - hash não encontrado (store mudou desde o export) — log, não erro.
//
// O artefato carrega o hash do texto vigente no export (o tradutor mexe só
// no valor), então a store intacta casa tudo; editada, casa o que sobrou.
func importE2EPrepare(t *testing.T, raw []byte, name string) dto.ImportSummary {
	t.Helper()
	clearEventsForTest(t)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
		"Terceira frase do evento",
	})

	impPath := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(impPath, raw, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	summary, err := NewMetadataService(nil).PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	return summary
}

// clearEventsForTest zera o store de events da versão (com cleanup).
func clearEventsForTest(t *testing.T) {
	t.Helper()
	event.ClearEvents(common.GameVersionFFX2)
	t.Cleanup(func() { event.ClearEvents(common.GameVersionFFX2) })
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)
}

// collectionRaw devolve a store raw de events (mods-first) para montar o
// artefato com hash/texto consistentes antes da edição.
func collectionRawForTest(t *testing.T) dto.Collection {
	t.Helper()
	raw, err := NewMetadataService(nil).GetCollection(KindEvents, common.GameVersionFFX2, nil, nil)
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	return raw
}

// TestImportE2ESwappedOrder: artefato com as rows em ordem trocada importa
// nas células certas (hash é a chave) e o resumo não tem erro nenhum.
func TestImportE2ESwappedOrder(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
		"Terceira frase do evento",
	})
	raw := collectionRawForTest(t)
	entry := raw["zev001"]
	if len(entry.Rows) != 3 {
		t.Fatalf("pré-condição: 3 rows, tem %d", len(entry.Rows))
	}

	// Edita as 3 (hash fica = texto do export) e embaralha a ordem.
	edited := make([]dto.TextRow, len(entry.Rows))
	copy(edited, entry.Rows)
	edited[0].Text[common.DefaultLocalization] = "Terceira traduzida"
	edited[1].Text[common.DefaultLocalization] = "Primeira traduzida"
	edited[2].Text[common.DefaultLocalization] = "Segunda traduzida"
	entry.Rows = []dto.TextRow{edited[2], edited[0], edited[1]}

	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	summary := importE2EPrepare(t, out, "swapped.json")
	if len(summary.Errors) != 0 {
		t.Fatalf("erros no resumo: %v", summary.Errors)
	}
	if summary.ChangedTexts != 3 {
		t.Fatalf("changed=%d, queria 3", summary.ChangedTexts)
	}
	if summary.Entries[0].Error != "" {
		t.Fatalf("entry com erro: %q", summary.Entries[0].Error)
	}
}

// TestImportE2EPartial: artefato com menos textos que o total (1 de 3).
func TestImportE2EPartial(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
		"Terceira frase do evento",
	})
	raw := collectionRawForTest(t)
	entry := raw["zev001"]

	// Só a row 2 vai no arquivo, traduzida; hash = texto do export.
	entry.Rows = []dto.TextRow{hEditRow(2, "", "Terceira frase do evento", "Terceira traduzida")}

	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	summary := importE2EPrepare(t, out, "partial.json")
	if len(summary.Errors) != 0 {
		t.Fatalf("erros no resumo: %v", summary.Errors)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1 (import parcial)", summary.ChangedTexts)
	}
	if summary.Entries[0].Error != "" {
		t.Fatalf("entry com erro: %q", summary.Entries[0].Error)
	}
}

// TestImportE2EStaleHash: a store mudou desde o export — hash não casa e a
// row sai só no log; o resumo NÃO tem erro (o revisor corrige o artefato).
func TestImportE2EStaleHash(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
	})
	raw := collectionRawForTest(t)
	entry := raw["zev001"]

	// A store muda DEPOIS do export: texto da row 1 vira outra coisa.
	entry.Rows[0].Text[common.DefaultLocalization] = "Tradução anterior"
	entry.Rows[1].Text[common.DefaultLocalization] = "Segunda traduzida"
	// O artefato é exportado com ESTE estado...
	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// ...e a store evolui: a frase "Tradução anterior" não existe mais.
	// A mutação é no FieldString do STORE (o DTO do GetCollection é cópia).
	ev := event.GetEvent(common.GameVersionFFX2, "zev001")
	if ev == nil {
		t.Fatal("pré-condição: zev001 no store")
	}
	ev.Strings[0].GetLocalizedContent(common.DefaultLocalization).SetRegularString("Tradução refeita")

	// O artefato (hash do texto antigo) não casa mais com a store nova:
	// 1 de 2 vai, e o resto é log.
	impPath := filepath.Join(t.TempDir(), "stale.json")
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	summary, err := NewMetadataService(nil).PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(summary.Errors) != 0 {
		t.Fatalf("hash não encontrado não pode ser erro do resumo: %v", summary.Errors)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1 (só a row que ainda casa)", summary.ChangedTexts)
	}
}

// TestImportE2EUnknownEntry: problema de uma entrada é "entrada não existe
// nesta versão" — vem como AVISO (a entrada será ignorada) e nunca bloqueia
// o arquivo.
func TestImportE2EUnknownEntry(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase qualquer aqui"})

	impPath := filepath.Join(t.TempDir(), "unknown.json")
	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{
		"zz_inexistente": dto.FileEntry{
			Metadata: dto.Metadata{Key: "ffx-2/event/zz/zzinexistente.bin", ID: "zz_inexistente"},
			Rows:     []dto.TextRow{hRow(0, "", "Texto solto")},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	summary, err := NewMetadataService(nil).PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(summary.Warnings) != 1 {
		t.Fatalf("warnings=%v, queria 1", summary.Warnings)
	}
	if got := summary.Warnings[0]; !contains(got, "entrada não existe nesta versão") {
		t.Fatalf("aviso %q, queria entrada inexistente", got)
	}
	if len(summary.Errors) != 0 {
		t.Fatalf("entrada inexistente não pode bloquear a importação: %v", summary.Errors)
	}
}

// TestImportE2EUnknownEntryDoesNotBlockBatch: artefato com uma entrada boa e
// uma inexistente — a problemática é avisada/ignorada e a boa importa (o
// "events all com 301, 1 corrompido" importa os 300 bons).
func TestImportE2EUnknownEntryDoesNotBlockBatch(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase boa do evento"})

	raw := collectionRawForTest(t)
	good := raw["zev001"]
	good.Rows[0].Text[common.DefaultLocalization] = "Frase traduzida"

	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{
		"zev001": good,
		"zz_inexistente": dto.FileEntry{
			Metadata: dto.Metadata{Key: "ffx-2/event/zz/zzinexistente.bin", ID: "zz_inexistente"},
			Rows:     []dto.TextRow{hRow(0, "", "Texto solto")},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	impPath := filepath.Join(t.TempDir(), "unknown-plus-good.json")
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewMetadataService(nil)
	summary, err := svc.PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(summary.Errors) != 0 {
		t.Fatalf("erros globais inesperados: %v", summary.Errors)
	}
	if len(summary.Warnings) != 1 || !contains(summary.Warnings[0], "zz_inexistente") {
		t.Fatalf("warnings=%v, queria só a entrada inexistente", summary.Warnings)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1 (a entrada boa, descontada a ruim)", summary.ChangedTexts)
	}

	// Confirmar aplica só a entrada boa: o aviso não bloqueia o arquivo.
	changed, err := svc.ImportFile(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("ImportFile bloqueado por aviso de entrada: %v", err)
	}
	if changed != 1 {
		t.Fatalf("ImportFile changed=%d, queria 1 (só a entrada boa)", changed)
	}
}

// TestImportE2EOverLimitEntrySkipped: entrada cuja tradução estoura o limite
// uint16 é avisada e EXCLUÍDA — a outra entrada do mesmo arquivo importa.
// Granularidade é o arquivo: o estouro de uma row derruba a entrada inteira.
func TestImportE2EOverLimitEntrySkipped(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase boa do evento"})
	seedEventsSynthetic(common.GameVersionFFX2, "zevbig", []string{"Frase curta"})

	raw := collectionRawForTest(t)
	good := raw["zev001"]
	good.Rows[0].Text[common.DefaultLocalization] = "Frase traduzida"
	big := raw["zevbig"]
	big.Rows[0].Text[common.DefaultLocalization] = strings.Repeat("A", 70000)

	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": good, "zevbig": big})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	impPath := filepath.Join(t.TempDir(), "overlimit.json")
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewMetadataService(nil)
	summary, err := svc.PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(summary.Errors) != 0 {
		t.Fatalf("erros globais inesperados: %v", summary.Errors)
	}
	if len(summary.Warnings) != 1 || !contains(summary.Warnings[0], "zevbig") ||
		!contains(summary.Warnings[0], "limite binário") {
		t.Fatalf("warnings=%v, queria o estouro da zevbig", summary.Warnings)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1 (zevbig descontada do resumo)", summary.ChangedTexts)
	}
	// Uso medido só do que será gravado: zevbig não tem barra de limite.
	if len(summary.Usages) != 1 || summary.Usages[0].ID != "zev001" {
		t.Fatalf("usages=%+v, queria só zev001", summary.Usages)
	}
	// A tabela de entradas marca a excluída para o modal.
	var bigInfo *dto.ImportEntryInfo
	for i := range summary.Entries {
		if summary.Entries[i].ID == "zevbig" {
			bigInfo = &summary.Entries[i]
		}
	}
	if bigInfo == nil || bigInfo.Error == "" {
		t.Fatalf("entrada zevbig devolve com erro: %+v", bigInfo)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestImportE2EStringsFormatRoundtrip: o mesmo fluxo parcial pelo formato
// .strings (dedup de valor + hash na chave da linha).
func TestImportE2EStringsFormatRoundtrip(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
	})
	raw := collectionRawForTest(t)
	entry := raw["zev001"]
	entry.Rows = []dto.TextRow{hEditRow(1, "", "Segunda frase do evento", "Segunda traduzida")}

	out, err := strfmt.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	summary := importE2EPrepare(t, out, "partial.strings")
	if len(summary.Errors) != 0 {
		t.Fatalf("erros no resumo: %v", summary.Errors)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1", summary.ChangedTexts)
	}
}

// TestImportE2ESkippedRowsInSummary: a row de hash stale que antes era só
// log agora aparece no resumo (Skipped) com id/índice/hash/reason e o
// contador por entrada (SkippedRows) — o botão de detalhes do modal pulsa
// a partir disso. Não vira erro nem warning: a importação segue.
func TestImportE2ESkippedRowsInSummary(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
	})
	raw := collectionRawForTest(t)
	entry := raw["zev001"]
	entry.Rows[0].Text[common.DefaultLocalization] = "Tradução anterior"
	entry.Rows[1].Text[common.DefaultLocalization] = "Segunda traduzida"
	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// A store evolui: o hash da row 0 do artefato não existe mais.
	ev := event.GetEvent(common.GameVersionFFX2, "zev001")
	if ev == nil {
		t.Fatal("pré-condição: zev001 no store")
	}
	ev.Strings[0].GetLocalizedContent(common.DefaultLocalization).SetRegularString("Tradução refeita")

	impPath := filepath.Join(t.TempDir(), "skipped.json")
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	summary, err := NewMetadataService(nil).PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("rows puladas não são erro nem aviso: %v / %v", summary.Errors, summary.Warnings)
	}
	if len(summary.Skipped) != 1 {
		t.Fatalf("skipped=%+v, queria 1 row (a de hash stale)", summary.Skipped)
	}
	issue := summary.Skipped[0]
	if issue.ID != "zev001" || issue.Index != 0 {
		t.Fatalf("issue id=%q index=%d, queria zev001/0", issue.ID, issue.Index)
	}
	if issue.Reason != reasonUnmatched {
		t.Fatalf("reason=%q, queria %q", issue.Reason, reasonUnmatched)
	}
	if issue.Text != "Tradução anterior" {
		t.Fatalf("text=%q, queria o texto da row do artefato", issue.Text)
	}
	if issue.Hash == "" {
		t.Fatal("hash vazio no issue — o revisor precisa dele para localizar a row")
	}
	if len(summary.Entries) != 1 || summary.Entries[0].SkippedRows != 1 {
		t.Fatalf("entries=%+v, queria zev001 com SkippedRows=1", summary.Entries)
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1 (só a row que casa)", summary.ChangedTexts)
	}
}

// TestImportE2ESkippedRowsReimportIsNoOp: re-import do MESMO artefato depois
// de aplicado — o hash (inglês do export) não casa mais, mas o binário já
// carrega exatamente o texto que cada row pede. Sem o filtro de no-op o
// Skipped explodiria com todas as rows e o botão de detalhes pulsa por nada;
// com ele, o re-import fica limpo (Skipped vazio, nada a revisar).
func TestImportE2ESkippedRowsReimportIsNoOp(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
	})

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "ffx-2/event/ze/zev001.bin", ID: "zev001"},
		Rows: []dto.TextRow{
			hEditRow(0, "", "Primeira frase do evento", "Primeira traduzida"),
			hEditRow(1, "", "Segunda frase do evento", "Segunda traduzida"),
		},
	}
	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	impPath := filepath.Join(t.TempDir(), "reimport.json")
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewMetadataService(nil)
	if _, err := svc.ImportFile(impPath, common.GameVersionFFX2); err != nil {
		t.Fatalf("primeiro import: %v", err)
	}

	// Segundo import do mesmo arquivo: tudo já aplicado.
	summary, err := svc.PreviewImport(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("re-import preview: %v", err)
	}
	if len(summary.Skipped) != 0 {
		t.Fatalf("skipped=%+v, queria vazio — row já aplicada é no-op, não revisável", summary.Skipped)
	}
	for _, e := range summary.Entries {
		if e.SkippedRows != 0 {
			t.Fatalf("entrada %s com SkippedRows=%d no re-import", e.ID, e.SkippedRows)
		}
	}
	if summary.ChangedTexts != 0 {
		t.Fatalf("changed=%d, queria 0 (nada muda no re-import)", summary.ChangedTexts)
	}
}

// TestImportE2ESkippedRowsCleanImport: tudo casa — Skipped é vazio e NÃO-nil
// (regra do DTO: slice nil vira JSON null e quebraria o frontend).
func TestImportE2ESkippedRowsCleanImport(t *testing.T) {
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
	})

	// Mesma semente do importE2EPrepare: row com hash do texto do export.
	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "ffx-2/event/ze/zev001.bin", ID: "zev001"},
		Rows:     []dto.TextRow{hEditRow(0, "", "Primeira frase do evento", "Primeira traduzida")},
	}
	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	summary := importE2EPrepare(t, out, "clean.json")
	if summary.Skipped == nil {
		t.Fatal("Skipped nil: o contrato do DTO pede slices não-nil (JSON null quebra o frontend)")
	}
	if len(summary.Skipped) != 0 {
		t.Fatalf("skipped=%+v, queria vazio no import limpo", summary.Skipped)
	}
	for _, e := range summary.Entries {
		if e.SkippedRows != 0 {
			t.Fatalf("entrada %s com SkippedRows=%d no import limpo", e.ID, e.SkippedRows)
		}
	}
	if summary.ChangedTexts != 1 {
		t.Fatalf("changed=%d, queria 1", summary.ChangedTexts)
	}
}

// TestImportE2ESkippedRowsTruncatesText: texto longo demais no artefato é
// truncado por rune com elipse — o modal continua legível, o log continua
// completo.
func TestImportE2ESkippedRowsTruncatesText(t *testing.T) {
	long := strings.Repeat("Frase longa com acentuação. ", 10) // 290 runes
	want := truncateRowIssue(long)
	runes := []rune(want)
	if len(runes) != 121 || !strings.HasSuffix(want, "…") {
		t.Fatalf("truncate devolve %d runes sem elipse: %q", len(runes), want)
	}
	if got := truncateRowIssue("curta"); got != "curta" {
		t.Fatalf("texto curto não pode ser alterado: %q", got)
	}
	if got := truncateRowIssue(""); got != "" {
		t.Fatalf("vazio deve continuar vazio: %q", got)
	}
}
