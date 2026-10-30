package builders

import (
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
)

// ProjectSourceTarget é a projeção de EXPORT para ferramentas CAT: devolve
// uma CÓPIA de original (data/ ou .vbf) com Source/Target preenchidos.
//
//   - Text/Hash/Divergent/Character/Kind continuam os da ORIGEM (o texto do
//     jogo, imutável): `text` é a referência e o `hash` acompanha `text`
//     (o ponteiro do dedup continua válido);
//   - Source é o texto do idioma fonte (`us`) da origem — inglês, read-only;
//   - Target é o texto do MESMO segmento na árvore mods/ (a tradução; o
//     PT-BR vive no slot `us`), casado por (Index, Name). Vazio quando a
//     entrada/row não existe em mods/ (nada traduzido ainda).
//
// A entrada de original pode vir de cache (originalFor): as rows e os mapas
// são copiados antes de receber Source/Target — o cache nunca é mutado.
// translated nil = só preenche Source (export sem mods/).
func ProjectSourceTarget(original, translated dto.Collection) dto.Collection {
	out := make(dto.Collection, len(original))
	for id, entry := range original {
		targets := make(map[string]string)
		if tEntry, ok := translated[id]; ok {
			for _, r := range tEntry.Rows {
				if t := r.Text[common.DefaultLocalization]; t != "" {
					targets[dto.RowKey(r)] = t
				}
			}
		}
		rows := make([]dto.TextRow, len(entry.Rows))
		for i, r := range entry.Rows {
			nr := r
			nr.Source = r.Text[common.DefaultLocalization]
			nr.Target = targets[dto.RowKey(r)]
			rows[i] = nr
		}
		projected := entry
		projected.Rows = rows
		out[id] = projected
	}
	return out
}

// ApplyTargetAsText devolve uma CÓPIA da collection com o texto alvo
// (Target) no slot `us` de Text — a forma que os appliers (builders.Apply*)
// consomem. Row sem target fica intacta: nada a importar ali (aplicar o
// texto da origem sobre uma célula idêntica é no-op).
func ApplyTargetAsText(c dto.Collection) dto.Collection {
	out := make(dto.Collection, len(c))
	for id, entry := range c {
		rows := make([]dto.TextRow, len(entry.Rows))
		for i, r := range entry.Rows {
			if r.Target == "" {
				rows[i] = r
				continue
			}
			text := make(map[string]string, len(r.Text)+1)
			for k, v := range r.Text {
				text[k] = v
			}
			text[common.DefaultLocalization] = r.Target
			r.Text = text
			rows[i] = r
		}
		e := entry
		e.Rows = rows
		out[id] = e
	}
	return out
}
