// Package strings implementa o formato Strings (java-like) de exportação/
// importação dos textos de localização. É um formato texto-para-humanos
// que vive ao lado do JSON: os builders montam o mesmo DTO
// (backend/dto) e cada formatter o serializa à sua maneira.
//
// Gramática:
//
//	<entry-block> := <header> <line>+
//	<header>      := "/*key=" <key> " row_count=" <n> "*/"
//	<line>        := <keyfields> "║" "$" <hash16> " " "=" " " <value>
//	<keyfields>   := [<prefix> ":"]... <id> [":" <name>] ":" <index> ":" <lang>
//	<value>       := <literal> | "$" <hash16>          // def | ref
//
// A versão NÃO vai na linha: vive só no header /*key=*/, de onde
// ParseKey a recupera — a linha começa no arquivo (id) em diante, como
// a key dos demais formatos. Âncora semântica à direita: último campo =
// lang, penúltimo = index; campos NOVOS (ex.: path/region) crescem pela
// esquerda sem quebrar o parser — versão no primeiro campo é erro de
// formato, nunca campo tolerado.
// "║" liga a chave ao hash de identidade; "=" liga ao valor com split no
// primeiro ("=" dentro do texto passa intacto; a chave nunca tem espaço).
// def: literal cujo xxh64 == hash da chave (verificação grátis na leitura).
// ref: "$hash" resolvido pela tabela hash→texto do próprio arquivo.
// Name é omitido quando vazio (events/help); objects/macro emitem
// <id>:<name>:<index>:<lang>.
package strings

import (
	"sort"
	"strconv"
	stdstrings "strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters"
	"ffxresources/backend/formatters/hash"
)

// extensionStrings é a extensão produzida pelo formatter Strings.
const extensionStrings = ".strings"

// ExtensionStrings retorna a extensão produzida pelo formatter Strings.
func ExtensionStrings() string {
	return extensionStrings
}

// keyValueSep liga a chave ao hash de identidade (U+2566).
const keyValueSep = "║"

// Marshal organiza a Collection pronta no formato Strings (todos os idiomas).
// Recebe apenas DTO pronto; não toca no domínio nem muta a entrada.
func Marshal(c dto.Collection) ([]byte, error) {
	return MarshalLangs(c, nil)
}

// MarshalNoDedup serializa SEM o dedup de conteúdo: todo texto sai literal,
// inclusive os que casariam como repetição (não viram ref "$hash"). É a
// direção "dupe/resolve" do menu/CLI: expandir os hashes de conteúdo de
// volta ao texto completo. O campo de identidade (hash da row) continua
// presente, como no Marshal.
func MarshalNoDedup(c dto.Collection) ([]byte, error) {
	return marshalLangsOpt(c, nil, false)
}

// MarshalNoDedupLangs é MarshalNoDedup com o filtro de idiomas
// (nil/vazio = todos).
func MarshalNoDedupLangs(c dto.Collection, langs []string) ([]byte, error) {
	return marshalLangsOpt(c, langs, false)
}

// MarshalLangs organiza a Collection pronta no formato Strings contendo
// só os idiomas pedidos (nil/vazio = todos). Uma linha por (row, idioma)
// não-vazio, com chaves e rows ordenadas (saída determinística).
func MarshalLangs(c dto.Collection, langs []string) ([]byte, error) {
	return marshalLangsOpt(c, langs, true)
}

// marshalLangsOpt é o motor único do Marshal/MarshalNoDedup: dedup=false
// desliga só o colapso do TEXTO repetido em ref "$hash" — o hash de
// identidade da linha permanece em qualquer modo.
func marshalLangsOpt(c dto.Collection, langs []string, dedup bool) ([]byte, error) {
	filter := langSet(langs)
	var out stdstrings.Builder
	seen := make(map[string]string) // hashHex bare -> texto (só default lang)
	for _, k := range c.SortedKeys() {
		entry := c[k]
		out.WriteString("/*key=")
		out.WriteString(entry.Metadata.Key)
		out.WriteString(" row_count=")
		out.WriteString(strconv.Itoa(len(entry.Rows)))
		out.WriteString("*/\n")
		rows := make([]dto.TextRow, len(entry.Rows))
		copy(rows, entry.Rows)
		dto.SortRows(rows)
		id := lineID(entry.Metadata, k)
		for _, row := range rows {
			for _, lang := range rowLangs(row, filter) {
				text := row.Text[lang]
				if text == "" && !formatters.HasSourceTarget(row, lang) {
					continue
				}
				h := row.Hash[lang]
				if h == "" {
					h = hash.Sum64Hex(text)
				}
				value := escapeValue(text)
				if dedup && lang == common.DefaultLocalization && !row.Divergent {
					if hash.IsDedupEligible(text) {
						if _, dup := seen[h]; dup {
							value = hash.Prefix(h)
						} else {
							seen[h] = text
						}
					}
				}
				out.WriteString(id)
				if row.Name != "" {
					out.WriteString(":")
					out.WriteString(row.Name)
				}
				// id:name:index:lang║$hash = <valor>[║source = <esc>][║target = <esc>]
				// source/target só existem na linha do idioma default (us): é
				// ali que vive a tradução; o escapeValue garante que o
				// separador ║ não aparece literal no texto.
				source, target := formatters.SourceTarget(row, lang)
				out.WriteString(":")
				out.WriteString(strconv.Itoa(row.Index))
				out.WriteString(":")
				out.WriteString(lang)
				out.WriteString(keyValueSep)
				out.WriteString(hash.Prefix(h))
				out.WriteString(" = ")
				out.WriteString(value)
				if source != "" {
					out.WriteString(keyValueSep)
					out.WriteString("source = ")
					out.WriteString(escapeValue(source))
				}
				if target != "" {
					out.WriteString(keyValueSep)
					out.WriteString("target = ")
					out.WriteString(escapeValue(target))
				}
				out.WriteString("\n")
			}
		}
	}
	return []byte(out.String()), nil
}

// lineID resolve o id da linha a partir da metadata (fallback: chave da
// Collection). A versão NÃO entra na linha — vive só no header /*key=*/,
// de onde ParseKey a recupera.
func lineID(m dto.Metadata, mapKey string) string {
	if id := m.ID; id != "" {
		return id
	}
	return mapKey
}

// langSet normaliza o filtro de idiomas (nil = todos).
func langSet(langs []string) map[string]bool {
	if len(langs) == 0 {
		return nil
	}
	set := make(map[string]bool, len(langs))
	for _, l := range langs {
		if l = stdstrings.TrimSpace(l); l != "" {
			set[l] = true
		}
	}
	return set
}

// rowLangs devolve os idiomas da row com texto, ordenados e filtrados.
// A linha de `us` entra também quando só há source/target: o target é o que
// o import aplica, mesmo sem texto no original.
func rowLangs(row dto.TextRow, filter map[string]bool) []string {
	langs := make([]string, 0, len(row.Text))
	for lang, text := range row.Text {
		if text == "" {
			continue
		}
		if filter != nil && !filter[lang] {
			continue
		}
		langs = append(langs, lang)
	}
	if formatters.HasSourceTarget(row, common.DefaultLocalization) && (filter == nil || filter[common.DefaultLocalization]) {
		hasUS := false
		for _, l := range langs {
			if l == common.DefaultLocalization {
				hasUS = true
				break
			}
		}
		if !hasUS {
			langs = append(langs, common.DefaultLocalization)
		}
	}
	sort.Strings(langs)
	return langs
}

// escapeValue escapa valor literal para uma linha: backslash, CR, LF e ║.
func escapeValue(s string) string {
	s = stdstrings.ReplaceAll(s, `\`, `\\`)
	s = stdstrings.ReplaceAll(s, "\r", `\r`)
	s = stdstrings.ReplaceAll(s, "\n", `\n`)
	s = stdstrings.ReplaceAll(s, keyValueSep, `\║`)
	return s
}

// unescapeValue desfaz escapeValue (leniente: sequência desconhecida
// mantém backslash + char).
func unescapeValue(s string) string {
	var out stdstrings.Builder
	out.Grow(len(s))
	esc := false
	for _, r := range s {
		if esc {
			switch r {
			case 'n':
				out.WriteRune('\n')
			case 'r':
				out.WriteRune('\r')
			case '\\':
				out.WriteRune('\\')
			case '║':
				out.WriteString(keyValueSep)
			default:
				out.WriteRune('\\')
				out.WriteRune(r)
			}
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		out.WriteRune(r)
	}
	if esc {
		out.WriteRune('\\')
	}
	return out.String()
}
