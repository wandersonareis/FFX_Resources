// Gerador: cria backend/core/encoding/charset_tables.go a partir dos .bin de
// encoding do testData, aplicando as correções de font (tabela abaixo).
//
// As correções são evidenciadas pela atlas de fonte do testData
// (font_0_0.png = slots pares, font_0_1.png = ímpares, grade 9×13):
// cada slot da tabela deve mapear ao glifo que o font desenha nele.
//
// Uso: go run ./scripts/gen_charset_tables
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// usSlotFixes corrige o conteúdo das tabelas us (font modificado: a-trema
// trocado por a-til) para refletir o que o font desenha (comprovado por
// recorte/comparação pixel a pixel da atlas):
//
//	0xA7: o gerador gravou ç minúsculo no bloco de maiúsculas; o font
//	      desenha Ç maiúsculo com cedilha (o ç minúsculo legítimo é 0xBE).
//	0x93: font desenha Œ — a tabela diz espaço (glifo vazio em 0x3A).
//	0x97: font desenha œ — idem.
//	0xA6: o font não tem a com til; o glifo do slot do Ä é Ã (troca feita
//	      no font do idioma us — cada idioma tem sua própria fonte).
//	0xBD: idem, minúsculo (ä → ã).
//	0xB4: idem para o O (Ö → Õ): o font us é a versão criada para o
//	      português — as vogais com til vivem nos slots do trema, que não
//	      existe em pt-BR. Complementa o par 0xA6/0xBD.
//	0xCB: idem, minúsculo (ö → õ).
//	0x3C: font desenha aspas RETAS; a tabela diz ” (curva). Com rune reto
//	      aqui, ” fica só em 0x95 (glifo inclinado) — sem duplicata.
//	0x41: font desenha apóstrofo reto; tabela diz ’ — mata duplicata com
//	      0xD5 (’ inclinado, usado 181× nos diálogos originais).
//
// Efeito: slots que antes duplicavam runes viram runes únicos, e o token
// {PUA:XX:CHAR} passa a aparecer apenas nas duplicatas genuínas do font
// (ex.: espaços extras 0xA0/0xDA/0xEA).
var usSlotFixes = map[uint]rune{
	0xA7: 'Ç',
	0x93: 'Œ',
	0x97: 'œ',
	0xA6: 'Ã',
	0xBD: 'ã',
	0xB4: 'Õ',
	0xCB: 'õ',
	0x3C: '"',
	0x41: '\'',
}

// usDefaultSlotFixes devolve as correções da tabela default: as mesmas do us
// EXCETO as trocas trema→til (0xA6/0xBD do A e 0xB4/0xCB do O) — os idiomas
// sp/fr/de/it têm fonts próprios com o glifo do trema intacto.
func usDefaultSlotFixes() map[uint]rune {
	m := make(map[uint]rune, len(usSlotFixes))
	for code, to := range usSlotFixes {
		m[code] = to
	}
	delete(m, 0xA6)
	delete(m, 0xBD)
	delete(m, 0xB4)
	delete(m, 0xCB)
	return m
}

type src struct {
	name    string // nome da constante Go
	version string // "FFX" | "FFX2"
	charset string
	path    string
	fixes   map[uint]rune // correções na fonte (nil = nenhuma)
}

func main() {
	root := `F:\ffxWails\FFX_Resources\testData`
	out := `F:\ffxWails\FFX_Resources\backend\core\encoding\charset_tables.go`
	var sources []src
	for _, cs := range []string{"ch", "cn", "jp", "kr", "us"} {
		var fixes map[uint]rune
		if cs == "us" {
			fixes = usSlotFixes
		}
		sources = append(sources, src{
			name:    "tableFFX" + upperFirst(cs),
			version: "FFX",
			charset: cs,
			path:    filepath.Join(root, "FFX", "binary", "ffx_ps2", "ffx", "master", "jppc", "ffx_encoding", "ffxsjistbl_"+cs+".bin"),
			fixes:   fixes,
		})
		sources = append(sources, src{
			name:    "tableFFX2" + upperFirst(cs),
			version: "FFX2",
			charset: cs,
			path:    filepath.Join(root, "FFX-2", "binary", "ffx_ps2", "ffx2", "master", "jppc", "ffx2_encoding", "ffx2sjistbl_"+cs+".bin"),
			fixes:   fixes,
		})
	}
	// Tabela "default": mesmo conteúdo do us com o a-trema intacto — para os
	// idiomas ocidentais cujo font não sofreu a troca do til (sp/fr/de/it).
	for _, g := range []struct{ name, version, tree, prefix string }{
		{"tableFFXDefault", "FFX", "FFX", "ffx"},
		{"tableFFX2Default", "FFX2", "FFX-2", "ffx2"},
	} {
		sources = append(sources, src{
			name:    g.name,
			version: g.version,
			charset: "default",
			path: filepath.Join(root, g.tree, "binary", "ffx_ps2", g.prefix, "master", "jppc",
				g.prefix+"_encoding", g.prefix+"sjistbl_us.bin"),
			fixes: usDefaultSlotFixes(),
		})
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("package ffxencoding\n\n")
	b.WriteString("import \"ffxresources/backend/common\"\n\n")
	b.WriteString(constsComment)
	b.WriteString("const (\n")
	for _, s := range sources {
		data, err := os.ReadFile(s.path)
		if err != nil {
			panic(err)
		}
		str := strings.TrimPrefix(string(data), "\ufeff")
		str = strings.TrimRight(str, "\r\n")
		runes := []rune(str)
		fixed := fmt.Sprintf(" // sem correções (%d slots)", len(runes))
		if s.fixes != nil {
			codes := make([]int, 0, len(s.fixes))
			for code := range s.fixes {
				codes = append(codes, int(code))
			}
			sort.Ints(codes)
			applied := ""
			for _, code := range codes {
				to := s.fixes[uint(code)]
				i := code - 0x30
				if i < 0 || i >= len(runes) {
					panic(fmt.Sprintf("%s: slot 0x%02X fora da tabela", s.name, code))
				}
				from := runes[i]
				runes[i] = to
				applied += fmt.Sprintf(" 0x%02X %c→%c", code, from, to)
			}
			fixed = fmt.Sprintf(" // fixes na fonte (%d slots):%s", len(runes), applied)
		}
		fmt.Fprintf(&b, "\t%s = %q%s\n", s.name, string(runes), fixed)
	}
	b.WriteString(")\n\n")
	b.WriteString(selector)

	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		panic(err)
	}
	fmt.Println("gerado:", out)
}

func upperFirst(s string) string {
	return fmt.Sprintf("%s%s", string(s[0]-('a'-'A')), s[1:])
}

const header = `// SPDX-License-Identifier: LGPL-3.0-or-later
//
// Este arquivo deriva das constantes de tabela do Fahrenheit
// (FhShiftJisTables, © 2023-2026 The Fahrenheit contributors, LGPL-3.0-or-later),
// que por sua vez constant-foldam as tabelas originais do jogo:
//
//	ffx_ps2/ffx/master/jppc/ffx_encoding/ffxsjistbl_{ch|cn|jp|kr|us}.bin
//	ffx_ps2/ffx2/master/jppc/ffx2_encoding/ffx2sjistbl_{ch|cn|jp|kr|us}.bin
//
// Correções na fonte estão documentadas em scripts/gen_charset_tables.go
// (usSlotFixes), evidenciadas pela atlas de fonte do testData
// (font_0_0.png/font_0_1.png). Regenere com: go run ./scripts/gen_charset_tables

`

const constsComment = `// Tabelas do encoding customizado do jogo ("sjistbl"). Cada rune da string é
// um slot: slot do jogo = 0x30 + índice. Correções aplicadas na fonte estão
// comentadas ao lado da constante; duplicatas restantes (glifo idêntico em
// dois slots) são resolvidas em runtime pelo token {PUA:XX:CHAR}
// (ver converter.ParseCommand).
`

const selector = `// getTable seleciona a tabela embutida para a versão e charset indicados.
// eternalcalm usa as tabelas do ffx e lastmiss as do ffx2 (mesmas árvores
// de encoding).
func getTable(version common.GameVersion, charset string) (string, bool) {
	if version == common.GameVersionEternalCalm {
		version = common.GameVersionFFX
	}
	if version == common.GameVersionLastMiss {
		version = common.GameVersionFFX2
	}
	if version != common.GameVersionFFX && version != common.GameVersionFFX2 {
		return "", false
	}
	if version == common.GameVersionFFX2 {
		switch charset {
		case "ch":
			return tableFFX2Ch, true
		case "cn":
			return tableFFX2Cn, true
		case "jp":
			return tableFFX2Jp, true
		case "kr":
			return tableFFX2Kr, true
		case "us":
			return tableFFX2Us, true
		case "default":
			return tableFFX2Default, true
		}
		return "", false
	}
	switch charset {
	case "ch":
		return tableFFXCh, true
	case "cn":
		return tableFFXCn, true
	case "jp":
		return tableFFXJp, true
	case "kr":
		return tableFFXKr, true
	case "us":
		return tableFFXUs, true
	case "default":
		return tableFFXDefault, true
	}
	return "", false
}
`
