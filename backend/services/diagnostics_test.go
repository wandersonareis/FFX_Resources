package services

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters/hash"
	"ffxresources/backend/loggingService"
)

// diagnosticsRoot monta gamefiles temporários com a estrutura mínima de
// events e devolve o serviço pronto.
func diagnosticsRoot(t *testing.T) *MetadataService {
	t.Helper()
	seedEventsTempRoot(t)
	event.ClearEvents(common.GameVersionFFX2)
	t.Cleanup(func() { event.ClearEvents(common.GameVersionFFX2) })
	clearDedupViewCache()
	t.Cleanup(clearDedupViewCache)
	// Log em UM arquivo por início de app (console silenciado).
	loggingService.ResetForTest(tempdir.TempDir(t))
	t.Cleanup(func() { loggingService.ResetForTest(tempdir.TempDir(t)) })
	return NewMetadataService(nil)
}

// logFileContent devolve o conteúdo do arquivo único do diretório ativo —
// log geral e diagnóstico (campo `key`) vivem no mesmo arquivo ("" quando
// ausente).
func logFileContent(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(loggingService.LogDir())
	if err != nil {
		t.Fatalf("ler logs: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ffx-") {
			b, err := os.ReadFile(filepath.Join(loggingService.LogDir(), e.Name()))
			if err != nil {
				t.Fatalf("ler log: %v", err)
			}
			return string(b)
		}
	}
	return ""
}

// resetLogFresh reinicia o sistema de logs em um diretório NOVO: o conteúdo
// dos arquivos de log passa a conter só o que vier depois (asserções de
// igualdade entre execuções do inventário).
func resetLogFresh(t *testing.T) {
	t.Helper()
	loggingService.ResetForTest(tempdir.TempDir(t))
}

// TestTreeDiagCountsParallelDeterministic: o inventário reporta as quatro
// situações — e o resultado não muda com o número de workers.
func TestTreeDiagCountsParallelDeterministic(t *testing.T) {
	svc := diagnosticsRoot(t)

	// 1 arquivo completo (data + mods), 1 só em data, 1 só em mods.
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase do zev001"})
	seedEventsData(t, common.GameVersionFFX2, "zev001") // data + mods

	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase do zev002"})
	seedEventsData(t, common.GameVersionFFX2, "zev002") // data + mods
	// remove mods/ do zev002: arquivo em data/ sem tradução.
	lroot := common.GetLocalizationRootForVersion(common.GameVersionFFX2, common.DefaultLocalization)
	rel, _ := event.EventRelPath("zev002")
	if err := os.Remove(filepath.Join(common.GameFilesRoot, common.ModsFolder, lroot, rel)); err != nil {
		t.Fatalf("remover mods: %v", err)
	}

	// zev003: só em mods/ (binário escrito direto na árvore mods/).
	seedEventsSynthetic(common.GameVersionFFX2, "zev003", []string{"Frase do zev003"})
	if err := event.ExportEventStringsToLocalizations(common.GameVersionFFX2, "zev003"); err != nil {
		t.Fatalf("export zev003: %v", err)
	}

	run := func() (info, warn string) {
		resetLogFresh(t)
		clearDedupViewCache()
		if _, err := svc.ListEntries(KindEvents, common.GameVersionFFX2); err != nil {
			t.Fatalf("list: %v", err)
		}
		// Arquivo único: info e warn saem do mesmo lugar.
		content := logFileContent(t)
		return content, content
	}

	prevWorkers := diagWorkers
	defer func() { diagWorkers = prevWorkers }()

	stripTime := regexp.MustCompile(`,"time":"[^"]+"`)
	normalize := func(s string) string { return stripTime.ReplaceAllString(s, "") }

	diagWorkers = 1
	info1, warn1 := run()
	diagWorkers = 4
	info4, warn4 := run()

	if !strings.Contains(info1, "arquivos em data/ = 2") ||
		!strings.Contains(info1, "faltam 1 para traduzir") {
		t.Fatalf("sumário info (1 worker): %q", info1)
	}
	if !strings.Contains(warn1, "mods com arquivo ausente em data/") {
		t.Fatalf("warn só em mods (1 worker): %q", warn1)
	}
	if normalize(info1) != normalize(info4) || normalize(warn1) != normalize(warn4) {
		t.Fatalf("resultado depende do nº de workers:\n1: %q / %q\n4: %q / %q", info1, warn1, info4, warn4)
	}
}

// TestNormalizeCollectionEmitsDivergenceInOrder: o merge em paralelo coleta
// as divergências e o aviso sai uma vez por entrada, com a key no texto.
func TestNormalizeCollectionEmitsDivergenceInOrder(t *testing.T) {
	svc := diagnosticsRoot(t)

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase do zev001", "Outra do zev001"})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase do zev002"})
	seedEventsData(t, common.GameVersionFFX2, "zev001") // data = original completo
	seedEventsData(t, common.GameVersionFFX2, "zev002") // data = original completo

	// A tradução em mods tem uma row a MENOS: mods com 2 rows em zev001? —
	// aqui simula o caso contrário: data com row extra (row 2 do original
	// não existe no mods). Remove a segunda string do store de zev001.
	ev := event.GetEvent(common.GameVersionFFX2, "zev001")
	ev.Strings = ev.Strings[:1]
	clearDedupViewCache()

	if _, err := svc.ListEntries(KindEvents, common.GameVersionFFX2); err != nil {
		t.Fatalf("list: %v", err)
	}
	entry, err := svc.GetEntry(KindEvents, "zev001", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	// A tabela é a UNIÃO dos dois lados: a row que só existe no original
	// entra marcada como ausente da tradução, com a coluna Traduzido vazia.
	if len(entry.Rows) != 2 {
		t.Fatalf("rows: %d", len(entry.Rows))
	}
	want := hash.Sum64Hex("Frase do zev001")
	if got := entry.Rows[0].Hash[common.DefaultLocalization]; got != want {
		t.Fatalf("ponteiro: %q != %q", got, want)
	}
	onlyOrig := entry.Rows[1]
	if !onlyOrig.MissingInTranslated {
		t.Fatal("row só no original não foi marcada como ausente da tradução")
	}
	if onlyOrig.MissingInOriginal {
		t.Fatal("row que existe no original não pode marcar MissingInOriginal")
	}
	if len(onlyOrig.Text) != 0 {
		t.Fatalf("texto do original vazou para a coluna Traduzido: %v", onlyOrig.Text)
	}
	if onlyOrig.Original == nil {
		t.Fatal("row só no original veio sem a coluna Original")
	}
	if entry.Rows[0].MissingInTranslated {
		t.Fatal("row casada não pode estar marcada como órfã")
	}

	// Diagnóstico: divergência com a key da entrada e a row só em data/.
	diag := logFileContent(t)
	if !strings.Contains(diag, `"key":"events/zev001 (ffx2)"`) {
		t.Fatalf("key ausente no diagnóstico:\n%s", diag)
	}
	if !strings.Contains(diag, `"rows_somente_data":[{"index":1`) {
		t.Fatalf("row só em data/ ausente:\n%s", diag)
	}
}

// TestParallelForOrderIndependent: parallelFor executa tudo, qualquer que
// seja o nº de workers.
func TestParallelForOrderIndependent(t *testing.T) {
	prev := diagWorkers
	defer func() { diagWorkers = prev }()

	for _, workers := range []int{1, 4} {
		diagWorkers = workers
		n := 64
		results := make([]int, n)
		parallelFor(n, func(i int) {
			results[i] = i * 2
		})
		for i := range results {
			if results[i] != i*2 {
				t.Fatalf("worker=%d: results[%d] = %d", workers, i, results[i])
			}
		}
	}
}

// TestTreeDiagObjectsInventoriesLayouts: entrada fantasma (sem arquivo em
// nenhuma árvore) não gera sumário — o inventário silencia o vazio.
func TestTreeDiagObjectsInventoriesLayouts(t *testing.T) {
	svc := diagnosticsRoot(t)
	resetLogFresh(t)
	diag := newTreeDiag(KindObjects, common.GameVersionFFX2)
	presence := svc.diagPresenceResolve(KindObjects, common.GameVersionFFX2, []string{"inexistente"})
	diag.add(presence[0], "inexistente", "")
	diag.emit()
	if c := logFileContent(t); strings.Contains(c, "inexistente") {
		t.Fatalf("entrada fantasma no sumário: %s", c)
	}
}
