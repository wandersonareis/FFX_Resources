// Package xlsx implementa o formato XLSX (Excel) para exportação/importação
// dos arquivos de localização. Usa github.com/xuri/excelize/v2.
package xlsx

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters"
	"ffxresources/backend/formatters/hash"

	"github.com/xuri/excelize/v2"
)

// extensionXLSX é a extensão produzida pelo formatter XLSX.
const extensionXLSX = ".xlsx"

// ExtensionXLSX retorna a extensão produzida pelo formatter XLSX.
func ExtensionXLSX() string {
	return extensionXLSX
}

// XLSXFormatter formata a Collection textual para XLSX.
type XLSXFormatter struct{}

// NewXLSXFormatter constrói o formatter XLSX.
func NewXLSXFormatter() XLSXFormatter {
	return XLSXFormatter{}
}

// Extension retorna a extensão produzida por este formatter.
func (XLSXFormatter) Extension() string {
	return extensionXLSX
}

// xlsxHeader define as colunas do XLSX. A coluna key é a metadata.key
// COMPLETA (version/pattern/arquivo) — a identidade da entrada no app: é
// dela que saem o kind, a versão e o casamento por basename, então um
// artefato com VÁRIAS entradas (events_all, objects_all) volta inteiro no
// import. id é o metadata.ID (ex.: chunk_07 do dicionário, que não vive
// no nome do arquivo).
// source = idioma fonte (us) de data/vbf, target = idioma alvo (pt-BR) de mods.
var xlsxHeader = []string{
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

// Índices das colunas calculadas no xlsxHeader (key=0 … divergent=9).
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

// Mensagem da validação das colunas de metadata (A–F): aparece quando
// alguém digita em metadata com a planilha desprotegida.
const (
	metadataEditErrorTitle = "Edição não permitida"
	metadataEditErrorMsg   = "Não altere os valores desta coluna."
)

// xlsxWidthMax é o teto das colunas calculadas (texto que passa disso
// fica com a linha quebrada pelo WrapText da coluna text).
const xlsxWidthMax = 80

// xlsxWidths rastreia a largura (em CARACTERES — runes, não bytes: o texto
// do jogo tem CJK e PUA) de cada coluna: TODAS começam em xlsxWidthBase e
// crescem até o maior texto observado, clampado no teto. excelize não tem
// autofit; esta é a aproximação padrão.
type xlsxWidths struct {
	current []int
	caps    []int
}

// xlsxWidthBase é a largura inicial (e mínima) de TODAS as colunas. Var
// (não const) de propósito: é o botão de ajuste fino — 10 já acomoda o
// maior nome de header ("divergent"), sem precisar medir nome antes do
// conteúdo.
var xlsxWidthBase = 10

// newXlsxWidths semeia TODAS as colunas com xlsxWidthBase — nada de medir
// o nome do header. Depois o observe() cuida do crescimento: coluna só
// aumenta ou fica igual, nunca encolhe abaixo da base.
func newXlsxWidths() *xlsxWidths {
	w := &xlsxWidths{
		current: make([]int, len(xlsxHeader)),
		caps:    make([]int, len(xlsxHeader)),
	}
	for i := range xlsxHeader {
		w.current[i] = xlsxWidthBase
		w.caps[i] = xlsxWidthMax
	}
	return w
}

// observe registra um texto: se é o maior da coluna (e respeita o teto),
// a largura da coluna sobe para ele. Texto maior que o teto clampa a
// coluna no teto (min(n, cap)) — nunca deixa a coluna presa no header
// quando TODOS os textos estouram.
func (w *xlsxWidths) observe(col int, text string) {
	n := utf8.RuneCountInString(text)
	if n <= w.current[col] {
		return
	}
	if n > w.caps[col] {
		n = w.caps[col]
	}
	w.current[col] = n
}

// Marshal organiza a Collection pronta no formato XLSX (todos os idiomas).
func (XLSXFormatter) Marshal(c dto.Collection) ([]byte, error) {
	return (XLSXFormatter{}).MarshalLangs(c, nil)
}

// MarshalLangs organiza a Collection pronta no formato XLSX contendo
// só os idiomas pedidos (nil/vazio = todos).
func (XLSXFormatter) MarshalLangs(c dto.Collection, langs []string) ([]byte, error) {
	filter := langSet(langs)

	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName(f.GetSheetName(0), sheet)

	// Header style
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})
	if err != nil {
		return nil, err
	}

	// Escreve header
	for i, h := range xlsxHeader {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Largura por coluna: TODAS começam em xlsxWidthBase (10) e crescem
	// pelo maior TEXTO observado (runes), teto 80 — excelize não tem
	// autofit. Aplicada no FINAL, quando o máximo é conhecido.
	widths := newXlsxWidths()

	// Estilo de queca de linha da coluna text: UM estilo, aplicado em
	// range no final. Com WrapText e SEM altura de linha explícita, o
	// Excel cresce a altura da linha sozinho ao abrir (regra: nunca
	// SetRowHeight, senão o excedente fica escondido). É uma das
	// colunas editáveis: Protection destrava a célula para quando a
	// planilha é protegida (ver protectMetadataColumns).
	textStyle, err := f.NewStyle(&excelize.Style{
		Alignment:  &excelize.Alignment{WrapText: true, Vertical: "top"},
		Protection: &excelize.Protection{Locked: false},
	})
	if err != nil {
		return nil, err
	}

	// Source column (H) style: editable with blue background to highlight
	// the source language (us/English from data/vbf).
	sourceStyle, err := f.NewStyle(&excelize.Style{
		Alignment:  &excelize.Alignment{WrapText: true, Vertical: "top"},
		Protection: &excelize.Protection{Locked: false},
		Fill:       excelize.Fill{Type: "pattern", Color: []string{"#0ea5e9"}, Pattern: 1},
	})
	if err != nil {
		return nil, err
	}

	// Target column (I) style: editable, no special background.
	targetStyle, err := f.NewStyle(&excelize.Style{
		Alignment:  &excelize.Alignment{WrapText: true, Vertical: "top"},
		Protection: &excelize.Protection{Locked: false},
	})
	if err != nil {
		return nil, err
	}

	// Divergent column (J) style: editable, no WrapText ("true"/"false" não quebra linha).
	divergentStyle, err := f.NewStyle(&excelize.Style{
		Protection: &excelize.Protection{Locked: false},
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]string) // hashHex bare -> texto (só default lang)

	rowIdx := 2
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
				ph := hash.Prefix(h)

				// Dedup: no idioma default, textos repetidos viram ref "$hash"
				value := text
				if lang == common.DefaultLocalization && !row.Divergent && hash.IsDedupEligible(text) {
					if _, dup := seen[h]; dup {
						value = ph
					} else {
						seen[h] = text
					}
				}

				divergent := "false"
				if row.Divergent {
					divergent = "true"
				}

				source, target := formatters.SourceTarget(row, lang)

				f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), key)
				f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), id)
				f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), row.Name)
				f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), row.Index)
				f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), lang)
				f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), ph)
				f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), value)
				f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), source)
				f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), target)
				f.SetCellValue(sheet, fmt.Sprintf("J%d", rowIdx), divergent)

				// Alimenta as larguras: TODAS as colunas observam o
				// conteúdo (runes) — hash pega o "$"+hex de 17, divergent o
				// "true"/"false".
				widths.observe(colKey, key)
				widths.observe(colID, id)
				widths.observe(colName, row.Name)
				widths.observe(colIndex, strconv.Itoa(row.Index))
				widths.observe(colLang, lang)
				widths.observe(colHash, ph)
				widths.observe(colText, value)
				widths.observe(colSource, source)
				widths.observe(colTarget, target)
				widths.observe(colDivergent, divergent)

				rowIdx++
			}
		}
	}

	// Larguras finais: só agora o máximo de cada coluna é conhecido.
	for i, w := range widths.current {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, float64(w))
	}

	// Queca de linha da coluna text (G) e destravamento da source, target, divergent
	// (H, I, J): UM estilo, UM range cada (não um por linha).
	if rowIdx > 2 {
		f.SetCellStyle(sheet, "G2", fmt.Sprintf("G%d", rowIdx-1), textStyle)
		// Source column (H) with blue background
		f.SetCellStyle(sheet, "H2", fmt.Sprintf("H%d", rowIdx-1), sourceStyle)
		// Target column (I) editable, no special background
		f.SetCellStyle(sheet, "I2", fmt.Sprintf("I%d", rowIdx-1), targetStyle)
		// Divergent column (J) editable, no WrapText
		f.SetCellStyle(sheet, "J2", fmt.Sprintf("J%d", rowIdx-1), divergentStyle)
	}

	// Auto-filter
	if rowIdx > 2 {
		endCell, _ := excelize.CoordinatesToCellName(len(xlsxHeader), rowIdx-1)
		f.AutoFilter(sheet, "A1:"+endCell, []excelize.AutoFilterOptions{})
		// Freeze primeira linha
		f.SetPanes(sheet, &excelize.Panes{
			Freeze:      true,
			YSplit:      1,
			TopLeftCell: "A2",
		})
	}

	// Proteção: metadata (A–F) e header travados, só text, source, target e divergent
	// editáveis — sem senha, é contra edição acidental.
	if err := protectMetadataColumns(f, sheet); err != nil {
		return nil, err
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
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
	// A linha de `us` entra também quando só há source/target: o target é o
	// que o import aplica, mesmo sem texto no original.
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

// protectMetadataColumns trava as colunas de metadata (A–F, header
// incluso) contra edição ERRADA — o alvo é o acidente, não a segurança:
// a planilha é protegida SEM senha (desproteger é um clique em Revisão,
// e colar bypassa a validação), então quem insistir ainda edita. Célula
// travada é o PADRÃO do xlsx; só text (G), source (H), target (I) e divergent (J)
// saem dos marshallers com estilo Protection{Locked: false}. O aviso
// customizado vive numa DataValidation "custom" sempre-inválida
// (Formula1 FALSE) sobre A:F: com a planilha desprotegida, digitar em
// metadata mostra "Edição não permitida" e rejeita — protegida, quem
// digita recebe o aviso padrão do próprio Excel (o formato não permite
// customizar aquele aviso).
func protectMetadataColumns(f *excelize.File, sheet string) error {
	if err := f.ProtectSheet(sheet, &excelize.SheetProtectionOptions{
		SelectLockedCells:   true, // deixa clicar/copiar células travadas
		SelectUnlockedCells: true,
		AutoFilter:          true, // mantém o AutoFilter da planilha usável
		Sort:                true,
	}); err != nil {
		return err
	}

	dv := excelize.NewDataValidation(true)
	dv.Sqref = "A:F"
	dv.Type = "custom"
	dv.Formula1 = "FALSE"
	dv.SetError(excelize.DataValidationErrorStyleStop, metadataEditErrorTitle, metadataEditErrorMsg)
	return f.AddDataValidation(sheet, dv)
}

// Unmarshal parseia o XLSX de volta para a Collection (DTO).
func (XLSXFormatter) Unmarshal(data []byte) (dto.Collection, error) {
	f, err := excelize.OpenReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return dto.Collection{}, nil
	}
	sheet := sheets[0]

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}
	if len(rows) == 0 {
		return dto.Collection{}, nil
	}

	// Valida header: as colunas do formato (nome E ordem). O XLSX aceita
	// colunas extras depois das conhecidas.
	if len(rows[0]) < len(xlsxHeader) {
		return nil, fmt.Errorf("invalid XLSX header: expected at least %d columns, got %d", len(xlsxHeader), len(rows[0]))
	}
	for i, want := range xlsxHeader {
		if got := strings.TrimSpace(rows[0][i]); got != want {
			return nil, fmt.Errorf("invalid XLSX header column %d: expected %q, got %q", i, want, got)
		}
	}

	c := make(dto.Collection)
	table := make(map[string]string) // hashHex bare -> texto canonical (default lang)

	// Primeira passada: coleta definições canônicas
	for _, rec := range rows[1:] {
		if len(rec) < len(xlsxHeader) {
			continue
		}
		lang := rec[colLang]
		hashHex, _ := hash.Strip(rec[colHash])
		text := rec[colText]
		divergent := rec[colDivergent] == "true"

		if lang == common.DefaultLocalization && !divergent && text != "" && !hash.IsRef(text) {
			if _, exists := table[hashHex]; !exists {
				table[hashHex] = text
			}
		}
	}

	// Segunda passada: constrói a Collection
	for _, rec := range rows[1:] {
		if len(rec) < len(xlsxHeader) {
			continue
		}
		key := strings.TrimSpace(rec[0])
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
			}
		}

		index, _ := strconv.Atoi(indexStr)

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

		// Preserva hashes/textos existentes da row
		hashMap := make(map[string]string)
		textMap := make(map[string]string)
		for _, r := range entry.Rows {
			if r.Index == index && r.Name == name {
				hashMap = r.Hash
				textMap = r.Text
				break
			}
		}
		if hashMap == nil {
			hashMap = make(map[string]string)
		}
		if textMap == nil {
			textMap = make(map[string]string)
		}
		hashMap[lang] = hashHex
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

func init() {
	formatters.Register("xlsx", NewXLSXFormatter())
	formatters.Register("xlsx-no-dedup", XLSXFormatterNoDedup{})
}

// XLSXFormatterNoDedup serializa SEM dedup (todo texto literal).
type XLSXFormatterNoDedup struct{}

func (XLSXFormatterNoDedup) Extension() string {
	return extensionXLSX
}

func (XLSXFormatterNoDedup) Marshal(c dto.Collection) ([]byte, error) {
	return marshalNoDedup(c)
}

func (XLSXFormatterNoDedup) MarshalLangs(c dto.Collection, langs []string) ([]byte, error) {
	return marshalNoDedupLangs(c, langs)
}

func (XLSXFormatterNoDedup) Unmarshal(data []byte) (dto.Collection, error) {
	return NewXLSXFormatter().Unmarshal(data)
}

func marshalNoDedup(c dto.Collection) ([]byte, error) {
	return marshalNoDedupLangs(c, nil)
}

func marshalNoDedupLangs(c dto.Collection, langs []string) ([]byte, error) {
	filter := langSet(langs)
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName(f.GetSheetName(0), sheet)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	for i, h := range xlsxHeader {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Mesma régua de larguras do MarshalLangs: base 10 + maior texto
	// observado (runes), teto 80 — aplicada no final. Mesmas duas
	// colunas editáveis também: text (WrapText + destravada) e divergent
	// (só destravada).
	widths := newXlsxWidths()
	// Text column style: WrapText + unlocked
	textStyle, _ := f.NewStyle(&excelize.Style{
		Alignment:  &excelize.Alignment{WrapText: true, Vertical: "top"},
		Protection: &excelize.Protection{Locked: false},
	})
	// Source column style: editable with blue background
	sourceStyle, _ := f.NewStyle(&excelize.Style{
		Alignment:  &excelize.Alignment{WrapText: true, Vertical: "top"},
		Protection: &excelize.Protection{Locked: false},
		Fill:       excelize.Fill{Type: "pattern", Color: []string{"#0ea5e9"}, Pattern: 1},
	})
	// Target column style: editable, no special background
	targetStyle, _ := f.NewStyle(&excelize.Style{
		Alignment:  &excelize.Alignment{WrapText: true, Vertical: "top"},
		Protection: &excelize.Protection{Locked: false},
	})
	// Divergent column style: unlocked, no WrapText
	divergentStyle, _ := f.NewStyle(&excelize.Style{
		Protection: &excelize.Protection{Locked: false},
	})

	rowIdx := 2
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
				ph := hash.Prefix(h) // hash no formato de artefato: "$"+hex
				source, target := formatters.SourceTarget(row, lang)
				divergent := "false"
				if row.Divergent {
					divergent = "true"
				}
				f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), key)
				f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), id)
				f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), row.Name)
				f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), row.Index)
				f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), lang)
				f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), ph)
				f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), text)
				f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), source)
				f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), target)
				f.SetCellValue(sheet, fmt.Sprintf("J%d", rowIdx), divergent)

				// Mesma régua do MarshalLangs: todas observam o conteúdo.
				widths.observe(colKey, key)
				widths.observe(colID, id)
				widths.observe(colName, row.Name)
				widths.observe(colIndex, strconv.Itoa(row.Index))
				widths.observe(colLang, lang)
				widths.observe(colHash, ph)
				widths.observe(colText, text)
				widths.observe(colSource, source)
				widths.observe(colTarget, target)
				widths.observe(colDivergent, divergent)
				rowIdx++
			}
		}
	}

	for i, w := range widths.current {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, float64(w))
	}
	if rowIdx > 2 {
		f.SetCellStyle(sheet, "G2", fmt.Sprintf("G%d", rowIdx-1), textStyle)
		// Source column (H) with blue background
		f.SetCellStyle(sheet, "H2", fmt.Sprintf("H%d", rowIdx-1), sourceStyle)
		// Target column (I) editable, no special background
		f.SetCellStyle(sheet, "I2", fmt.Sprintf("I%d", rowIdx-1), targetStyle)
		// Divergent column (J) editable, no WrapText
		f.SetCellStyle(sheet, "J2", fmt.Sprintf("J%d", rowIdx-1), divergentStyle)
	}

	if rowIdx > 2 {
		endCell, _ := excelize.CoordinatesToCellName(len(xlsxHeader), rowIdx-1)
		f.AutoFilter(sheet, "A1:"+endCell, []excelize.AutoFilterOptions{})
		f.SetPanes(sheet, &excelize.Panes{
			Freeze:      true,
			YSplit:      1,
			TopLeftCell: "A2",
		})
	}

	// Mesma proteção do MarshalLangs: metadata travada, text/divergent
	// editáveis.
	if err := protectMetadataColumns(f, sheet); err != nil {
		return nil, err
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
