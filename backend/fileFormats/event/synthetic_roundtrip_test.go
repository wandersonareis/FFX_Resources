// Binários SINTÉTICOS para validar decode+encode do codec events (stride 8).
//
// Os binários do jogo não são commitados (zona cinza): a garantia permanente
// do codec vem DAQUI — tabelas montadas em código, uma por particularidade do
// formato, todas com a mesma tríplice de asserção:
//
//  1. fidelidade por string — encode(decode(bytes)) == bytes (regular e
//     simplified): o conteúdo nunca muda ao passar pelo charset;
//  2. round-trip SEMÂNTICO — parse → rebuild → re-parse devolve as MESMAS
//     strings, na mesma ordem: o que o jogo lê não muda. É a asserção que
//     pega a classe do bug de linha vazia sem terminador próprio (a row
//     vazia voltava com o texto da primeira string);
//  3. ponto fixo — o arquivo que o writer gravou, relido e regravado, sai
//     byte a byte igual: o save nunca deriva.
//
// O arquivo REPLICA o original? Não precisa — e não deve ser exigido: o
// formato é uma gambiarra de ponteiros trazida do PS2, e otimizações de
// layout dentro do arquivo (offsets compartilhados no dedupe de
// armazenamento) são aceitáveis. O que a garantia cobre é o conteúdo; o
// resto se valida jogando.
package event_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/fileFormats/event"
)

const synthCharset = "us"

var synthVersion = common.GameVersionFFX

func prepareCharsets(t *testing.T) {
	t.Helper()
	if err := ffxencoding.PrepareVersionCharsets(synthVersion); err != nil {
		t.Fatalf("charsets: %v", err)
	}
}

// synthEntry é uma row sintética: os textos viram bytes pelo próprio
// charset do app (o que se valida é decode→encode, não a proveniência do
// byte).
type synthEntry struct {
	regular     string
	simplified  string // vazio ⇒ mesmo offset do regular
	regFlags    int
	regChoices  int
	simpFlags   int
	simpChoices int
}

// montaOriginal grava a tabela NO LAYOUT DO JOGO: cada row com offset próprio
// (sem dedup de armazenamento), linha vazia com terminador próprio — o
// formato que data/ usa (ex.: menumain.bin com 118 offsets distintos).
//
// Toda entry é conferida na hora: encode(texto) tem que decodificar de volta
// no MESMO texto — fixture com caractere fora do charset falha aqui, não
// silenciosamente no meio do round-trip. O charset us cobre o inventário
// de acentos do pt-BR pelas vogais com til (ã/õ nos slots do trema — ver
// usSlotFixes em scripts/gen_charset_tables).
func montaOriginal(t *testing.T, entries []synthEntry) []byte {
	t.Helper()
	contentOffset := len(entries) * 8
	var content bytes.Buffer
	headers := make([]byte, 0, contentOffset)
	for _, e := range entries {
		assertTextoSobrevive(t, e.regular)
		if e.simplified != "" && e.simplified != e.regular {
			assertTextoSobrevive(t, e.simplified)
		}
		reg := contentOffset + content.Len()
		regBytes, err := converter.StringToBytes(e.regular, synthCharset, synthVersion)
		if err != nil {
			t.Fatalf("codificar %q: %v", e.regular, err)
		}
		content.Write(regBytes)
		content.WriteByte(0x00)

		simp := reg // simplified vazio: os dois lados no mesmo lugar
		if e.simplified != "" && e.simplified != e.regular {
			simp = contentOffset + content.Len()
			simpBytes, err := converter.StringToBytes(e.simplified, synthCharset, synthVersion)
			if err != nil {
				t.Fatalf("codificar simplified %q: %v", e.simplified, err)
			}
			content.Write(simpBytes)
			content.WriteByte(0x00)
		}

		h1 := make([]byte, 4)
		binary.LittleEndian.PutUint32(h1, uint32(reg|e.regFlags<<16|e.regChoices<<24))
		h2 := make([]byte, 4)
		binary.LittleEndian.PutUint32(h2, uint32(simp|e.simpFlags<<16|e.simpChoices<<24))
		headers = append(headers, h1...)
		headers = append(headers, h2...)
	}
	return append(headers, content.Bytes()...)
}

// montaBinario remonta o arquivo como o writer do codec: cabeçalhos das rows
// + conteúdo reconstruído.
func montaBinario(rows []*event.FieldString, content []byte) []byte {
	var buf bytes.Buffer
	for _, fs := range rows {
		buf.Write(fs.ToRegularHeaderBytes())
		buf.Write(fs.ToSimplifiedHeaderBytes())
	}
	buf.Write(content)
	return buf.Bytes()
}

// assertTextoSobrevive: o texto do fixture tem que sobreviver ao charset —
// encode→decode devolve o MESMO texto. Caractere que o encode derruba
// (LogWarning "Unknown character") seria engolido pelo round-trip, que só
// confere autoconsistência; aqui a fixture amarra o contrato.
func assertTextoSobrevive(t *testing.T, s string) {
	t.Helper()
	enc, err := converter.StringToBytes(s, synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("codificar %q: %v", s, err)
	}
	if dec := converter.BytesToString(enc, synthCharset, synthVersion); dec != s {
		t.Fatalf("texto do fixture não sobrevive ao charset:\n entrada: %q\n decodado: %q", s, dec)
	}
}

// roundTripSynthetico roda a tríplice de asserções sobre um binário.
func roundTripSynthetico(t *testing.T, raw []byte) {
	t.Helper()
	rows, err := event.FromFieldStringData(raw, synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// (1) fidelidade por string: encode(decode(bytes)) == bytes.
	for i, fs := range rows {
		assertFidelidadePorString(t, i, fs)
	}

	// (2) round-trip semântico: as strings do jogo não mudam.
	rebuilt := montaBinario(rows, event.RebuildFieldStrings(rows, synthCharset, synthVersion))
	rows2, err := event.FromFieldStringData(rebuilt, synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("releitura do regravado: %v", err)
	}
	assertMesmasStrings(t, rows, rows2)

	// (3) ponto fixo: regravar o regravado sai byte a byte igual.
	rebuilt2 := montaBinario(rows2, event.RebuildFieldStrings(rows2, synthCharset, synthVersion))
	if !bytes.Equal(rebuilt, rebuilt2) {
		i := 0
		for i < len(rebuilt) && i < len(rebuilt2) && rebuilt[i] == rebuilt2[i] {
			i++
		}
		t.Fatalf("ponto fixo violado: 1º save=%d bytes, 2º save=%d, 1º desvio em %#x", len(rebuilt), len(rebuilt2), i)
	}
}

// assertMesmasStrings: mesma contagem e mesmas strings (regular e
// simplified), row a row — o que o jogo lê não mudou.
func assertMesmasStrings(t *testing.T, antes, depois []*event.FieldString) {
	t.Helper()
	if len(antes) != len(depois) {
		t.Fatalf("contagem de rows mudou: %d -> %d", len(antes), len(depois))
	}
	for i := range antes {
		if antes[i].GetRegularString() != depois[i].GetRegularString() {
			t.Errorf("row %d: regular mudou no round-trip:\n antes: %q\n depois: %q",
				i, antes[i].GetRegularString(), depois[i].GetRegularString())
		}
		if antes[i].GetSimplifiedString() != depois[i].GetSimplifiedString() {
			t.Errorf("row %d: simplified mudou no round-trip:\n antes: %q\n depois: %q",
				i, antes[i].GetSimplifiedString(), depois[i].GetSimplifiedString())
		}
	}
}

// assertFidelidadePorString: encode(decode(bytes)) == bytes nos dois lados.
func assertFidelidadePorString(t *testing.T, i int, fs *event.FieldString) {
	t.Helper()
	enc, err := converter.StringToBytes(fs.GetRegularString(), synthCharset, synthVersion)
	if err != nil {
		t.Errorf("row %d: encode regular: %v (%q)", i, err, fs.GetRegularString())
	} else if !bytes.Equal(enc, fs.RegularBytes) {
		t.Errorf("row %d: encode(decode(bytes)) != bytes (regular): %q", i, fs.GetRegularString())
	}
	encS, err := converter.StringToBytes(fs.GetSimplifiedString(), synthCharset, synthVersion)
	if err != nil {
		t.Errorf("row %d: encode simplified: %v (%q)", i, err, fs.GetSimplifiedString())
	} else if !bytes.Equal(encS, fs.SimplifiedBytes) {
		t.Errorf("row %d: encode(decode(bytes)) != bytes (simplified): %q", i, fs.GetSimplifiedString())
	}
}

// TestSyntheticRoundTripIntacto cobre o arquivo com TODAS as particularidades
// de uma vez: tags, acentos, linhas vazias no meio, repetição sem dedup
// (cópias próprias, como data/ grava), simplified distinto e flags/choices no
// header. O writer pode otimizar o layout (offsets compartilhados) — as
// strings é que não podem mudar.
func TestSyntheticRoundTripIntacto(t *testing.T) {
	prepareCharsets(t)
	entries := []synthEntry{
		{regular: "{TIME:00}Select {CLR:YELLOW}Sphere Grid{CLR:WHITE} from the Main Menu."},
		{regular: "{TIME:00}Use {BUTTON:48:Direcional LEFT}{BUTTON:41:Direcional UP} para{TEXT_NEWLINE}mover o cursor."},
		{regular: ""}, // linha vazia no meio, terminador próprio
		{regular: "{TIME:00}As informações serão mostradas na parte de cima.",
			simplified: "{TIME:00}Versão simplificada distinta da regular.",
			regFlags:   1, regChoices: 2, simpFlags: 3, simpChoices: 4},
		{regular: "{TIME:00}{PC:00:TIDUS} tem 4 {CLR:YELLOW}Nv. E{CLR:WHITE},{TEXT_NEWLINE}então ele pode mover até {CLR:YELLOW}4{CLR:WHITE} passos."},
		{regular: "{TIME:00}Repetição: a mesma frase aparece duas vezes, em cópias próprias."},
		{regular: ""},
		{regular: "{TIME:00}Repetição: a mesma frase aparece duas vezes, em cópias próprias."}, // repetida, offset próprio
		{regular: "{TIME:00}Fim.", regFlags: 7, regChoices: 9},
	}
	roundTripSynthetico(t, montaOriginal(t, entries))
}

// TestSyntheticRoundTripTudoVazio é o caso do arquivo só de linhas vazias (o
// new_jppc de battle/btl/menumain na natureza): cada row com terminador
// próprio — a linha vazia NÃO pode voltar com o texto da primeira string.
func TestSyntheticRoundTripTudoVazio(t *testing.T) {
	prepareCharsets(t)
	entries := make([]synthEntry, 8)
	for i := range entries {
		entries[i] = synthEntry{}
	}
	raw := montaOriginal(t, entries)
	roundTripSynthetico(t, raw)

	// regressão pontual do bug do escritor antigo: a linha vazia lê VAZIA no
	// arquivo regravado — o offset dela aponta para um terminador real (0x00),
	// nunca para o começo do texto de outra linha.
	rebuilt := montaBinario(mustParse(t, raw), event.RebuildFieldStrings(mustParse(t, raw), synthCharset, synthVersion))
	rows2 := mustParse(t, rebuilt)
	for i, fs := range rows2 {
		if fs.GetRegularString() != "" {
			continue
		}
		if fs.RegularOffset >= len(rebuilt) || rebuilt[fs.RegularOffset] != 0x00 {
			t.Errorf("linha %d vazia não aponta para terminador próprio (offset=%d)", i, fs.RegularOffset)
		}
	}
}

// TestSyntheticRoundTripDedupCompartilhado: o jogo TAMBÉM grava repetições
// com offset compartilhado — o writer preserva o compartilhamento no dedupe
// de armazenamento e as strings seguem iguais.
func TestSyntheticRoundTripDedupCompartilhado(t *testing.T) {
	prepareCharsets(t)
	texto, err := converter.StringToBytes("{TIME:00}Frase compartilhada.", synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("codificar: %v", err)
	}
	// 3 rows: row 0 e row 2 apontam para o MESMO offset; row 1 tem conteúdo próprio.
	var content bytes.Buffer
	offA := 3*8 + content.Len()
	content.Write(texto)
	content.WriteByte(0x00)
	offB := 3*8 + content.Len()
	content.Write([]byte("própria"))
	content.WriteByte(0x00)

	headers := make([]byte, 0, 24)
	for _, off := range []int{offA, offB, offA} {
		h1 := make([]byte, 4)
		binary.LittleEndian.PutUint32(h1, uint32(off))
		h2 := make([]byte, 4)
		binary.LittleEndian.PutUint32(h2, uint32(off)) // simplified == regular
		headers = append(headers, h1...)
		headers = append(headers, h2...)
	}
	roundTripSynthetico(t, append(headers, content.Bytes()...))
}

// TestSyntheticEdicaoPontual: uma row editada ⇒ as outras preservam os bytes
// originais no save, a edição aparece no arquivo e o segundo save é ponto
// fixo.
func TestSyntheticEdicaoPontual(t *testing.T) {
	prepareCharsets(t)
	entries := []synthEntry{
		{regular: "{TIME:00}Primeira linha com acentos: informações, posição, número."},
		{regular: "{TIME:00}Segunda linha com {CLR:YELLOW}tag{CLR:WHITE} e {TEXT_NEWLINE}quebra."},
		{regular: "{TIME:00}Terceira, que será editada."},
		{regular: "{TIME:00}Repetição da primeira? Não: texto comum de menu."},
		{regular: ""},
	}
	raw := montaOriginal(t, entries)
	rows, err := event.FromFieldStringData(raw, synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	const editado = "{TIME:00}Terceira linha EDITADA, com acentuação: você verá."
	rows[2].SetRegularString(editado, synthCharset)

	rebuilt := montaBinario(rows, event.RebuildFieldStrings(rows, synthCharset, synthVersion))
	rows2, err := event.FromFieldStringData(rebuilt, synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("releitura: %v", err)
	}
	if got := rows2[2].GetRegularString(); got != editado {
		t.Fatalf("row editada não gravou: %q", got)
	}
	for i := range rows {
		if i == 2 {
			continue
		}
		if !bytes.Equal(rows[i].RegularBytes, rows2[i].RegularBytes) {
			t.Errorf("row %d intocada mudou de bytes no save da edição: %q -> %q",
				i, rows[i].GetRegularString(), rows2[i].GetRegularString())
		}
	}

	// Ponto fixo: reler e regravar sem editar reproduz o mesmo binário.
	rebuilt2 := montaBinario(rows2, event.RebuildFieldStrings(rows2, synthCharset, synthVersion))
	if !bytes.Equal(rebuilt, rebuilt2) {
		i := 0
		for i < len(rebuilt) && i < len(rebuilt2) && rebuilt[i] == rebuilt2[i] {
			i++
		}
		t.Fatalf("segundo save não é ponto fixo: %d vs %d bytes, 1º desvio em %#x", len(rebuilt), len(rebuilt2), i)
	}
}

// TestSyntheticAcentosCharsetName: o charset us é bijetivo no inventário de
// acentos do pt-BR — as vogais com til vivem nos slots do trema (ã=slot do
// ä, Õ/õ=slots do Ö/ö: o font us é a versão criada para o português, então
// trema não existe na tabela us). O inventário com trema é contrato da
// tabela DEFAULT (TestAcentosDefaultCharsetName) — alemão e cia.
func TestSyntheticAcentosCharsetName(t *testing.T) {
	prepareCharsets(t)
	for _, ch := range "áàâãçéêíóôõúüÁÀÂÃÇÉÊÍÓÔÕÚÜ" {
		s := string(ch)
		enc, err := converter.StringToBytes(s, synthCharset, synthVersion)
		if err != nil {
			t.Errorf("encode %q: %v", s, err)
			continue
		}
		if dec := converter.BytesToString(enc, synthCharset, synthVersion); dec != s {
			t.Errorf("round-trip %q: decodou %q", s, dec)
		}
	}
}

// TestAcentosDefaultCharsetName: a tabela default (sp/fr/de/it — fonts com o
// glifo do trema intacto) mantém ä/ö/ü bijetivos. O alemão usa trema de
// verdade; a troca por til é exclusiva do par de tabelas us.
func TestAcentosDefaultCharsetName(t *testing.T) {
	prepareCharsets(t)
	for _, ch := range "äöüÄÖÜß" {
		s := string(ch)
		enc, err := converter.StringToBytes(s, "default", synthVersion)
		if err != nil {
			t.Errorf("encode %q: %v", s, err)
			continue
		}
		if dec := converter.BytesToString(enc, "default", synthVersion); dec != s {
			t.Errorf("round-trip %q: decodou %q", s, dec)
		}
	}
}

func mustParse(t *testing.T, raw []byte) []*event.FieldString {
	t.Helper()
	rows, err := event.FromFieldStringData(raw, synthCharset, synthVersion)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return rows
}
