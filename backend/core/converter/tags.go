package converter

import (
	"fmt"
	"sort"
	"strings"
)

// IsTextTag classifica uma tag {..} como formatação pura de texto: ausência,
// troca ou excesso na tradução NÃO é erro — o tradutor pode mudar a cor,
// aplicar itálico onde não tem, e o \n é sempre ajustado para quebrar linha
// no ponto certo do parágrafo.
//
// É ALLOWLIST: qualquer tag desconhecida ou futura nasce como CONTROLE e
// entra na igualdade. As tags de texto são exatamente as de ParseCommand que
// não tocam no fluxo do diálogo: TEXT_ITALIC/TEXT_NORMAL, \n/TEXT_NEWLINE e
// CLR:/COLOR:.
func IsTextTag(cmd string) bool {
	switch {
	case cmd == "TEXT_ITALIC", cmd == "TEXT_NORMAL":
		return true
	case cmd == "\\n", cmd == "TEXT_NEWLINE":
		return true
	case strings.HasPrefix(cmd, "CLR:"), strings.HasPrefix(cmd, "COLOR:"):
		return true
	}
	return false
}

// TagIssue é uma divergência de tag de controle entre o original e a
// tradução. Content é o conteúdo entre chaves ("ICON:01:02"); First é o
// primeiro segmento antes de ':' ("ICON"), a identidade da tag, que no log
// separa "parâmetro alterado" (mesmo First) de "tag renomeada/traduzida"
// (First diferente). Delta é contagem(original) − contagem(tradução):
// positivo = falta na tradução, negativo = sobra.
type TagIssue struct {
	Content string
	First   string
	Delta   int
}

// String formata a issue para o log.
func (i TagIssue) String() string {
	if i.Delta > 0 {
		return fmt.Sprintf("falta %d × {%s}", i.Delta, i.Content)
	}
	return fmt.Sprintf("sobra %d × {%s}", -i.Delta, i.Content)
}

// ControlTagDiff compara as tags {..} de controle de dois textos por
// IGUALDADE de multiconjunto: mesmo conteúdo entre chaves, mesma contagem.
//
// A ORDEM NÃO ENTRA na comparação — e não é omisson: a tag precisa mudar de
// lugar conforme a ordem das palavras do alvo. VAR representa dados criados
// na sessão, e a mesma frase sai em inglês de três formas:
//
//	"Pegou VAR itens na missão" / "A missão foi completada com VAR itens."
//	/"VAR itens foram obtidos na missão"
//
// Comparar sequência transformaria essas três em erro.
//
// Tags de texto são filtradas antes (IsTextTag). Devolve nil quando as duas
// contagens são idênticas em todo conteúdo de controle.
func ControlTagDiff(orig, have string) []TagIssue {
	o := tagCounts(orig)
	h := tagCounts(have)

	keys := make([]string, 0, len(o)+len(h))
	for k := range o {
		keys = append(keys, k)
	}
	for k := range h {
		if _, ok := o[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var out []TagIssue
	for _, k := range keys {
		if d := o[k] - h[k]; d != 0 {
			out = append(out, TagIssue{Content: k, First: firstSegment(k), Delta: d})
		}
	}
	return out
}

// firstSegment devolve o primeiro segmento de uma tag (antes de ':'): em
// "ICON:01:02" → "ICON", em "PAUSE" → "PAUSE".
func firstSegment(cmd string) string {
	if i := strings.IndexByte(cmd, ':'); i >= 0 {
		return cmd[:i]
	}
	return cmd
}

// tagCounts conta as tags de controle de um texto: conteúdo entre chaves →
// ocorrências. Tags de texto são ignoradas.
//
// O varredor espelha o de FillByteList: procura '{', acha o '}' seguinte e
// pula para depois dele. '{' sem fechamento não é tag — o ParseCommand
// também devolve nil nesse caso e o caractere vira texto literal.
func tagCounts(s string) map[string]int {
	var out map[string]int
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '{' {
			continue
		}
		end := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == '}' {
				end = j
				break
			}
		}
		if end < 0 {
			continue // '{' literal: segue varrendo a partir dele
		}
		cmd := string(runes[i+1 : end])
		if !IsTextTag(cmd) {
			cmd = normalizeControlTag(cmd)
			if out == nil {
				out = make(map[string]int, 4)
			}
			out[cmd]++
		}
		i = end
	}
	return out
}

// normalizeControlTag remove o payload de DISPLAY das tags de macro:
// `MCR:s06:l31:"Belgemine"` → `MCR:s06:l31`. O trecho entre aspas só existe
// quando o dicionário está carregado (writeMacro), não é gravado no binário e,
// portanto, não pode entrar na igualdade de tags de controle: o MESMO byte
// decodifica com e sem payload em sessões diferentes (export sem macros importa
// com macros) e o texto do dicionário difere por idioma.
func normalizeControlTag(cmd string) string {
	if m := reMCR.FindStringSubmatchIndex(cmd); m != nil {
		return strings.TrimSuffix(cmd[:m[1]], ":")
	}
	return cmd
}
