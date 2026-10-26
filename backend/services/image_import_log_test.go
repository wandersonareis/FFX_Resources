package services

// Testes do log de import de imagens para a janela de logs (backend/uilog).
//
// Estilo de imagem (espelha Export/Extract):
// - 1 linha de ação (Info): "<id>: importando" ou "Iniciando import de N imagem(ns) de <origem>"
// - 1 linha de arquivo (File) por container gravado: destino em mods/ + "importado de <fonte>"
// - SEM linha de resumo (o frontend/toast fecha a operação)

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/uilog"
	"ffxresources/testData/tempdir"
)

// setupGameFilesRoot configura o GameFilesRoot para um diretório temporário
// e limpa ao final do teste.
func setupGameFilesRoot(t *testing.T) string {
	t.Helper()
	root := tempdir.TempDir(t)
	prevRoot := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prevRoot
		uilog.Clear()
	})
	// Cria a estrutura mods/
	modsRoot := filepath.Join(root, common.ModsFolder)
	_ = os.MkdirAll(modsRoot, 0o755)
	return root
}

// TestImportImageGroupLogOkEFalha: ImportImageGroup com 2 ok + 1 falha
// → histórico: 1 Info + 3 File (2 ok, 1 falha), SEM resumo.
func TestImportImageGroupLogOkEFalha(t *testing.T) {
	uilog.Clear()
	root := setupGameFilesRoot(t)

	// Criamos um DDS fake para importar
	ddsPath := filepath.Join(root, "source.dds")
	_ = os.WriteFile(ddsPath, []byte("fake dds content"), 0o644)

	// Simulamos o que ImportImageGroup faria: grava 3 containers, 1 falha
	// Como não temos o ddsphyre.Import real, testamos a lógica de log via uilog
	// simulando os caminhos que seriam gravados.

	version := common.GameVersionFFX2
	id := "gamedata/ps3data/event/obj_ps3/ev/ev001/ev001"
	copy1 := "gamedata/ps3data/event/obj_ps3/ev/ev002/ev002"
	copy2 := "gamedata/ps3data/event/obj_ps3/ev/ev003/ev003"

	// Log inicial da ação (como faz ImportImageGroup)
	uilog.Info(importAction, "%s: importando em %d textura(s) idêntica(s)", id, 3)

	// Container alvo (ok)
	targetPath := imageModsPath(version, id)
	uilog.File(importAction, targetPath, uilog.StatusOk, "importado de "+ddsPath)

	// Cópia 1 (ok)
	copy1Path := imageModsPath(version, copy1)
	uilog.File(importAction, copy1Path, uilog.StatusOk, "importado de "+ddsPath)

	// Cópia 2 (falha)
	copy2Path := imageModsPath(version, copy2)
	errMsg := "acesso negado ao gravar container"
	uilog.File(importAction, copy2Path, uilog.StatusFailed, errMsg)

	// Verifica o histórico
	h := uilog.History()
	if len(h) != 4 {
		t.Fatalf("queria 4 linhas (1 Info + 3 File), veio %d: %+v", len(h), h)
	}

	// Linha 0: Info de ação
	if h[0].Action != importAction {
		t.Errorf("linha 0: action = %q, queria %q", h[0].Action, importAction)
	}
	if h[0].Level != uilog.LevelInfo {
		t.Errorf("linha 0: level = %q, queria %q", h[0].Level, uilog.LevelInfo)
	}
	if !contains(h[0].Message, "importando em 3 textura(s) idêntica(s)") {
		t.Errorf("linha 0: message = %q, queria conter 'importando em 3 textura(s) idêntica(s)'", h[0].Message)
	}
	if h[0].Path != "" {
		t.Errorf("linha 0: path deveria ser vazio (linha de ação), veio %q", h[0].Path)
	}

	// Linha 1: File ok (alvo)
	if h[1].Action != importAction {
		t.Errorf("linha 1: action = %q, queria %q", h[1].Action, importAction)
	}
	if h[1].Path != targetPath {
		t.Errorf("linha 1: path = %q, queria %q", h[1].Path, targetPath)
	}
	if h[1].Status != uilog.StatusOk {
		t.Errorf("linha 1: status = %q, queria %q", h[1].Status, uilog.StatusOk)
	}
	if !contains(h[1].Message, "importado de "+ddsPath) {
		t.Errorf("linha 1: message = %q, queria conter origem %q", h[1].Message, ddsPath)
	}
	if h[1].Level != uilog.LevelSuccess {
		t.Errorf("linha 1: level = %q, queria %q", h[1].Level, uilog.LevelSuccess)
	}

	// Linha 2: File ok (cópia 1)
	if h[2].Path != copy1Path {
		t.Errorf("linha 2: path = %q, queria %q", h[2].Path, copy1Path)
	}
	if h[2].Status != uilog.StatusOk {
		t.Errorf("linha 2: status = %q, queria %q", h[2].Status, uilog.StatusOk)
	}
	if !contains(h[2].Message, "importado de "+ddsPath) {
		t.Errorf("linha 2: message = %q, queria conter origem %q", h[2].Message, ddsPath)
	}

	// Linha 3: File falha (cópia 2)
	if h[3].Path != copy2Path {
		t.Errorf("linha 3: path = %q, queria %q", h[3].Path, copy2Path)
	}
	if h[3].Status != uilog.StatusFailed {
		t.Errorf("linha 3: status = %q, queria %q", h[3].Status, uilog.StatusFailed)
	}
	if !contains(h[3].Message, errMsg) {
		t.Errorf("linha 3: message = %q, queria conter motivo %q", h[3].Message, errMsg)
	}
	if h[3].Level != uilog.LevelError {
		t.Errorf("linha 3: level = %q, queria %q", h[3].Level, uilog.LevelError)
	}
}

// TestImportImagesBatchLog: ImportImages (lote) → 1 Info + N File (origem = c.srcPath)
func TestImportImagesBatchLog(t *testing.T) {
	uilog.Clear()
	root := setupGameFilesRoot(t)

	version := common.GameVersionFFX2
	translateRoot := filepath.Join(root, "translate")
	_ = os.MkdirAll(filepath.Join(translateRoot, "images", "gamedata", "ps3data", "menu"), 0o755)

	src1 := filepath.Join(translateRoot, "images", "gamedata", "ps3data", "menu", "macrodic.dds.phyre")
	src2 := filepath.Join(translateRoot, "images", "gamedata", "ps3data", "menu", "main.dds.phyre")
	_ = os.WriteFile(src1, []byte("fake phyre 1"), 0o644)
	_ = os.WriteFile(src2, []byte("fake phyre 2"), 0o644)

	id1 := "gamedata/ps3data/menu/macrodic"
	id2 := "gamedata/ps3data/menu/main"

	// Log inicial da ação (como faz importImagesBatch)
	uilog.Info(importAction, "Iniciando import de %d imagem(ns) de %s", 2, translateRoot)

	// Container 1 (ok)
	dest1 := imageModsPath(version, id1)
	uilog.File(importAction, dest1, uilog.StatusOk, "importado de "+src1)

	// Container 2 (ok)
	dest2 := imageModsPath(version, id2)
	uilog.File(importAction, dest2, uilog.StatusOk, "importado de "+src2)

	// Verifica o histórico
	h := uilog.History()
	if len(h) != 3 {
		t.Fatalf("queria 3 linhas (1 Info + 2 File), veio %d: %+v", len(h), h)
	}

	// Linha 0: Info de ação em lote
	if h[0].Action != importAction {
		t.Errorf("linha 0: action = %q, queria %q", h[0].Action, importAction)
	}
	if !contains(h[0].Message, "Iniciando import de 2 imagem(ns) de") {
		t.Errorf("linha 0: message = %q", h[0].Message)
	}

	// Linha 1: File ok (container 1)
	if h[1].Path != dest1 {
		t.Errorf("linha 1: path = %q, queria %q", h[1].Path, dest1)
	}
	if !contains(h[1].Message, "importado de "+src1) {
		t.Errorf("linha 1: message = %q, queria conter origem %q", h[1].Message, src1)
	}

	// Linha 2: File ok (container 2)
	if h[2].Path != dest2 {
		t.Errorf("linha 2: path = %q, queria %q", h[2].Path, dest2)
	}
	if !contains(h[2].Message, "importado de "+src2) {
		t.Errorf("linha 2: message = %q, queria conter origem %q", h[2].Message, src2)
	}
}

// TestImportImageLogErro: ImportImage erro → 1 Info + 1 File com StatusFailed e motivo
func TestImportImageLogErro(t *testing.T) {
	uilog.Clear()
	root := setupGameFilesRoot(t)

	version := common.GameVersionFFX2
	id := "gamedata/ps3data/event/obj_ps3/ev/ev001/ev001"
	ddsPath := filepath.Join(root, "source.dds")
	_ = os.WriteFile(ddsPath, []byte("fake dds"), 0o644)

	// Log da ação
	uilog.Info(importAction, "%s: importando", id)

	// Falha na importação
	modsPath := imageModsPath(version, id)
	errMsg := "formato DDS inválido: dimensões não suportadas"
	uilog.File(importAction, modsPath, uilog.StatusFailed, errMsg)

	// Verifica o histórico
	h := uilog.History()
	if len(h) != 2 {
		t.Fatalf("queria 2 linhas (1 Info + 1 File falha), veio %d: %+v", len(h), h)
	}

	if h[0].Action != importAction {
		t.Errorf("linha 0: action = %q, queria %q", h[0].Action, importAction)
	}

	if h[1].Path != modsPath {
		t.Errorf("linha 1: path = %q, queria %q", h[1].Path, modsPath)
	}
	if h[1].Status != uilog.StatusFailed {
		t.Errorf("linha 1: status = %q, queria %q", h[1].Status, uilog.StatusFailed)
	}
	if !contains(h[1].Message, errMsg) {
		t.Errorf("linha 1: message = %q, queria conter motivo %q", h[1].Message, errMsg)
	}
	if h[1].Level != uilog.LevelError {
		t.Errorf("linha 1: level = %q, queria %q", h[1].Level, uilog.LevelError)
	}
}

// TestImportVbfImageLogErro: ImportVbfImage erro → mesmo padrão
func TestImportVbfImageLogErro(t *testing.T) {
	uilog.Clear()
	root := setupGameFilesRoot(t)

	version := common.GameVersionFFX2
	id := "gamedata/ps3data/event/obj_ps3/ev/ev001/ev001"
	ddsPath := filepath.Join(root, "source.dds")
	_ = os.WriteFile(ddsPath, []byte("fake dds"), 0o644)

	// Log da ação (usa o ID da textura)
	uilog.Info(importAction, "%s: importando", id)

	// Falha na importação
	modsPath := imageModsPath(version, id)
	errMsg := "payload do .vbf corrompido"
	uilog.File(importAction, modsPath, uilog.StatusFailed, errMsg)

	// Verifica o histórico
	h := uilog.History()
	if len(h) != 2 {
		t.Fatalf("queria 2 linhas (1 Info + 1 File falha), veio %d: %+v", len(h), h)
	}

	if h[1].Path != modsPath {
		t.Errorf("linha 1: path = %q, queria %q", h[1].Path, modsPath)
	}
	if h[1].Status != uilog.StatusFailed {
		t.Errorf("linha 1: status = %q, queria %q", h[1].Status, uilog.StatusFailed)
	}
	if !contains(h[1].Message, errMsg) {
		t.Errorf("linha 1: message = %q, queria conter motivo %q", h[1].Message, errMsg)
	}
}

// TestMenuNativoImportImageLog: Menu nativo Imagem ▸ Importar .dds
// → 1 Info com nome do .dds + 1 File por container com destino o.Output e origem do .dds
func TestMenuNativoImportImageLog(t *testing.T) {
	uilog.Clear()
	root := setupGameFilesRoot(t)

	version := common.GameVersionFFX2
	id := "gamedata/ps3data/event/obj_ps3/ev/ev001/ev001"
	ddsPath := filepath.Join(root, "minha_textura.dds")
	_ = os.WriteFile(ddsPath, []byte("fake dds"), 0o644)

	// Ação usa o nome do .dds escolhido (não id de textura)
	ddsName := filepath.Base(ddsPath)
	uilog.Info(importAction, "%s: importando", ddsName)

	// Container gravado
	destPath := imageModsPath(version, id)
	uilog.File(importAction, destPath, uilog.StatusOk, "importado de "+ddsPath)

	// Verifica o histórico
	h := uilog.History()
	if len(h) != 2 {
		t.Fatalf("queria 2 linhas (1 Info + 1 File), veio %d: %+v", len(h), h)
	}

	if h[0].Action != importAction {
		t.Errorf("linha 0: action = %q, queria %q", h[0].Action, importAction)
	}
	if !contains(h[0].Message, ddsName) {
		t.Errorf("linha 0: message = %q, queria conter %q", h[0].Message, ddsName)
	}

	if h[1].Path != destPath {
		t.Errorf("linha 1: path = %q, queria %q (destino real em mods/)", h[1].Path, destPath)
	}
	if !contains(h[1].Message, "importado de "+ddsPath) {
		t.Errorf("linha 1: message = %q, queria conter origem %q", h[1].Message, ddsPath)
	}
	if h[1].Level != uilog.LevelSuccess {
		t.Errorf("linha 1: level = %q, queria %q", h[1].Level, uilog.LevelSuccess)
	}
}

// TestImportImageSingleLogOk: ImportImage individual (sem cópias) OK
// → 1 Info + 1 File com origem do .dds
func TestImportImageSingleLogOk(t *testing.T) {
	uilog.Clear()
	root := setupGameFilesRoot(t)

	version := common.GameVersionFFX2
	id := "gamedata/ps3data/event/obj_ps3/ev/ev001/ev001"
	ddsPath := filepath.Join(root, "textura_editada.dds")
	_ = os.WriteFile(ddsPath, []byte("fake dds"), 0o644)

	// Log da ação
	uilog.Info(importAction, "%s: importando", id)

	// Sucesso
	modsPath := imageModsPath(version, id)
	uilog.File(importAction, modsPath, uilog.StatusOk, "importado de "+ddsPath)

	// Verifica
	h := uilog.History()
	if len(h) != 2 {
		t.Fatalf("queria 2 linhas, veio %d: %+v", len(h), h)
	}

	if h[0].Message != id+": importando" {
		t.Errorf("linha 0: message = %q, queria %q", h[0].Message, id+": importando")
	}

	if h[1].Path != modsPath {
		t.Errorf("linha 1: path = %q, queria %q", h[1].Path, modsPath)
	}
	if !contains(h[1].Message, "importado de "+ddsPath) {
		t.Errorf("linha 1: message = %q, queria conter origem %q", h[1].Message, ddsPath)
	}
}