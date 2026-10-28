package services

// Escopo do export de EVENTS por versão. As expansões (Eternal Calm sob a
// árvore do FFX, Last Mission sob a do FFX-2) dividem o MESMO bucket do
// datastore, mas a régua de grupos é por aba:
//   - eternalcalm exporta só o grupo "sc" (sc/scene*);
//   - ffx NÃO exporta o grupo "sc" (é conteúdo do Eternal Calm);
//   - lastmiss exporta só o grupo "lm"; ffx2 não o exporta.
//
// Antes esta régua existia só na listagem da árvore (ListEntries): o export
// bulk varria o bucket inteiro e a aba Eternal Calm gravava TODOS os eventos
// do FFX. O teste falha sem o filtro no exportKind.
//
// Os fixtures são os binários reais de data/ copiados para um GameFilesRoot
// temporário (mesma receita de tableBulkFixtures): o export lê de data/ e
// grava em mods/edits, tudo contido no temp — sem tocar a árvore real.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/event"
)

// eventsVersionFixtures copia os binários 'us' dos eventos pedidos
// (relativos a <gamefiles>/<raiz de localização>) para um GameFilesRoot
// temporário e devolve o caminho absoluto do diretório obj_ps3 semeado.
// Fixture ausente em data/ pula o teste (ambiente sem o jogo extraído).
func eventsVersionFixtures(t *testing.T, version common.GameVersion, rels []string) {
	t.Helper()
	dataRoot := repoDataRootLocal(t)
	if _, err := os.Stat(dataRoot); err != nil {
		t.Skip("build/bin/data indisponível")
	}
	root := tempdir.TempDir(t)
	prevRoot, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = root
	common.DisableMods = true
	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		common.DisableMods = prevMods
		event.ClearAllEvents()
	})
	event.ClearAllEvents()

	locRoot := filepath.FromSlash(common.GetLocalizationRootForVersion(version, common.DefaultLocalization))
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(dataRoot, locRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Skipf("fixture ausente: %s (%v)", rel, err)
		}
		dst := filepath.Join(root, locRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("criar diretório de %s: %v", rel, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("copiar %s: %v", rel, err)
		}
	}
}

// idsDoArtefato lê as chaves de topo de um artefato JSON de eventos (o
// universo que o export realmente gravou).
func idsDoArtefato(t *testing.T, path string) []string {
	t.Helper()
	keys := parseBulkArtifact(t, "json", path).SortedKeys()
	if len(keys) == 0 {
		t.Fatalf("artefato %s sem entradas", path)
	}
	return keys
}

// Fixture mínimo e real: um evento do grupo "az" (FFX) e um do grupo "sc"
// (Eternal Calm), os dois na árvore compartilhada do FFX.
var eventsScopeRels = []string{
	"event/obj_ps3/az/azit0000/azit0000.bin",
	"event/obj_ps3/sc/scene1/scene1.bin",
}

// A aba Eternal Calm exporta SÓ os arquivos dela: o universo do artefato é
// o grupo "sc", e o nome do arquivo carrega o sufixo da versão.
func TestExportEventsEternalCalmScope(t *testing.T) {
	eventsVersionFixtures(t, common.GameVersionEternalCalm, eventsScopeRels)
	if err := ensureVersionReady(common.GameVersionEternalCalm); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	svc := NewMetadataService(nil)

	paths, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindEvents}}, common.GameVersionEternalCalm, nil, "json")
	if err != nil {
		t.Fatalf("export eternalcalm: %v", err)
	}
	if base := filepath.Base(paths[0]); base != "events_all_localizations_eternalcalm.json" {
		t.Fatalf("nome do artefato: %s", base)
	}

	ids := idsDoArtefato(t, paths[0])
	for _, id := range ids {
		if !strings.HasPrefix(id, EternalCalmShortened) {
			t.Fatalf("export da aba Eternal Calm vazou evento de outra versão: %s", id)
		}
	}
	if len(ids) == 0 {
		t.Fatal("export vazio: era para trazer os eventos do Eternal Calm")
	}
}

// A aba FFX NÃO exporta o grupo "sc" — é conteúdo do Eternal Calm e não
// deve aparecer no artefato canônico do FFX.
func TestExportEventsFfxScopeHidesEternalCalm(t *testing.T) {
	eventsVersionFixtures(t, common.GameVersionFFX, eventsScopeRels)
	if err := ensureVersionReady(common.GameVersionFFX); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	svc := NewMetadataService(nil)

	paths, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindEvents}}, common.GameVersionFFX, nil, "json")
	if err != nil {
		t.Fatalf("export ffx: %v", err)
	}
	if base := filepath.Base(paths[0]); base != "events_all_localizations_ffx.json" {
		t.Fatalf("nome do artefato: %s", base)
	}

	ids := idsDoArtefato(t, paths[0])
	if len(ids) == 0 {
		t.Fatal("export vazio: era para trazer os eventos do FFX")
	}
	for _, id := range ids {
		if strings.HasPrefix(id, EternalCalmShortened) {
			t.Fatalf("export do FFX vazou evento do Eternal Calm: %s", id)
		}
	}
}

// O artefato é ÚNICO por versão: o export da Eternal Calm não sobrescreve
// o do FFX (nome canônico com o sufixo da versão) e vice-versa.
func TestExportEventsVersionArtifactsDoNotCollide(t *testing.T) {
	eventsVersionFixtures(t, common.GameVersionEternalCalm, eventsScopeRels)
	if err := ensureVersionReady(common.GameVersionEternalCalm); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	svc := NewMetadataService(nil)

	ffx, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindEvents}}, common.GameVersionFFX, nil, "json")
	if err != nil {
		t.Fatalf("export ffx: %v", err)
	}
	calm, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindEvents}}, common.GameVersionEternalCalm, nil, "json")
	if err != nil {
		t.Fatalf("export eternalcalm: %v", err)
	}
	if ffx[0] == calm[0] {
		t.Fatalf("os artefatos das duas versões colidiram em %s", ffx[0])
	}
	for _, p := range [][]string{ffx, calm} {
		if _, err := os.Stat(p[0]); err != nil {
			t.Fatalf("artefato não gravado: %v", err)
		}
	}
}

// O caminho canônico continua independente do filtro: exportar um único
// evento do Eternal Calm grava o artefato por entrada, não o bulk.
func TestExportEventsSingleEntryKeepsCanonicalName(t *testing.T) {
	eventsVersionFixtures(t, common.GameVersionEternalCalm, eventsScopeRels)
	if err := ensureVersionReady(common.GameVersionEternalCalm); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	svc := NewMetadataService(nil)

	// id do grupo sc no fixture semeado: event/obj_ps3/sc/scene1/scene1.bin.
	scID := "scene1"

	paths, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindEvents, IDs: []string{scID}}},
		common.GameVersionEternalCalm, nil, "json")
	if err != nil {
		t.Fatalf("export unitário: %v", err)
	}
	want, err := eventsExportPath(common.GameVersionEternalCalm, []string{scID}, "")
	if err != nil {
		t.Fatalf("caminho canônico: %v", err)
	}
	if base := filepath.Base(paths[0]); base != filepath.Base(want) {
		t.Fatalf("artefato por entrada: %s, queria %s", base, filepath.Base(want))
	}
	if got := idsDoArtefato(t, paths[0]); len(got) != 1 || got[0] != scID {
		t.Fatalf("artefato por entrada com universo %v, queria [%s]", got, scID)
	}
}
