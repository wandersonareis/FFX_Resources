// Dialogs nativos do Wails v3: seletores de arquivo, mensagens de
// sucesso/incompatível/erro e perguntas. É a ÚNICA via de comunicação das
// operações do menu com o usuário — nada de frontend JS.
//
// A classificação de erros de filesystem (permissão, inexistência,
// diretório, rede/UNC, arquivo em uso) vive em errors_windows.go /
// errors_other.go; o Windows carrega os códigos nativos, o resto usa as
// sentinelas io/fs.
package appmenu

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"ffxresources/backend/appcore"
	"ffxresources/backend/common"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// app devolve a instância viva do Wails (nil fora do ciclo de vida — os
// handlers de menu só rodam dentro dele, mas os helpers defendem).
func app() *application.App {
	return application.Get()
}

// info é o dialog de sucesso (verde, botão OK).
func (c *Controller) info(title, message string) {
	if a := app(); a != nil {
		a.Dialog.Info().SetTitle(title).SetMessage(message).Show()
	}
}

// warn é o dialog de aviso (arquivo incompatível, operação pulada).
func (c *Controller) warn(title, message string) {
	if a := app(); a != nil {
		a.Dialog.Warning().SetTitle(title).SetMessage(message).Show()
	}
}

// errBox é o dialog de erro técnico (falha de acesso, gravação, rede).
func (c *Controller) errBox(title, message string) {
	if a := app(); a != nil {
		a.Dialog.Error().SetTitle(title).SetMessage(message).Show()
	}
}

// failure classifica o erro e abre o dialog correspondente: incompatível
// (appcore.IncompatibleError) vira aviso; o resto vira erro técnico com a
// causa legível (permissão, rede, ...).
func (c *Controller) failure(op string, err error) {
	var inc *appcore.IncompatibleError
	if errors.As(err, &inc) {
		c.warn(op+" — arquivo incompatível", inc.Error())
		return
	}
	c.errBox(op+" — erro", classifyError(err))
}

// askVersion pergunta a versão do jogo quando a detecção não resolve
// (Question com botões FFX / FFX-2; ok=false = cancelado).
func (c *Controller) askVersion(what string) (common.GameVersion, bool) {
	a := app()
	if a == nil {
		return common.GameVersion{}, false
	}
	chosen := common.GameVersion{}
	cancelled := false

	dlg := a.Dialog.Question().
		SetTitle("Versão do jogo").
		SetMessage("Não foi possível adivinhar a versão de:\n" + what + "\n\nQual árvore de jogo usar?")
	ffx := dlg.AddButton("FINAL FANTASY X")
	ffx2 := dlg.AddButton("FINAL FANTASY X-2")
	cancel := dlg.AddButton("Cancelar")
	dlg.SetDefaultButton(ffx)
	dlg.SetCancelButton(cancel)

	ffx.OnClick(func() { chosen = common.GameVersionFFX })
	ffx2.OnClick(func() { chosen = common.GameVersionFFX2 })
	cancel.OnClick(func() { cancelled = true })

	dlg.Show()
	if cancelled || chosen.String() == "" {
		return common.GameVersion{}, false
	}
	return chosen, true
}

// askMacrodic pergunta e coleta o macrodic.dcp quando o runtime de texto
// não o achou na pasta padrão (o TODO do appcore aponta exatamente aqui:
// apontar sob demanda no menu; detectar/flag no CLI).
func (c *Controller) askMacrodic() string {
	a := app()
	if a == nil {
		return ""
	}
	proceed := false
	dlg := a.Dialog.Question().
		SetTitle("Dicionário de macros").
		SetMessage("Os binários de texto precisam do macrodic.dcp para resolver tags MCR,\n" +
			"e ele não foi encontrado na pasta padrão (árvore do jogo).\n\n" +
			"Apontar o arquivo agora?")
	yes := dlg.AddButton("Apontar macrodic.dcp")
	cancel := dlg.AddButton("Cancelar")
	dlg.SetDefaultButton(yes)
	dlg.SetCancelButton(cancel)
	yes.OnClick(func() { proceed = true })
	dlg.Show()

	if !proceed {
		return ""
	}
	path, ok := c.pickOpenSingle("Selecionar macrodic.dcp", []application.FileFilter{
		{DisplayName: "Dicionário de macros (*.dcp)", Pattern: "*.dcp"},
		{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	})
	if !ok {
		return ""
	}
	return path
}

// confirmOverwrite é a pergunta de certeza antes de sobrescrever
// (dedupe/dupe). ok=false = cancelado (nada é feito).
func (c *Controller) confirmOverwrite(target string) bool {
	a := app()
	if a == nil {
		return false
	}
	confirmed := false
	dlg := a.Dialog.Question().
		SetTitle("Sobrescrever arquivo").
		SetMessage("O arquivo abaixo já existe e será SOBSCRITO:\n" + target + "\n\nContinuar?")
	yes := dlg.AddButton("Sobrescrever")
	cancel := dlg.AddButton("Cancelar")
	dlg.SetDefaultButton(cancel) // seguro por padrão
	dlg.SetCancelButton(cancel)
	yes.OnClick(func() { confirmed = true })
	dlg.Show()
	return confirmed
}

// pickOpenSingle abre o seletor nativo de arquivo (um). ok=false = cancelado.
func (c *Controller) pickOpenSingle(title string, filters []application.FileFilter) (string, bool) {
	a := app()
	if a == nil {
		return "", false
	}
	path, err := a.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:   title,
		Filters: filters,
	}).PromptForSingleSelection()
	if err != nil || strings.TrimSpace(path) == "" {
		return "", false
	}
	return path, true
}

// pickOpenMulti abre o seletor nativo MULTI-arquivo. ok=false = cancelado
// (ou seleção vazia).
func (c *Controller) pickOpenMulti(title string, filters []application.FileFilter) ([]string, bool) {
	a := app()
	if a == nil {
		return nil, false
	}
	paths, err := a.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:   title,
		Filters: filters,
	}).PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return nil, false
	}
	return paths, true
}

// textBinaryFilters são os filtros de seleção de binários de texto
// (extensões conhecidas do app).
func textBinaryFilters() []application.FileFilter {
	return []application.FileFilter{
		{DisplayName: "Binários de texto do jogo (*.bin; *.dcp; *.sps2; *.msb)", Pattern: "*.bin;*.dcp;*.sps2;*.msb"},
		{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	}
}

// artifactFilters filtra artefatos pelo formato (json/strings/csv/xlsx/po).
func artifactFilters(format appcore.ArtifactFormat, both bool) []application.FileFilter {
	if both {
		return []application.FileFilter{
			{DisplayName: "Artefatos de texto (*.json; *.strings; *.csv; *.xlsx; *.po)", Pattern: "*.json;*.strings;*.csv;*.xlsx;*.po"},
			{DisplayName: "JSON (*.json)", Pattern: "*.json"},
			{DisplayName: "Strings (*.strings)", Pattern: "*.strings"},
			{DisplayName: "CSV (*.csv)", Pattern: "*.csv"},
			{DisplayName: "XLSX (*.xlsx)", Pattern: "*.xlsx"},
			{DisplayName: "PO (*.po)", Pattern: "*.po"},
			{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
		}
	}
	ext := format.String()
	return []application.FileFilter{
		{DisplayName: fmt.Sprintf("Artefato %s (*.%s)", strings.ToUpper(ext), ext), Pattern: "*." + ext},
		{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	}
}

// classifyError traduz erros de filesystem para mensagens legíveis
// (permissão, inexistência, diretório, rede/UNC, em uso). A parte
// Windows-específica mora em errors_windows.go.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		err = linkErr.Err
	}
	if sysMsg := classifySystemError(err); sysMsg != "" {
		return sysMsg
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "Sem permissão para ler/gravar neste caminho (execute como administrador ou verifique as permissões da pasta)."
	case errors.Is(err, fs.ErrNotExist):
		return "Arquivo ou pasta não encontrada (o caminho mudou ou foi removido)."
	case errors.Is(err, fs.ErrExist):
		return "O arquivo de destino já existe."
	}
	return err.Error()
}
