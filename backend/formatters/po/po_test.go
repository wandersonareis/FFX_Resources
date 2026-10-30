package po

import (
	"fmt"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
)

// sampleSource é o original (data/) da row de exemplo.
const sampleSource = "{PC:00:TIDUS}{TEXT_NEWLINE}Whew!"

// sampleHash é o hash do original de exemplo (o do artefato: "$"+hex).
func sampleHash() string {
	return hash.Sum64Hex(sampleSource)
}

// sampleCollection é uma entrada de battletext com o par us (original +
// tradução) e as referências dos outros idiomas.
func sampleCollection() dto.Collection {
	return dto.Collection{
		"bika00_10": {
			Metadata: dto.Metadata{
				Key: "ffx/battle/btl/bika00_10/bika00_10.bin",
				ID:  "bika00_10",
			},
			Rows: []dto.TextRow{
				{
					Index: 0,
					Hash: map[string]string{
						common.DefaultLocalization: sampleHash(),
					},
					Text: map[string]string{
						common.DefaultLocalization: sampleSource,
						"sp":                       "¡Ufa!",
						"fr":                       "Ouf !",
					},
					Source: sampleSource,
					Target: "Ufa!",
				},
			},
		},
	}
}

// TestMarshalShape fixa o formato do artefato: cabeçalho PO, metadados no
// comentário extraído, referência de arquivo, contexto e o par msgid/msgstr.
func TestMarshalShape(t *testing.T) {
	raw, err := POFormatter{}.Marshal(sampleCollection())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := "msgid \"\"\n" +
		"msgstr \"\"\n" +
		"\"Project-Id-Version: FFX Resources\\n\"\n" +
		"\"MIME-Version: 1.0\\n\"\n" +
		"\"Content-Type: text/plain; charset=UTF-8\\n\"\n" +
		"\"Content-Transfer-Encoding: 8bit\\n\"\n" +
		"\"Language: pt-BR\\n\"\n" +
		"\n" +
		fmt.Sprintf("#. import: {\"file\":\"ffx/battle/btl/bika00_10/bika00_10.bin\",\"name\":\"bika00_10\",\"index\":0,\"lang\":\"us\",\"hash\":\"$%s\"}\n", sampleHash()) +
		"#. ref[fr]: Ouf !\n" +
		"#. ref[sp]: ¡Ufa!\n" +
		"#: ffx/battle/btl/bika00_10/bika00_10.bin:0\n" +
		"msgctxt \"bika00_10|0\"\n" +
		"msgid \"{PC:00:TIDUS}{TEXT_NEWLINE}Whew!\"\n" +
		"msgstr \"Ufa!\"\n"
	if string(raw) != want {
		t.Fatalf("artefato inesperado:\n--- got ---\n%s\n--- want ---\n%s", raw, want)
	}
}

// TestMarshalReferences: os idiomas além do `us` saem como referência
// read-only, e MarshalLangs filtra exatamente como os demais formatos.
func TestMarshalReferences(t *testing.T) {
	raw, err := POFormatter{}.Marshal(sampleCollection())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "#. ref[fr]: Ouf !\n") {
		t.Fatalf("referência do fr ausente:\n%s", got)
	}
	if !strings.Contains(got, "#. ref[sp]: ¡Ufa!\n") {
		t.Fatalf("referência do sp ausente:\n%s", got)
	}
	if strings.Contains(got, "ref[us]") {
		t.Fatalf("o idioma default não pode virar referência:\n%s", got)
	}

	raw, err = POFormatter{}.MarshalLangs(sampleCollection(), []string{common.DefaultLocalization, "sp"})
	if err != nil {
		t.Fatalf("marshal langs: %v", err)
	}
	got = string(raw)
	if !strings.Contains(got, "#. ref[sp]: ¡Ufa!\n") {
		t.Fatalf("referência do sp ausente no filtro:\n%s", got)
	}
	if strings.Contains(got, "ref[fr]") {
		t.Fatalf("referência do fr não deveria entrar no filtro:\n%s", got)
	}
}

// TestMarshalSkipsBlankRow: linha sem original e sem tradução não vira
// entrada (msgid/msgstr vazios são ruído no PO).
func TestMarshalSkipsBlankRow(t *testing.T) {
	c := dto.Collection{
		"vazio": {
			Metadata: dto.Metadata{Key: "ffx/event/obj_ps3/va/va/va.bin", ID: "va"},
			Rows:     []dto.TextRow{{Index: 0, Text: map[string]string{}}},
		},
	}
	raw, err := POFormatter{}.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "msgctxt") {
		t.Fatalf("row em branco virou entrada:\n%s", raw)
	}
}

// TestMarshalEscapes: aspas, barras e quebras de linha saem escapadas — o
// texto do jogo pode trazer qualquer coisa.
func TestMarshalEscapes(t *testing.T) {
	c := dto.Collection{
		"esc": {
			Metadata: dto.Metadata{Key: "ffx/event/obj_ps3/es/es/es.bin", ID: "es"},
			Rows: []dto.TextRow{{
				Index:  3,
				Text:   map[string]string{common.DefaultLocalization: "a"},
				Source: "he said \"hi\"\\nreally",
				Target: "linha1\nlinha2",
			}},
		},
	}
	raw, err := POFormatter{}.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `msgid "he said \"hi\"\\nreally"`) {
		t.Fatalf("escaping do msgid errado:\n%s", got)
	}
	if !strings.Contains(got, `msgstr "linha1\nlinha2"`) {
		t.Fatalf("escaping do msgstr errado:\n%s", got)
	}
}

// TestUnmarshalMetadata: o JSON do comentário extraído reconstrói a
// coordenada (arquivo, entrada, índice, campo) e o hash de referência.
func TestUnmarshalMetadata(t *testing.T) {
	raw := []byte(fmt.Sprintf("#. import: {\"file\":\"ffx/battle/btl/bika00_10/bika00_10.bin\",\"name\":\"bika00_10\",\"index\":7,\"field\":\"name\",\"lang\":\"us\",\"hash\":\"$%s\"}\n", hash.Sum64Hex("Whew!")) +
		"#: ffx/battle/btl/bika00_10/bika00_10.bin:7\n" +
		"msgctxt \"bika00_10|7|name\"\n" +
		"msgid \"Whew!\"\n" +
		"msgstr \"Ufa!\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := c["bika00_10"]
	if !ok {
		t.Fatalf("entrada ausente: %v", c.SortedKeys())
	}
	if entry.Metadata.Key != "ffx/battle/btl/bika00_10/bika00_10.bin" {
		t.Fatalf("key errada: %q", entry.Metadata.Key)
	}
	if len(entry.Rows) != 1 {
		t.Fatalf("rows: %d, queria 1", len(entry.Rows))
	}
	row := entry.Rows[0]
	if row.Index != 7 || row.Name != "name" {
		t.Fatalf("coordenada errada: index=%d name=%q", row.Index, row.Name)
	}
	if row.Hash[common.DefaultLocalization] != hash.Sum64Hex("Whew!") {
		t.Fatalf("hash errado: %q", row.Hash[common.DefaultLocalization])
	}
	if row.Source != "Whew!" || row.Target != "Ufa!" {
		t.Fatalf("textos errados: source=%q target=%q", row.Source, row.Target)
	}
	if row.Text[common.DefaultLocalization] != "Whew!" {
		t.Fatalf("text[us] errado: %q", row.Text[common.DefaultLocalization])
	}
}

// TestUnmarshalWithoutMetadata: sem o JSON (editor removeu o comentário),
// o msgctxt e a referência "#:" ainda reconstroem a row.
func TestUnmarshalWithoutMetadata(t *testing.T) {
	raw := []byte("#: ffx/battle/btl/bika00_10/bika00_10.bin:4\n" +
		"msgctxt \"bika00_10|4\"\n" +
		"msgid \"Whew!\"\n" +
		"msgstr \"Ufa!\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := c["bika00_10"]
	if !ok {
		t.Fatalf("entrada ausente: %v", c.SortedKeys())
	}
	if entry.Metadata.Key != "ffx/battle/btl/bika00_10/bika00_10.bin" {
		t.Fatalf("key errada: %q", entry.Metadata.Key)
	}
	row := entry.Rows[0]
	if row.Index != 4 {
		t.Fatalf("index errado: %d", row.Index)
	}
	if row.Target != "Ufa!" {
		t.Fatalf("target errado: %q", row.Target)
	}
}

// TestUnmarshalEmptyMsgstr: msgstr vazio = não traduzido (o import não
// aplica nada naquela row).
func TestUnmarshalEmptyMsgstr(t *testing.T) {
	raw := []byte("#: ffx/battle/btl/bika00_10/bika00_10.bin:0\n" +
		"msgctxt \"bika00_10|0\"\n" +
		"msgid \"Whew!\"\n" +
		"msgstr \"\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	row := c["bika00_10"].Rows[0]
	if row.Target != "" {
		t.Fatalf("target deveria estar vazio: %q", row.Target)
	}
	if row.Source != "Whew!" {
		t.Fatalf("source errado: %q", row.Source)
	}
}

// TestUnmarshalEmptyMsgidWithContext: msgid vazio COM msgctxt é uma row
// normal (original em branco traduzido em mods/), não o cabeçalho do PO.
func TestUnmarshalEmptyMsgidWithContext(t *testing.T) {
	raw := []byte("msgid \"\"\n" +
		"msgstr \"\"\n" +
		"\"Language: pt-BR\\n\"\n" +
		"\n" +
		"#: ffx/battle/btl/bika00_10/bika00_10.bin:2\n" +
		"msgctxt \"bika00_10|2\"\n" +
		"msgid \"\"\n" +
		"msgstr \"traduzido\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(c) != 1 {
		t.Fatalf("entradas: %v, queria só a row com msgid vazio", c.SortedKeys())
	}
	row := c["bika00_10"].Rows[0]
	if row.Index != 2 || row.Target != "traduzido" {
		t.Fatalf("row errada: index=%d target=%q", row.Index, row.Target)
	}
}

// TestUnmarshalContinuation: strings quebradas em várias linhas (como o
// Poedit grava) voltam inteiras.
func TestUnmarshalContinuation(t *testing.T) {
	raw := []byte("#: ffx/battle/btl/bika00_10/bika00_10.bin:0\n" +
		"msgctxt \"bika00_10|0\"\n" +
		"msgid \"\"\n" +
		"\"linha1\\n\"\n" +
		"\"linha2\"\n" +
		"msgstr \"\"\n" +
		"\"trad com aspas\\\"\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	row := c["bika00_10"].Rows[0]
	if row.Source != "linha1\nlinha2" {
		t.Fatalf("source com continuação errado: %q", row.Source)
	}
	if row.Target != "trad com aspas\"" {
		t.Fatalf("target com continuação errado: %q", row.Target)
	}
}

// TestUnmarshalReferences: as referências voltam como Text[lang] (read-only)
// e nunca como o texto do idioma default.
func TestUnmarshalReferences(t *testing.T) {
	raw := []byte("#. ref[sp]: ¡Ufa!\n" +
		"#. ref[fr]: Ouf !\n" +
		"#: ffx/battle/btl/bika00_10/bika00_10.bin:0\n" +
		"msgctxt \"bika00_10|0\"\n" +
		"msgid \"Whew!\"\n" +
		"msgstr \"Ufa!\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	row := c["bika00_10"].Rows[0]
	if row.Text["sp"] != "¡Ufa!" || row.Text["fr"] != "Ouf !" {
		t.Fatalf("referências erradas: %v", row.Text)
	}
	if row.Text[common.DefaultLocalization] != "Whew!" {
		t.Fatalf("text[us] não pode ser a referência: %q", row.Text[common.DefaultLocalization])
	}
}

// TestUnmarshalSkipsPlural: entradas com msgid_plural/msgstr[n] são
// ignoradas (o jogo não tem plurais) e NÃO derrubam o resto do arquivo.
func TestUnmarshalSkipsPlural(t *testing.T) {
	raw := []byte("#: ffx/battle/btl/bika00_10/bika00_10.bin:0\n" +
		"msgctxt \"bika00_10|0\"\n" +
		"msgid \"Whew!\"\n" +
		"msgstr \"Ufa!\"\n" +
		"\n" +
		"msgid \"one apple\"\n" +
		"msgid_plural \"many apples\"\n" +
		"msgstr[0] \"uma maçã\"\n" +
		"msgstr[1] \"muitas maçãs\"\n")
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := c["bika00_10"]
	if !ok {
		t.Fatalf("entrada ausente: %v", c.SortedKeys())
	}
	if len(entry.Rows) != 1 {
		t.Fatalf("rows: %d, queria 1 (o plural é ignorado)", len(entry.Rows))
	}
	if entry.Rows[0].Target != "Ufa!" {
		t.Fatalf("target errado: %q", entry.Rows[0].Target)
	}
}

// TestRoundTrip: marshal → unmarshal preserva coordenada, source e target.
func TestRoundTrip(t *testing.T) {
	raw, err := POFormatter{}.Marshal(sampleCollection())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	c, err := POFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := c["bika00_10"]
	if !ok {
		t.Fatalf("entrada ausente: %v", c.SortedKeys())
	}
	if entry.Metadata.Key != "ffx/battle/btl/bika00_10/bika00_10.bin" {
		t.Fatalf("key errada: %q", entry.Metadata.Key)
	}
	row := entry.Rows[0]
	if row.Index != 0 {
		t.Fatalf("index errado: %d", row.Index)
	}
	if row.Source != "{PC:00:TIDUS}{TEXT_NEWLINE}Whew!" {
		t.Fatalf("source errado: %q", row.Source)
	}
	if row.Target != "Ufa!" {
		t.Fatalf("target errado: %q", row.Target)
	}
	if row.Hash[common.DefaultLocalization] != sampleHash() {
		t.Fatalf("hash errado: %q", row.Hash[common.DefaultLocalization])
	}
	if row.Text["sp"] != "¡Ufa!" {
		t.Fatalf("referência sp perdida: %q", row.Text["sp"])
	}
}
