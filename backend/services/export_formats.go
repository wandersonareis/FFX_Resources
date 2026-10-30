// Export agrupado de textos: UMA operação com um step por kind — o
// frontend chama UM binding (ExportArtifactBatch) e o modal de progresso
// é dirigido por eventos (backend/progress). Os formatos (json, strings,
// csv, xlsx) compartilham a MESMA extração; só o write muda.
package services

import (
	"fmt"
	"path/filepath"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/formatters"
	_ "ffxresources/backend/formatters/csv"
	jsonfmt "ffxresources/backend/formatters/json"
	_ "ffxresources/backend/formatters/strings"
	_ "ffxresources/backend/formatters/xlsx"
	"ffxresources/backend/progress"
	"ffxresources/backend/uilog"
)

// ExportPlanItem é UMA ação do lote: um kind e os ids dele (vazio = tudo
// do kind; com seleção, os ids marcados na árvore).
//
// Complete marca a seleção TOTAL do kind — o checkbox da raiz na árvore.
// Com ele o backend desvia para o artefato bulk ÚNICO (um arquivo, dedup
// global entre entradas), com o prefixo treeBulkPrefix no base para NÃO
// colidir com o artefato canônico do export completo. O prefixo é só a
// marca da origem: o import decide o kind pelo metadata.key de cada
// entrada (detectImportKind), nunca pelo nome do arquivo.
type ExportPlanItem struct {
	Kind     string   `json:"kind"`
	IDs      []string `json:"ids"`
	Complete bool     `json:"complete"`
}

// kindStepLabels são os rótulos PT dos steps do modal de export — os
// mesmos nomes da árvore (KIND_LABELS do frontend).
var kindStepLabels = map[string]string{
	KindEvents:     "Eventos",
	KindObjects:    "Sistema",
	KindMacro:      "Dicionário",
	KindLockit:     "Loc Kit",
	KindHelp:       "Painel de ajuda",
	KindBattleText: "Textos de batalha",
	KindCloud:      "Cloud",
	KindTutorial:   "Tutorial",
	KindMenuMain:   "Texto menu principal",
}

// ExportArtifactBatch exporta o lote como UMA operação agrupada: um step
// por kind, cada um com a própria barra (extração tick a tick — p.ex.
// "Eventos 0→301/301"). Formato: "json" | "strings" | "csv" | "xlsx".
// O progresso sai pelos eventos do wails (ProgressStart/Update/End) — o
// frontend só renderiza o modal; erro após o início do batch aparece lá.
func (s *MetadataService) ExportArtifactBatch(items []ExportPlanItem, version common.GameVersion, langs []string, format string) (paths []string, retErr error) {
	// Validação ANTES do batch: erros daqui ainda são toast do frontend.
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "json", "strings", "csv", "xlsx":
	default:
		return nil, fmt.Errorf("formato de export desconhecido: %s", format)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("nada para exportar: lote vazio")
	}
	for _, it := range items {
		if kind := strings.ToLower(strings.TrimSpace(it.Kind)); kindStepLabels[kind] == "" {
			return nil, fmt.Errorf("unknown kind: %s", it.Kind)
		}
	}

	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = kindStepLabels[strings.ToLower(strings.TrimSpace(it.Kind))]
	}
	batch := s.progress.NewBatch(fmt.Sprintf("Exportando textos (%s)…", exportFormatLabel(format)))
	steps := batch.Steps(labels...)
	// Sem emitter (testes/CLI): batch/Steps nil — cada ação ganha um step
	// nil, e TODO método de *Step é nil-safe (o Reporter vira no-op).
	if steps == nil {
		steps = make([]*progress.Step, len(items))
	}

	uilog.Info("Export", "Iniciando export de %d kind(s) como %s", len(items), format)

	var all []string
	for i, it := range items {
		kind := strings.ToLower(strings.TrimSpace(it.Kind))
		uilog.Info("Export", "%s: exportando", kindStepLabels[kind])
		ps, err := s.exportKind(steps[i], kind, format, version, it.IDs, it.Complete, langs)
		all = append(all, ps...)
		if err != nil {
			steps[i].Fail(err.Error())
			batch.End()
			uilog.Error("Export", "Falha no export: %v", err)
			return all, err
		}
	}
	batch.SetSummary(len(all), exportSummaryDestination(all))
	batch.End()
	uilog.Success("Export", "Export concluído: %d arquivo(s) em %s", len(all), exportSummaryDestination(all))
	return all, nil
}

// exportFormatLabel é o nome do formato nos títulos.
func exportFormatLabel(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "JSON":
		return "JSON"
	case "STRINGS":
		return "Strings"
	case "CSV":
		return "CSV"
	case "XLSX":
		return "XLSX"
	}
	return format
}

// exportSummaryDestination resume o destino para o estado final do modal:
// caminho único, ou "N arquivos em <diretório>".
func exportSummaryDestination(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return paths[0]
	}
	dir := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		if filepath.Dir(p) != dir {
			return fmt.Sprintf("%d arquivos", len(paths))
		}
	}
	return fmt.Sprintf("%d arquivos em %s", len(paths), dir)
}

// exportKind extrai e escreve o artefato de UM kind no formato pedido.
// O Reporter r é o step do batch: a extração (GetCollection → builders)
// faz SetTotal + um tick por entrada; o write em si é rápido e não ticka.
//
// complete = a seleção marcou o kind INTEIRO (checkbox da raiz na árvore):
// o artefato vira o bulk único, com prefixo treeBulkPrefix. Kinds sem
// caminho bulk (lockit) ignoram a marca e seguem por-entrada — a seleção
// continua válida, só não há artefato único para eles.
func (s *MetadataService) exportKind(r progress.Reporter, kind, format string, version common.GameVersion, ids []string, complete bool, langs []string) (paths []string, retErr error) {
	r = progress.OrNop(r)
	kind = strings.ToLower(strings.TrimSpace(kind))
	prefix := ""
	if complete {
		if !hasBulkArtifact(kind) {
			// Sem artefato bulk registrado: seleção normal, por entrada.
			complete = false
		} else {
			prefix = treeBulkPrefix
			// Kind inteiro = o universo completo: ids vazios É o contrato
			// de bulk do exportKind (e events carrega tudo de propósito).
			ids = nil
		}
	}
	// scope fica com o PEDIDO (normalizado, como antes) e é o que nomeia o
	// artefato — bulk / uma entrada / subconjunto. A extração pode filtrar
	// o universo sem mudar o nome: o escopo é do pedido, não do resultado.
	scope := ids
	if kind == KindEvents || kind == KindMacro {
		scope = s.normalizeIDs(scope)
	}

	// Universo da EXTRAÇÃO. Events é o único kind com régua de versão: os
	// grupos "sc" (Eternal Calm) e "lm" (Last Mission) moram na árvore do
	// jogo-pai e cada aba serve só o seu (mesma lista de ListEntries, que
	// a árvore exibe). Sem esse filtro o export da aba Eternal Calm
	// varria o bucket inteiro do datastore e gravava TODOS os eventos do
	// FFX — e o export do FFX levava os arquivos do Eternal Calm.
	extract := scope
	if kind == KindEvents {
		if len(scope) == 0 {
			// Bulk: carrega TODOS os eventos, não só os já visitados.
			if err := s.ensureAllEventsLoaded(version); err != nil {
				return nil, err
			}
			extract = filterEventIDsForVersion(event.GetAllEventIDs(version), version)
		} else {
			extract = filterEventIDsForVersion(scope, version)
		}
		if len(extract) == 0 {
			// Universo vazio NÃO pode significar "tudo": ids vazio é o
			// contrato de bulk dos builders e desandaria no balde do
			// jogo-pai. Sem eventos da versão, o export não tem alvo.
			return nil, fmt.Errorf("nenhum evento de %s na árvore desta versão", version)
		}
	}
	r.Begin("", 0) // total indeterminado: os builders fazem SetTotal
	defer r.End()

	c, err := s.exportCollection(kind, version, helpExportIDs(kind, extract), r)
	if err != nil {
		return nil, err
	}
	c = filterExportRows(c)

	// Escreve pelo formato pedido — caminhos canônicos por escopo (só a
	// extensão muda), regra do dedup por arquivo preservada. O formatter
	// vem do registry e a gravação é a camada de arquivo padronizada
	// (formatters.NewWriter): o switch abaixo é SÓ a política de caminho
	// por kind/escopo — a mesma para os quatro formatos.
	f, ok := formatters.Get(format)
	if !ok {
		return nil, fmt.Errorf("formatter %s não registrado", format)
	}
	write := func(col dto.Collection, jsonPath string) (string, error) {
		return formatters.NewWriter(f, col, formatters.SwapExt(f, jsonPath), langs).Write()
	}

	switch kind {
	case KindEvents:
		path, perr := eventsExportPath(version, scope, prefix)
		if perr != nil {
			return nil, perr
		}
		p, werr := write(c, path)
		if werr != nil {
			uilog.File("Export", p, uilog.StatusFailed, werr.Error())
		} else {
			uilog.File("Export", p, uilog.StatusOk, fmt.Sprintf("exportado como %s", format))
		}
		return []string{p}, werr
	case KindMacro:
		path, perr := macroExportPath(version, scope, prefix)
		if perr != nil {
			return nil, perr
		}
		p, werr := write(c, path)
		if werr != nil {
			uilog.File("Export", p, uilog.StatusFailed, werr.Error())
		} else {
			uilog.File("Export", p, uilog.StatusOk, fmt.Sprintf("exportado como %s", format))
		}
		return []string{p}, werr
	case KindHelp:
		path, perr := jsonfmt.HelpJSONPath(version, prefix)
		if perr != nil {
			return nil, perr
		}
		p, werr := write(c, path)
		if werr != nil {
			uilog.File("Export", p, uilog.StatusFailed, werr.Error())
		} else {
			uilog.File("Export", p, uilog.StatusOk, fmt.Sprintf("exportado como %s", format))
		}
		return []string{p}, werr
	case KindObjects, KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		if len(ids) == 0 {
			// Export completo: UM arquivo por kind (bulk, dedup global).
			var path string
			var perr error
			if kind == KindObjects {
				path, perr = objectsBulkPath(version, prefix)
			} else {
				path, perr = tableBulkPath(kind, version, prefix)
			}
			if perr != nil {
				return nil, perr
			}
			p, werr := write(c, path)
			if werr != nil {
				uilog.File("Export", p, uilog.StatusFailed, werr.Error())
			} else {
				uilog.File("Export", p, uilog.StatusOk, fmt.Sprintf("exportado como %s", format))
			}
			return []string{p}, werr
		}
		fallthrough
	case KindLockit:
		// Um arquivo por entrada (mesmo nome do JSON por entrada; dedup
		// intra-arquivo = self-contained).
		for _, key := range c.SortedKeys() {
			jp, perr := jsonfmt.ObjectsJSONPath(key, version)
			if perr != nil {
				return nil, perr
			}
			p, werr := write(dto.Collection{key: c[key]}, jp)
			if werr != nil {
				uilog.File("Export", p, uilog.StatusFailed, werr.Error())
				return nil, werr
			}
			uilog.File("Export", p, uilog.StatusOk, fmt.Sprintf("exportado como %s", format))
			paths = append(paths, p)
		}
		return paths, nil
	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}
