package services

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/ddsphyre"
)

// Árvore de teste com dois grupos: a/b cópias idênticas e c imagem outra.
func dupTreeFixture(t *testing.T) (root, idA, idB, idC string) {
	t.Helper()
	sampleA, sampleC := realSampleWithPayload(t)

	root = tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		ddsphyre.InvalidateIndexes()
	})

	idA = "gamedata/ps3data/zzz_dup/a" // 1º alfabético do grupo: representante
	idB = "gamedata/ps3data/zzz_dup/b" // cópia de a (oculta na árvore)
	idC = "gamedata/ps3data/zzz_other/c"

	writePhyre(t, root, idA, sampleA)
	writePhyre(t, root, idB, append([]byte(nil), sampleA...))
	writePhyre(t, root, idC, sampleC)
	return root, idA, idB, idC
}

func entryIDs(entries []EntrySummary) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

// A árvore de imagens mostra UM item por grupo de cópias: o representante
// (1º alfabético do grupo). As cópias continuam acessíveis — a lista
// "Repetidas" do painel é quem as exibe e as serve pelo id direto.
func TestListEntriesHidesDuplicateCopies(t *testing.T) {
	_, idA, idB, idC := dupTreeFixture(t)

	svc := NewMetadataService(nil)
	entries, err := svc.ListEntries(KindImages, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	want := []string{idA, idC}
	if got := entryIDs(entries); !reflect.DeepEqual(got, want) {
		t.Errorf("árvore = %v, esperado só os representantes %v (cópia %s oculta)", got, want, idB)
	}

	// O painel continua sabendo da cópia oculta.
	entry, err := svc.GetImage(KindImages, idA, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if len(entry.Duplicates) != 1 || entry.Duplicates[0].ID != idB {
		t.Errorf("painel perdeu a cópia: %+v", entry.Duplicates)
	}
	// E a cópia segue servível pelo id (é assim que a lista navega até ela).
	copied, err := svc.GetImage(KindImages, idB, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage(cópia oculta): %v", err)
	}
	if copied.Metadata.ID != idB || copied.PNGData == "" {
		t.Errorf("cópia oculta não serviu: id=%q png=%d bytes", copied.Metadata.ID, len(copied.PNGData))
	}
}

// "Abrir até o arquivo" precisa apontar para o arquivo que está VALENDO:
// mods/ quando existe (é o que o jogo carrega), data/ como queda.
func TestRevealTargetPathPrefersMods(t *testing.T) {
	root, idA, _, idC := dupTreeFixture(t)

	modsPath := filepath.Join(root, common.ModsFolder, ddsphyre.RelPath(common.GameVersionFFX, idA))
	if err := common.EnsurePathExists(modsPath); err != nil {
		t.Fatalf("criando mods/: %v", err)
	}
	if err := os.WriteFile(modsPath, []byte("overlay"), 0o644); err != nil {
		t.Fatalf("gravando mods/: %v", err)
	}

	got, err := revealTargetPath(KindImages, idA, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("revealTargetPath(mods): %v", err)
	}
	if want := filepath.Clean(modsPath); filepath.Clean(got) != want {
		t.Errorf("revelou %s, esperado o overlay %s", got, want)
	}

	// Sem overlay: o pristine de data/.
	dataPath := filepath.Join(root, ddsphyre.RelPath(common.GameVersionFFX, idC))
	gotC, err := revealTargetPath(KindImages, idC, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("revealTargetPath(data): %v", err)
	}
	if want := filepath.Clean(dataPath); filepath.Clean(gotC) != want {
		t.Errorf("revelou %s, esperado o pristine %s", gotC, want)
	}

	// Arquivo inexistente: erro claro — abrir pasta sem o item seria
	// enganoso (o Explorer ainda abriria, mas sem seleção alguma).
	if _, err := revealTargetPath(KindImages, "gamedata/ps3data/zzz_dup/naoexiste",
		common.GameVersionFFX); err == nil {
		t.Error("revelou arquivo inexistente sem erro")
	}
}

// Árvore REAL do jogo (quando o repositório tem build/bin/data): o filtro
// precisa deixar UM representante por grupo — e só o representante — sem
// perder imagem única alguma.
func TestListEntriesOnRealTreeKeepsOnlyRepresentatives(t *testing.T) {
	root := filepath.Join("..", "..", "build", "bin", "data")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Skip("árvore build/bin/data indisponível")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	prev := common.GameFilesRoot
	common.GameFilesRoot = abs
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		ddsphyre.InvalidateIndexes()
	})

	svc := NewMetadataService(nil)
	entries, err := svc.ListEntries(KindImages, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) == 0 {
		t.Skip("nenhum .dds.phyre em data/ (árvore não extraída do FFX_Data.vbf)")
	}
	visible := make(map[string]bool, len(entries))
	for _, e := range entries {
		visible[e.ID] = true
	}

	ids, _, _, err := ddsphyre.Scan(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	ix, err := ddsphyre.IndexFor(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("IndexFor: %v", err)
	}

	singles, groupsWithCopies := 0, 0
	for _, id := range ids {
		group := ix.OriginalGroup(id)
		if len(group) < 2 {
			// Imagem única (ou fora do índice): linha própria, sempre.
			singles++
			if !visible[id] {
				t.Errorf("imagem única %s sumiu da árvore", id)
			}
			continue
		}
		if group[0] != id {
			continue // só o representante audita o grupo
		}
		groupsWithCopies++
		shown := 0
		for _, member := range group {
			if visible[member] {
				shown++
			}
		}
		if shown != 1 || !visible[id] {
			t.Errorf("grupo %v com %d visíveis (esperado 1, o representante %s)", group, shown, id)
		}
	}
	if groupsWithCopies == 0 {
		t.Log("árvore real sem duplicatas agrupadas nesta versão")
	}
	t.Logf("árvore real: %d ids, %d linhas servidas (%d imagens únicas, %d grupos com cópias)",
		len(ids), len(entries), singles, groupsWithCopies)
	if len(entries) >= len(ids) {
		t.Errorf("nada foi ocultado: %d linhas para %d texturas", len(entries), len(ids))
	}
}

// Extração em lote cobre as cópias escolhidas e recusa alvo de outro grupo.
func TestExtractImageGroupCoversChosenCopies(t *testing.T) {
	_, idA, idB, idC := dupTreeFixture(t)
	svc := NewMetadataService(nil)

	res, err := svc.ExtractImageGroup(KindImages, idA, []string{idB}, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ExtractImageGroup: %v", err)
	}
	if res.Total != 2 || len(res.Done) != 2 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado 2 concluídas e 0 falhas", res)
	}
	for _, id := range []string{idA, idB} {
		ddsPath, pngPath := ddsphyre.ExportPaths(common.GameVersionFFX, id)
		for _, p := range []string{ddsPath, pngPath} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("artefato de %s ausente: %v", id, err)
			}
		}
	}

	// "Só esta": lista de alvos vazia = um arquivo, não o grupo.
	only, err := svc.ExtractImageGroup(KindImages, idA, nil, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ExtractImageGroup(só esta): %v", err)
	}
	if only.Total != 1 || len(only.Done) != 1 {
		t.Errorf("só esta = %+v, esperado 1 arquivo", only)
	}

	// Alvo de OUTRO grupo: recusado e nada gravado.
	if _, err := svc.ExtractImageGroup(KindImages, idA, []string{idC},
		common.GameVersionFFX); err == nil {
		t.Fatal("esperava recusa para alvo de outro grupo")
	} else if !strings.Contains(err.Error(), "não é cópia idêntica") {
		t.Errorf("erro sem o motivo: %v", err)
	}
	ddsC, _ := ddsphyre.ExportPaths(common.GameVersionFFX, idC)
	if _, err := os.Stat(ddsC); !os.IsNotExist(err) {
		t.Errorf("alvo recusado foi extraído (%s)", ddsC)
	}
}

func TestExtractImageSelectionOnlyExtractsSelectedIDs(t *testing.T) {
	_, idA, idB, idC := dupTreeFixture(t)
	res, err := NewMetadataService(nil).ExtractImageSelection(KindImages, []string{idA, idC}, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ExtractImageSelection: %v", err)
	}
	if res.Total != 2 || len(res.Done) != 2 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado apenas os 2 ids selecionados", res)
	}
	for _, id := range []string{idA, idC} {
		ddsPath, pngPath := ddsphyre.ExportPaths(common.GameVersionFFX, id)
		for _, path := range []string{ddsPath, pngPath} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("artefato selecionado ausente %s: %v", path, err)
			}
		}
	}
	copyDDS, _ := ddsphyre.ExportPaths(common.GameVersionFFX, idB)
	if _, err := os.Stat(copyDDS); !os.IsNotExist(err) {
		t.Errorf("cópia não selecionada foi extraída: %s", copyDDS)
	}
}

func TestDeleteImageSelectionAppliesCopyOptionToEverySelectedID(t *testing.T) {
	_, idA, idB, idC := dupTreeFixture(t)
	svc := NewMetadataService(nil)
	copies, err := svc.ImageSelectionCopies(KindImages, []string{idA}, common.GameVersionFFX)
	if err != nil || !reflect.DeepEqual(copies, []string{idB}) {
		t.Fatalf("cópias do lote = %v, err=%v; esperado apenas %s", copies, err, idB)
	}
	copies, err = svc.ImageSelectionCopies(KindImages, []string{idA, idB}, common.GameVersionFFX)
	if err != nil || len(copies) != 0 {
		t.Fatalf("cópias já selecionadas não devem ser contadas novamente: %v, err=%v", copies, err)
	}
	res, err := svc.DeleteImageSelection(KindImages, []string{idA}, false, ddsphyre.DeleteMods, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("DeleteImageSelection sem cópias: %v", err)
	}
	if res.Total != 1 || len(res.Done) != 1 || len(res.Failed) != 0 {
		t.Fatalf("resultado sem cópias = %+v", res)
	}
	if inData, inMods := ddsphyre.Exists(common.GameVersionFFX, idA); !inData || inMods {
		t.Errorf("DeleteMods não preservou apenas o original: data=%v mods=%v", inData, inMods)
	}
	if inData, inMods := ddsphyre.Exists(common.GameVersionFFX, idB); !inData || inMods {
		t.Errorf("cópia não selecionada foi alterada: data=%v mods=%v", inData, inMods)
	}

	// Com a opção de cópias, remove a selecionada e a sua cópia, mas não o
	// arquivo independente.
	res, err = svc.DeleteImageSelection(KindImages, []string{idA}, true, ddsphyre.DeleteBoth, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("DeleteImageSelection com cópias: %v", err)
	}
	if res.Total != 2 || len(res.Done) != 2 || len(res.Failed) != 0 {
		t.Fatalf("resultado com cópias = %+v, esperado 2 ids únicos", res)
	}
	for _, id := range []string{idA, idB} {
		if inData, inMods := ddsphyre.Exists(common.GameVersionFFX, id); inData || inMods {
			t.Errorf("%s permaneceu após delete both: data=%v mods=%v", id, inData, inMods)
		}
	}
	if inData, inMods := ddsphyre.Exists(common.GameVersionFFX, idC); !inData || inMods {
		t.Errorf("arquivo não selecionado foi alterado: data=%v mods=%v", inData, inMods)
	}
}

// Replicar leva a IMAGEM ABERTA para as cópias em mods/, sem tocar na
// origem e sem diálogo de arquivo.
func TestReplicateImageCopiesOpenedTexture(t *testing.T) {
	root, idA, idB, idC := dupTreeFixture(t)
	svc := NewMetadataService(nil)

	// Origem importada (mods/ tem o container): é ela que deve ser lida.
	srcMods := filepath.Join(root, common.ModsFolder, ddsphyre.RelPath(common.GameVersionFFX, idA))
	if err := common.EnsurePathExists(srcMods); err != nil {
		t.Fatalf("criando mods/ da origem: %v", err)
	}
	pristine, err := os.ReadFile(filepath.Join(root, ddsphyre.RelPath(common.GameVersionFFX, idA)))
	if err != nil {
		t.Fatalf("lendo pristine: %v", err)
	}
	if err := os.WriteFile(srcMods, pristine, 0o644); err != nil {
		t.Fatalf("gravando mods/ da origem: %v", err)
	}

	res, err := svc.ReplicateImage(KindImages, idA, []string{idB}, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ReplicateImage: %v", err)
	}
	if res.Total != 1 || len(res.Done) != 1 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado 1 cópia concluída", res)
	}

	// A cópia recebeu o arquivo e passou a contar como importada…
	copiedMods := filepath.Join(root, common.ModsFolder, ddsphyre.RelPath(common.GameVersionFFX, idB))
	if _, err := os.Stat(copiedMods); err != nil {
		t.Fatalf("cópia não recebeu o arquivo em mods/: %v", err)
	}
	entry, err := svc.GetImage(KindImages, idB, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage(cópia): %v", err)
	}
	if !entry.Modded {
		t.Error("cópia replicada não ficou modded")
	}
	// …e as duas voltaram a ser idênticas (é o objetivo do dupe).
	if len(entry.Duplicates) != 1 || !entry.Duplicates[0].Identical {
		t.Errorf("cópia divergente depois de replicar: %+v", entry.Duplicates)
	}

	// A origem NÃO é tocada: só as cópias recebem arquivo.
	if _, err := os.Stat(srcMods); err != nil {
		t.Errorf("origem foi regravada: %v", err)
	}

	// Sem cópias não há o que replicar (imagem única).
	if _, err := svc.ReplicateImage(KindImages, idC, nil, common.GameVersionFFX); err == nil {
		t.Error("replicou imagem única sem erro")
	} else if !strings.Contains(err.Error(), "não tem cópias") {
		t.Errorf("erro sem o motivo: %v", err)
	}
}

// Deletar respeita o escopo pedido e nunca é cascata implícita: as cópias
// entram só quando o diálogo as marcou.
func TestDeleteImagesRespectsScopeAndConfirmedCopies(t *testing.T) {
	root, idA, idB, idC := dupTreeFixture(t)
	svc := NewMetadataService(nil)

	dataPathOf := func(id string) string {
		return filepath.Join(root, ddsphyre.RelPath(common.GameVersionFFX, id))
	}
	modsPathOf := func(id string) string {
		return filepath.Join(root, common.ModsFolder, ddsphyre.RelPath(common.GameVersionFFX, id))
	}
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	// Overlay em mods/ nas duas (para o escopo mods ter o que apagar).
	for _, id := range []string{idA, idB} {
		if err := common.EnsurePathExists(modsPathOf(id)); err != nil {
			t.Fatalf("criando mods/: %v", err)
		}
		if err := os.WriteFile(modsPathOf(id), []byte("overlay"), 0o644); err != nil {
			t.Fatalf("gravando mods/: %v", err)
		}
	}

	// Alvo de OUTRO grupo é recusado ANTES de apagar um byte.
	if _, err := svc.DeleteImages(KindImages, idA, []string{idC},
		ddsphyre.DeleteBoth, common.GameVersionFFX); err == nil {
		t.Error("aceitou apagar alvo de outro grupo")
	} else if !strings.Contains(err.Error(), "não é cópia idêntica") {
		t.Errorf("erro sem o motivo: %v", err)
	}
	if !exists(dataPathOf(idC)) || !exists(modsPathOf(idA)) {
		t.Error("recusa não veio antes das gravações")
	}

	// Escopo mods, só a cópia confirmada: o original fica (sem cascata).
	res, err := svc.DeleteImages(KindImages, idB, nil,
		ddsphyre.DeleteMods, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("DeleteImages(mods): %v", err)
	}
	if res.Total != 1 || len(res.Done) != 1 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado 1 concluída", res)
	}
	if exists(modsPathOf(idB)) {
		t.Error("cópia não foi apagada de mods/")
	}
	if !exists(modsPathOf(idA)) {
		t.Error("original apagado junto — cascata implícita")
	}
	if !exists(dataPathOf(idA)) || !exists(dataPathOf(idB)) {
		t.Error("escopo mods mexeu em data/")
	}

	// Escopo both confirmado nas duas: sai das duas árvores.
	res, err = svc.DeleteImages(KindImages, idA, []string{idB},
		ddsphyre.DeleteBoth, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("DeleteImages(both): %v", err)
	}
	if res.Total != 2 || len(res.Done) != 2 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado 2 concluídas", res)
	}
	for _, id := range []string{idA, idB} {
		for _, path := range []string{dataPathOf(id), modsPathOf(id)} {
			if exists(path) {
				t.Errorf("sobrou %s", path)
			}
		}
	}

	// Escopo inválido: recusado pelo serviço, nada é tocado.
	if _, err := svc.DeleteImages(KindImages, idC, nil,
		"qualquer", common.GameVersionFFX); err == nil {
		t.Error("escopo inválido aceito")
	}
}
