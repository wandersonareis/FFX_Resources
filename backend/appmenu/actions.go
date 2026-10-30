// Handlers do menu: coletam argumentos com dialogs nativos, chamam o
// backend/appcore e renderizam os outcomes em dialogs de sucesso/aviso/
// erro. Nenhuma regra de negócio aqui — só a coreografia do diálogo.
//
// Colisões e on-demand seguem as decisões registradas:
//
//	export/extrair — sem colisão grava direto (nome canônico); com colisão,
//	  avisa e pergunta: prefixar (export_/extract_) / sobrescrever / cancelar;
//	dedupe/dupe — sempre dedupe_/dupe_, sobrescrevendo com pergunta de certeza;
//	import — imported/ ao lado de cada binário; colisão pergunta
//	  sobrescrever/cancelar; macrodic ausente pergunta apontar sob demanda.
package appmenu

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"ffxresources/backend/appcore"
	"ffxresources/backend/appupdater"
	"ffxresources/backend/common"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/uilog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ctxOp é o contexto das operações de menu (sem cancelamento externo hoje;
// o appcore respeita o ctx para um CLI futuro poder abortar).
func ctxOp() context.Context { return context.Background() }

// onExportText é Texto ▸ <kind> ▸ Exportar JSON|Strings|CSV|XLSX|PO.
func (c *Controller) onExportText(kind string, format appcore.ArtifactFormat) {
	label := kindLabel(kind)

	inputs, ok := c.pickOpenMulti("Exportar "+label+" — selecione os binários", textBinaryFilters())
	if !ok {
		return
	}

	// Versão: adivinhada pela detecção (caminho, header, nome); só pergunta
	// quando o arquivo não diz nada.
	groups, ok := c.resolveVersions(inputs)
	if !ok {
		return
	}

	var outcomes []appcore.FileOutcome
	for version, batch := range groups {
		req := appcore.ExportRequest{
			Inputs:    batch,
			Kind:      kind,
			Format:    format,
			Version:   appcore.VersionRequest{Known: true, V: version},
			Collision: appcore.CollisionFail,
		}
		res, err := c.exportWithMacrodicRetry(req)
		if err != nil {
			c.failure("Exportar "+label, err)
			return
		}
		outcomes = append(outcomes, res.Outcomes...)
	}

	// Colisões: pergunta por arquivo (prefixar / sobrescrever / cancelar).
	outcomes = c.resolveExportCollisions(func(input string, policy appcore.CollisionPolicy) (appcore.FileOutcome, bool) {
		var version common.GameVersion
		for v, batch := range groups {
			for _, in := range batch {
				if strings.EqualFold(in, input) {
					version = v
				}
			}
		}
		res, err := c.exportWithMacrodicRetry(appcore.ExportRequest{
			Inputs:    []string{input},
			Kind:      kind,
			Format:    format,
			Version:   appcore.VersionRequest{Known: true, V: version},
			Collision: policy,
		})
		if err != nil {
			c.failure("Exportar "+label, err)
			return appcore.FileOutcome{}, false
		}
		if len(res.Outcomes) == 0 {
			return appcore.FileOutcome{}, false
		}
		return res.Outcomes[0], true
	}, outcomes)

	c.report("Exportar "+label, outcomes)
}

// exportWithMacrodicRetry roda o export e, quando o runtime de texto não
// achou o macrodic na pasta padrão, pergunta e repete com o apontado (o
// TODO do appcore aponta exatamente este fluxo). O caminho fica guardado
// para o resto da sessão.
func (c *Controller) exportWithMacrodicRetry(req appcore.ExportRequest) (appcore.ExportResult, error) {
	req.MacrodicPath = c.macrodicPath
	res, err := appcore.ExportText(ctxOp(), req)
	if errors.Is(err, appcore.ErrMacrodicUnavailable) {
		if path := c.askMacrodic(); path != "" {
			c.macrodicPath = path
			req.MacrodicPath = path
			return appcore.ExportText(ctxOp(), req)
		}
	}
	return res, err
}

// importWithMacrodicRetry é o mesmo gancho do import.
func (c *Controller) importWithMacrodicRetry(req appcore.ImportRequest) (appcore.ImportResult, error) {
	req.MacrodicPath = c.macrodicPath
	res, err := appcore.ImportText(ctxOp(), req)
	if errors.Is(err, appcore.ErrMacrodicUnavailable) {
		if path := c.askMacrodic(); path != "" {
			c.macrodicPath = path
			req.MacrodicPath = path
			return appcore.ImportText(ctxOp(), req)
		}
	}
	return res, err
}

// resolveVersions agrupa os inputs pela versão detectada; os que não dão
// sinal viram UMA pergunta (FFX / FFX-2) e entram no grupo respondido.
// ok=false = o usuário cancelou a pergunta (operação abortada).
func (c *Controller) resolveVersions(inputs []string) (map[common.GameVersion][]string, bool) {
	groups := make(map[common.GameVersion][]string)
	var ambiguous []string
	var ambiguousNames []string

	for _, input := range inputs {
		d, err := appcore.DetectFile(input)
		if err != nil || !d.VersionKnown {
			ambiguous = append(ambiguous, input)
			ambiguousNames = append(ambiguousNames, filepath.Base(input))
			continue
		}
		groups[d.Version] = append(groups[d.Version], input)
	}
	if len(ambiguous) > 0 {
		v, ok := c.askVersion(strings.Join(ambiguousNames, ", "))
		if !ok {
			return nil, false
		}
		groups[v] = append(groups[v], ambiguous...)
	}
	if len(groups) == 0 {
		// só entradas inválidas — deixa o appcore reportar os incompatíveis
		groups[common.GameVersionFFX] = inputs
	}
	return groups, true
}

// collisionChoice é a resposta do dialog de colisão.
type collisionChoice int

const (
	collisionCancelled collisionChoice = iota
	collisionPrefix
	collisionOverwrite
)

// askExportCollision pergunta o que fazer com o destino existente.
func (c *Controller) askExportCollision(target string) collisionChoice {
	a := app()
	if a == nil {
		return collisionCancelled
	}
	choice := collisionCancelled
	dlg := a.Dialog.Question().
		SetTitle("Arquivo já existe").
		SetMessage("O destino abaixo já existe:\n" + target + "\n\nO que fazer?")
	prefix := dlg.AddButton("Criar com prefixo")
	overwrite := dlg.AddButton("Sobrescrever")
	cancel := dlg.AddButton("Cancelar")
	dlg.SetDefaultButton(prefix)
	dlg.SetCancelButton(cancel)
	prefix.OnClick(func() { choice = collisionPrefix })
	overwrite.OnClick(func() { choice = collisionOverwrite })
	dlg.Show()
	return choice
}

// resolveExportCollisions pergunta por cada colisão e re-roda a operação
// para o arquivo com a política escolhida.
func (c *Controller) resolveExportCollisions(rerun func(input string, policy appcore.CollisionPolicy) (appcore.FileOutcome, bool), outcomes []appcore.FileOutcome) []appcore.FileOutcome {
	out := make([]appcore.FileOutcome, 0, len(outcomes))
	for _, o := range outcomes {
		if o.Status != appcore.StatusCollision {
			out = append(out, o)
			continue
		}
		switch c.askExportCollision(o.Reason) {
		case collisionPrefix:
			if again, ok := rerun(o.Input, appcore.CollisionPrefix); ok {
				out = append(out, again)
			}
		case collisionOverwrite:
			if again, ok := rerun(o.Input, appcore.CollisionOverwrite); ok {
				out = append(out, again)
			}
		default:
			// cancelado: o outcome de colisão vira ignorado silencioso
			loggingService.Info("menu: colisão cancelada para %s", o.Input)
		}
	}
	return out
}

// onDedupe é Dedupe ▸ Substituir repetidos por hash.
func (c *Controller) onDedupe() {
	c.artifactOp("Dedupe", "Substituir repetidos por hash", appcore.DedupeArtifact)
}

// onDupe é Dedupe ▸ Resolver hashes.
func (c *Controller) onDupe() {
	c.artifactOp("Dupe", "Resolver hashes (texto completo)", appcore.DupeArtifact)
}

// artifactOp coreografia comum do dedupe/dupe: escolhe artefato, confere o
// destino prefixado (pergunta a certeza quando existe) e roda a operação.
func (c *Controller) artifactOp(op, what string, run func(context.Context, appcore.ArtifactRequest) (appcore.FileOutcome, error)) {
	path, ok := c.pickOpenSingle(op+" — selecione o artefato", artifactFilters(appcore.FormatJSON, true))
	if !ok {
		return
	}

	prefix := appcore.PrefixDedupe
	if op == "Dupe" {
		prefix = appcore.PrefixDupe
	}
	if appcore.ArtifactTargetExists(path, prefix) {
		if !c.confirmOverwrite(appcore.PrefixedPath(path, prefix)) {
			return
		}
	}

	outcome, err := run(ctxOp(), appcore.ArtifactRequest{Path: path})
	if err != nil {
		c.failure(op, err)
		return
	}
	switch outcome.Status {
	case appcore.StatusWritten, appcore.StatusOverwritten:
		c.info(op+" — concluído", what+"\n\nGerado: "+outcome.Output)
	case appcore.StatusMismatch:
		c.warn(op+" — arquivo incompatível", outcome.Reason)
	default:
		c.errBox(op+" — erro", outcome.Reason)
	}
}

// onExtractDDS é Imagem ▸ Extrair .dds.
func (c *Controller) onExtractDDS() { c.extractImage(appcore.ImageDDS) }

// onExtractPNG é Imagem ▸ Extrair .png.
func (c *Controller) onExtractPNG() { c.extractImage(appcore.ImagePNG) }

// extractImage coreografia da extração: multi-seleção de .dds.phyre,
// colisão pergunta (régua do export), resultado em Info/Warning.
func (c *Controller) extractImage(artifact appcore.ImageArtifact) {
	phyres, ok := c.pickOpenMulti("Extrair ."+artifact.String()+" — selecione as texturas", []application.FileFilter{
		{DisplayName: "Texturas (*.dds.phyre)", Pattern: "*.dds.phyre"},
		{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	})
	if !ok {
		return
	}

	res, err := appcore.ExtractImage(ctxOp(), appcore.ExtractImageRequest{
		Phyres:    phyres,
		Artifact:  artifact,
		Collision: appcore.CollisionFail,
	})
	if err != nil {
		c.failure("Extrair "+artifact.String(), err)
		return
	}

	outcomes := c.resolveExportCollisions(func(input string, policy appcore.CollisionPolicy) (appcore.FileOutcome, bool) {
		again, err := appcore.ExtractImage(ctxOp(), appcore.ExtractImageRequest{
			Phyres:    []string{input},
			Artifact:  artifact,
			Collision: policy,
		})
		if err != nil {
			c.failure("Extrair "+artifact.String(), err)
			return appcore.FileOutcome{}, false
		}
		if len(again.Outcomes) == 0 {
			return appcore.FileOutcome{}, false
		}
		return again.Outcomes[0], true
	}, res.Outcomes)

	c.report("Extrair "+artifact.String(), outcomes)
}

// onImportImage é Imagem ▸ Importar .dds.
func (c *Controller) onImportImage() {
	dds, ok := c.pickOpenSingle("Importar .dds — selecione a imagem", []application.FileFilter{
		{DisplayName: "DirectDraw Surface (*.dds)", Pattern: "*.dds"},
		{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	})
	if !ok {
		return
	}
	phyres, ok := c.pickOpenMulti("Importar .dds — selecione os .dds.phyre", []application.FileFilter{
		{DisplayName: "Texturas (*.dds.phyre)", Pattern: "*.dds.phyre"},
		{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	})
	if !ok {
		return
	}

	res, err := appcore.ImportImageFile(ctxOp(), appcore.ImportImageRequest{
		DDS:       dds,
		Phyres:    phyres,
		Collision: appcore.CollisionFail,
	})
	if err != nil {
		c.failure("Importar imagem", err)
		return
	}

	// Colisão: sobrescrever ou cancelar (binário com prefixo não faz
	// sentido — o nome é o real do container).
	outcomes := make([]appcore.FileOutcome, 0, len(res.Outcomes))
	for _, o := range res.Outcomes {
		if o.Status != appcore.StatusCollision {
			outcomes = append(outcomes, o)
			continue
		}
		if c.confirmOverwrite(o.Reason) {
			again, err := appcore.ImportImageFile(ctxOp(), appcore.ImportImageRequest{
				DDS:       dds,
				Phyres:    []string{o.Input},
				Collision: appcore.CollisionOverwrite,
			})
			if err != nil {
				c.failure("Importar imagem", err)
				continue
			}
			if len(again.Outcomes) > 0 {
				outcomes = append(outcomes, again.Outcomes[0])
			}
		}
	}

	// Log para janela de logs: ação + arquivo por container
	uilog.Info("Import", "%s: importando", filepath.Base(dds))
	for _, o := range outcomes {
		dest := o.Output
		if dest == "" {
			dest = o.Input
		}
		if o.Status == appcore.StatusFailed || o.Status == appcore.StatusMismatch {
			uilog.File("Import", dest, uilog.StatusFailed, o.Reason)
		} else {
			uilog.File("Import", dest, uilog.StatusOk, "importado de "+dds)
		}
	}

	c.reportImport("Importar imagem", outcomes)
}

// onImportText é Importar ▸ <kind> ▸ Importar JSON|Strings|CSV|XLSX|PO.
func (c *Controller) onImportText(kind string, format appcore.ArtifactFormat) {
	label := kindLabel(kind)

	text, ok := c.pickOpenSingle("Importar "+label+" — selecione o texto", artifactFilters(format, false))
	if !ok {
		return
	}
	binaries, ok := c.pickOpenMulti("Importar "+label+" — selecione os binários (busque *.bin para todos)", textBinaryFilters())
	if !ok {
		return
	}

	res, err := c.importWithMacrodicRetry(appcore.ImportRequest{
		Text:      text,
		Binaries:  binaries,
		Kind:      kind,
		Collision: appcore.CollisionFail,
	})
	if err != nil {
		uilog.Error("Import", "%s falhou: %v", filepath.Base(text), err)
		c.failure("Importar "+label, err)
		return
	}

	// Colisão: sobrescrever ou cancelar (o nome do binário é o real).
	outcomes := make([]appcore.FileOutcome, 0, len(res.Outcomes))
	for _, o := range res.Outcomes {
		if o.Status != appcore.StatusCollision {
			outcomes = append(outcomes, o)
			continue
		}
		if c.confirmOverwrite(o.Reason) {
			again, err := c.importWithMacrodicRetry(appcore.ImportRequest{
				Text:      text,
				Binaries:  []string{o.Input},
				Kind:      kind,
				Collision: appcore.CollisionOverwrite,
			})
			if err != nil {
				uilog.Error("Import", "%s falhou: %v", o.Input, err)
				c.failure("Importar "+label, err)
				continue
			}
			if len(again.Outcomes) > 0 {
				outcomes = append(outcomes, again.Outcomes[0])
			}
		}
	}

	// Log da janela: a ação já aconteceu — o artefato e cada binário com o
	// resultado na MESMA linha (gravado/sobrescrito/falha/ignorado).
	uilog.Info("Import", "artefato %s — %d binário(s)", filepath.Base(text), len(outcomes))
	for _, o := range outcomes {
		detail := o.Reason
		if detail == "" && o.Output != "" {
			detail = o.Output
		}
		uilog.File("Import", o.Input, o.Status.String(), detail)
	}

	c.reportImport("Importar "+label, outcomes)
	c.emitImportDone()
}

// emitImportDone avisa a UI aberta (se houver) para recarregar a árvore —
// o mesmo contrato do import do frontend (game-version-tab escuta o evento).
func (c *Controller) emitImportDone() {
	if a := app(); a != nil {
		a.Event.Emit("ImportDone")
	}
}

// onCheckUpdates é Ajuda ▸ Procurar atualizações…: abre a janela nativa do
// updater e percorre check → download → verificação → stage. Sem release nova
// a janela permanece aberta no estado "Você está atualizado" e o usuário a
// fecha; quando há erro, a própria janela mostra o estado de falha (daí o log
// e não um dialog duplo).
func (c *Controller) onCheckUpdates() {
	if err := appupdater.CheckNow(); err != nil {
		loggingService.Error("menu: checagem de atualização falhou: %s", err)
	}
}

// report renderiza o resumo do lote em UM dialog: sucesso com os caminhos
// gerados, aviso quando houver incompatíveis, erro quando houver falhas.
func (c *Controller) report(op string, outcomes []appcore.FileOutcome) {
	c.reportOutcomes(op, outcomes, false)
}

// reportImport é o report do import (ignorados não viram dialog — só
// contam no resumo, como combinado: "ignora sem aviso").
func (c *Controller) reportImport(op string, outcomes []appcore.FileOutcome) {
	c.reportOutcomes(op, outcomes, true)
}

func (c *Controller) reportOutcomes(op string, outcomes []appcore.FileOutcome, silentIgnored bool) {
	if len(outcomes) == 0 {
		c.info(op, "Nada foi processado.")
		return
	}

	var written, overwritten, ignored, mismatch, failed int
	var outputs, problems []string
	for _, o := range outcomes {
		switch o.Status {
		case appcore.StatusWritten:
			written++
			outputs = append(outputs, o.Output)
		case appcore.StatusOverwritten:
			overwritten++
			outputs = append(outputs, o.Output)
		case appcore.StatusIgnored:
			ignored++
		case appcore.StatusMismatch:
			mismatch++
			problems = append(problems, o.Reason)
		case appcore.StatusFailed:
			failed++
			problems = append(problems, o.Reason)
		}
	}

	summary := fmt.Sprintf("%d arquivo(s) gerado(s)", written+overwritten)
	if overwritten > 0 {
		summary += fmt.Sprintf(" (%d sobrescrito(s))", overwritten)
	}
	if ignored > 0 && !silentIgnored {
		summary += fmt.Sprintf(", %d ignorado(s)", ignored)
	}
	if ignored > 0 && silentIgnored {
		summary += fmt.Sprintf(", %d sem correspondência no texto (ignorado(s))", ignored)
	}
	if len(outputs) > 0 {
		summary += "\n\n" + joinCapped(outputs, 12)
	}

	if mismatch > 0 || failed > 0 {
		summary += "\n\n" + fmt.Sprintf("%d incompatível(eis)/falha(s):\n", mismatch+failed) + joinCapped(problems, 8)
	}

	if written+overwritten == 0 {
		c.warn(op+" — nada processado", summary)
		return
	}
	if mismatch > 0 || failed > 0 {
		c.warn(op+" — concluído com ressalvas", summary)
		return
	}
	c.info(op+" — concluído", summary)
}

// joinCapped junta as linhas limitando a lista (o dialog não faz scroll
// infinito; o resto vira "… e mais N").
func joinCapped(lines []string, max int) string {
	if len(lines) <= max {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:max], "\n") + fmt.Sprintf("\n… e mais %d", len(lines)-max)
}

// kindLabel devolve o rótulo humano do kind (o mesmo da árvore do frontend).
func kindLabel(kind string) string {
	for _, k := range textKinds {
		if k.kind == kind {
			return k.label
		}
	}
	return kind
}
