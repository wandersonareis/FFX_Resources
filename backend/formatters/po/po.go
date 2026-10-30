// Package po implementa o formato PO (gettext) de exportação/importação dos
// textos de localização — o artefato que as ferramentas CAT de verdade
// (Poedit, OmegaT, Crowdin) abrem sem reclamar de XML nem de esquema.
//
// Gramática do artefato:
//
//	<header> := msgid "" / msgstr "" + linhas "Campo: valor\n"
//	<entry>  := <comentário extraído> <referência> <msgctxt> <msgid> <msgstr>
//
// O canal de metadados é o comentário extraído "#. import: {json}": o JSON
// carrega o que o import precisa para reconstruir a coordenada da row no
// binário (arquivo, entrada, índice, campo e hash de referência). O msgctxt
// ("<entrada>|<índice>") e o "#:" ("<arquivo>:<índice>") são a cópia legível:
// o tradutor vê onde a string está, e o import ainda funciona se o editor
// bagunçar (ou remover) o JSON.
//
// O par traduzível é o do slot `us`: msgid = o original de data/ (Source) e
// msgstr = a tradução que vive em mods/ (Target). Os demais idiomas do jogo
// (ch/de/fr/it/jp/kr/sp) entram como REFERÊNCIAS read-only em
// "#. ref[<idioma>]" — as mesmas referências dos demais artefatos de CAT.
package po

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters"
	"ffxresources/backend/formatters/hash"
)

// extensionPO é a extensão produzida pelo formatter PO.
const extensionPO = ".po"

const (
	// metaPrefix marca o comentário extraído com o JSON de metadados:
	// "#. import: {...}". O prefixo é o que separa o canal de metadados
	// dos comentários "#." que o tradutor escreve à mão.
	metaPrefix = "import:"
	// refPrefix marca o comentário extraído com a referência de um idioma:
	// "#. ref[sp]: texto".
	refPrefix = "ref["
)

// importMeta é o JSON de metadados que viaja no comentário "#. import:".
// É o canal principal do import: com ele a row é reconstruída sem depender
// de parsing de msgctxt nem do "#:" (que o editor pode reformatar/remover).
type importMeta struct {
	// File é a key canônica do store (versão + caminho do arquivo).
	File string `json:"file"`
	// Name é o id da entrada (chave da Collection).
	Name string `json:"name"`
	// Index é a posição da row no binário (a coordenada com Field).
	Index *int `json:"index,omitempty"`
	// Field é o qualificador da row (objects: name/desc/ability1…;
	// lockit: game/utf8). Vazio para events/help/tabelas.
	Field string `json:"field,omitempty"`
	// Lang é o slot do par traduzível (us).
	Lang string `json:"lang"`
	// Hash é o hash do ORIGINAL (data/) no formato de artefato ("$"+hex).
	// Vazio quando o original é em branco (linha sem hash).
	Hash string `json:"hash"`
}

// POFormatter serializa/deserializa a Collection no formato PO (gettext).
type POFormatter struct{}

// Extension retorna a extensão produzida por este formatter.
func (POFormatter) Extension() string {
	return extensionPO
}

// Marshal serializa a Collection completa (todos os idiomas como
// referência).
func (f POFormatter) Marshal(c dto.Collection) ([]byte, error) {
	return f.marshal(c, nil)
}

// MarshalLangs serializa a Collection com as REFERÊNCIAS apenas nos idiomas
// pedidos (nil/vazio = todos os idiomas suportados além do `us`).
func (f POFormatter) MarshalLangs(c dto.Collection, langs []string) ([]byte, error) {
	return f.marshal(c, langs)
}

// Unmarshal parseia o PO de volta para a Collection (DTO), com rows ordenadas
// por Index para a reconstrução posicional do binário.
func (f POFormatter) Unmarshal(data []byte) (dto.Collection, error) {
	return unmarshal(data)
}

// marshal é o motor único da escrita: cabeçalho PO + uma entrada por row,
// na ordem das entradas (SortedKeys) e das rows (Index).
func (f POFormatter) marshal(c dto.Collection, langs []string) ([]byte, error) {
	refs := referenceLanguages(langs)
	blocks := make([]string, 0)
	for _, id := range c.SortedKeys() {
		entry := c[id]
		rows := make([]dto.TextRow, len(entry.Rows))
		copy(rows, entry.Rows)
		dto.SortRows(rows)
		for _, row := range rows {
			if block := writeEntry(entry, id, row, refs); block != "" {
				blocks = append(blocks, block)
			}
		}
	}
	var b strings.Builder
	writeHeader(&b)
	for _, block := range blocks {
		b.WriteString("\n") // linha em branco entre as entradas
		b.WriteString(block)
	}
	return []byte(b.String()), nil
}

// writeHeader grava o cabeçalho PO: a entrada msgid ""/msgstr "" que as
// ferramentas CAT leem como metadados do arquivo (o idioma alvo é o PT-BR —
// a tradução vive no slot `us` do binário, como em todo o app).
func writeHeader(b *strings.Builder) {
	b.WriteString("msgid \"\"\n")
	b.WriteString("msgstr \"\"\n")
	for _, field := range []string{
		"Project-Id-Version: FFX Resources",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"Language: " + common.TranslationLanguage,
	} {
		b.WriteString("\"" + escapePO(field) + "\\n\"\n")
	}
}

// writeEntry monta UMA row como entrada PO: metadados (JSON), referências
// dos outros idiomas, referência de arquivo, contexto e o par msgid/msgstr.
// "" quando a row não tem nem original nem tradução (não vira entrada:
// msgid/msgstr vazios são ruído no PO).
func writeEntry(entry dto.FileEntry, id string, row dto.TextRow, refs []string) string {
	source := row.Source
	if source == "" {
		source = row.Text[common.DefaultLocalization]
	}
	target := row.Target
	if source == "" && target == "" {
		return ""
	}

	entryID := entryName(entry, id)
	index := row.Index
	meta := importMeta{
		File:  entry.Metadata.Key,
		Name:  entryID,
		Index: &index,
		Field: row.Name,
		Lang:  common.DefaultLocalization,
		Hash:  hash.Prefix(row.Hash[common.DefaultLocalization]),
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		// json.Marshal de uma struct simples não falha — mas o artefato não
		// pode perder o canal de metadados por causa disso.
		common.LogWarning("po: serializando metadados: %v", err)
	}

	var b strings.Builder
	b.WriteString("#. " + metaPrefix + " " + string(payload) + "\n")
	for _, lang := range refs {
		text := row.Text[lang]
		if text == "" {
			continue
		}
		b.WriteString("#. " + refPrefix + lang + "]: " + escapePO(text) + "\n")
	}
	b.WriteString("#: " + entry.Metadata.Key + ":" + strconv.Itoa(row.Index) + "\n")
	b.WriteString("msgctxt \"" + escapePO(contextOf(entryID, row)) + "\"\n")
	b.WriteString("msgid \"" + escapePO(source) + "\"\n")
	b.WriteString("msgstr \"" + escapePO(target) + "\"\n")
	return b.String()
}

// entryName resolve o id da entrada (metadata.ID; a chave da Collection é o
// fallback).
func entryName(entry dto.FileEntry, mapKey string) string {
	if entry.Metadata.ID != "" {
		return entry.Metadata.ID
	}
	return mapKey
}

// contextOf monta o msgctxt: "<entrada>|<índice>" — mais o campo da row
// quando existe (objects/lockit compartilham índice entre campos).
func contextOf(id string, row dto.TextRow) string {
	ctx := id + "|" + strconv.Itoa(row.Index)
	if row.Name != "" {
		ctx += "|" + row.Name
	}
	return ctx
}

// referenceLanguages devolve os idiomas de REFERÊNCIA: todos os suportados
// menos o `us` (que é o par traduzível). langs nil/vazio = todos — a mesma
// regra dos demais formatos (o menu exporta sem lista = tudo).
func referenceLanguages(langs []string) []string {
	if len(langs) == 0 {
		out := make([]string, 0, len(common.SupportedLanguages))
		for _, code := range common.SupportedLanguageCodes() {
			if code != common.DefaultLocalization {
				out = append(out, code)
			}
		}
		return out
	}
	wanted := make(map[string]bool, len(langs))
	for _, lang := range langs {
		lang = strings.TrimSpace(lang)
		if lang != "" && lang != common.DefaultLocalization {
			wanted[lang] = true
		}
	}
	out := make([]string, 0, len(wanted))
	for _, code := range common.SupportedLanguageCodes() {
		if wanted[code] {
			out = append(out, code)
		}
	}
	return out
}

// escapePO escapa um texto para uma string PO (C-like): barra invertida,
// aspas, LF, CR e TAB. O texto do jogo pode trazer qualquer coisa — as tags
// ({TEXT_NEWLINE}, {PC:00:TIDUS}) não precisam de escape; aspas e quebras
// de linha, sim.
func escapePO(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unescapePO desfaz escapePO. Sequência desconhecida mantém o backslash e o
// caractere (leniente, como o parser de .strings do app).
func unescapePO(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	esc := false
	for _, r := range s {
		if esc {
			switch r {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			default:
				b.WriteByte('\\')
				b.WriteRune(r)
			}
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		b.WriteRune(r)
	}
	if esc {
		b.WriteByte('\\')
	}
	return b.String()
}

// poEntry é uma entrada já parseada do arquivo PO.
type poEntry struct {
	extracted []string // comentários "#."
	refs      []string // referências "#:"
	msgctxt   string
	msgid     string
	msgstr    string
	hasMsgid  bool
	hasMsgstr bool
	plural    bool // msgid_plural/msgstr[n]: o jogo não tem plurais
}

// addComment guarda um comentário da entrada. Só os extraídos ("#.") e as
// referências ("#:") interessam; comentário do tradutor ("# ") e flags
// ("#,") são ignorados.
func (e *poEntry) addComment(line string) {
	switch {
	case strings.HasPrefix(line, "#."):
		e.extracted = append(e.extracted, strings.TrimSpace(line[2:]))
	case strings.HasPrefix(line, "#:"):
		e.refs = append(e.refs, strings.TrimSpace(line[2:]))
	}
}

// parsePO quebra o arquivo em entradas. Tolera: comentários em qualquer
// ordem, entradas sem linha em branco entre elas, strings quebradas em
// linhas de continuação e entradas obsoletas do gettext ("#~").
func parsePO(text string) ([]poEntry, error) {
	var out []poEntry
	var cur poEntry
	flush := func() {
		if cur.hasMsgid || cur.hasMsgstr || cur.msgctxt != "" ||
			len(cur.extracted) > 0 || len(cur.refs) > 0 {
			out = append(out, cur)
		}
		cur = poEntry{}
	}

	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#~") {
			continue // entrada obsoleta (gettext): ignorada
		}
		if strings.HasPrefix(line, "#") {
			cur.addComment(line)
			continue
		}

		switch {
		case isPOKeyword(line, "msgctxt"):
			if cur.hasMsgid || cur.hasMsgstr {
				flush() // entrada nova sem linha em branco no meio
			}
			value, last, err := readPOValue(lines, i, "msgctxt")
			if err != nil {
				return nil, err
			}
			cur.msgctxt = value
			i = last
		case isPOKeyword(line, "msgid_plural"):
			cur.plural = true
			i = skipPOValue(lines, i)
		case isPOKeyword(line, "msgid"):
			if cur.hasMsgid || cur.hasMsgstr {
				flush() // entrada nova sem linha em branco no meio
			}
			value, last, err := readPOValue(lines, i, "msgid")
			if err != nil {
				return nil, err
			}
			cur.msgid = value
			cur.hasMsgid = true
			i = last
		case strings.HasPrefix(line, "msgstr["):
			cur.plural = true
			i = skipPOValue(lines, i)
		case isPOKeyword(line, "msgstr"):
			value, last, err := readPOValue(lines, i, "msgstr")
			if err != nil {
				return nil, err
			}
			cur.msgstr = value
			cur.hasMsgstr = true
			i = last
		}
	}
	flush()
	return out, nil
}

// isPOKeyword informa se a linha começa pela keyword PO exata (o nome
// precisa ser seguido de espaço — "msgid_plural" não é "msgid").
func isPOKeyword(line, keyword string) bool {
	return line == keyword || strings.HasPrefix(line, keyword+" ")
}

// readPOValue lê o valor de uma keyword PO a partir da linha i, consumindo
// as linhas de continuação ("..."). Devolve o valor e o índice da última
// linha consumida.
func readPOValue(lines []string, i int, keyword string) (string, int, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), keyword))
	if !strings.HasPrefix(rest, "\"") {
		return "", i, fmt.Errorf("po: %s sem valor na linha %d", keyword, i+1)
	}
	var b strings.Builder
	b.WriteString(unescapePO(stripQuotes(rest)))
	last := i
	for last+1 < len(lines) {
		next := strings.TrimSpace(strings.TrimRight(lines[last+1], "\r"))
		if !strings.HasPrefix(next, "\"") {
			break
		}
		b.WriteString(unescapePO(stripQuotes(next)))
		last++
	}
	return b.String(), last, nil
}

// stripQuotes remove as aspas que cercam o valor PO.
func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return strings.Trim(s, `"`)
}

// skipPOValue consome o valor de uma keyword e as linhas de continuação,
// sem exigir que o valor seja uma string entre aspas — é o caso das
// formas plurais ("msgstr[0] ..."), que o app ignora.
func skipPOValue(lines []string, i int) int {
	last := i
	for last+1 < len(lines) {
		next := strings.TrimSpace(strings.TrimRight(lines[last+1], "\r"))
		if !strings.HasPrefix(next, "\"") {
			break
		}
		last++
	}
	return last
}

// unmarshal parseia o PO de volta para a Collection (DTO).
//
// A row é montada pelo JSON de metadados quando ele existe; sem ele, o
// msgctxt ("<entrada>|<índice>[|<campo>]") e a referência "#:"
// ("<arquivo>:<índice>") reconstroem a mesma coordenada — a redundância é
// proposital: o tradutor sempre vê o contexto, e o import não depende de um
// único canal.
func unmarshal(data []byte) (dto.Collection, error) {
	entries, err := parsePO(string(data))
	if err != nil {
		return nil, err
	}

	collection := make(dto.Collection)
	for _, e := range entries {
		if e.plural {
			continue // o jogo não tem plurais
		}
		if e.msgctxt == "" && e.msgid == "" {
			continue // cabeçalho do PO
		}
		row, id, key, ok := e.toRow()
		if !ok {
			continue
		}
		entry, exists := collection[id]
		if !exists {
			entry = dto.FileEntry{Metadata: dto.Metadata{Key: key, ID: id}}
		}
		entry.Rows = append(entry.Rows, row)
		collection[id] = entry
	}

	for id, entry := range collection {
		dto.SortRows(entry.Rows)
		entry.Metadata = entry.Metadata.WithRowCount(len(entry.Rows))
		collection[id] = entry
	}
	return collection, nil
}

// toRow monta a row da entrada: coordenada (Index, Name), hash de
// referência, source (msgid) e target (msgstr).
func (e *poEntry) toRow() (dto.TextRow, string, string, bool) {
	meta, hasMeta := e.metadata()

	// msgctxt: "<entrada>|<índice>[|<campo>]".
	ctxParts := strings.Split(e.msgctxt, "|")
	ctxID := strings.TrimSpace(ctxParts[0])
	ctxIndex := -1
	if len(ctxParts) > 1 {
		if n, err := strconv.Atoi(strings.TrimSpace(ctxParts[1])); err == nil && n >= 0 {
			ctxIndex = n
		}
	}
	ctxField := ""
	if len(ctxParts) > 2 {
		ctxField = strings.TrimSpace(strings.Join(ctxParts[2:], "|"))
	}

	// Referência "#: <arquivo>:<índice>".
	refFile, refIndex := "", -1
	if len(e.refs) > 0 {
		refFile, refIndex = splitReference(e.refs[0])
	}

	key := meta.File
	if key == "" {
		key = refFile
	}
	id := meta.Name
	if id == "" {
		id = ctxID
	}
	if id == "" {
		id = stemOf(key)
	}
	if id == "" || key == "" {
		common.LogWarning("po: entrada sem arquivo/identificação — ignorada (msgctxt %q)", e.msgctxt)
		return dto.TextRow{}, "", "", false
	}

	index := ctxIndex
	if hasMeta && meta.Index != nil {
		index = *meta.Index
	}
	if index < 0 {
		index = refIndex
	}
	if index < 0 {
		index = 0
	}
	field := meta.Field
	if field == "" {
		field = ctxField
	}

	hashHex := ""
	if hasMeta {
		hashHex, _ = hash.Strip(meta.Hash)
	}

	row := dto.TextRow{
		Index:  index,
		Name:   field,
		Source: e.msgid,
		Target: e.msgstr,
		Text:   map[string]string{},
	}
	if row.Source != "" {
		row.Text[common.DefaultLocalization] = row.Source
	}
	if hashHex != "" {
		row.Hash = map[string]string{common.DefaultLocalization: hashHex}
	}
	for lang, text := range e.referenceTexts() {
		if lang == common.DefaultLocalization || text == "" {
			continue
		}
		row.Text[lang] = text
	}

	// Integridade: o hash gravado é o do original de data/ — se o msgid do
	// artefato não bate com ele, o msgid foi editado fora do app.
	if hashHex != "" && row.Source != "" {
		if got := hash.Sum64Hex(row.Source); got != hashHex {
			common.LogWarning("po: msgid alterado no artefato (%s: hash %s != %s)",
				id, hashHex, got)
		}
	}
	return row, id, key, true
}

// metadata devolve o JSON de metadados da entrada (quando existe e é válido).
func (e *poEntry) metadata() (importMeta, bool) {
	for _, comment := range e.extracted {
		if !strings.HasPrefix(comment, metaPrefix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(comment, metaPrefix))
		var meta importMeta
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			common.LogWarning("po: metadados inválidos (%v) — usando msgctxt/referência", err)
			return importMeta{}, false
		}
		return meta, true
	}
	return importMeta{}, false
}

// referenceTexts devolve as referências read-only da entrada, por idioma
// ("#. ref[sp]: texto").
func (e *poEntry) referenceTexts() map[string]string {
	out := make(map[string]string)
	for _, comment := range e.extracted {
		if !strings.HasPrefix(comment, refPrefix) {
			continue
		}
		rest := strings.TrimPrefix(comment, refPrefix)
		end := strings.Index(rest, "]:")
		if end < 0 {
			continue
		}
		lang := strings.TrimSpace(rest[:end])
		text := unescapePO(strings.TrimSpace(rest[end+2:]))
		if lang == "" || text == "" {
			continue
		}
		out[lang] = text
	}
	return out
}

// splitReference quebra a referência "#:" em arquivo e índice:
// "<arquivo>:<índice>". Sem índice, devolve -1.
func splitReference(ref string) (file string, index int) {
	ref = strings.TrimSpace(ref)
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		return ref, -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(ref[idx+1:]))
	if err != nil || n < 0 {
		return ref, -1
	}
	return strings.TrimSpace(ref[:idx]), n
}

// stemOf devolve o nome do arquivo sem diretório e sem extensão — o id da
// entrada quando o msgctxt não traz o nome.
func stemOf(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndex(path, "."); i > 0 {
		path = path[:i]
	}
	return path
}

func init() {
	formatters.Register("po", POFormatter{})
}
