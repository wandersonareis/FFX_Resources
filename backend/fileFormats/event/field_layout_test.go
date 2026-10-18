package event

import "testing"

// O layout decide POSICIONAMENTO: dedupe por conteudo (1a ocorrencia emite,
// repeticao reaproveita), linha vazia com terminador proprio e os dois lados
// vazios compartilhando um unico terminador. Sem conversao de bytes aqui.
func TestFieldLayout(t *testing.T) {
	cases := []struct {
		name       string
		regular    []string
		simplified []string
		wantEmit   []string
		wantReg    []int
		wantSimp   []int
	}{
		{
			name:       "repetido reaproveita a emissao",
			regular:    []string{"A", "B", "A"},
			simplified: []string{"A", "B", "A"},
			wantEmit:   []string{"A", "B"},
			wantReg:    []int{0, 1, 0},
			wantSimp:   []int{0, 1, 0},
		},
		{
			name:       "linha vazia tem emissao propria",
			regular:    []string{"A", "", "B"},
			simplified: []string{"A", "", "B"},
			wantEmit:   []string{"A", "", "B"},
			wantReg:    []int{0, 1, 2},
			wantSimp:   []int{0, 1, 2},
		},
		{
			name:       "dois lados vazios usam o mesmo terminador",
			regular:    []string{"A", ""},
			simplified: []string{"A", ""},
			wantEmit:   []string{"A", ""},
			wantReg:    []int{0, 1},
			wantSimp:   []int{0, 1},
		},
		{
			name:       "simplified vazio com regular preenchido nao herda o texto",
			regular:    []string{"A"},
			simplified: []string{""},
			wantEmit:   []string{"A", ""},
			wantReg:    []int{0},
			wantSimp:   []int{1},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reg, simp, emit := fieldLayout(c.regular, c.simplified)
			if len(emit) != len(c.wantEmit) {
				t.Fatalf("emit: %v, esperado %v", emit, c.wantEmit)
			}
			for i, want := range c.wantEmit {
				if emit[i] != want {
					t.Fatalf("emit[%d] = %q, esperado %q", i, emit[i], want)
				}
			}
			for i, want := range c.wantReg {
				if int(reg[i]) != want {
					t.Fatalf("reg[%d] = %d, esperado %d", i, reg[i], want)
				}
			}
			for i, want := range c.wantSimp {
				if int(simp[i]) != want {
					t.Fatalf("simp[%d] = %d, esperado %d", i, simp[i], want)
				}
			}
		})
	}
}
