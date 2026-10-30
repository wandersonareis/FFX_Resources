// Dedup de textos idênticos (refs "$hash"). O hash é PONTEIRO de dupe —
// nunca validador de texto: na entrega display ele aponta para o ORIGINAL
// de data/ (imutável, uma vez na leitura da árvore); nos fluxos RAW é o
// hash do texto do arquivo. O store nunca guarda dedup — sempre uma cópia
// aplicada na borda (entrega ou apply); a serialização dos artefatos tem
// a sua própria regra nos formatters.
package builders

import (
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
)

// DedupDTO é o dedup RAW (texto atual contra o texto da def): usado só
// onde não há original para comparar (coleções RAW, testes). O colapso
// exige que o texto atual da twin seja IGUAL ao da def — dupe sobre texto
// traduzido (diferente da def) é proibido por decisão: dedup economiza a
// carga do tradutor, e uma tradução divergente é carga real.
//
// Para a entrega display (GetEntry) use o par HashOrder + DedupDisplayDTO:
// o estado "traduzido" é avaliado contra Original (data/) e a 1ª ocorrência
// é resolvida na ordem da versão inteira, sem exigir a def no Collection.
func DedupDTO(c dto.Collection) dto.Collection {
	defs := make(map[string]string) // ponteiro bare -> texto atual da def
	out := make(dto.Collection, len(c))
	for _, k := range c.SortedKeys() {
		entry := c[k]
		rows := make([]dto.TextRow, len(entry.Rows))
		copy(rows, entry.Rows)
		dto.SortRows(rows)
		for i := range rows {
			r := &rows[i]
			if r.Divergent {
				// Divergência é conteúdo próprio: não cria/consome a def
				// canônica do hash e permanece literal no artefato.
				continue
			}
			t := r.Text[common.DefaultLocalization]
			h := r.Hash[common.DefaultLocalization]
			if t == "" || h == "" || !hash.IsDedupEligible(t) {
				continue
			}
			if _, ok := defs[h]; !ok {
				defs[h] = t
				continue
			}
			// Dupe: só quando o texto atual é IGUAL ao da def (tradução
			// divergente sob o mesmo original é conteúdo próprio).
			if defs[h] != t {
				continue
			}
			text := make(map[string]string, len(r.Text))
			for lk, lv := range r.Text {
				text[lk] = lv
			}
			text[common.DefaultLocalization] = hash.Prefix(h)
			r.Text = text
		}
		entry.Rows = rows
		out[k] = entry
	}
	return out
}

// ResolveDedupRefs devolve uma cópia da Collection com as refs "$hash"
// expandidas: a tabela hash → texto é construída das PRÓPRIAS defs (rows
// literais) do collection — o mesmo mecanismo da 2ª passada do unmarshal
// JSON/strings. Ref sem def no collection permanece como está (identidade:
// o texto atual vence). A entrada não é mutada.
//
// É o passo de aplicação do lote vindo do frontend (view dedupado) nos
// formatos cujo apply não expande refs por conta própria (macro: o DTO
// mesclado vai direto para o rebuild). Para o APPLY prefira
// ResolveApplyRefs, que também descarta a ref órfã.
func ResolveDedupRefs(c dto.Collection) dto.Collection {
	defs := make(map[string]string)
	for _, k := range c.SortedKeys() {
		for _, row := range c[k].Rows {
			if row.Divergent {
				continue // divergência não substitui a definição canônica
			}
			t := row.Text[common.DefaultLocalization]
			if t == "" {
				continue
			}
			if _, isRef := refBare(row, t); isRef {
				continue
			}
			if h := row.Hash[common.DefaultLocalization]; h != "" {
				defs[h] = t
			}
		}
	}
	out := make(dto.Collection, len(c))
	for _, k := range c.SortedKeys() {
		entry := c[k]
		rows := make([]dto.TextRow, len(entry.Rows))
		copy(rows, entry.Rows)
		for i := range rows {
			r := &rows[i]
			bare, isRef := refBare(*r, r.Text[common.DefaultLocalization])
			if !isRef {
				continue
			}
			resolved, ok := defs[bare]
			if !ok {
				continue // ref órfã: identidade (mantém o texto atual)
			}
			text := make(map[string]string, len(r.Text))
			for lk, lv := range r.Text {
				text[lk] = lv
			}
			text[common.DefaultLocalization] = resolved
			r.Text = text
		}
		entry.Rows = rows
		out[k] = entry
	}
	return out
}

// refBare devolve o hash por trás de uma ref "$hash" da própria row.
// Espelha a segurança do unmarshal do JSON/strings: prefixo `$` só conta
// se casar com o hash PRÓPRIO da row (literal que comece com `$` passa
// reto — o texto vence o hash).
func refBare(row dto.TextRow, text string) (string, bool) {
	bare, ok := hash.Strip(text)
	if !ok || bare != row.Hash[common.DefaultLocalization] {
		return "", false
	}
	return bare, true
}

// ResolveApplyRefs devolve uma cópia da Collection pronta para o APPLY: as
// refs "$hash" do lote (DTO de DISPLAY do Save) são expandidas contra as
// defs do PRÓPRIO lote e as refs SEM def (órfãs) saem do lote — a row não
// pede alteração nenhuma, então o binário mantém o texto atual.
//
// É a régua do collectTableChanges aplicada a quem grava a row direto
// (objects/lockit): a ref NUNCA pode virar TEXTO no binário. Quando isso
// acontece, a carga seguinte lê "$<hash>" como literal, o dedup de display
// não consegue colapsar (texto ≠ original) e a view mostra o hash cru —
// exatamente o que corrompeu o lm_accesary (57 rows) e o command (2 rows).
//
// Def divergente não é referência canônica (mesma regra do
// ResolveDedupRefs). Entrada sem nada a aplicar fica fora do resultado.
func ResolveApplyRefs(c dto.Collection) dto.Collection {
	defs := make(map[string]string)
	for _, k := range c.SortedKeys() {
		for _, row := range c[k].Rows {
			if row.Divergent {
				continue
			}
			t := row.Text[common.DefaultLocalization]
			if t == "" {
				continue
			}
			if _, isRef := refBare(row, t); isRef {
				continue
			}
			if h := row.Hash[common.DefaultLocalization]; h != "" {
				defs[h] = t
			}
		}
	}

	out := make(dto.Collection, len(c))
	for _, k := range c.SortedKeys() {
		entry := c[k]
		rows := make([]dto.TextRow, 0, len(entry.Rows))
		for _, r := range entry.Rows {
			bare, isRef := refBare(r, r.Text[common.DefaultLocalization])
			if !isRef {
				rows = append(rows, r)
				continue
			}
			resolved, ok := defs[bare]
			if !ok {
				continue // órfã: nada a aplicar nesta row
			}
			text := make(map[string]string, len(r.Text))
			for lk, lv := range r.Text {
				text[lk] = lv
			}
			text[common.DefaultLocalization] = resolved
			r.Text = text
			rows = append(rows, r)
		}
		if len(rows) == 0 {
			continue // entrada sem nada a aplicar não entra no lote
		}
		entry.Rows = rows
		out[k] = entry
	}
	return out
}

// HashOrder é a régua "1ª ocorrência = def" do dedup de display, versão
// inteira: para cada ponteiro Hash[us] de uma Collection RAW guarda a
// posição da PRIMEIRA ocorrência (ordem SortedKeys → SortRows, contador
// global de rows) e o texto dela.
//
// É construída sobre o estado CRU (sem refs, hashes = ponteiro do texto
// do arquivo). Para rows NÃO traduzidas mods-ponteiros == ponteiro do
// original (mesmo texto ⇒ mesmo XXH64), então a ordem é válida também no
// domínio display. Traduções registram o hash DELAS (não o do original):
// não poluem grupos — colisão exigiria duas frases com o mesmo conteúdo,
// e o ponteiro é endereçado por conteúdo.
type HashOrder struct {
	first map[string]defRef // ponteiro bare -> def (1ª ocorrência)
	base  map[string]int    // key -> nº de rows antes dela na ordem canônica
	owner []string          // id da entrada dona de cada posição global
	// pointerAt marca o ponteiro registrado em cada posição ("" = row sem
	// texto/hash): o que Replace usa para liberar defs que saíram da
	// posição.
	pointerAt []string
}

// defRef é a def registrada de um ponteiro: a posição na ordem, o texto
// atual dela, o pristine (Original — para o editor aberto através do link)
// e a identidade da row (Index/Name — o que o payload/rascunho endereça).
type defRef struct {
	pos   int
	text  string
	orig  string
	index int
	name  string
	// onlyInTranslation marca a row que existe só na tradução (sem
	// contraparte em data/): sem Original, mas já é tradução.
	onlyInTranslation bool
}

// NewHashOrder percorre a Collection RAW em ordem canônica (SortedKeys →
// SortRows) e registra a 1ª ocorrência de cada ponteiro.
func NewHashOrder(c dto.Collection) *HashOrder {
	ho := &HashOrder{
		first: map[string]defRef{},
		base:  map[string]int{},
	}
	for _, k := range c.SortedKeys() {
		ho.Extend(k, entryRows(c[k]))
	}
	return ho
}

// entryRows devolve cópia ordenada das rows (a ordem canônica das posições
// é a estável por Index — a mesma que a view serve).
func entryRows(entry dto.FileEntry) []dto.TextRow {
	rows := make([]dto.TextRow, len(entry.Rows))
	copy(rows, entry.Rows)
	dto.SortRows(rows)
	return rows
}

// Extend registra uma entrada nova no FIM da ordem e devolve o offset (base)
// dela. É a régua do store de sessão do .vbf: inserção append-only — a 1ª
// ocorrência de cada ponteiro é a def, e o que já foi servido não muda.
func (ho *HashOrder) Extend(id string, rows []dto.TextRow) int {
	base := len(ho.owner)
	ho.base[id] = base
	sorted := make([]dto.TextRow, len(rows))
	copy(sorted, rows)
	dto.SortRows(sorted)
	for _, r := range sorted {
		t := r.Text[common.DefaultLocalization]
		h := r.Hash[common.DefaultLocalization]
		pos := len(ho.owner)
		ho.owner = append(ho.owner, id)
		ho.pointerAt = append(ho.pointerAt, "")
		if t != "" && h != "" {
			ho.pointerAt[pos] = h
			if _, seen := ho.first[h]; !seen {
				ho.first[h] = defRef{
					pos:               pos,
					text:              t,
					orig:              r.Original[common.DefaultLocalization],
					index:             r.Index,
					name:              r.Name,
					onlyInTranslation: r.MissingInOriginal,
				}
			}
		}
	}
	return base
}

// Replace reposiciona os registros de uma entrada que JÁ está na ordem
// (mesma quantidade de rows — a estrutura do binário não muda) para o
// estado novo dela. No domínio DISPLAY o ponteiro é do ORIGINAL e não
// muda quando a def é traduzida — o que muda é o texto: se a posição é a
// 1ª ocorrência, o texto da def é atualizado (o link das cópias passa a
// mostrar a tradução). No domínio RAW (row sem Original) o ponteiro É o
// hash do texto atual: célula traduzida sob outro ponteiro perde a def
// antiga e as cópias deixam de colapsar — conservador e previsível.
func (ho *HashOrder) Replace(id string, rows []dto.TextRow) {
	base, ok := ho.base[id]
	if !ok {
		ho.Extend(id, rows)
		return
	}
	sorted := make([]dto.TextRow, len(rows))
	copy(sorted, rows)
	dto.SortRows(sorted)
	for i := range sorted {
		pos := base + i
		if pos >= len(ho.owner) {
			break // encolheu além do registrado: defesa estrutural
		}
		old := ho.pointerAt[pos]
		t := sorted[i].Text[common.DefaultLocalization]
		h := sorted[i].Hash[common.DefaultLocalization]
		if t == "" {
			h = ""
		}
		if old != "" && old != h && ho.first[old].pos == pos {
			delete(ho.first, old)
		}
		ho.owner[pos] = id
		ho.pointerAt[pos] = ""
		if t != "" && h != "" {
			ho.pointerAt[pos] = h
			if _, seen := ho.first[h]; !seen {
				ho.first[h] = defRef{
					pos:               pos,
					text:              t,
					orig:              sorted[i].Original[common.DefaultLocalization],
					index:             sorted[i].Index,
					name:              sorted[i].Name,
					onlyInTranslation: sorted[i].MissingInOriginal,
				}
			} else if ho.first[h].pos == pos {
				// Mesma def, texto novo (tradução aplicada): o link das
				// cópias passa a mostrar o estado atual.
				ref := ho.first[h]
				ref.text = t
				ref.orig = sorted[i].Original[common.DefaultLocalization]
				ref.index = sorted[i].Index
				ref.name = sorted[i].Name
				ref.onlyInTranslation = sorted[i].MissingInOriginal
				ho.first[h] = ref
			}
		}
	}
}

// Resolve localiza a def de um ponteiro: (id da entrada, Index/Name da
// row dela). É o que a anotação de ref linkada usa para indicar onde vive
// a def e resgatar o texto atual.
func (ho *HashOrder) Resolve(h string) (id string, index int, name string, ok bool) {
	ref, found := ho.first[h]
	if !found || ref.pos >= len(ho.owner) {
		return "", 0, "", false
	}
	return ho.owner[ref.pos], ref.index, ref.name, true
}

// Offset devolve o nº de rows que antecedem a entrada `key` na ordem
// canônica da coleção que originou o HashOrder.
func (ho *HashOrder) Offset(key string) (int, bool) {
	off, ok := ho.base[key]
	return off, ok
}

// RawFirst devolve a posição global da 1ª ocorrência do ponteiro, se existe.
func (ho *HashOrder) RawFirst(h string) (int, bool) {
	ref, ok := ho.first[h]
	return ref.pos, ok
}

// defText devolve o texto da 1ª ocorrência do ponteiro (o estado no build
// do RAW), para o modo RAW comparar o texto atual da twin contra a def.
func (ho *HashOrder) defText(h string) (string, bool) {
	ref, ok := ho.first[h]
	return ref.text, ok
}

// DefPending informa se a def do ponteiro AINDA está pendente de tradução
// (texto atual == original de data/). É a metade que falta da régua de
// dedupe: original → tradução apenas enquanto a def não foi traduzida —
// uma def traduzida nunca é alvo de colapso, mesmo com a twin no mesmo
// ponteiro (mesmo original). Ref só existe entre células não traduzidas.
//
// Sem Original na def (row só na tradução, ou entrada sem binário em
// mods/) o estado não é aferível pelo texto: sem mods/ a def está por
// construção pendente, e row só na tradução é tradução — o teste é pelo
// Original da row registrada na ordem.
func (ho *HashOrder) DefPending(h string) bool {
	ref, ok := ho.first[h]
	if !ok {
		return false
	}
	if ref.orig != "" {
		return ref.text == ref.orig
	}
	// Sem Original: ou a entrada não tem binário em mods/ (nunca saiu de
	// data/ — pendente), ou a row existe só na tradução (já traduzida).
	return !ref.onlyInTranslation
}

// collapseInto escreve a ref "$h" no texto 'us' da row (cópia do map).
func collapseInto(r *dto.TextRow, h string) {
	text := make(map[string]string, len(r.Text))
	for lk, lv := range r.Text {
		text[lk] = lv
	}
	text[common.DefaultLocalization] = hash.Prefix(h)
	r.Text = text
}

// DedupDisplayDTO devolve uma cópia da entry (já merged com o Original de
// data/) com o dedup de DISPLAY aplicado: colapsa em ref "$ponteiro" a
// repetição de um ORIGINAL ainda NÃO traduzida (Text[us] == Original[us])
// que não seja a 1ª ocorrência do ponteiro na ordem global (base+i).
//
// Regras:
//   - Célula traduzida (texto difere do original) → nunca colapsa: dupe
//     sobre texto já traduzido é proibido por decisão — é carga real de
//     revisão.
//   - Célula sem Original (divergência/sem contraparte) → o estado não é
//     aferível contra data/ e o colapso volta a ser por texto, igual ao da
//     1ª ocorrência (comportamento pré-original).
//   - Def traduzida → nunca atrai a twin: dedupe é só de original para
//     original, e uma def pendente é pré-requisito de ref (DefPending).
//     Com a def traduzida, a gêmea ainda em inglês é exibida em inglês —
//     sem ref apontando para a tradução de outro binário.
//
// base é o nº de rows que precedem a entrada na ordem canônica da versão
// (HashOrder.Offset). A entrada não é mutada.
func DedupDisplayDTO(entry dto.FileEntry, base int, ho *HashOrder) dto.FileEntry {
	if ho == nil {
		return entry
	}
	rows := make([]dto.TextRow, len(entry.Rows))
	copy(rows, entry.Rows)
	dto.SortRows(rows)

	for i := range rows {
		r := &rows[i]
		origUS := r.Original[common.DefaultLocalization]
		t := r.Text[common.DefaultLocalization]
		h := r.Hash[common.DefaultLocalization]
		if t == "" || h == "" {
			continue
		}
		// Elegibilidade por domínio: com Original prova-se não traduzida
		// por conteúdo (texto == original, dentro da régua de runes do
		// ORIGINAL); sem Original o estado não é aferível — o colapso
		// volta a ser por texto, igual ao da 1ª ocorrência (comportamento
		// pré-original).
		elegivel := false
		if origUS != "" {
			elegivel = t == origUS && hash.IsDedupEligible(origUS)
		} else if def, ok := ho.defText(h); ok {
			elegivel = def == t && hash.IsDedupEligible(t)
		}
		// Dedupe é só de original para original: a def precisa estar
		// pendente (sem tradução). Def traduzida nunca atrai a twin,
		// mesmo com o mesmo ponteiro — tradução repetida fica repetida,
		// e o gêmeo ainda em inglês é exibido em inglês, sem ref.
		if elegivel && !ho.DefPending(h) {
			elegivel = false
		}
		if !elegivel {
			continue
		}
		first, ok := ho.RawFirst(h)
		if !ok || base+i <= first {
			// 1ª ocorrência do ponteiro (ou ponteiro desconhecido): é a def
			// da entrada — permanece literal.
			continue
		}
		collapseInto(r, h)
		// Anotação de link para a UI: o texto atual da def, o pristine dela
		// e onde ela vive (arquivo + Index/Name da row). Refs nunca colapsam
		// sem def registrada — o ponteiro está em first, então a def existe.
		if entry.Refs == nil {
			entry.Refs = make(map[string]dto.RefLink, 1)
		}
		ref := ho.first[h]
		entry.Refs[dto.RowKey(*r)] = dto.RefLink{
			Text:        ref.text,
			Original:    ref.orig,
			SourceID:    ho.owner[ref.pos],
			SourceIndex: ref.index,
			SourceName:  ref.name,
		}
	}
	entry.Rows = rows
	return entry
}
