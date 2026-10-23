package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters/hash"
)

// Regressão do self-deadlock do view dedupado (achado pelo e2e
// mods-toggle-navigation.spec): rawViewOf construía o view cru segurando o
// dedupViewMu, e a própria build (GetCollection → EnsureAllEventsFresh)
// podia recarregar um evento vencido e chamar clearDedupViewCache na
// MESMA goroutine — mutex não é reentrante, a carga travava nela mesma e
// TODA carga de events a partir dali bloqueava no lock mantido.
//
// Gatilho do mundo real: o binário em mods/ muda depois do carimbo (o
// usuário edita/copiou a tradução, ou o toggle EnableMods troca a árvore
// que os carimbos resolvem), o cache do view cai, e a PRÓXIMA carga
// precisa reconstruí-lo — recarregando o evento vencido NO MEIO da
// build. O teste reproduz esse estado e exige que a carga sobre viva (e
// que o protocolo do cache descarte a build que atravessou o clear).
func TestGetEntrySurvivesFreshReloadInsideViewBuild(t *testing.T) {
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	defer event.ClearEvents(common.GameVersionFFX2)
	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase do primeiro evento"})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase do segundo evento"})
	svc := NewMetadataService(nil)

	// Aquece o view cache com o estado atual do store.
	if _, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2); err != nil {
		t.Fatalf("primeira carga: %v", err)
	}

	// zev002 ganha um binário REAL em mods/ (o apply grava o binário
	// compilado): é o estado de um arquivo que o usuário editou/copiou.
	const translated = "Tradução aplicada do segundo evento"
	entry, err := svc.GetEntry(KindEvents, "zev002", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get zev002: %v", err)
	}
	entry.Rows[0].Text[common.DefaultLocalization] = translated
	entry.Rows[0].Hash[common.DefaultLocalization] = hash.Sum64Hex(translated)
	if err := svc.ApplyTextCollection(KindEvents, common.GameVersionFFX2, dto.Collection{"zev002": entry}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Toggle de mods: os carimbos passam a resolver mods/ primeiro, o
	// zev002 entra na vigia de frescura e o disco MUDA depois do carimbo.
	// O cache do view cai (o apply já limpou; o clear aqui é o estado
	// explícito do clique seguinte) — é exatamente o clique pós-toggle.
	common.DisableMods = false
	rel, err := event.EventRelPath("zev002")
	if err != nil {
		t.Fatalf("rel path: %v", err)
	}
	locRel := filepath.Join(
		common.GetLocalizationRootForVersion(common.GameVersionFFX2, common.DefaultLocalization),
		rel,
	)
	modsBin := filepath.Join(common.GameFilesRoot, common.ModsFolder, filepath.FromSlash(locRel))
	if _, err := os.Stat(modsBin); err != nil {
		t.Fatalf("binário do apply deveria existir em %s: %v", modsBin, err)
	}
	event.StampEvent(common.GameVersionFFX2, "zev002")
	future := time.Now().Add(3 * time.Second)
	if err := os.Chtimes(modsBin, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	clearDedupViewCache()

	// A carga que reconstrói o view com um evento vencido no meio do
	// caminho: antes do fix, EnsureAllEventsFresh → clearDedupViewCache
	// travava aqui dentro para sempre (watchdog vira dump de stack).
	viewKey := common.GameVersionFFX2.String() + "|" + KindEvents
	loaded := make(chan error, 1)
	go func() {
		_, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
		loaded <- err
	}()
	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("carga pós-clear: %v", err)
		}
	case <-time.After(30 * time.Second):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("carga de events travou (self-deadlock do view dedupado):\n%s", buf[:n])
	}

	// A build atravessou o clear interno (o reload re-carimbou o zev002):
	// ela não pode gravar no cache — o próximo leitor reconstrói.
	if _, cached := dedupViewCached(viewKey); cached {
		t.Fatal("build que atravessou o clear não deveria popular o cache")
	}

	// A carga seguinte reconstrói SEM evento vencido e grava no cache.
	if _, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2); err != nil {
		t.Fatalf("segunda carga: %v", err)
	}
	if _, cached := dedupViewCached(viewKey); !cached {
		t.Fatal("segunda carga deveria popular o cache do view")
	}
}
