// Package csv implementa o formato CSV (RFC 4180) para exportação/importação
// dos arquivos de localização. É um formato tabular amplamente suportado
// por planilhas (Excel, LibreOffice, Google Sheets).
package csv

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters"
	"ffxresources/backend/formatters/hash"
)

// extensionCSV é a extensão produzida pelo formatter CSV.
const extensionCSV = ".csv"

// ExtensionCSV retorna a extensão produzida pelo formatter CSV.
func ExtensionCSV() string {
	return extensionCSV
}

// CSVFormatter formata a Collection textual para CSV.
type CSVFormatter struct{}

// NewCSVFormatter constrói o formatter CSV.
func NewCSVFormatter() CSVFormatter {
	return CSVFormatter{}
}

// Extension retorna a extensão produzida por este formatter.
func (CSVFormatter) Extension() string {
	return extensionCSV
}

// csvHeader define as colunas do CSV. A coluna key é a metadata.key
// COMPLETA (version/pattern/arquivo) — a identidade da entrada no app: é
// dela que saem o kind, a versão e o casamento por basename, então um
// artefato com VÁRIAS entradas (events_all, objects_all) volta inteiro no
// import. id é o metadata.ID (ex.: chunk_07 do dicionário, que não vive
// no nome do arquivo).
//
// source/target são a projeção de export para CAT (ver dto.TextRow): source
// é a referência `us` de data//.vbf e target é a tradução de mods/ — o
// ÚNICO texto que o import aplica. As duas colunas só vêm preenchidas nas
// linhas do idioma `us`.
var csvHeader = []string{
	"key",
	"id",
	"name",
	"index",
	"lang",
	"hash",
	"text",
	"source",
	"target",
	"divergent",
}

// Índices das colunas (csvHeader).
const (
	colKey       = 0
	colID        = 1
	colName      = 2
	colIndex     = 3
	colLang      = 4
	colHash      = 5
	colText      = 6
	colSource    = 7
	colTarget    = 8
	colDivergent = 9
)

// Marshal organiza a Collection pronta no formato CSV (todos os idiomas).
// Recebe apenas DTO pronto; não toca no domínio nem muta a entrada.
func (CSVFormatter) Marshal(c dto.Collection) ([]byte, error) {
	return (CSVFormatter{}).MarshalLangs(c, nil)
}

// MarshalLangs organiza a Collection pronta no formato CSV contendo
// só os idiomas pedidos (nil/vazio = todos). Uma linha por (row, idioma)
// não-vazio, com chaves e rows ordenadas (saída determinística).
func (CSVFormatter) MarshalLangs(c dto.Collection, langs []string) ([]byte, error) {
	filter := langSet(langs)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Comma = ','
	writer.UseCRLF = false // LF padrão; planilhas leem ambos

	// Escreve header
	if err := writer.Write(csvHeader); err != nil {
		return nil, err
	}

	seen := make(map[string]string) // hashHex bare -> texto (só default lang)

	for _, k := range c.SortedKeys() {
		entry := c[k]
		rows := make([]dto.TextRow, len(entry.Rows))
		copy(rows, entry.Rows)
		dto.SortRows(rows)

		key, id := entryColumns(entry, k)

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

				// Dedup: no idioma default, textos repetidos viram ref "$hash"
				value := text
				if lang == common.DefaultLocalization && !row.Divergent && hash.IsDedupEligible(text) {
					if _, dup := seen[h]; dup {
						value = hash.Prefix(h)
					} else {
						seen[h] = text
					}
				}

				divergent := "false"
				if row.Divergent {
					divergent = "true"
				}

				source, target := formatters.SourceTarget(row, lang)
				record := []string{
					key,
					id,
					row.Name,
					fmt.Sprintf("%d", row.Index),
					lang,
					hash.Prefix(h),
					value,
					source,
					target,
					divergent,
				}
				if err := writer.Write(record); err != nil {
					return nil, err
				}
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// langSet normaliza o filtro de idiomas (nil = todos).
func langSet(langs []string) map[string]bool {
	if len(langs) == 0 {
		return nil
	}
	set := make(map[string]bool, len(langs))
	for _, l := range langs {
		if l = strings.TrimSpace(l); l != "" {
			set[l] = true
		}
	}
	return set
}

// rowLangs devolve os idiomas da row com texto, ordenados e filtrados.
// A linha de `us` entra também quando só há source/target (row sem texto no
// original mas com tradução em mods) — o target precisa chegar ao import.
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
	if formatters.HasSourceTarget(row, common.DefaultLocalization) {
		if filter == nil || filter[common.DefaultLocalization] {
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
	}
	sort.Strings(langs)
	return langs
}

// Unmarshal parseia o CSV de volta para a Collection (DTO),
// com rows ordenados por Index para reconstrução posicional do binário.
func (CSVFormatter) Unmarshal(data []byte) (dto.Collection, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = ','
	reader.LazyQuotes = true // mais tolerante
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}
	if len(records) == 0 {
		return dto.Collection{}, nil
	}

	// Valida header
	if len(records[0]) < len(csvHeader) {
		return nil, fmt.Errorf("invalid CSV header: expected at least %d columns, got %d", len(csvHeader), len(records[0]))
	}
	for i, col := range records[0] {
		if i < len(csvHeader) && col != csvHeader[i] {
			return nil, fmt.Errorf("invalid CSV header column %d: expected %q, got %q", i, csvHeader[i], col)
		}
	}

	c := make(dto.Collection)
	table := make(map[string]string) // hashHex bare -> texto canonical (default lang)

	// Primeira passada: coleta definições canônicas
	for _, rec := range records[1:] {
		if len(rec) < len(csvHeader) {
			continue // pula linha malformada
		}
		lang := rec[colLang]
		hashHex, _ := hash.Strip(rec[colHash])
		text := rec[colText]
		divergent := rec[colDivergent] == "true"

		// Só registra canônica no idioma default e não-divergent
		if lang == common.DefaultLocalization && !divergent && text != "" && !hash.IsRef(text) {
			if canonical, exists := table[hashHex]; !exists {
				table[hashHex] = text
			} else if canonical != text {
				// Divergência: editor externo mudou o texto sob o mesmo hash
			}
		}
	}

	// Segunda passada: constrói a Collection
	for _, rec := range records[1:] {
		if len(rec) < len(csvHeader) {
			continue
		}
		key := strings.TrimSpace(rec[colKey])
		if key == "" {
			// Sem a key completa não há kind/versão/casamento — o formato
			// exige uma key por linha (nada de reconstruir por id).
			return nil, fmt.Errorf("linha sem metadata.key (re-exporte o arquivo no formato atual)")
		}
		id := rec[colID]
		name := rec[colName]
		indexStr := rec[colIndex]
		lang := rec[colLang]
		// Polícia estrita do formato: todo hash de artefato tem prefixo $
		// (a mesma régua do .strings). Célula vazia é tolerada — artefato
		// feito à mão, o import recalcula pelo texto.
		hashHex, hasPrefix := hash.Strip(rec[colHash])
		if !hasPrefix && hashHex != "" {
			return nil, fmt.Errorf("hash %q sem prefixo $ (re-exporte o arquivo no formato atual)", rec[colHash])
		}
		text := rec[colText]
		source := rec[colSource]
		target := rec[colTarget]
		divergent := rec[colDivergent] == "true"

		// Row sem texto no original ainda entra quando carrega o par
		// source/target: o target é o que o import aplica.
		if text == "" && source == "" && target == "" {
			continue
		}

		// Resolve referências "$hash"
		if refHash, isRef := hash.Strip(text); isRef {
			if canonical, ok := table[refHash]; ok {
				text = canonical
			} else {
				// Órfão: mantém como está (JSON damaged)
			}
		}

		index, _ := parseInt(indexStr)

		entry, ok := c[id]
		if !ok {
			entry = dto.FileEntry{
				Metadata: dto.Metadata{
					Key: key,
					ID:  id,
				},
				Rows: make([]dto.TextRow, 0),
			}
		}

		// Adiciona/atualiza o hash
		hashMap := make(map[string]string)
		if entry.Rows != nil {
			// Procurar row existente para preservar outros hashes
			found := false
			for _, r := range entry.Rows {
				if r.Index == index && r.Name == name {
					hashMap = r.Hash
					found = true
					break
				}
			}
			if !found {
				hashMap = make(map[string]string)
			}
		} else {
			hashMap = make(map[string]string)
		}
		hashMap[lang] = hashHex

		textMap := make(map[string]string)
		if entry.Rows != nil {
			for _, r := range entry.Rows {
				if r.Index == index && r.Name == name {
					textMap = r.Text
					break
				}
			}
		}
		if textMap == nil {
			textMap = make(map[string]string)
		}
		textMap[lang] = text

		row := dto.TextRow{
			Index:     index,
			Name:      name,
			Hash:      hashMap,
			Text:      textMap,
			Divergent: divergent,
			Character: "",
			Kind:      "",
			// A projeção source/target só existe na linha do idioma default
			// (é lá que a tradução vive); o import lê apenas o target.
			Source: source,
			Target: target,
		}

		// Remove row existente se houver
		newRows := make([]dto.TextRow, 0, len(entry.Rows)+1)
		for _, r := range entry.Rows {
			if !(r.Index == index && r.Name == name) {
				newRows = append(newRows, r)
			}
		}
		newRows = append(newRows, row)
		dto.SortRows(newRows)

		entry.Rows = newRows
		// Chave do mapa = id (como no JSON/DTO); a key completa vive na
		// Metadata — é dela que saem kind/versão/casamento.
		c[id] = entry
	}

	// Ordena rows finais
	for k, entry := range c {
		dto.SortRows(entry.Rows)
		c[k] = entry
	}

	return c, nil
}

// parseInt converte string para int (leniente).
func parseInt(s string) (int, error) {
	var n int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n, nil
}

func init() {
	formatters.Register("csv", NewCSVFormatter())
	formatters.Register("csv-no-dedup", CSVFormatterNoDedup{})
}

// CSVFormatterNoDedup serializa SEM dedup (todo texto literal).
type CSVFormatterNoDedup struct{}

func (CSVFormatterNoDedup) Extension() string {
	return extensionCSV
}

func (CSVFormatterNoDedup) Marshal(c dto.Collection) ([]byte, error) {
	return marshalNoDedup(c)
}

func (CSVFormatterNoDedup) MarshalLangs(c dto.Collection, langs []string) ([]byte, error) {
	return marshalNoDedupLangs(c, langs)
}

func (CSVFormatterNoDedup) Unmarshal(data []byte) (dto.Collection, error) {
	return NewCSVFormatter().Unmarshal(data)
}

func marshalNoDedup(c dto.Collection) ([]byte, error) {
	return marshalNoDedupLangs(c, nil)
}

func marshalNoDedupLangs(c dto.Collection, langs []string) ([]byte, error) {
	filter := langSet(langs)
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Comma = ','
	if err := writer.Write(csvHeader); err != nil {
		return nil, err
	}
	for _, k := range c.SortedKeys() {
		entry := c[k]
		rows := make([]dto.TextRow, len(entry.Rows))
		copy(rows, entry.Rows)
		dto.SortRows(rows)
		key, id := entryColumns(entry, k)
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
				divergent := "false"
				if row.Divergent {
					divergent = "true"
				}
				source, target := formatters.SourceTarget(row, lang)
				record := []string{key, id, row.Name, fmt.Sprintf("%d", row.Index), lang, hash.Prefix(h), text, source, target, divergent}
				if err := writer.Write(record); err != nil {
					return nil, err
				}
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// entryColumns devolve a coluna key (metadata.key completa) e a coluna id
// (metadata.ID, com fallback para a chave da Collection) de uma entrada.
func entryColumns(entry dto.FileEntry, mapKey string) (key, id string) {
	key = entry.Metadata.Key
	id = entry.Metadata.ID
	if id == "" {
		id = mapKey
	}
	return key, id
}
