// Testes do appcore: detecção de formato/versão pelos fixtures reais
// (testData/FFX e testData/FFX-2), nome canônico do export, dedupe/dupe e
// o round-trip completo arquivo→arquivo (export → editar artefato →
// import em cópia avulsa → conferir o texto reconstruído).
package appcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	jsonfmt "ffxresources/backend/formatters/json"
	"ffxresources/backend/services"
)

// useFFXRoot aponta a raiz de gamefiles para os fixtures do FFX (macrodic,
// lockit e afins são lidos da árvore pelo runtime de texto).
func useFFXRoot(t *testing.T) string {
	t.Helper()
	root := abs(t, filepath.Join("..", "..", "testData", "FFX", "binary"))
	prev, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		common.DisableMods = prevMods
	})
	return root
}

// useFFX2Root é o useFFXRoot da árvore do FFX-2.
func useFFX2Root(t *testing.T) string {
	t.Helper()
	root := abs(t, filepath.Join("..", "..", "testData", "FFX-2", "binary"))
	prev, prevMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		common.DisableMods = prevMods
	})
	return root
}

// ffxFixture devolve o caminho absoluto de um fixture do FFX (relativo à
// raiz new_uspc).
func ffxFixture(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(useFFXRoot(t), "ffx_ps2", "ffx", "master", "new_uspc", filepath.FromSlash(rel))
}

// ffx2Fixture devolve o caminho absoluto de um fixture do FFX-2.
func ffx2Fixture(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(useFFX2Root(t), "ffx_ps2", "ffx2", "master", "new_uspc", filepath.FromSlash(rel))
}

func abs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return a
}

// ---- detecção -----------------------------------------------------------------

func TestDetectChunkmapV1(t *testing.T) {
	d, err := DetectFile(ffxFixture(t, "battle/kernel/command.bin"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if d.Format != FormatChunkmapV1 {
		t.Errorf("formato = %v, esperado FormatChunkmapV1", d.Format)
	}
	if d.Kind != services.KindObjects {
		t.Errorf("kind = %q, esperado objects", d.Kind)
	}
	if !d.VersionKnown || d.Version != common.GameVersionFFX {
		t.Errorf("versão = %v (known=%v), esperado ffx", d.Version, d.VersionKnown)
	}
}

func TestDetectChunkmapV2(t *testing.T) {
	d, err := DetectFile(ffx2Fixture(t, "battle/kernel/a_ability.bin"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if d.Format != FormatChunkmapV2 {
		t.Errorf("formato = %v, esperado FormatChunkmapV2", d.Format)
	}
	if !d.VersionKnown || d.Version != common.GameVersionFFX2 {
		t.Errorf("versão = %v (known=%v), esperado ffx2", d.Version, d.VersionKnown)
	}
}

func TestDetectEvents(t *testing.T) {
	d, err := DetectFile(ffxFixture(t, "event/obj_ps3/az/azit0000/azit0000.bin"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if d.Format != FormatEvent {
		t.Errorf("formato = %v, esperado FormatEvent", d.Format)
	}
	if d.Kind != services.KindEvents {
		t.Errorf("kind = %q, esperado events", d.Kind)
	}
}

func TestDetectMacroDic(t *testing.T) {
	d, err := DetectFile(ffxFixture(t, "menu/macrodic.dcp"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if d.Format != FormatMacroDic || d.Kind != services.KindMacro {
		t.Errorf("formato=%v kind=%q, esperado macrodic/macro", d.Format, d.Kind)
	}
}

func TestDetectLockit(t *testing.T) {
	path := filepath.Join(useFFXRoot(t), "ffx_data", "gamedata", "ps3data", "lockit", "ffx_loc_kit_ps3_us.bin")
	d, err := DetectFile(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if d.Format != FormatLockit || d.Kind != services.KindLockit {
		t.Errorf("formato=%v kind=%q, esperado lockit", d.Format, d.Kind)
	}
	if !d.VersionKnown || d.Version != common.GameVersionFFX {
		t.Errorf("versão = %v (known=%v), esperado ffx pelo stem", d.Version, d.VersionKnown)
	}
}

func TestDetectHelp(t *testing.T) {
	d, err := DetectFile(ffxFixture(t, "help/dvdcopy/dvdcopy.sps2"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if d.Format != FormatHelp || d.Kind != services.KindHelp {
		t.Errorf("formato=%v kind=%q, esperado help", d.Format, d.Kind)
	}
	if !d.VersionKnown || d.Version != common.GameVersionFFX {
		t.Errorf("versão = %v, esperado ffx (help é exclusivo)", d.Version)
	}
}

func TestDetectSkipListPeloNome(t *testing.T) {
	dir := tempdir.TempDir(t)
	path := filepath.Join(dir, "bltz0001.bin") // entrada da lista de skip do FFX
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, err := DetectFile(path)
	if err == nil {
		t.Fatal("esperava incompatível (lista de skip)")
	}
	inc, ok := err.(*IncompatibleError)
	if !ok {
		t.Fatalf("erro não é IncompatibleError: %T", err)
	}
	if !strings.Contains(inc.Reason, "skip") {
		t.Errorf("motivo = %q, esperava skip", inc.Reason)
	}
}

func TestDetectExtensaoForaDaLista(t *testing.T) {
	dir := tempdir.TempDir(t)
	path := filepath.Join(dir, "aleatorio.exe")
	if err := os.WriteFile(path, []byte("MZ..."), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if _, err := DetectFile(path); err == nil {
		t.Fatal("esperava incompatível (extensão)")
	}
}

func TestDetectArtefatos(t *testing.T) {
	dir := tempdir.TempDir(t)

	j := filepath.Join(dir, "artefato.json")
	if err := os.WriteFile(j, []byte(`{"ffx/x.bin":{"rows":[]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := DetectFile(j)
	if err != nil || d.Format != FormatArtifactJSON {
		t.Fatalf("json: d=%+v err=%v", d, err)
	}

	s := filepath.Join(dir, "artefato.strings")
	if err := os.WriteFile(s, []byte("/*key=ffx/x.bin row_count=0*/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err = DetectFile(s)
	if err != nil || d.Format != FormatArtifactStrings {
		t.Fatalf("strings: d=%+v err=%v", d, err)
	}
}

// ---- nomes canônicos ----------------------------------------------------------

func TestExportBaseName(t *testing.T) {
	if got := ExportBaseName(services.KindEvents, common.GameVersionFFX, "azit0000", FormatJSON); got != "event_azit0000_all_localizations_ffx.json" {
		t.Errorf("events: %q", got)
	}
	if got := ExportBaseName(services.KindObjects, common.GameVersionFFX2, "command", FormatStrings); got != "command_all_localizations_ffx2.strings" {
		t.Errorf("objects: %q", got)
	}
	if got := ExportBaseName(services.KindMacro, common.GameVersionFFX, "chunk_00", FormatJSON); got != "macro_dictionary_all_localizations_ffx.json" {
		t.Errorf("macro: %q", got)
	}
}

// ---- dedupe/dupe --------------------------------------------------------------

func sampleDedupeCollection(t *testing.T) dto.Collection {
	t.Helper()
	c := dto.Collection{}
	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "ffx/event/obj_ps3/az/azit0000/azit0000.bin", ID: "azit0000"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Frase longa o bastante para dedupe"}, Hash: map[string]string{"us": "aaaa1111aaaa1111"}},
			{Index: 1, Text: map[string]string{"us": "Frase longa o bastante para dedupe"}, Hash: map[string]string{"us": "aaaa1111aaaa1111"}},
			{Index: 2, Text: map[string]string{"us": "Outra frase diferente e longa"}, Hash: map[string]string{"us": "bbbb2222bbbb2222"}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(len(entry.Rows))
	c["azit0000"] = entry
	return c
}

func TestDedupeDupeRoundTrip(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	c := sampleDedupeCollection(t)

	// Artefato ORIGINAL: sem dedup (texto literal em todas as rows).
	original, err := jsonfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "teste.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	// DEDUPE: a repetição vira ref "$hash".
	out, err := DedupeArtifact(ctx, ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != StatusWritten {
		t.Fatalf("dedupe status = %v (%s)", out.Status, out.Reason)
	}
	if filepath.Base(out.Output) != "dedupe_teste.json" {
		t.Errorf("saída dedupe = %q", out.Output)
	}
	deduped, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(deduped), "$aaaa1111aaaa1111") {
		t.Errorf("esperava ref $hash na saída dedupe:\n%s", string(deduped))
	}

	// DUPE: o hash de conteúdo resolve de volta ao texto completo.
	out, err = DupeArtifact(ctx, ArtifactRequest{Path: out.Output})
	if err != nil {
		t.Fatalf("dupe: %v", err)
	}
	if out.Status != StatusWritten {
		t.Fatalf("dupe status = %v (%s)", out.Status, out.Reason)
	}
	if filepath.Base(out.Output) != "dupe_dedupe_teste.json" {
		t.Errorf("saída dupe = %q", out.Output)
	}
	expanded, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}

	// O dupe devolve o MESMO conteúdo do original (identidade textual).
	var want, got map[string]dto.FileEntry
	if err := json.Unmarshal(original, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expanded, &got); err != nil {
		t.Fatal(err)
	}
	for id, wantEntry := range want {
		gotEntry, ok := got[id]
		if !ok {
			t.Fatalf("entrada %s sumiu no dupe", id)
		}
		if len(gotEntry.Rows) != len(wantEntry.Rows) {
			t.Fatalf("rows: got %d want %d", len(gotEntry.Rows), len(wantEntry.Rows))
		}
		for i, row := range wantEntry.Rows {
			if gotEntry.Rows[i].Text["us"] != row.Text["us"] {
				t.Errorf("row %d: texto %q != %q", i, gotEntry.Rows[i].Text["us"], row.Text["us"])
			}
		}
	}
}

// ---- export/import arquivo→arquivo ---------------------------------------------

func TestExportTextEvents(t *testing.T) {
	root := useFFXRoot(t)
	out := tempdir.TempDir(t)

	res, err := ExportText(context.Background(), ExportRequest{
		Inputs:    []string{ffxFixture(t, "event/obj_ps3/az/azit0000/azit0000.bin")},
		Kind:      services.KindEvents,
		Format:    FormatJSON,
		Version:   VersionRequest{Known: true, V: common.GameVersionFFX},
		OutDir:    out,
		Collision: CollisionFail,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(res.Outcomes) != 1 {
		t.Fatalf("outcomes = %d", len(res.Outcomes))
	}
	o := res.Outcomes[0]
	if o.Status != StatusWritten {
		t.Fatalf("status = %v (%s)", o.Status, o.Reason)
	}
	if filepath.Base(o.Output) != "event_azit0000_all_localizations_ffx.json" {
		t.Errorf("nome canônico = %q", filepath.Base(o.Output))
	}
	if _, err := os.Stat(o.Output); err != nil {
		t.Fatalf("artefato não gravado: %v", err)
	}
	_ = root
}

func TestExportTextKindDivergente(t *testing.T) {
	useFFXRoot(t)
	out := tempdir.TempDir(t)

	res, err := ExportText(context.Background(), ExportRequest{
		Inputs:    []string{ffxFixture(t, "battle/kernel/command.bin")},
		Kind:      services.KindEvents, // submenu errado de propósito
		Format:    FormatJSON,
		Version:   VersionRequest{Known: true, V: common.GameVersionFFX},
		OutDir:    out,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	o := res.Outcomes[0]
	if o.Status != StatusMismatch {
		t.Fatalf("status = %v (%s), esperado mismatch", o.Status, o.Reason)
	}
	if !strings.Contains(o.Reason, "esperado Eventos") || !strings.Contains(o.Reason, "Sistema") {
		t.Errorf("motivo deveria nomear os dois lados: %s", o.Reason)
	}
}

// TestImportTextEventsRoundTrip é o ciclo completo do acesso externo:
// export do evento → edição do artefato → import numa CÓPIA avulsa (pasta
// temporária, fora da árvore) → o binário em imported/ traz o texto editado.
func TestImportTextEventsRoundTrip(t *testing.T) {
	root := useFFXRoot(t)
	ctx := context.Background()
	work := tempdir.TempDir(t)

	// (1) Export do fixture para o diretório de trabalho.
	src := ffxFixture(t, "event/obj_ps3/az/azit0000/azit0000.bin")
	res, err := ExportText(ctx, ExportRequest{
		Inputs:  []string{src},
		Kind:    services.KindEvents,
		Format:  FormatJSON,
		Version: VersionRequest{Known: true, V: common.GameVersionFFX},
		OutDir:  work,
	})
	if err != nil || res.Outcomes[0].Status != StatusWritten {
		t.Fatalf("export: %+v err=%v", res.Outcomes, err)
	}
	artifact := res.Outcomes[0].Output

	// (2) Edita o artefato: todas as rows do 'us' ganham texto único.
	raw, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	c, err := jsonfmt.JSONFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	edited := map[string]string{}
	for _, id := range c.SortedKeys() {
		entry := c[id]
		for i := range entry.Rows {
			want := "TEXTO EDITADO " + strings.ToUpper(id) + " " + itoa(i)
			entry.Rows[i].Text["us"] = want
			edited[itoa(int(entry.Rows[i].Index))] = want
		}
		c[id] = entry
	}
	editedRaw, err := jsonfmt.JSONFormatter{}.MarshalLangs(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, editedRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	// (3) Cópia avulsa do binário: fora da árvore, só o nome importa.
	avulso := filepath.Join(work, "azit0000.bin")
	if err := copyFile(src, avulso); err != nil {
		t.Fatal(err)
	}

	// (4) Import: a versão vem da metadata.key do artefato (sem perguntar).
	ires, err := ImportText(ctx, ImportRequest{
		Text:      artifact,
		Binaries:  []string{avulso},
		Kind:      services.KindEvents,
		Collision: CollisionFail,
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(ires.Outcomes) != 1 {
		t.Fatalf("outcomes = %d", len(ires.Outcomes))
	}
	o := ires.Outcomes[0]
	if o.Status != StatusWritten {
		t.Fatalf("import status = %v (%s)", o.Status, o.Reason)
	}
	want := filepath.Join(work, ImportDir, "azit0000.bin")
	if o.Output != want {
		t.Errorf("saída = %q, esperado %q", o.Output, want)
	}

	// (5) O binário reconstruído traz exatamente os textos editados.
	rebuilt, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	d := Detection{Format: FormatEvent, Kind: services.KindEvents}
	decoded, err := decodeAvulso(d, common.GameVersionFFX, want, rebuilt, "us")
	if err != nil {
		t.Fatalf("decode da saída: %v", err)
	}
	entry, ok := decoded["azit0000"]
	if !ok {
		t.Fatal("entrada azit0000 sumiu na saída")
	}
	if len(entry.Rows) != len(edited) {
		t.Fatalf("rows = %d, esperado %d", len(entry.Rows), len(edited))
	}
	for _, row := range entry.Rows {
		if got := row.Text["us"]; got != edited[itoa(int(row.Index))] {
			t.Errorf("row %d: texto %q, esperado %q", row.Index, got, edited[itoa(int(row.Index))])
		}
	}
	_ = root
}

// TestImportTextIgnoraSemCorrespondencia: binário que não está no texto é
// ignorado SEM erro — só conta no resumo.
func TestImportTextIgnoraSemCorrespondencia(t *testing.T) {
	useFFXRoot(t)
	ctx := context.Background()
	work := tempdir.TempDir(t)

	src := ffxFixture(t, "event/obj_ps3/az/azit0000/azit0000.bin")
	res, err := ExportText(ctx, ExportRequest{
		Inputs:  []string{src},
		Kind:    services.KindEvents,
		Format:  FormatJSON,
		Version: VersionRequest{Known: true, V: common.GameVersionFFX},
		OutDir:  work,
	})
	if err != nil || res.Outcomes[0].Status != StatusWritten {
		t.Fatalf("export: %+v err=%v", res.Outcomes, err)
	}

	// Binário de OUTRO evento: não há entrada dele no artefato.
	other := ffxFixture(t, "event/obj_ps3/az/azit0300/azit0300.bin")
	ires, err := ImportText(ctx, ImportRequest{
		Text:     res.Outcomes[0].Output,
		Binaries: []string{other},
		Kind:     services.KindEvents,
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if ires.Outcomes[0].Status != StatusIgnored {
		t.Errorf("status = %v (%s), esperado ignorado", ires.Outcomes[0].Status, ires.Outcomes[0].Reason)
	}
}

// TestImportTextFormatoIncompativel: events no JSON de Sistema nomeia os
// dois lados no erro.
func TestImportTextFormatoIncompativel(t *testing.T) {
	useFFXRoot(t)
	ctx := context.Background()
	work := tempdir.TempDir(t)

	// Artefato de Sistema (objects).
	res, err := ExportText(ctx, ExportRequest{
		Inputs:  []string{ffxFixture(t, "battle/kernel/command.bin")},
		Kind:    services.KindObjects,
		Format:  FormatJSON,
		Version: VersionRequest{Known: true, V: common.GameVersionFFX},
		OutDir:  work,
	})
	if err != nil || res.Outcomes[0].Status != StatusWritten {
		t.Fatalf("export: %+v err=%v", res.Outcomes, err)
	}

	// Binário de events apontado contra o artefato de Sistema.
	eventBin := ffxFixture(t, "event/obj_ps3/az/azit0000/azit0000.bin")
	ires, err := ImportText(ctx, ImportRequest{
		Text:     res.Outcomes[0].Output,
		Binaries: []string{eventBin},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	o := ires.Outcomes[0]
	if o.Status != StatusMismatch {
		t.Fatalf("status = %v (%s), esperado mismatch", o.Status, o.Reason)
	}
	if !strings.Contains(o.Reason, "command_all_localizations") || !strings.Contains(o.Reason, "azit0000.bin") {
		t.Errorf("erro deveria nomear texto e binário: %s", o.Reason)
	}
}

// ---- export dos demais kinds (smoke: gera artefato com conteúdo) -------------

func TestExportTextObjectsLockitMacroHelp(t *testing.T) {
	ctx := context.Background()
	work := tempdir.TempDir(t)

	cases := []struct {
		name  string
		path  string
		kind  string
		quiet bool
	}{
		{"objects", ffxFixture(t, "battle/kernel/arms_txt.bin"), services.KindObjects, false},
		{"lockit", filepath.Join(useFFXRoot(t), "ffx_data", "gamedata", "ps3data", "lockit", "ffx_loc_kit_ps3_us.bin"), services.KindLockit, false},
		{"macro", ffxFixture(t, "menu/macrodic.dcp"), services.KindMacro, false},
		{"help", ffxFixture(t, "help/dvdcopy/dvdcopy.sps2"), services.KindHelp, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExportText(ctx, ExportRequest{
				Inputs:  []string{tc.path},
				Kind:    tc.kind,
				Format:  FormatJSON,
				Version: VersionRequest{Known: true, V: common.GameVersionFFX},
				OutDir:  work,
			})
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			o := res.Outcomes[0]
			if o.Status != StatusWritten {
				t.Fatalf("%s: status = %v (%s)", tc.name, o.Status, o.Reason)
			}
			raw, err := os.ReadFile(o.Output)
			if err != nil {
				t.Fatal(err)
			}
			var c dto.Collection
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatalf("%s: artefato não é JSON válido: %v", tc.name, err)
			}
			if len(c) == 0 {
				t.Errorf("%s: artefato sem entradas", tc.name)
			}
		})
	}
}

// TestExportTextoLivreDentroDaArvore: o menu avulso grava ao lado do binário
// apontado MESMO dentro da árvore do jogo (data/) — a trava de mods/ é dos
// escritores do frontend (common.WriteBytesToFile), não do appcore.
func TestExportTextoLivreDentroDaArvore(t *testing.T) {
	ctx := context.Background()
	fxRoot, err := filepath.Abs(filepath.Join("..", "..", "testData", "FFX", "binary"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	macrodic := filepath.Join(fxRoot, "ffx_ps2", "ffx", "master", "new_uspc", "menu", "macrodic.dcp")
	src := filepath.Join(fxRoot, "ffx_ps2", "ffx", "master", "new_uspc", "event", "obj_ps3", "az", "azit0000", "azit0000.bin")

	// Raiz de jogo temporária com o binário DENTRO de data/ (o cenário real
	// que o menu pega: exportar ao lado do arquivo apontado na árvore).
	root := tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() { common.GameFilesRoot = prev })

	inner := filepath.Join(root, "data", "new_uspc", "event", "obj_ps3", "az", "azit0000", "azit0000.bin")
	if err := os.MkdirAll(filepath.Dir(inner), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := copyFile(src, inner); err != nil {
		t.Fatalf("cópia: %v", err)
	}

	res, err := ExportText(ctx, ExportRequest{
		Inputs:       []string{inner},
		Kind:         services.KindEvents,
		Format:       FormatJSON,
		Version:      VersionRequest{Known: true, V: common.GameVersionFFX},
		MacrodicPath: macrodic,
		// OutDir vazio = ao lado da entrada, dentro de data/.
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	o := res.Outcomes[0]
	if o.Status != StatusWritten {
		t.Fatalf("status = %v (%s), esperado gravado", o.Status, o.Reason)
	}
	want := filepath.Join(filepath.Dir(inner), "event_azit0000_all_localizations_ffx.json")
	if o.Output != want {
		t.Errorf("saída = %q, esperado %q", o.Output, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("artefato não gravado dentro de data/: %v", err)
	}
}

// ---- helpers -------------------------------------------------------------------

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
