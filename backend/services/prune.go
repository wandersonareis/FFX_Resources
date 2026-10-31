package services

import (
	"fmt"
	"sort"
	"strings"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/uilog"
)

// PrunedRowView é uma linha que o save vai excluir por não existir no
// original — o mesmo formato vai para o modal do frontend e para o log.
type PrunedRowView struct {
	// File é a chave canônica da entrada (ex.: ffx2/menu/macrodic.dcp).
	File string `json:"file"`
	// Block é o bloco (chunk) quando o formato tem; nil = só índice.
	Block *int `json:"block,omitempty"`
	// Index é o índice da linha dentro do arquivo/bloco.
	Index int `json:"index"`
	// Name é o campo da linha ("name", "name_simplified"…).
	Name string `json:"name"`
	// Text é o texto COMPLETO da linha na localização padrão.
	Text string `json:"text,omitempty"`
	// Langs são os idiomas cujo texto será removido (vazio = linha inteira).
	Langs []string `json:"langs,omitempty"`
	// Version/Kind identificam a origem.
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// pruneLogThreshold: até esse número de linhas o log mostra UMA linha por
// exclusão (com o texto inteiro); acima, só o resumo — o arquivo de
// diagnóstico guarda sempre a lista completa.
const pruneLogThreshold = 5

// pruneViews monta as views do relatório de poda de UMA entrada.
func pruneViews(kind string, version common.GameVersion, entry dto.FileEntry, rows []builders.PrunedRow) []PrunedRowView {
	file := entry.Metadata.Key
	if file == "" {
		file = entry.Metadata.ID
	}
	var block *int
	if n, ok := dto.ChunkIndexFromID(entry.Metadata.ID); ok {
		block = &n
	}
	views := make([]PrunedRowView, 0, len(rows))
	for _, r := range rows {
		views = append(views, PrunedRowView{
			File:    file,
			Block:   block,
			Index:   r.Index,
			Name:    r.Name,
			Text:    r.SampleText(common.DefaultLocalization),
			Langs:   r.Langs,
			Version: version.String(),
			Kind:    kind,
		})
	}
	return views
}

// pruneLocation devolve "arquivo · bloco 07 · índice 109" (ou sem o bloco).
func pruneLocation(v PrunedRowView) string {
	if v.Block != nil {
		return fmt.Sprintf("%s · bloco %02d · índice %d", v.File, *v.Block, v.Index)
	}
	return fmt.Sprintf("%s · índice %d", v.File, v.Index)
}

// langLabel descreve o alcance da poda: idiomas afetados ou a linha inteira.
func langLabel(langs []string) string {
	if len(langs) == 0 {
		return "linha inteira"
	}
	return "idioma " + strings.Join(langs, ", ")
}

// logPrunedRows narra as exclusões: janela de logs (uma linha por exclusão
// até pruneLogThreshold; acima disso um resumo por arquivo) e arquivo de
// diagnóstico com a lista COMPLETA (texto inteiro).
func logPrunedRows(views []PrunedRowView) {
	if len(views) == 0 {
		return
	}

	// Arquivo/console: um evento por arquivo, sempre com tudo.
	byFile := make(map[string][]PrunedRowView)
	for _, v := range views {
		byFile[v.File] = append(byFile[v.File], v)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		group := byFile[f]
		details := make([]map[string]any, 0, len(group))
		for _, v := range group {
			row := map[string]any{"index": v.Index, "name": v.Name, "text": v.Text}
			if v.Block != nil {
				row["block"] = *v.Block
			}
			if len(v.Langs) > 0 {
				row["langs"] = v.Langs
			}
			details = append(details, row)
		}
		loggingService.DiagWarn(
			fmt.Sprintf("save-poda/%s", f),
			fmt.Sprintf("poda no save: %d linha(s) sem par no original removida(s) de %s",
				len(group), f),
			map[string]any{
				"kind":           group[0].Kind,
				"version":        group[0].Version,
				"rows_excluidas": details,
			},
		)
	}

	// Janela de logs: completo só até o limite; acima, resumo por arquivo.
	if len(views) <= pruneLogThreshold {
		for _, v := range views {
			uilog.Warn("Save", "%s · %s · \"%s\" (%s) — sem par no original (excluída)",
				pruneLocation(v), v.Name, v.Text, langLabel(v.Langs))
		}
		return
	}
	for _, f := range files {
		group := byFile[f]
		blocks := make(map[int]bool)
		for _, v := range group {
			if v.Block != nil {
				blocks[*v.Block] = true
			}
		}
		if len(blocks) > 0 {
			nums := make([]int, 0, len(blocks))
			for b := range blocks {
				nums = append(nums, b)
			}
			sort.Ints(nums)
			parts := make([]string, 0, len(nums))
			for _, b := range nums {
				parts = append(parts, fmt.Sprintf("%02d", b))
			}
			uilog.Warn("Save", "%s · blocos %s — %d linha(s) sem par no original excluída(s) (detalhes no log de diagnóstico)",
				f, strings.Join(parts, ", "), len(group))
			continue
		}
		uilog.Warn("Save", "%s — %d linha(s) sem par no original excluída(s) (detalhes no log de diagnóstico)",
			f, len(group))
	}
}

// previewMacroPrune devolve as linhas que o próximo save do dicionário
// removeria (fonte preferida vs original de data/), sem gravar nada.
func (s *MetadataService) previewMacroPrune(version common.GameVersion) ([]PrunedRowView, error) {
	full, err := builders.BuildMacroDTO(version, nil)
	if err != nil {
		return nil, err
	}
	original, err := builders.BuildMacroDTOFromSource(version, originalSource, nil)
	if err != nil {
		return nil, err
	}
	_, report, err := builders.PruneNotInOriginal(full, original)
	if err != nil {
		return nil, err
	}
	views := make([]PrunedRowView, 0)
	for id, rows := range report {
		views = append(views, pruneViews(KindMacro, version, full[id], rows)...)
	}
	sort.Slice(views, func(a, b int) bool {
		if views[a].File != views[b].File {
			return views[a].File < views[b].File
		}
		if views[a].Index != views[b].Index {
			return views[a].Index < views[b].Index
		}
		return views[a].Name < views[b].Name
	})
	return views, nil
}

// PreviewSavePrune devolve, para o frontend, as linhas que o save do kind
// excluiria por não existirem no original. Hoje só o dicionário (macro)
// reconstrói o arquivo inteiro no save; nos demais kinds o payload é podado
// só nas entradas sujas e o frontend conta as órfãs que conhece.
func (s *MetadataService) PreviewSavePrune(kind string, version common.GameVersion) ([]PrunedRowView, error) {
	if strings.EqualFold(strings.TrimSpace(kind), KindMacro) {
		return s.previewMacroPrune(version)
	}
	return nil, nil
}
