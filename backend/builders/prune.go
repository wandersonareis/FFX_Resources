package builders

import (
	"fmt"
	"sort"

	"ffxresources/backend/dto"
)

// PrunedRow é uma linha (ou um idioma de uma linha) removida no save por não
// existir no original. O texto vai COMPLETO para o log — é o que permite
// reconhecer uma tradução que estava numa linha com ponteiro errado antes de
// confirmar a exclusão.
type PrunedRow struct {
	Index int               `json:"index"`
	Name  string            `json:"name"`
	Text  map[string]string `json:"text,omitempty"`
	// Langs são os idiomas cujo texto foi removido (vazio = a linha inteira).
	Langs []string `json:"langs,omitempty"`
}

// TextOf devolve o texto da localização pedida (vazio quando não houver).
func (p PrunedRow) TextOf(loc string) string {
	if p.Text == nil {
		return ""
	}
	return p.Text[loc]
}

// sampleText devolve o texto de exibição do relatório: o da localização
// pedida quando existir, senão o primeiro idioma removido (ordem estável).
func (p PrunedRow) SampleText(loc string) string {
	if text := p.TextOf(loc); text != "" {
		return text
	}
	langs := make([]string, 0, len(p.Text))
	for lang := range p.Text {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		if text := p.Text[lang]; text != "" {
			return text
		}
	}
	return ""
}

// PruneNotInOriginal remove de `c` o que não existe no `original` da MESMA
// entrada (id), POR IDIOMA:
//
//   - linha (Index, Name) que o original não tem → sai inteira;
//   - linha que o original tem só em alguns idiomas → mantém a linha e
//     remove os textos dos idiomas que o original não tem (o caso do
//     macrodic traduzido com "simplified" só no idioma que não o tem);
//   - se sobrar linha sem nenhum texto, ela vira placeholder (o rebuild grava
//     entrada zero, preservando a posição).
//
// Guarda: as rows do original são intocáveis. Se uma entrada do payload não
// contiver algum (Index, Name) do original — ou ficar com menos rows que o
// original — a poda aborta com erro: o binário não suporta linhas a menos que
// o original (a tradução parcial completa com as linhas originais).
func PruneNotInOriginal(c, original dto.Collection) (dto.Collection, map[string][]PrunedRow, error) {
	out := make(dto.Collection, len(c))
	pruned := make(map[string][]PrunedRow)

	for id, entry := range c {
		orig, ok := original[id]
		if !ok {
			// Sem original (arquivo só em mods ou fora de data/): nada a
			// comparar — mantém a entrada como veio.
			out[id] = entry
			continue
		}

		originalRows := make(map[string]dto.TextRow, len(orig.Rows))
		for _, r := range orig.Rows {
			originalRows[rowIdentity(r)] = r
		}

		keep := make([]dto.TextRow, 0, len(entry.Rows))
		foundOriginal := make(map[string]bool, len(orig.Rows))
		var dropped []PrunedRow
		for _, r := range entry.Rows {
			key := rowIdentity(r)
			origRow, hasRow := originalRows[key]
			if !hasRow {
				dropped = append(dropped, PrunedRow{
					Index: r.Index,
					Name:  r.Name,
					Text:  r.Text,
					Langs: sortedLangs(r.Text),
				})
				continue
			}
			foundOriginal[key] = true

			// A linha existe no original: mantém, limpando os idiomas cujo
			// texto o original não tem. Sem limpeza, o ponteiro errado de um
			// idioma seria regravado no rebuild.
			kept := make(map[string]string, len(r.Text))
			lost := make(map[string]string)
			for lang, text := range r.Text {
				if text == "" {
					continue
				}
				if origRow.Text[lang] == "" {
					lost[lang] = text
					continue
				}
				kept[lang] = text
			}
			if len(lost) > 0 {
				dropped = append(dropped, PrunedRow{
					Index: r.Index,
					Name:  r.Name,
					Text:  lost,
					Langs: sortedLangs(lost),
				})
			}
			if len(kept) > 0 || len(r.Text) == 0 {
				r.Text = kept
			} else {
				// Tudo que a linha tinha era texto sem par: fica só a posição.
				r.Text = map[string]string{}
			}
			keep = append(keep, r)
		}

		if missing := len(originalRows) - len(foundOriginal); missing > 0 {
			return nil, nil, fmt.Errorf(
				"entrada %q: %d linha(s) do original não estão no rascunho — o binário não aceita menos linhas que o original (%d)",
				id, missing, len(orig.Rows),
			)
		}
		if len(keep) < len(orig.Rows) {
			return nil, nil, fmt.Errorf(
				"entrada %q: rascunho com %d linha(s) e o original tem %d — o binário não aceita menos linhas que o original",
				id, len(keep), len(orig.Rows),
			)
		}

		if len(dropped) > 0 {
			sort.Slice(dropped, func(a, b int) bool {
				if dropped[a].Index != dropped[b].Index {
					return dropped[a].Index < dropped[b].Index
				}
				return dropped[a].Name < dropped[b].Name
			})
			pruned[id] = dropped
		}
		entry.Rows = keep
		entry.Metadata = entry.Metadata.WithRowCount(len(keep))
		out[id] = entry
	}
	return out, pruned, nil
}

// rowIdentity é a chave de casamento do display: (Index, Name).
func rowIdentity(r dto.TextRow) string {
	return fmt.Sprintf("%d\x00%s", r.Index, r.Name)
}

// sortedLangs devolve os idiomas com texto não-vazio, em ordem estável.
func sortedLangs(text map[string]string) []string {
	langs := make([]string, 0, len(text))
	for lang, value := range text {
		if value == "" {
			continue
		}
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}
