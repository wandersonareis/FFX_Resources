package services

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters/hash"
)

// seedEventsSynthetic registra eventos sintéticos no store (sem fixtures
// de disco): cada string vira um row com FieldString 'us'.
func seedEventsSynthetic(version common.GameVersion, id string, texts []string) {
	charset := ffxencoding.GetCharsetForLanguage(common.DefaultLocalization)
	objs := make([]*event.LocalizedFieldStringObject, 0, len(texts))
	for _, text := range texts {
		fs := event.NewEmptyFieldString(charset, version)
		fs.SetRegularString(text)
		objs = append(objs, event.NewLocalizedFieldStringObjectWithContent(common.DefaultLocalization, fs))
	}
	event.SetEvent(version, id, &event.EventFile{ID: id, Version: version, Strings: objs})
}

// seedEventsTempRoot aponta o GameFilesRoot para um diretório temporário
// (os applys gravam os binários compilados em mods/ sem tocar fixtures).
func seedEventsTempRoot(t *testing.T) {
	t.Helper()
	prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = tempdir.TempDir(t)
	common.DisableMods = true
	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		common.DisableMods = prevMods
	})
}

func eventsRowIsRef(r dto.TextRow) bool {
	bare, ok := hash.Strip(r.Text[common.DefaultLocalization])
	return ok && bare == r.Hash[common.DefaultLocalization]
}

// GetEntry de events entrega o DTO DEDUPLICADO com escopo global da versão
// (o 1º texto visto, em ordem de chave, é a def; as repetições — inclusive
// nos outros eventos — vêm como refs "$hash" para ocultar na tabela).
func TestGetEntryEventsDedupsDTO(t *testing.T) {
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	defer event.ClearEvents(common.GameVersionFFX2)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase compartilhada entre eventos"})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase única deste evento", "Frase compartilhada entre eventos"})

	// GetCollection continua RAW: nenhuma ref na store.
	raw, err := NewMetadataService(nil).GetCollection(KindEvents, common.GameVersionFFX2, nil, nil)
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	for _, k := range raw.SortedKeys() {
		for _, r := range raw[k].Rows {
			if eventsRowIsRef(r) {
				t.Fatalf("collection raw com ref em %s[%d]", k, r.Index)
			}
		}
	}

	// View: a cópia em zev002 vira ref (def em zev001, ordem de chave).
	entry, err := NewMetadataService(nil).GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if !eventsRowIsRef(entry.Rows[1]) {
		t.Fatalf("zev002[1] deveria ser ref de zev001[0]: %q", entry.Rows[1].Text["us"])
	}
	if eventsRowIsRef(entry.Rows[0]) {
		t.Fatalf("zev002[0] é texto próprio, não ref: %q", entry.Rows[0].Text["us"])
	}

	// Toda ref tem a def literal correspondente em algum evento da versão.
	defs := make(map[string]bool)
	for _, k := range raw.SortedKeys() {
		for _, r := range raw[k].Rows {
			if h := r.Hash[common.DefaultLocalization]; h != "" && r.Text[common.DefaultLocalization] != "" {
				defs[h] = true
			}
		}
	}
	for _, r := range entry.Rows {
		if !eventsRowIsRef(r) {
			continue
		}
		bare, _ := hash.Strip(r.Text[common.DefaultLocalization])
		if !defs[bare] {
			t.Fatalf("ref %s sem def literal na versão", bare)
		}
	}
}

// O cache do view dedupado é invalidado por apply/import: após editar e
// aplicar, a próxima entrega reflete o texto novo — inclusive nas cópias
// gêmeas (que eram refs e passam a carregar a tradução via store).
func TestEventsDedupViewCacheInvalidatedOnApply(t *testing.T) {
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	defer event.ClearEvents(common.GameVersionFFX2)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase compartilhada entre eventos"})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase compartilhada entre eventos"})
	svc := NewMetadataService(nil)

	const newText = "Tradução da frase compartilhada"
	entry, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if entry.Rows[0].Text[common.DefaultLocalization] == newText {
		t.Fatal("pré-condição: texto ainda não traduzido")
	}
	entry.Rows[0].Text[common.DefaultLocalization] = newText
	entry.Rows[0].Hash[common.DefaultLocalization] = hash.Sum64Hex(newText)

	if err := svc.ApplyTextCollection(KindEvents, common.GameVersionFFX2, dto.Collection{"zev001": entry}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// O cache foi invalidado: a entrega reflete o estado novo. A def
	// (zev001) carrega a tradução literal; a cópia gêmea (zev002) vira
	// ref da MESMA tradução — mesma regra de def do bulk export.
	defEntry, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get def pós-apply: %v", err)
	}
	if got := defEntry.Rows[0].Text[common.DefaultLocalization]; got != newText {
		t.Fatalf("zev001[0] deveria carregar a tradução: %q", got)
	}
	after, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get entry pós-apply: %v", err)
	}
	if !eventsRowIsRef(after.Rows[0]) {
		t.Fatalf("zev002[0] deveria ser ref da tradução: %q", after.Rows[0].Text["us"])
	}
	bare, _ := hash.Strip(after.Rows[0].Text[common.DefaultLocalization])
	if bare != hash.Sum64Hex(newText) {
		t.Fatalf("ref não aponta para a tradução: %q", bare)
	}
}

// eventsExportPath: escopo do pedido vira nome do artefato — bulk, 1
// arquivo ou subconjunto, sem colisão entre eles.
func TestEventsExportPathScopes(t *testing.T) {
	seedEventsTempRoot(t)

	bulk, err := eventsExportPath(common.GameVersionFFX2, nil, "")
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if filepath.Base(bulk) != "events_all_localizations_ffx2.json" {
		t.Fatalf("bulk: %s", bulk)
	}

	single, err := eventsExportPath(common.GameVersionFFX2, []string{"azit0000"}, "")
	if err != nil {
		t.Fatalf("single: %v", err)
	}
	if filepath.Base(single) != "event_azit0000_all_localizations_ffx2.json" {
		t.Fatalf("single: %s", single)
	}

	sel, err := eventsExportPath(common.GameVersionFFX2, []string{"dnfr0100", "dnfr0300", "dnfr0500"}, "")
	if err != nil {
		t.Fatalf("sel: %v", err)
	}
	if filepath.Base(sel) != "events_sel_3_dnfr0100_all_localizations_ffx2.json" {
		t.Fatalf("sel: %s", sel)
	}
	if sel == bulk {
		t.Fatal("subconjunto não pode colidir com o bulk completo")
	}
	if _, err := os.Stat(filepath.Dir(sel)); err != nil {
		t.Fatalf("diretório edits não criado: %v", err)
	}
}

// macroExportPath: o artefato canônico é só do export completo; chunk
// individual e subconjunto ganham nome próprio (não sobrescrevem o bulk).
func TestMacroExportPathScopes(t *testing.T) {
	seedEventsTempRoot(t)

	full, err := macroExportPath(common.GameVersionFFX2, nil, "")
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if filepath.Base(full) != "macro_dictionary_all_localizations_ffx2.json" {
		t.Fatalf("full: %s", full)
	}

	chunk, err := macroExportPath(common.GameVersionFFX2, []string{"chunk_03"}, "")
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if filepath.Base(chunk) != "macro_chunk_03_all_localizations_ffx2.json" {
		t.Fatalf("chunk: %s", chunk)
	}

	sel, err := macroExportPath(common.GameVersionFFX2, []string{"chunk_03", "chunk_07"}, "")
	if err != nil {
		t.Fatalf("sel: %v", err)
	}
	if filepath.Base(sel) != "macro_sel_2_chunk_03_all_localizations_ffx2.json" {
		t.Fatalf("sel: %s", sel)
	}
	if sel == full || chunk == full {
		t.Fatal("export parcial não pode colidir com o artefato canônico")
	}
}
