package services_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/interactions"
	"ffxresources/backend/services"
	"ffxresources/testData"
)

// Testes do sumário (preview) e da aplicação por seleção do import de
// imagens traduzidas — o fluxo que o modal tela:
//
//   - uma entrada por .dds.phyre encontrado, com caminho relativo à raiz
//     traduzida (com extensão);
//   - novas nascem marcadas, iguais ao binário desmarcadas, inválidas com erro;
//   - cópias AUSENTES só no collapse da primeira entrada do grupo;
//   - o apply grava SÓ a seleção (imagens + cópias marcadas), inclusive a
//     regravação de uma idêntica ao binário.
//
// A amostra de container é um .dds.phyre REAL (build/bin/data); sem ela os
// testes pulam (regra do projeto: teste por existência).
func setupImageImport(t *testing.T) (version common.GameVersion, tmpRoot string, sample []byte) {
	t.Helper()
	testcommon.SetBuildBinPath()

	// Amostra real: primeiro .dds.phyre da árvore do jogo.
	var found string
	for _, root := range []string{
		filepath.Join("..", "..", "build", "bin", "data", "ffx-2_data"),
		filepath.Join("..", "..", "build", "bin", "data", "ffx_data"),
	} {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() || found != "" {
				return nil
			}
			if strings.HasSuffix(d.Name(), ddsphyre.Suffix) {
				found = path
			}
			return nil
		})
		if err == nil && found != "" {
			break
		}
	}
	if found == "" {
		t.Skip("nenhum .dds.phyre de amostra disponível em build/bin/data")
	}
	raw, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("lendo amostra: %v", err)
	}

	version = common.GameVersionFFX2
	tmpRoot = t.TempDir()

	// Árvore de texturas da versão: grupo de 3 cópias idênticas (a, b, c) +
	// uma imagem diferente (d).
	tex, perr := ddsphyre.Parse(raw)
	if perr != nil {
		t.Fatalf("Parse da amostra: %v", perr)
	}
	game := filepath.Join(tmpRoot, "game")
	writePhyreAt(t, game, "gamedata/ps3data/img/a", raw)
	writePhyreAt(t, game, "gamedata/ps3data/img/b", raw)
	writePhyreAt(t, game, "gamedata/ps3data/img/c", raw)
	other := append([]byte(nil), raw...)
	off := int(tex.DataOffset)
	other[off] ^= 0xFF // payload diferente = imagem outra
	writePhyreAt(t, game, "gamedata/ps3data/img/d", other)

	// interactions: aponta GameFiles/TranslateLocation para o tmp.
	config := interactions.NewAppConfig()
	config.SetGameVersion(version)
	config.SetLocation("GameFilesLocation", game)
	config.SetLocation("TranslateLocation", filepath.Join(tmpRoot, "translated"))
	interactions.NewInteractionServiceWithConfig(config)

	ddsphyre.InvalidateIndexes()
	t.Cleanup(func() {
		ddsphyre.InvalidateIndexes()
	})
	return version, filepath.Join(tmpRoot, "translated"), raw
}

// writePhyreAt grava um container em <root>/<versão>/<id>.dds.phyre.
func writePhyreAt(t *testing.T, root, id string, raw []byte) {
	t.Helper()
	dest := filepath.Join(root, "ffx-2_data", filepath.FromSlash(id)+ddsphyre.Suffix)
	if err := common.EnsurePathExists(dest); err != nil {
		t.Fatalf("criando árvore: %v", err)
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatalf("gravando %s: %v", dest, err)
	}
}

// writeTranslated grava um .dds.phyre na raiz traduzida (relativo a ela,
// COM "gamedata/ps3data/...").
func writeTranslated(t *testing.T, translateRoot, id string, raw []byte) {
	t.Helper()
	dest := filepath.Join(translateRoot, "images", filepath.FromSlash(id)+ddsphyre.Suffix)
	if err := common.EnsurePathExists(dest); err != nil {
		t.Fatalf("criando pasta traduzida: %v", err)
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatalf("gravando %s: %v", dest, err)
	}
}

// newTestDataOffset devolve o DataOffset do container (limite header/payload)
// via o Parse exportado — o helper interno dataOffsetOf não sai do pacote.
func newTestDataOffset(t *testing.T, raw []byte) uint64 {
	t.Helper()
	tex, err := ddsphyre.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return tex.DataOffset
}

func modsContainerPath(t *testing.T, version common.GameVersion, id string) string {
	t.Helper()
	return filepath.Join(common.GameFilesRoot, common.ModsFolder, ddsphyre.RelPath(version, id))
}

func findEntry(preview dto.ImageImportPreview, id string) *dto.ImageImportPreviewEntry {
	for i := range preview.Entries {
		if preview.Entries[i].ID == id {
			return &preview.Entries[i]
		}
	}
	return nil
}

// TestPreviewImportImagesDefaults: 8 arquivos na pasta (a–h do grupo + d),
// 2 cópias ausentes (i, j) → lista PLANA com caminho relativo COM extensão,
// novas marcadas, cópias ausentes só no collapse da primeira do grupo.
func TestPreviewImportImagesDefaults(t *testing.T) {
	version, translateRoot, sample := setupImageImport(t)
	svc := services.NewMetadataService(nil)

	// Mesmo grupo: a, b, c em data/; pasta traduzida com a, b (c ausente)
	// e mais 6 entradas solo (h1..h6) e a imagem diferente d.
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/a", sample)
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/b", sample)
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/d", sample)
	for _, id := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		writeTranslated(t, translateRoot, "gamedata/ps3data/img/"+id, sample)
	}

	preview, err := svc.PreviewImportImages(version)
	if err != nil {
		t.Fatalf("PreviewImportImages: %v", err)
	}

	if preview.Max != 9 || preview.NewCount != 9 {
		t.Fatalf("max/new = %d/%d, queria 9/9 (nada em mods/ = tudo novo)", preview.Max, preview.NewCount)
	}
	if len(preview.Errors) != 0 {
		t.Fatalf("erros inesperados: %v", preview.Errors)
	}

	// Ordem canônica (id) e caminho relativo COM extensão.
	wantDisplay := "gamedata/ps3data/img/a.dds.phyre"
	a := findEntry(preview, "gamedata/ps3data/img/a")
	if a == nil {
		t.Fatal("entrada a ausente")
	}
	if a.DisplayPath != wantDisplay {
		t.Errorf("DisplayPath = %q, queria %q", a.DisplayPath, wantDisplay)
	}
	if !a.Selected {
		t.Error("imagem nova deveria nascer marcada")
	}

	// Cópias: grupo tem 3 (a, b, c); na pasta estão a e b → c ausente.
	// Só a PRIMEIRA entrada do grupo (a) carrega o collapse.
	if a.CopyCount != 2 {
		t.Errorf("CopyCount = %d, queria 2", a.CopyCount)
	}
	if a.CopyMissingCount != 1 || len(a.Copies) != 1 {
		t.Fatalf("cópias ausentes = %d/%d, queria 1", a.CopyMissingCount, len(a.Copies))
	}
	if a.Copies[0].ID != "gamedata/ps3data/img/c" {
		t.Errorf("cópia ausente = %q, queria gamedata/ps3data/img/c", a.Copies[0].ID)
	}
	if a.Copies[0].DisplayPath != "gamedata/ps3data/img/c.dds.phyre" {
		t.Errorf("display da cópia = %q", a.Copies[0].DisplayPath)
	}
	if !a.Copies[0].Selected {
		t.Error("cópia ausente deveria nascer marcada (default acompanha a imagem)")
	}
	b := findEntry(preview, "gamedata/ps3data/img/b")
	if b == nil {
		t.Fatal("entrada b ausente")
	}
	if len(b.Copies) != 0 {
		t.Errorf("segunda entrada do grupo NÃO deveria ter collapse, veio %d", len(b.Copies))
	}
}

// TestPreviewImportImagesComparaBinario: container idêntico em mods/ nasce
// desmarcado com Differs=false; inválido nasce com erro e desmarcado.
func TestPreviewImportImagesComparaBinario(t *testing.T) {
	version, translateRoot, sample := setupImageImport(t)
	svc := services.NewMetadataService(nil)

	// a já importada: container em mods/ idêntico ao traduzido.
	if err := ddsphyre.ImportContainer(version, "gamedata/ps3data/img/a", sample); err != nil {
		t.Fatalf("semeando mods/: %v", err)
	}
	// h1 com lixo (validação falha).
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/a", sample)
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/h1", []byte("isto não é um phyre"))

	preview, err := svc.PreviewImportImages(version)
	if err != nil {
		t.Fatalf("PreviewImportImages: %v", err)
	}

	a := findEntry(preview, "gamedata/ps3data/img/a")
	if a == nil || a.Differs || a.Selected {
		t.Errorf("entrada a: differs=%v selected=%v (queria false/false)", a.Differs, a.Selected)
	}
	if preview.IdenticalCount != 1 {
		t.Errorf("IdenticalCount = %d, queria 1", preview.IdenticalCount)
	}

	h1 := findEntry(preview, "gamedata/ps3data/img/h1")
	if h1 == nil || h1.ValidationError == "" || h1.Selected {
		t.Errorf("entrada h1: erro=%q selected=%v (queria erro e desmarcada)", h1.ValidationError, h1.Selected)
	}
	if len(preview.Errors) == 0 {
		t.Error("preview deveria agregar o erro de validação")
	}
}

// TestImportImagesSelectionAplicaSomenteMarcado: só a seleção grava; cópia
// ausente marcada recebe os bytes; entrada não marcada não é tocada.
func TestImportImagesSelectionAplicaSomenteMarcado(t *testing.T) {
	version, translateRoot, sample := setupImageImport(t)
	svc := services.NewMetadataService(nil)

	// Traduzido: imagem NOVA para a (diferente do pristine) + h1 nova.
	novo := append([]byte(nil), sample...)
	off := int(newTestDataOffset(t, sample))
	novo[off+1] ^= 0xFF
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/a", novo)
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/h1", sample)

	preview, err := svc.PreviewImportImages(version)
	if err != nil {
		t.Fatalf("PreviewImportImages: %v", err)
	}
	a := findEntry(preview, "gamedata/ps3data/img/a")
	if a == nil {
		t.Fatal("entrada a ausente")
	}
	if !a.Differs {
		t.Fatal("a deveria diferir do binário")
	}

	// Seleção: grava a imagem a e propaga para a cópia b; h1 e a cópia c
	// NÃO entram (só o que está marcado é aplicado).
	result, err := svc.ImportImages(version, dto.ImageImportSelection{
		Entries: []dto.ImageImportSelectionEntry{
			{ID: "gamedata/ps3data/img/a", Image: true, Copies: []string{"gamedata/ps3data/img/b"}},
		},
	})
	if err != nil {
		t.Fatalf("ImportImages: %v", err)
	}
	if result.Total != 2 || len(result.Updated) != 2 {
		t.Fatalf("total/updated = %d/%d (%v)", result.Total, len(result.Updated), result.Updated)
	}

	gravado, err := os.ReadFile(modsContainerPath(t, version, "gamedata/ps3data/img/a"))
	if err != nil {
		t.Fatalf("container de a não foi gravado: %v", err)
	}
	if string(gravado) != string(novo) {
		t.Error("container de a não tem os bytes do traduzido")
	}
	// A cópia marcada (b) recebe os MESMOS bytes da imagem de origem.
	copia, err := os.ReadFile(modsContainerPath(t, version, "gamedata/ps3data/img/b"))
	if err != nil {
		t.Fatalf("container da cópia b não foi gravado: %v", err)
	}
	if string(copia) != string(novo) {
		t.Error("container da cópia b deveria ter os bytes da imagem a")
	}
	// h1 (não selecionado) e a cópia c (não marcada) seguem intocados.
	for _, id := range []string{"gamedata/ps3data/img/h1", "gamedata/ps3data/img/c"} {
		if _, err := os.Stat(modsContainerPath(t, version, id)); !os.IsNotExist(err) {
			t.Errorf("%s não deveria ter sido gravado (err=%v)", id, err)
		}
	}
}

// TestImportImagesRegravaBinarioIdentico: imagem igual ao binário só é
// regravada quando o modal pede (Image=true) — sem isso nada é escrito.
func TestImportImagesRegravaBinarioIdentico(t *testing.T) {
	version, translateRoot, sample := setupImageImport(t)
	svc := services.NewMetadataService(nil)

	if err := ddsphyre.ImportContainer(version, "gamedata/ps3data/img/a", sample); err != nil {
		t.Fatalf("semeando mods/: %v", err)
	}
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/a", sample)

	preview, err := svc.PreviewImportImages(version)
	if err != nil {
		t.Fatalf("PreviewImportImages: %v", err)
	}
	a := findEntry(preview, "gamedata/ps3data/img/a")
	if a == nil || a.Differs {
		t.Fatalf("a deveria ser idêntica ao binário (differs=%v)", a.Differs)
	}

	// Forçada (modal marcou a linha cinza): regrava o binário.
	before, _ := os.Stat(modsContainerPath(t, version, "gamedata/ps3data/img/a"))
	result, err := svc.ImportImages(version, dto.ImageImportSelection{
		Entries: []dto.ImageImportSelectionEntry{
			{ID: "gamedata/ps3data/img/a", Image: true},
		},
	})
	if err != nil {
		t.Fatalf("ImportImages: %v", err)
	}
	if len(result.Updated) != 1 {
		t.Fatalf("updated = %v, queria 1 regravação", result.Updated)
	}
	after, _ := os.Stat(modsContainerPath(t, version, "gamedata/ps3data/img/a"))
	if !after.ModTime().After(before.ModTime()) {
		t.Error("binário idêntico deveria ter sido regravado (mtime)")
	}

	// Sem seleção: nada é gravado.
	vazio, err := svc.ImportImages(version, dto.ImageImportSelection{})
	if err != nil {
		t.Fatalf("ImportImages vazio: %v", err)
	}
	if len(vazio.Updated) != 0 || vazio.Total != 0 {
		t.Errorf("seleção vazia deveria ser no-op, veio %+v", vazio)
	}
}

// TestImportImagesCopiasForaDoGrupoSaoRecusadas: cópia que não é do grupo
// de payload original da imagem não recebe bytes.
func TestImportImagesCopiasForaDoGrupoSaoRecusadas(t *testing.T) {
	version, translateRoot, sample := setupImageImport(t)
	svc := services.NewMetadataService(nil)
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/a", sample)
	writeTranslated(t, translateRoot, "gamedata/ps3data/img/d", sample)

	result, err := svc.ImportImages(version, dto.ImageImportSelection{
		Entries: []dto.ImageImportSelectionEntry{
			{
				ID:     "gamedata/ps3data/img/a",
				Image:  false,
				Copies: []string{"gamedata/ps3data/img/d"}, // Grupo diferente
			},
		},
	})
	if err != nil {
		t.Fatalf("ImportImages: %v", err)
	}
	if len(result.Updated) != 0 || len(result.Failed) != 1 {
		t.Fatalf("updated/failed = %d/%d, queria 0/1", len(result.Updated), len(result.Failed))
	}
	if !strings.Contains(result.Failed[0], "fora do grupo") {
		t.Errorf("motivo da falha = %q", result.Failed[0])
	}
}
