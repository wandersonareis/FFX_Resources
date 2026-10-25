package progress

import (
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/loggingService"
)

// captured é a seam de publicação: grava (evento, snapshot) sem tocar no
// wails.
type captured struct {
	events []string
	snaps  []BatchSnapshot
}

func newTestEmitter(warn func(string)) (*Emitter, *captured) {
	em := NewEmitter(warn)
	cap := &captured{}
	em.emit = func(event string, snap BatchSnapshot) {
		cap.events = append(cap.events, event)
		cap.snaps = append(cap.snaps, snap)
	}
	return em, cap
}

func (c *captured) count(event string) int {
	n := 0
	for _, e := range c.events {
		if e == event {
			n++
		}
	}
	return n
}

func (c *captured) last() BatchSnapshot {
	return c.snaps[len(c.snaps)-1]
}

// O ciclo agrupado: steps declarados upfront (o ProgressStart leva a
// lista completa), conduzidos como Reporter, encerrados um a um até o
// ProgressEnd com o estado final.
func TestBatchCycle(t *testing.T) {
	em, cap := newTestEmitter(nil)
	batch := em.NewBatch("Exportando textos (XLSX)…")
	steps := batch.Steps("Eventos", "Sistema")
	if len(steps) != 2 {
		t.Fatalf("steps declarados: %d, esperado 2", len(steps))
	}

	// Primeira declaração é o ProgressStart: modal abre com os dois steps
	// pendentes.
	if got := cap.count("ProgressStart"); got != 1 {
		t.Fatalf("ProgressStart: %d, esperado 1", got)
	}
	start := cap.last()
	if start.Title != "Exportando textos (XLSX)…" || len(start.Steps) != 2 {
		t.Fatalf("start: %+v", start)
	}
	for _, st := range start.Steps {
		if st.State != StepPending {
			t.Fatalf("step %q deveria abrir pendente, veio %q", st.Label, st.State)
		}
	}

	// Ação 1: begin indeterminado, total chega com a extração (SetTotal),
	// tick por item, end. Ticks de meio de lote são throttled — o estado
	// completo só aparece na transição (End publica forçado).
	steps[0].Begin("", 0)
	steps[0].SetTotal(301)
	for i := 0; i < 3; i++ {
		steps[0].Step("ev")
	}
	if snap := cap.last(); snap.Current != 0 || snap.Steps[0].State != StepRunning {
		t.Fatalf("durante ação 1: %+v", snap)
	}
	steps[0].End()
	if snap := cap.last(); snap.Steps[0].Total != 301 || snap.Steps[0].Processed != 3 || snap.Steps[0].State != StepDone {
		t.Fatalf("end da ação 1 deveria levar 3/301 done: %+v", snap.Steps[0])
	}

	// Ação 2: começa e encerra — a corrente anda.
	steps[1].Begin("", 1)
	steps[1].Step("command")
	steps[1].End()
	if snap := cap.last(); snap.Current != 1 || snap.Steps[1].State != StepDone {
		t.Fatalf("durante ação 2: %+v", snap)
	}

	batch.SetSummary(7, "mods/edits")
	batch.End()
	end := cap.last()
	if !end.Done || end.Written != 7 || end.Destination != "mods/edits" {
		t.Fatalf("end: %+v", end)
	}
	if end.Steps[0].Processed != 3 || end.Steps[0].State != StepDone {
		t.Fatalf("end step 1: %+v", end.Steps[0])
	}
	if end.Steps[1].Processed != 1 || end.Steps[1].State != StepDone {
		t.Fatalf("end step 2: %+v", end.Steps[1])
	}
	if end.Current != 1 {
		t.Fatalf("current final %d, esperado 1 (última ação)", end.Current)
	}

	// End idempotente: nenhum evento extra. Steps pós-End: canais
	// soltos, publicam nada.
	before := len(cap.events)
	batch.End()
	batch.Step("Tarde").Begin("", 0)
	if len(cap.events) != before {
		t.Fatalf("End/step pós-End publicou %d evento(s)", len(cap.events)-before)
	}
}

// O throttle: rajada de ticks no mesmo instante não re-renderiza a UI —
// só transições (Begin/End) publicam; o End do step leva o estado CHEIO.
func TestBatchThrottle(t *testing.T) {
	em, cap := newTestEmitter(nil)
	batch := em.NewBatch("Extração")
	step := batch.Step("Arquivos")
	step.Begin("", 100)
	updates := cap.count("ProgressUpdate")
	for i := 0; i < 100; i++ {
		step.Step("f")
	}
	if more := cap.count("ProgressUpdate") - updates; more > 5 {
		t.Fatalf("rajada de 100 ticks publicou %d updates (throttle de 10/s)", more)
	}
	step.End()
	if last := cap.last(); last.Steps[0].Processed != 100 || last.Steps[0].State != StepDone {
		t.Fatalf("o End deveria publicar o estado cheio, veio %+v", last.Steps[0])
	}
}

// Issue: conta na ação, avança o item e tosta com cap — além do cap o
// erro segue só no diagnóstico.
func TestStepIssueCap(t *testing.T) {
	loggingService.ResetForTest(tempdir.TempDir(t))
	t.Cleanup(func() { loggingService.ResetForTest(tempdir.TempDir(t)) })

	warns := 0
	em, cap := newTestEmitter(func(string) { warns++ })
	batch := em.NewBatch("Lote")
	step := batch.Step("Eventos")
	step.Begin("", 20)
	for i := 0; i < maxIssueToasts+5; i++ {
		step.Issue("ev", "falha")
	}
	if warns != maxIssueToasts {
		t.Fatalf("toasts: %d, esperado o cap %d", warns, maxIssueToasts)
	}
	step.End()
	batch.End()
	end := cap.last()
	if end.IssueCount != maxIssueToasts+5 {
		t.Fatalf("issueCount %d, esperado %d", end.IssueCount, maxIssueToasts+5)
	}
	if end.Steps[0].Processed != maxIssueToasts+5 {
		t.Fatalf("issue deveria contar como item concluído: processed %d", end.Steps[0].Processed)
	}
}

// Fail marca a ação como falha no resultado — vale mesmo depois do End
// (o autor do batch decide o veredito) — e o batch sai failed.
func TestStepFail(t *testing.T) {
	em, cap := newTestEmitter(nil)
	batch := em.NewBatch("Export")
	step := batch.Step("Macro")
	step.Begin("", 10)
	step.End()
	step.Fail("no matching macro chunks")
	batch.End()
	final := cap.last()
	if !final.Failed || final.Steps[0].State != StepError || !strings.Contains(final.Steps[0].Error, "macro") {
		t.Fatalf("batch falho: %+v", final)
	}
	if final.Steps[0].State == StepDone {
		t.Fatalf("Fail deveria sobrescrever o done: %+v", final.Steps[0])
	}
}

// Sem emitter (testes/CLI/appcore): batch nil, steps nil, tudo no-op —
// nenhuma operação derruba por falta de progresso.
func TestNilSafety(t *testing.T) {
	var em *Emitter
	batch := em.NewBatch("Nada")
	if batch != nil {
		t.Fatal("emitter nil deveria dar batch nil")
	}
	if steps := batch.Steps("A", "B"); steps != nil {
		t.Fatal("steps de batch nil deveria ser nil")
	}
	if step := batch.Step("A"); step != nil {
		t.Fatal("step de batch nil deveria ser nil")
	}
	// Todos os canais são nil-receiver-safe: chamar direto não pânica.
	var nilStep *Step
	nilStep.Begin("", 0)
	nilStep.SetTotal(1)
	nilStep.Step("x")
	nilStep.Issue("x", "boom")
	nilStep.End()
	nilStep.Fail("boom")
	batch.SetSummary(0, "")
	batch.Fail()
	batch.End()

	// OrNop: nil vira no-op explícito.
	r := OrNop(nil)
	r.Begin("", 0)
	r.Step("x")
	r.Issue("x", "boom")
	r.End()
	if _, ok := r.(nopReporter); !ok {
		t.Fatal("OrNop(nil) deveria devolver o no-op")
	}
}
