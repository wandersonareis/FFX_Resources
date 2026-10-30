// Package dto define o contrato neutro de texto entre os builders
// (binário/store → DTO) e os formatters (DTO → bytes/arquivo).
//
// Os formatters recebem apenas DTO pronto e devolvem DTO;
// quem monta o DTO é o intermediário backend/builders e quem
// aplica o DTO de volta no binário é o applier do domínio.
// Nenhum formatter importa pacotes de domínio concretos.
package dto

import (
	"fmt"
	"sort"
)

// TextRow é uma frase com seus textos por idioma e o PONTEIRO de dupe
// (xxHash64 hex 16 chars) por idioma. O hash NUNCA valida texto: é o
// endereço de conteúdo usado para agrupar repetições.
//
// Origem do ponteiro (dois domínios, nunca misturados no mesmo lote):
//   - RAW (GetCollection → export/import/apply): hash do TEXTO do arquivo
//     (mods-first) — os bytes dos artefatos dependem dele;
//   - DISPLAY (GetEntry): hash do ORIGINAL de data/, criado uma única vez
//     na leitura pristine; coluna Traduzido (Text) reusa o MESMO ponteiro.
//
// Na view, hash(Text) == hash(Original) ⇔ a célula AINDA NÃO FOI traduzida
// (o binário de mods pode ter tradução parcial/total; células intactas
// mantêm o dupe possível). Hash vazio/ausente significa texto vazio.
//
// Index posiciona a row na reconstrução do binário (events: índice da
// string; objects: índice do objeto; macro: índice da string no chunk;
// lockit: posição no grupo do tipo, game 0..G-1 e utf8 G..G+U-1).
// Name qualifica a row quando um índice carrega vários campos
// (objects: chave do segmento — "name", "desc"/"help", "ability1", ...; macro:
// "name"/"name_simplified"; lockit: codificação da linha — "game" ou
// "utf8"). Vazio para events e omitido no JSON.
//
// Original é EXCLUSIVO de exibição (GetEntry): o texto do binário em
// data/, que é a fonte da verdade. Text continua sendo o estado atual
// (mods-first = último save). O campo nunca entra em apply/export — o
// payload de apply leva só index/name/hash/text.
//
// Source e Target são a projeção de EXPORT para ferramentas CAT: `source`
// é o texto do idioma fonte (us) vindo de data/ (ou do .vbf) — referência
// read-only; `target` é o texto do MESMO segmento em mods/ (a tradução, que
// vive no slot us). O `target` é o ÚNICO texto que o import aplica no
// binário. Nenhum dos dois existe fora do export/import de artefato: display
// e apply continuam usando só Text/Hash.
//
// A tabela é a UNIÃO das rows dos dois lados; os dois flags dizem em qual
// lado a row falta e é ele que o frontend pinta em vermelho:
//
//	MissingInOriginal  → só existe na tradução (a coluna Original é vazia);
//	MissingInTranslated → só existe no original (a coluna Traduzido é vazia,
//	                       Text fica nil — o texto nunca vaza de um lado
//	                       para o outro).
//
// Original e flags Missing* são exclusivos de exibição e nunca entram em
// apply/export. Divergent é o inverso: apply-only, ignorado em display/export.
type TextRow struct {
	Index int `json:"index"`
	// Character/Kind são o dono de cada nome de w_name.bin, resolvido pelos
	// model_ids do próprio chunk (schema/ffx/wname.go): personagem dono do
	// nome e tipo do equipamento ("weapon"/"shield"). São APENAS referência
	// de exibição — o model_id não vira row nem campo editável; a identidade
	// da row é Index+Name e o import resolve a posição pelos model_ids do
	// registro. Nada além de text/hash é editável.
	Character string            `json:"character,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Name      string            `json:"name,omitempty"`
	Hash      map[string]string `json:"hash,omitempty"`
	Text      map[string]string `json:"text"`
	Original  map[string]string `json:"original,omitempty"`
	// Source é o texto do idioma fonte (us) vindo de data/ ou do .vbf —
	// referência read-only para ferramentas CAT.
	Source string `json:"source,omitempty"`
	// Target é o texto do mesmo segmento em mods/ (a tradução) — o texto
	// que o import aplica no slot us do binário.
	Target string `json:"target,omitempty"`
	// MissingInOriginal/MissingInTranslated marcam a row órfã de um dos
	// lados da comparação (união da tabela).
	MissingInOriginal   bool `json:"missingInOriginal,omitempty"`
	MissingInTranslated bool `json:"missingInTranslated,omitempty"`
	// Divergent é APPLY-ONLY: marca a edição desta row como divergência —
	// grava só NELA, sem expandir às cópias iguais (self-target) e sem ser
	// arrastada pela edição da def no mesmo lote. É o caminho in-app do
	// que o round-trip externo já permitia: conteúdo válido no lugar do
	// $hash numa cópia. O editor de traduções o seta via checkbox
	// "salvar nesta cópia"; reverter a divergência (escrever o original de
	// volta) também o usa. NUNCA entra em export/display: o formatter
	// reconstrói a row campo a campo e o dedupe de display não o lê.
	Divergent bool `json:"divergent,omitempty"`
}

// RefLink é a anotação de EXIBIÇÃO de uma row de referência dedupada
// (Text[us] == "$hash"): o texto atual da def, o pristine dela e onde ela
// vive (arquivo + Index/Name da row). É o que alimenta o texto linkado na
// UI (cor própria), o tooltip "repetição de" e o editor aberto na def
// através do link. Como Original, é exclusivo de exibição e nunca entra em
// apply/export (payload de apply leva só index/name/hash/text).
type RefLink struct {
	Text     string `json:"text"`
	Original string `json:"original,omitempty"`
	// Originals é o pristine da def em TODOS os idiomas (lang → texto). O
	// Original acima é só 'us' (tooltip); o editor aberto pela ref precisa
	// do mapa inteiro para exibir a coluna de idiomas extras — sem ele, a
	// ref apontando para uma def em OUTRO arquivo mostraria os extras vazios.
	Originals   map[string]string `json:"originals,omitempty"`
	SourceID    string            `json:"sourceId,omitempty"`
	SourceIndex int               `json:"sourceIndex"`
	SourceName  string            `json:"sourceName,omitempty"`
}

// RowKey devolve a chave estável da row (index:name) — a mesma que o
// frontend usa para rascunho e que o map Refs usa para achar a anotação.
func RowKey(r TextRow) string {
	return fmt.Sprintf("%d:%s", r.Index, r.Name)
}

// FileEntry é um arquivo sem extensão como chave da Collection:
// metadata (mesmos campos dos geradores legados) + rows (ex-strings do JSON).
// Refs é a anotação de EXIBIÇÃO das rows de ref dedupada (rowKey → def);
// só existe na entrega GetEntry/GetVbfTextEntry — o payload de apply
// nunca a carrega.
type FileEntry struct {
	Metadata Metadata           `json:"metadata"`
	Rows     []TextRow          `json:"rows"`
	Refs     map[string]RefLink `json:"refs,omitempty"`
}

// Collection é o documento textual completo: chave = nome do arquivo
// sem extensão (events: eventID; objects: basename; macro: chunk_N).
type Collection map[string]FileEntry

// SortedKeys devolve as chaves ordenadas para saída determinística.
func (c Collection) SortedKeys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SortRows ordena as rows por Index de forma estável, necessário para a
// reconstrução posicional do binário.
func SortRows(rows []TextRow) {
	sort.SliceStable(rows, func(a, b int) bool {
		return rows[a].Index < rows[b].Index
	})
}
