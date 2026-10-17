package services

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/encoding"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters/hash"
)

// displayRowIsRef: ref quando texto é "$h" e h == hash próprio da row.
func displayRowIsRef(r dto.TextRow) bool {
	bare, ok := hash.Strip(r.Text[common.DefaultLocalization])
	return ok && bare == r.Hash[common.DefaultLocalization]
}

// seedEventsMultiLang semeia um evento com texto em VÁRIOS idiomas ('us' +
// jp): é o caso que o editor abre-pela-ref precisa exibir na coluna de
// idiomas extras (RefLink.Originals).
func seedEventsMultiLang(version common.GameVersion, id, usText, jpText string) {
	obj := event.NewLocalizedFieldStringObject()
	us := event.NewEmptyFieldString(ffxencoding.GetCharsetForLanguage(common.DefaultLocalization), version)
	us.SetRegularString(usText)
	obj.SetLocalizedContent(common.DefaultLocalization, us)
	jp := event.NewEmptyFieldString(ffxencoding.GetCharsetForLanguage("jp"), version)
	jp.SetRegularString(jpText)
	obj.SetLocalizedContent("jp", jp)
	event.SetEvent(version, id, &event.EventFile{
		ID: id, Version: version, Strings: []*event.LocalizedFieldStringObject{obj},
	})
}

const sharedOriginal = "Frase compartilhada entre eventos"

// seedDisplayRoot prepara o ambiente sintético comum aos testes de display.
func seedDisplayRoot(t *testing.T) *MetadataService {
	t.Helper()
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	t.Cleanup(func() { event.ClearEvents(common.GameVersionFFX2) })
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)
	return NewMetadataService(nil)
}

// seedEventsData grava o binário dos events no gamefiles e o espelha em
// data/ (o export vai sempre para mods/; data/ é a fonte do original).
func seedEventsData(t *testing.T, version common.GameVersion, id string) {
	t.Helper()
	if err := event.ExportEventStringsToLocalizations(version, id); err != nil {
		t.Fatalf("export %s: %v", id, err)
	}
	rel, err := event.EventRelPath(id)
	if err != nil {
		t.Fatalf("relpath %s: %v", id, err)
	}
	for loc := range common.SupportedLanguages {
		lroot := common.GetLocalizationRootForVersion(version, loc)
		modsPath := filepath.Join(common.GameFilesRoot, common.ModsFolder, lroot, rel)
		dataPath := filepath.Join(common.GameFilesRoot, lroot, rel)
		b, rerr := os.ReadFile(modsPath)
		if rerr != nil {
			t.Fatalf("ler mods %s: %v", modsPath, rerr)
		}
		if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dataPath, err)
		}
		if err := os.WriteFile(dataPath, b, 0o644); err != nil {
			t.Fatalf("gravar data %s: %v", dataPath, err)
		}
	}
}

// mutateStoreEvent troca o texto 'us' de strings[0] no STORE (sem gravar).
func mutateStoreEvent(t *testing.T, version common.GameVersion, id, newText string) {
	t.Helper()
	ev := event.GetEvent(version, id)
	if ev == nil || len(ev.Strings) == 0 || ev.Strings[0] == nil {
		t.Fatalf("evento %s não semeado", id)
	}
	fs := ev.Strings[0].GetLocalizedContent(common.DefaultLocalization)
	if fs == nil {
		t.Fatalf("evento %s sem conteúdo 'us'", id)
	}
	fs.SetRegularString(newText)
}

// TestGetEntryDisplayPointerIsOriginalHash: def traduzida (mods) + twin não
// traduzida — o grupo estrutural sobrevive à tradução da def (ponteiro = o
// do ORIGINAL nos dois), mas a twin NÃO colapsa: dedupe é só de original
// para original, e gêmea em inglês é exibida em inglês.
func TestGetEntryDisplayPointerIsOriginalHash(t *testing.T) {
	svc := seedDisplayRoot(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{sharedOriginal})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{sharedOriginal})

	// data/ recebe o ORIGINAL pristine dos dois eventos.
	seedEventsData(t, common.GameVersionFFX2, "zev001")
	seedEventsData(t, common.GameVersionFFX2, "zev002")

	// zev001 é traduzido no store e gravado em mods/ (zev002 fica sem mods:
	// continua não traduzido, lendo o original de data/).
	const translation = "Tradução da frase compartilhada"
	mutateStoreEvent(t, common.GameVersionFFX2, "zev001", translation)
	if err := event.ExportEventStringsToLocalizations(common.GameVersionFFX2, "zev001"); err != nil {
		t.Fatalf("export pós-tradução: %v", err)
	}
	clearDedupViewCache()

	defEntry, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get def: %v", err)
	}
	wantPointer := hash.Sum64Hex(sharedOriginal)
	if got := defEntry.Rows[0].Hash[common.DefaultLocalization]; got != wantPointer {
		t.Fatalf("ponteiro da def deveria ser o do ORIGINAL: %q != %q", got, wantPointer)
	}
	if got := defEntry.Rows[0].Text[common.DefaultLocalization]; got != translation {
		t.Fatalf("def deveria carregar a tradução: %q", got)
	}
	if displayRowIsRef(defEntry.Rows[0]) {
		t.Fatal("def nunca deveria colapsar em ref")
	}

	after, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get twin: %v", err)
	}
	// Dedupe é só de original para original: com a def traduzida, a twin
	// ainda em inglês NÃO colapsa — fica célula própria, em inglês, e o
	// ponteiro dela continua sendo o do ORIGINAL (grupo estrutural intacto).
	if displayRowIsRef(after.Rows[0]) {
		t.Fatalf("twin não traduzida não pode colapsar sob def traduzida: %q", after.Rows[0].Text["us"])
	}
	if got := after.Rows[0].Hash[common.DefaultLocalization]; got != wantPointer {
		t.Fatalf("ponteiro da twin deveria continuar o do ORIGINAL: %q != %q", got, wantPointer)
	}
	if got := after.Rows[0].Text[common.DefaultLocalization]; got != sharedOriginal {
		t.Fatalf("twin deveria seguir em inglês (estado real do binário): %q", got)
	}
}

// TestGetEntryTranslatedTwinStaysLiteral: traduções divergentes NÃO
// colapsam — dupe sobre texto já traduzido é proibido por decisão.
func TestGetEntryTranslatedTwinStaysLiteral(t *testing.T) {
	svc := seedDisplayRoot(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{sharedOriginal})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{sharedOriginal})

	seedEventsData(t, common.GameVersionFFX2, "zev001")
	seedEventsData(t, common.GameVersionFFX2, "zev002")

	const a = "Tradução A da frase"
	const b = "Tradução B divergente"
	mutateStoreEvent(t, common.GameVersionFFX2, "zev001", a)
	if err := event.ExportEventStringsToLocalizations(common.GameVersionFFX2, "zev001"); err != nil {
		t.Fatalf("export zev001: %v", err)
	}
	mutateStoreEvent(t, common.GameVersionFFX2, "zev002", b)
	if err := event.ExportEventStringsToLocalizations(common.GameVersionFFX2, "zev002"); err != nil {
		t.Fatalf("export zev002: %v", err)
	}
	clearDedupViewCache()

	e1, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get zev001: %v", err)
	}
	e2, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get zev002: %v", err)
	}

	if displayRowIsRef(e1.Rows[0]) {
		t.Fatalf("tradução A deveria permanecer literal: %q", e1.Rows[0].Text["us"])
	}
	if displayRowIsRef(e2.Rows[0]) {
		t.Fatalf("tradução B divergente deveria permanecer literal: %q", e2.Rows[0].Text["us"])
	}
	if got := e1.Rows[0].Text[common.DefaultLocalization]; got != a {
		t.Fatalf("tradução A: %q", got)
	}
	if got := e2.Rows[0].Text[common.DefaultLocalization]; got != b {
		t.Fatalf("tradução B div: %q", got)
	}

	// E as duas compartilham o PONTEIRO do mesmo original: a identidade
	// estrutural sobrevive às traduções.
	wantPointer := hash.Sum64Hex(sharedOriginal)
	if got := e1.Rows[0].Hash[common.DefaultLocalization]; got != wantPointer {
		t.Fatalf("ponteiro A: %q != %q", got, wantPointer)
	}
	if got := e2.Rows[0].Hash[common.DefaultLocalization]; got != wantPointer {
		t.Fatalf("ponteiro B: %q != %q", got, wantPointer)
	}
}

// stripEventMods remove o binário de mods/ do evento: simula a entrada NÃO
// traduzida (pristine só em data/) — o cenário em que o HashOrder cru nasce
// sem a coluna Original para a def.
func stripEventMods(t *testing.T, version common.GameVersion, id string) {
	t.Helper()
	rel, err := event.EventRelPath(id)
	if err != nil {
		t.Fatalf("relpath %s: %v", id, err)
	}
	for loc := range common.SupportedLanguages {
		lroot := common.GetLocalizationRootForVersion(version, loc)
		modsPath := filepath.Join(common.GameFilesRoot, common.ModsFolder, lroot, rel)
		if err := os.Remove(modsPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remover mods %s: %v", modsPath, err)
		}
	}
}

// TestGetEntryRefAnnotationCarriesDefOriginal: def NÃO traduzida (sem
// mods/, pristine só em data/) colapsa a twin em ref e a anotação de ref
// precisa carregar o Original da def — é o texto do tooltip "Repetição de:".
// Regressão: o HashOrder cru (normalizeCollection) só merge Original de
// entradas COM mods/, então a anotação da def não traduzida vinha vazia e o
// tooltip saía truncado ("Repetição de: " sem texto).
func TestGetEntryRefAnnotationCarriesDefOriginal(t *testing.T) {
	svc := seedDisplayRoot(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{sharedOriginal})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{sharedOriginal})

	seedEventsData(t, common.GameVersionFFX2, "zev001")
	seedEventsData(t, common.GameVersionFFX2, "zev002")
	// Def e twin sem mods/: nenhuma tradução — o caso comum do dedup.
	stripEventMods(t, common.GameVersionFFX2, "zev001")
	stripEventMods(t, common.GameVersionFFX2, "zev002")

	twin, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get twin: %v", err)
	}
	if !displayRowIsRef(twin.Rows[0]) {
		t.Fatalf("twin não traduzida deveria colapsar sob a def: %q", twin.Rows[0].Text["us"])
	}
	link, ok := twin.Refs[dto.RowKey(twin.Rows[0])]
	if !ok {
		t.Fatal("row de ref sem anotação Refs")
	}
	if link.SourceID != "zev001" {
		t.Fatalf("anotação deveria apontar para a def zev001: %q", link.SourceID)
	}
	if link.Original != sharedOriginal {
		t.Fatalf("Original da anotação deveria carregar o pristine da def: %q != %q",
			link.Original, sharedOriginal)
	}
}

// TestGetEntryRefAnnotationCarriesAllLangOriginals: a anotação de ref deve
// carregar o pristine da def em TODOS os idiomas (RefLink.Originals), não só
// 'us'. É o que o editor abre-pela-ref mostra na coluna de idiomas extras —
// sem o mapa, a ref cuja def vive em OUTRO arquivo perdia os extras e mostrava
// só 'us' (o Original acima é string única de 'us').
func TestGetEntryRefAnnotationCarriesAllLangOriginals(t *testing.T) {
	const jpOriginal = "共有フレーズ"
	svc := seedDisplayRoot(t)
	seedEventsMultiLang(common.GameVersionFFX2, "zev001", sharedOriginal, jpOriginal)
	seedEventsMultiLang(common.GameVersionFFX2, "zev002", sharedOriginal, jpOriginal)

	seedEventsData(t, common.GameVersionFFX2, "zev001")
	seedEventsData(t, common.GameVersionFFX2, "zev002")
	stripEventMods(t, common.GameVersionFFX2, "zev001")
	stripEventMods(t, common.GameVersionFFX2, "zev002")

	twin, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get twin: %v", err)
	}
	if !displayRowIsRef(twin.Rows[0]) {
		t.Fatalf("twin não traduzida deveria colapsar sob a def: %q", twin.Rows[0].Text["us"])
	}
	link, ok := twin.Refs[dto.RowKey(twin.Rows[0])]
	if !ok {
		t.Fatal("row de ref sem anotação Refs")
	}
	if link.SourceID != "zev001" {
		t.Fatalf("anotação deveria apontar para a def zev001: %q", link.SourceID)
	}
	if link.Originals == nil {
		t.Fatal("anotação deveria carregar o mapa de idiomas da def (Originals)")
	}
	if link.Originals[common.DefaultLocalization] != sharedOriginal {
		t.Fatalf("Originals['us'] = %q, esperado %q", link.Originals[common.DefaultLocalization], sharedOriginal)
	}
	if link.Originals["jp"] != jpOriginal {
		t.Fatalf("Originals['jp'] = %q, esperado %q — idioma extra perdido na ref", link.Originals["jp"], jpOriginal)
	}
}

// Divergir no editor gera o mesmo estado válido que uma edição externa:
// ao aplicar em binário, a def e as cópias normais recebem A, a cópia marcada
// recebe B; ao recarregar, A/B são traduções literais (não refs azuis).
func TestApplyDivergentCopyKeepsDefinitionGroupSeparate(t *testing.T) {
	svc := seedDisplayRoot(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{sharedOriginal})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{sharedOriginal})
	seedEventsSynthetic(common.GameVersionFFX2, "zev003", []string{sharedOriginal})
	for _, id := range []string{"zev001", "zev002", "zev003"} {
		seedEventsData(t, common.GameVersionFFX2, id)
		stripEventMods(t, common.GameVersionFFX2, id)
	}

	def, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get definition: %v", err)
	}
	copyEntry, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get copy: %v", err)
	}
	if !displayRowIsRef(copyEntry.Rows[0]) {
		t.Fatalf("pré-condição: copy deveria chegar como ref; got %q", copyEntry.Rows[0].Text["us"])
	}

	const translationA = "Translation A from definition"
	const translationB = "Translation B for this copy"
	def.Rows[0].Text[common.DefaultLocalization] = translationA
	copyEntry.Rows[0].Text[common.DefaultLocalization] = translationB
	copyEntry.Rows[0].Divergent = true

	if err := svc.ApplyTextCollection(KindEvents, common.GameVersionFFX2, dto.Collection{
		"zev001": def,
		"zev002": copyEntry,
	}); err != nil {
		t.Fatalf("apply translations: %v", err)
	}

	for id, want := range map[string]string{
		"zev001": translationA,
		"zev002": translationB,
		"zev003": translationA,
	} {
		got := event.GetEvent(common.GameVersionFFX2, id).Strings[0].GetLocalizedContent(common.DefaultLocalization).GetRegularString()
		if got != want {
			t.Errorf("binary %s = %q, esperado %q", id, got, want)
		}
	}

	for id, want := range map[string]string{"zev002": translationB, "zev003": translationA} {
		entry, err := svc.GetEntry(KindEvents, id, common.GameVersionFFX2)
		if err != nil {
			t.Fatalf("reload %s: %v", id, err)
		}
		if displayRowIsRef(entry.Rows[0]) {
			t.Errorf("reloaded translated row %s should be literal, got ref %q", id, entry.Rows[0].Text[common.DefaultLocalization])
		}
		if got := entry.Rows[0].Text[common.DefaultLocalization]; got != want {
			t.Errorf("reloaded %s = %q, esperado %q", id, got, want)
		}
	}

	// Reverter a cópia para o pristine é outro self-target: o binário dela
	// volta ao inglês. A def (zev001) continua traduzida, então o view
	// seguinte entrega a cópia como célula própria, em inglês — dedupe é
	// só de original para original e nunca pinta a tradução da def em cima
	// de binário não traduzido.
	reverted, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get divergent copy to revert: %v", err)
	}
	reverted.Rows[0].Text[common.DefaultLocalization] = sharedOriginal
	reverted.Rows[0].Divergent = true
	if err := svc.ApplyTextCollection(KindEvents, common.GameVersionFFX2, dto.Collection{"zev002": reverted}); err != nil {
		t.Fatalf("apply reversion: %v", err)
	}
	afterRevert, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("reload reverted copy: %v", err)
	}
	if displayRowIsRef(afterRevert.Rows[0]) {
		t.Fatalf("cópia revertida não pode virar ref sob def traduzida, got %q", afterRevert.Rows[0].Text[common.DefaultLocalization])
	}
	if got := afterRevert.Rows[0].Text[common.DefaultLocalization]; got != sharedOriginal {
		t.Fatalf("cópia revertida deveria exibir o original em inglês, got %q", got)
	}
}

// TestGetEntryWithoutOriginalKeepsRawPointers: sem contraparte em data/
// a entrada degrada — sem Original e sem reescrita de ponteiro; o colapso
// cai no ramo por texto, sem ponteiros de origem mista.
func TestGetEntryWithoutOriginalKeepsRawPointers(t *testing.T) {
	svc := seedDisplayRoot(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{sharedOriginal})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{sharedOriginal})

	// GameFilesRoot é um temp dir: nenhum arquivo em data/ nem mods/.
	e, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if e.Rows[0].Original != nil {
		t.Fatal("entrada sem contraparte em data/ não deveria ter Original")
	}
	wantRaw := hash.Sum64Hex(sharedOriginal)
	if got := e.Rows[0].Hash[common.DefaultLocalization]; got != wantRaw {
		t.Fatalf("ponteiro RAW deveria prevalecer: %q != %q", got, wantRaw)
	}
}
