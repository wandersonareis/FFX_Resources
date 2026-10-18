// Layout da tabela de strings: o que e gravado e em que ordem.
//
// Separa a decisao de POSICIONAMENTO (dedupe por conteudo, linha vazia com
// terminador proprio) da conversao string para bytes, que fica no converter.
// O layout devolve INDICES de emissao: o tamanho em bytes so existe depois
// que o converter grava, entao nao e assunto aqui.
//
// Dedupe aqui e ECONOMIA DE ARMAZENAMENTO (offsets compartilhados, como no
// original do jogo) e nao tem relacao com o dedupe de traducao (refs de
// backend/builders), que acontece no DTO bem antes de chegar no binario.
package event

// fieldSlot e a posicao de um dos lados de uma linha: o indice da emissao
// que detem os bytes dele.
type fieldSlot int

// fieldLayout devolve, por linha, o slot de regular/simplified e a lista de
// textos a gravar (cada um com terminador), na ordem do arquivo.
//
// Regras:
//   - texto nao-vazio: a 1a ocorrencia emite e guarda o indice; repeticoes
//     reaproveitam (dedupe de armazenamento);
//   - texto vazio: emite o terminador sozinho (indice proprio) - apontar para
//     o inicio do conteudo faria a carga ler a PRIMEIRA string;
//   - linha com os DOIS lados vazios: um terminador so, o mesmo indice.
func fieldLayout(regular, simplified []string) (reg, simp []fieldSlot, emit []string) {
	seen := make(map[string]fieldSlot)
	place := func(t string) fieldSlot {
		if t != "" {
			if s, ok := seen[t]; ok {
				return s
			}
		}
		s := fieldSlot(len(emit))
		emit = append(emit, t)
		if t != "" {
			seen[t] = s
		}
		return s
	}

	reg = make([]fieldSlot, len(regular))
	simp = make([]fieldSlot, len(regular))
	for i := range regular {
		reg[i] = place(regular[i])
		if i >= len(simplified) {
			continue
		}
		// Os dois lados vazios: um terminador so (o original grava os
		// offsets iguais). Simplified vazio com regular preenchido tem o
		// terminador proprio.
		if simplified[i] == "" && regular[i] == "" {
			simp[i] = reg[i]
			continue
		}
		simp[i] = place(simplified[i])
	}
	return reg, simp, emit
}
