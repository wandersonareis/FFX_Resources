package converter

import "testing"

// Matriz da validação de tags de controle (data/ ou VBF):
// T-b def != def igual, T-c falta, T-d sobra, T-e trocada, T-f ausência
// vs igual, T-g multiconjunto ignora ordem, T-i cor/variação textual,
// T-j mídia etc. são CONTROLE (allowlist: só formatação é texto).
func TestControlTagDiff(t *testing.T) {
	cases := []struct {
		name      string
		orig, hav string
		want      []TagIssue
	}{
		{"iguais", "Pegou {VAR:12} itens", "Pegou {VAR:12} itens", nil},
		{"sem tags dos dois lados", "texto puro", "tradução", nil},
		{"falta controle", "Pegou {VAR:12} itens {PAUSE}", "Pegou {VAR:12} itens",
			[]TagIssue{{Content: "PAUSE", First: "PAUSE", Delta: 1}}},
		{"sobra controle", "Pegou {VAR:12} itens", "Pegou {VAR:12} itens {PAUSE} {PAUSE}",
			[]TagIssue{{Content: "PAUSE", First: "PAUSE", Delta: -2}}},
		{"trocada", "Missão {ICON:01:02} completa", "Missão {ICON:02:01} completa",
			[]TagIssue{{Content: "ICON:01:02", First: "ICON", Delta: 1},
				{Content: "ICON:02:01", First: "ICON", Delta: -1}}},
		{"ordem não importa", "{PAUSE} A {VAR:1} B", "A {VAR:1} B {PAUSE}", nil},
		{"cor mudou é texto", "A {CLR:0001}wow{CLR:0000}", "A {CLR:0002}wow", nil},
		{"newline/text tag não conta", "A {\\n} {TEXT_ITALIC}B{TEXT_NORMAL}", "A{TEXT_NEWLINE}B", nil},
		{"mídia é controle", "ver {VAR:12} fim", "ver fim",
			[]TagIssue{{Content: "VAR:12", First: "VAR", Delta: 1}}},
		{"paramentro da tag mudou é trocada", "Missão {ICON:01:A} completa", "Missão {ICON:02:B} completa",
			[]TagIssue{{Content: "ICON:01:A", First: "ICON", Delta: 1},
				{Content: "ICON:02:B", First: "ICON", Delta: -1}}},
		{"chave não fechada é literal igual nos dois lados", "a {PAUSE b c", "a {PAUSE b x", nil},
		// O payload entre aspas da MCR é só display (writeMacro) e não é
		// gravado no binário: com/sem dicionário é a MESMA tag.
		{"MCR com payload vs sem payload", `{MCR:s06:l31:"Belgemine"}Olá`, "{MCR:s06:l31}Olá", nil},
		{"MCR payload diferente por idioma", `{MCR:s06:l31:"Belgemine"}Olá`, `{MCR:s06:l31:"Belgemine"}Oi`, nil},
		{"MCR de outra seção continua divergente", `{MCR:s06:l31:"Belgemine"}Olá`, `{MCR:s06:l32:"Belgemine"}Olá`,
			[]TagIssue{{Content: "MCR:s06:l31", First: "MCR", Delta: 1},
				{Content: "MCR:s06:l32", First: "MCR", Delta: -1}}},
		{"MCR com payload e sem dicionário", `{MCR:s06:l31:"Belgemine"}Olá`, "{MCR:s06:l31:<Missing>}Olá", nil},
	}
	for _, tc := range cases {
		got := ControlTagDiff(tc.orig, tc.hav)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %+v, queria %+v", tc.name, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: issue %d = %+v, queria %+v", tc.name, i, got[i], tc.want[i])
			}
		}
	}
}

func TestIsTextTag(t *testing.T) {
	for _, c := range []string{"TEXT_ITALIC", "TEXT_NORMAL", "\\n", "TEXT_NEWLINE", "CLR:0001", "COLOR:12:34"} {
		if !IsTextTag(c) {
			t.Fatalf("IsTextTag(%q) = false, queria true", c)
		}
	}
	for _, c := range []string{"PAUSE", "VAR:12", "ICON:01:02", "WAV:sound", "CHR:x"} {
		if IsTextTag(c) {
			t.Fatalf("IsTextTag(%q) = true, queria false (controle)", c)
		}
	}
}
