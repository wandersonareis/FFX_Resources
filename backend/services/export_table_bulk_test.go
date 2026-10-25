package services

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/eventtable"
	"ffxresources/backend/formatters"
	jsonfmt "ffxresources/backend/formatters/json"
	strfmt "ffxresources/backend/formatters/strings"
)

// repoDataRootLocal sobe do arquivo de teste até o go.mod e devolve
// build/bin/data. Inline (não usa testData): o mock notifier daquele pacote
// importa services, e teste interno + testData = ciclo de import.
func repoDataRootLocal(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("caminho do fonte indisponível")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "build", "bin", "data")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("go.mod não encontrado na hierarquia")
		}
		dir = parent
	}
}

// tableBulkFixtures copia os binários 'us' mínimos (codec events) de
// build/bin/data para um GameFilesRoot temporário: o export lê de data/ e
// grava o artefato em mods/edits — tudo contido no temp, sem tocar a árvore
// real. Fixture ausente pula o teste (ambiente sem o jogo extraído).
func tableBulkFixtures(t *testing.T, version common.GameVersion, rels []string) {
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
		eventtable.ClearStore()
	})
	eventtable.ClearStore()

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

// TestTableBulkPathScopes: o nome do artefato bulk por kind segue o padrão
// <família>_all_localizations_<versão>, cria edits/ e NUNCA colide com o
// artefato por-entrada (régua de export_paths.go: seleção não pode
// sobrescrever o bulk do tradutor).
func TestTableBulkPathScopes(t *testing.T) {
	seedEventsTempRoot(t)

	cases := []struct {
		kind string
		want string
	}{
		{KindBattleText, "battletext_all_localizations_ffx.json"},
		{KindCloud, "cloudsave_all_localizations_ffx.json"},
		{KindMenuMain, "mainmenu_all_localizations_ffx.json"},
		{KindTutorial, "tuto_all_localizations_ffx.json"},
	}
	bulks := make(map[string]bool, len(cases))
	for _, tc := range cases {
		p, err := tableBulkPath(tc.kind, common.GameVersionFFX)
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if base := filepath.Base(p); base != tc.want {
			t.Fatalf("%s: bulk %q, queria %q", tc.kind, base, tc.want)
		}
		if bulks[p] {
			t.Fatalf("%s: bulk colide com outro kind: %s", tc.kind, p)
		}
		bulks[p] = true
		if _, err := os.Stat(filepath.Dir(p)); err != nil {
			t.Fatalf("%s: diretório edits não criado: %v", tc.kind, err)
		}
	}

	// O artefato por-entrada é <id>_all_localizations_<v>: nenhum id servido
	// pelos kinds de tabela pode ter o mesmo nome de um bulk (cloud e
	// menumain têm entrada com o mesmo nome do kind — o bulk usa a família
	// física: cloudsave/mainmenu).
	for _, id := range []string{
		"bika00_10", "zzzz03_19", // battletext
		"cloud", "cloudv", // cloud
		"menumain", "menumain_btl", // menumain
		"tutorial", "tuto0000", // tutorial
	} {
		perEntry, err := jsonfmt.ObjectsJSONPath(id, common.GameVersionFFX)
		if err != nil {
			t.Fatalf("%s: artefato por-entrada: %v", id, err)
		}
		if bulks[perEntry] {
			t.Fatalf("bulk colide com o artefato por-entrada de %q: %s", id, perEntry)
		}
	}

	if _, err := tableBulkPath(KindEvents, common.GameVersionFFX); err == nil {
		t.Fatal("kind sem nome bulk registrado deve falhar (events tem path próprio)")
	}
}

// TestExportTableKindsBulkSingleFile: "exportar tudo" (ids vazio) dos kinds
// de tabela escreve UM artefato por kind — nunca um arquivo por entrada —
// e o arquivo volta a importar com o kind detectado pela metadata.key
// (regressão da ordem de kindFromKey: menumain_btl mora em /battle/btl/).
func TestExportTableKindsBulkSingleFile(t *testing.T) {
	tableBulkFixtures(t, common.GameVersionFFX, []string{
		"battle/btl/bika00_10/bika00_10.bin",
		"battle/btl/zzzz03_19/zzzz03_19.bin",
		"battle/btl/menumain/menumain.bin",
		"cloudsave/cloud.bin",
		"cloudsave/cloudv.bin",
		"menu/menumain.bin",
	})
	if err := ensureVersionReady(common.GameVersionFFX); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	svc := NewMetadataService(nil)

	// battletext: bulk único com as entradas da árvore (bika00_10 e
	// zzzz03_19; menumain/ é excluído do battletext pela régua do eventtable).
	paths, err := svc.ExportArtifactBatch([]ExportPlanItem{{Kind: KindBattleText}}, common.GameVersionFFX, nil, "json")
	if err != nil {
		t.Fatalf("export battletext: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("exportar tudo de battletext: %d arquivo(s) (%v), queria 1", len(paths), paths)
	}
	if base := filepath.Base(paths[0]); base != "battletext_all_localizations_ffx.json" {
		t.Fatalf("bulk battletext: %q", base)
	}
	read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, paths[0]).Read()
	if err != nil {
		t.Fatalf("releitura do bulk: %v", err)
	}
	for _, id := range []string{"bika00_10", "zzzz03_19"} {
		if _, ok := read[id]; !ok {
			t.Fatalf("entrada %s ausente no bulk battletext (%v)", id, read.SortedKeys())
		}
	}
	plan, err := svc.prepareImport(paths[0], common.GameVersionFFX)
	if err != nil {
		t.Fatalf("prepare import do bulk battletext: %v", err)
	}
	if plan.kind != KindBattleText {
		t.Fatalf("import detectou kind %q, queria %q", plan.kind, KindBattleText)
	}
	if len(plan.summary.Errors) > 0 {
		t.Fatalf("bulk battletext deveria importar limpo: %v", plan.summary.Errors)
	}

	// Espelho .strings: mesmo basename, extensão trocada, também único.
	spaths, err := svc.ExportArtifactBatch([]ExportPlanItem{{Kind: KindBattleText}}, common.GameVersionFFX, nil, "strings")
	if err != nil {
		t.Fatalf("export .strings battletext: %v", err)
	}
	if len(spaths) != 1 || filepath.Base(spaths[0]) != "battletext_all_localizations_ffx.strings" {
		t.Fatalf("bulk .strings battletext: %v", spaths)
	}

	// Seleção mantém o artefato por-entrada (dedup intra-arquivo) e não pode
	// sobrescrever o bulk.
	sel, err := svc.ExportArtifactBatch([]ExportPlanItem{{Kind: KindBattleText, IDs: []string{"bika00_10"}}}, common.GameVersionFFX, nil, "json")
	if err != nil {
		t.Fatalf("export seleção battletext: %v", err)
	}
	if len(sel) != 1 || filepath.Base(sel[0]) != "bika00_10_all_localizations_ffx.json" {
		t.Fatalf("seleção battletext: %v", sel)
	}
	if sel[0] == paths[0] {
		t.Fatal("seleção sobrescreveu o bulk (colisão de nomes)")
	}

	// menumain: bulk único com menumain + menumain_btl; o import detecta o
	// kind certo mesmo com a key da entrada btl dentro de /battle/btl/.
	mpaths, err := svc.ExportArtifactBatch([]ExportPlanItem{{Kind: KindMenuMain}}, common.GameVersionFFX, nil, "json")
	if err != nil {
		t.Fatalf("export menumain: %v", err)
	}
	if len(mpaths) != 1 || filepath.Base(mpaths[0]) != "mainmenu_all_localizations_ffx.json" {
		t.Fatalf("bulk menumain: %v", mpaths)
	}
	mread, err := formatters.NewReader(jsonfmt.JSONFormatter{}, mpaths[0]).Read()
	if err != nil {
		t.Fatalf("releitura do bulk menumain: %v", err)
	}
	for _, id := range []string{"menumain", "menumain_btl"} {
		if _, ok := mread[id]; !ok {
			t.Fatalf("entrada %s ausente no bulk menumain (%v)", id, mread.SortedKeys())
		}
	}
	mplan, err := svc.prepareImport(mpaths[0], common.GameVersionFFX)
	if err != nil {
		t.Fatalf("prepare import do bulk menumain: %v", err)
	}
	if mplan.kind != KindMenuMain {
		t.Fatalf("import detectou kind %q, queria %q (ordem de kindFromKey?)", mplan.kind, KindMenuMain)
	}

	// cloud: bulk cloudsave_* não pode colidir com o artefato por-entrada de
	// "cloud" (mesmo nome do kind).
	cpaths, err := svc.ExportArtifactBatch([]ExportPlanItem{{Kind: KindCloud}}, common.GameVersionFFX, nil, "json")
	if err != nil {
		t.Fatalf("export cloud: %v", err)
	}
	if len(cpaths) != 1 || filepath.Base(cpaths[0]) != "cloudsave_all_localizations_ffx.json" {
		t.Fatalf("bulk cloud: %v", cpaths)
	}
	perCloud, err := jsonfmt.ObjectsJSONPath("cloud", common.GameVersionFFX)
	if err != nil {
		t.Fatalf("artefato por-entrada de cloud: %v", err)
	}
	if cpaths[0] == perCloud {
		t.Fatal("bulk cloud colide com o artefato por-entrada de \"cloud\"")
	}
	cread, err := formatters.NewReader(jsonfmt.JSONFormatter{}, cpaths[0]).Read()
	if err != nil {
		t.Fatalf("releitura do bulk cloud: %v", err)
	}
	for _, id := range []string{"cloud", "cloudv"} {
		if _, ok := cread[id]; !ok {
			t.Fatalf("entrada %s ausente no bulk cloud (%v)", id, cread.SortedKeys())
		}
	}
}

// ---- round-trip export bulk → import bulk → binário ---------------------------
//
// TestExportBulkImportRoundtripBinary: o ciclo completo do "exportar tudo →
// importar tudo" com I/O real e conferência de BINÁRIO byte-a-byte, nos dois
// formatos (JSON e .strings):
//
//  1. export completo do kind (bulk único em edits/) lendo os binários de data/;
//  2. ImportFile do artefato SEM edição — nada muda: changed=0 e nenhum
//     binário regravado em mods/ (o apply só salva o que recebeu texto novo);
//  3. ImportFile do artefato COM edição (1 caractere regular, mesmo
//     comprimento em bytes): changed=1, só o binário da entrada editada em
//     mods/, mesmo tamanho do original, exatamente 1 byte divergente, e o
//     texto novo volta na releitura direta de mods/.
func TestExportBulkImportRoundtripBinary(t *testing.T) {
	t.Run("json", func(t *testing.T) { bulkImportRoundtripScenario(t, "json") })
	t.Run("strings", func(t *testing.T) { bulkImportRoundtripScenario(t, "strings") })
}

// bulkImportRoundtripScenario roda o ciclo em um GameFilesRoot temporário
// próprio (os dois formatos não compartilham estado de store/mods).
func bulkImportRoundtripScenario(t *testing.T, format string) {
	t.Helper()
	tableBulkFixtures(t, common.GameVersionFFX, []string{
		"battle/btl/bika00_10/bika00_10.bin",
		"battle/btl/zzzz03_19/zzzz03_19.bin",
	})
	if err := ensureVersionReady(common.GameVersionFFX); err != nil {
		t.Fatalf("prepare version: %v", err)
	}
	svc := NewMetadataService(nil)

	// (1) Export completo: o bulk único de battletext.
	paths, err := svc.ExportArtifactBatch([]ExportPlanItem{{Kind: KindBattleText}}, common.GameVersionFFX, nil, format)
	if err != nil {
		t.Fatalf("export %s: %v", format, err)
	}
	if len(paths) != 1 {
		t.Fatalf("export %s: %d arquivo(s) (%v), queria o bulk único", format, len(paths), paths)
	}
	artifact := paths[0]

	// (2) Import do artefato SEM edição: nada a regravar.
	changed, err := svc.ImportFile(artifact, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("import %s sem edição: %v", format, err)
	}
	if changed != 0 {
		t.Fatalf("import sem edição: changed=%d, queria 0 (artefato idêntico à store)", changed)
	}
	if bins := modsBinFiles(t); len(bins) > 0 {
		t.Fatalf("import sem edição regravou binário(s): %v", bins)
	}

	// (3) Edita UMA row do artefato: 1 caractere regular após a última tag
	// (mesmo comprimento em bytes — offsets da tabela de strings intactos),
	// hash preservado (a régua do tradutor: só o valor muda).
	imported := parseBulkArtifact(t, format, artifact)
	editID, editIndex, oldText, newText := pickBulkEditableRow(t, imported)
	for i := range imported[editID].Rows {
		if imported[editID].Rows[i].Index == editIndex {
			imported[editID].Rows[i].Text[common.DefaultLocalization] = newText
			break
		}
	}
	edited := filepath.Join(tempdir.TempDir(t), "edited."+format)
	if err := os.WriteFile(edited, marshalBulkArtifact(t, format, imported), 0o644); err != nil {
		t.Fatalf("gravar artefato editado: %v", err)
	}

	changed, err = svc.ImportFile(edited, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("import %s com edição: %v", format, err)
	}
	if changed != 1 {
		t.Fatalf("import com edição: changed=%d, queria 1", changed)
	}

	// (4) mods/: somente o binário da entrada editada (nenhum da outra).
	for _, bin := range modsBinFiles(t) {
		if !strings.Contains(filepath.ToSlash(bin), "/battle/btl/"+editID+"/") {
			t.Fatalf("binário inesperado em mods/ (entrada intocada?): %s", bin)
		}
	}

	// (5) Binário regravado: mesmo tamanho e exatamente 1 byte divergente do
	// original de data/ — a tabela de strings inteira foi recompilada e a
	// troca de mesmo comprimento não moveu nenhum offset.
	locRoot := filepath.FromSlash(common.GetLocalizationRootForVersion(common.GameVersionFFX, common.DefaultLocalization))
	origData, err := os.ReadFile(filepath.Join(common.GameFilesRoot, locRoot, "battle", "btl", editID, editID+".bin"))
	if err != nil {
		t.Fatalf("original de data/: %v", err)
	}
	modData, err := os.ReadFile(filepath.Join(common.GameFilesRoot, common.ModsFolder, locRoot, "battle", "btl", editID, editID+".bin"))
	if err != nil {
		t.Fatalf("binário de mods/: %v", err)
	}
	if len(origData) != len(modData) {
		t.Fatalf("binário de mods/ mudou de tamanho: orig=%d mod=%d", len(origData), len(modData))
	}
	diffs := byteDiffPositions(origData, modData)
	if len(diffs) != 1 {
		t.Fatalf("binário de mods/ divergiu em %d byte(s) (%v…), queria exatamente 1 (só a troca de caractere)",
			len(diffs), firstFewInts(diffs))
	}

	// (6) O texto novo volta na releitura DIRETA de mods/, na posição certa.
	strs, err := eventtable.ReadLocalizedStringsFrom(KindBattleText, editID, common.GameVersionFFX, common.SourceMods)
	if err != nil {
		t.Fatalf("releitura de mods/: %v", err)
	}
	if editIndex >= len(strs) {
		t.Fatalf("posição editada %d fora da tabela regravada (%d strings)", editIndex, len(strs))
	}
	if got := strs[editIndex].GetLocalizedString(common.DefaultLocalization); got != newText {
		t.Fatalf("releitura de mods/: texto %q, queria %q (antes era %q)", got, newText, oldText)
	}
}

// parseBulkArtifact lê e parseia o artefato exportado de volta ao DTO.
func parseBulkArtifact(t *testing.T, format, path string) dto.Collection {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler artefato: %v", err)
	}
	if format == "json" {
		c, err := jsonfmt.JSONFormatter{}.Unmarshal(raw)
		if err != nil {
			t.Fatalf("parse json: %v", err)
		}
		return c
	}
	c, err := strfmt.StringsFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("parse strings: %v", err)
	}
	return c
}

// marshalBulkArtifact serializa o DTO editado de volta ao formato de origem.
func marshalBulkArtifact(t *testing.T, format string, c dto.Collection) []byte {
	t.Helper()
	if format == "json" {
		raw, err := jsonfmt.JSONFormatter{}.Marshal(c)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		return raw
	}
	raw, err := strfmt.Marshal(c)
	if err != nil {
		t.Fatalf("marshal strings: %v", err)
	}
	return raw
}

// pickBulkEditableRow escolhe a row do lote para a edição do round-trip:
// texto 'us' literal (não-ref de dedup) e ÚNICO entre as entradas (a edição
// não pode propagar por dedup para a outra entrada), com final pós-tags em
// ASCII e ≥2 caracteres distintos — a troca de 1 caractere preserva o
// comprimento em bytes e, com ele, os offsets da tabela de strings.
func pickBulkEditableRow(t *testing.T, c dto.Collection) (id string, index int, oldText, newText string) {
	t.Helper()
	count := make(map[string]int)
	for _, k := range c.SortedKeys() {
		for _, r := range c[k].Rows {
			count[r.Text[common.DefaultLocalization]]++
		}
	}
	for _, k := range c.SortedKeys() {
		for _, r := range c[k].Rows {
			old := r.Text[common.DefaultLocalization]
			if old == "" || count[old] != 1 || strings.HasPrefix(old, "$") {
				continue
			}
			cut := strings.LastIndex(old, "}") + 1 // após a última tag fechada
			tail := old[cut:]
			ascii := len(tail) >= 2
			for i := 0; i < len(tail); i++ {
				if tail[i] >= 0x80 {
					ascii = false // fora do ASCII: comprimento em bytes incerto
					break
				}
			}
			if !ascii || tail[0] == tail[1] {
				continue
			}
			return k, r.Index, old, old[:cut] + string(tail[1]) + tail[1:]
		}
	}
	t.Fatal("nenhuma row editável no lote (texto único + final ASCII ≥2)")
	return
}

// modsBinFiles lista os .bin gravados sob mods/ da raiz de trabalho corrente.
func modsBinFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(filepath.Join(common.GameFilesRoot, common.ModsFolder),
		func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(d.Name()), ".bin") {
				out = append(out, path)
			}
			return nil
		})
	return out
}

// byteDiffPositions devolve as posições em que a e b divergem (mesmo
// comprimento conferido pelo chamador).
func byteDiffPositions(a, b []byte) []int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var diffs []int
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			diffs = append(diffs, i)
		}
	}
	return diffs
}

// firstFewInts corta a lista para a mensagem de falha não despejar tudo.
func firstFewInts(v []int) []int {
	if len(v) > 8 {
		return v[:8]
	}
	return v
}
