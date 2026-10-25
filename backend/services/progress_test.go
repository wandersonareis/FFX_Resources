package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/progress"
)

// A carga de eventos é IMPLÍCITA (boot/carga de árvore) e não abre modal:
// o loader coleta os pulados em Skipped e uma carga feliz deixa a lista
// VAZIA — quem reporta é o chamador (ensureEventsLoaded).
func TestLoadFromBinarySilentProgress(t *testing.T) {
	root := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })

	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase do zev001"})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase do zev002"})
	seedEventsData(t, common.GameVersionFFX2, "zev001") // export → mods + copy → data
	seedEventsData(t, common.GameVersionFFX2, "zev002")

	b := event.NewEventsBinaryFile(common.GameVersionFFX2)
	if err := b.LoadFromBinary(); err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(b.Skipped) != 0 {
		t.Fatalf("carga feliz não deveria pular nada, pulou: %+v", b.Skipped)
	}
}

// Evento com binário corrompido é pulado com Skipped (o services reporta
// cada um: toast com cap + arquivo de diagnóstico) e o restante carrega.
func TestLoadFromBinarySkipsBrokenEvent(t *testing.T) {
	root := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() { common.GameFilesRoot = prevRoot })

	if err := ensureVersionReady(common.GameVersionFFX2); err != nil {
		t.Fatalf("prepare version: %v", err)
	}

	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{"Frase do zev001"})
	seedEventsSynthetic(common.GameVersionFFX2, "zev002", []string{"Frase do zev002"})
	seedEventsData(t, common.GameVersionFFX2, "zev001")
	seedEventsData(t, common.GameVersionFFX2, "zev002")

	// zev002: TODOS os binários de leitura (data/ por localização) viram
	// DIRETÓRIOS → a leitura falha em todos, o evento sai vazio e é PULADO
	// enquanto o resto carrega.
	for loc := range common.SupportedLanguages {
		lroot := common.GetLocalizationRootForVersion(common.GameVersionFFX2, loc)
		rel, _ := event.EventRelPath("zev002")
		broken := filepath.Join(common.GameFilesRoot, lroot, rel)
		if err := os.Remove(broken); err != nil {
			t.Fatalf("remover binário %s: %v", broken, err)
		}
		if err := os.MkdirAll(broken, 0o755); err != nil {
			t.Fatalf("transformar em diretório: %v", err)
		}
	}

	prevMods := common.DisableMods
	common.DisableMods = true
	t.Cleanup(func() { common.DisableMods = prevMods })

	b := event.NewEventsBinaryFile(common.GameVersionFFX2)
	if err := b.LoadFromBinary(); err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(b.Skipped) != 1 || b.Skipped[0].ID != "zev002" {
		t.Fatalf("zev002 deveria vir pulado em Skipped, veio %+v", b.Skipped)
	}
	if b.Objects == nil || b.Objects.Len() != 1 {
		t.Fatalf("a carga deveria seguir com o evento válido: %v", b.Objects)
	}

	// reportEventSkips: diagnóstico + toast com cap (além dele, silencia).
	loggingService.ResetForTest(tempdir.TempDir(t))
	t.Cleanup(func() { loggingService.ResetForTest(tempdir.TempDir(t)) })

	fake := &fakeNotifier{}
	s := NewMetadataService(fake)
	s.reportEventSkips(b.Skipped)
	if len(fake.warns) != 1 || !strings.Contains(fake.warns[0], "zev002") {
		t.Fatalf("skip deveria virar toast de aviso, veio %v", fake.warns)
	}
	many := make([]event.EventSkip, maxSkipToasts+5)
	for i := range many {
		many[i] = event.EventSkip{ID: "zev", Reason: "x"}
	}
	fake.warns = nil
	s.reportEventSkips(many)
	if len(fake.warns) != maxSkipToasts {
		t.Fatalf("cap de toasts: %d, esperado %d", len(fake.warns), maxSkipToasts)
	}
}

// (fakeNotifier vive em vbf_service_test.go — reutilizado aqui.)

// Sem emitter, o MetadataService segue operando sem progresso: os batches
// são nil e todos os métodos no-op — nada derruba por falta de ponte.
func TestMetadataServiceWithoutProgress(t *testing.T) {
	s := NewMetadataService(nil)
	batch, step := s.progressStep("Teste", 3)
	if batch != nil || step != nil {
		t.Fatalf("sem emitter deveria devolver (nil, nil)")
	}
	// Canais nil-safe: chamar direto não pânica.
	step.Begin("", 3)
	step.Step("x")
	step.Issue("x", "boom")
	step.End()
	step.Fail("boom")
	batch.SetSummary(0, "")
	batch.End()

	if _, ok := progress.OrNop(nil).(interface{ Step(string) }); !ok {
		t.Fatal("OrNop(nil) deveria devolver um Reporter no-op")
	}
}

// PreloadVersions é idempotente e aborta quando a árvore muda no meio.
func TestPreloadVersionsIdempotentAndInvalidated(t *testing.T) {

	// Ambiente sem gamefiles válidos: a pré-carga falha por versão — mas o
	// Set marca/consome igualmente (idempotência é sobre a fila, não sobre o
	// sucesso).
	version := common.GameVersionFFX2
	if !markPreloaded(version) {
		t.Fatal("primeira marca deveria ser nova")
	}
	if markPreloaded(version) {
		t.Fatal("segunda marca deveria ser ignorada")
	}

	treeGeneration.Add(1)
	InvalidateViewCaches() // geração nova zera o Set de pré-carga
	if !markPreloaded(version) {
		t.Fatal("invalidação deveria liberar a versão de novo")
	}
}
