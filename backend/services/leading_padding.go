package services

import (
	"regexp"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
)

// leadingPaddingRe é o espelho Go de LEADING_PADDING_RE (frontend
// src/lib/ffx/leading-padding.ts): a sequência contínua de espaços/tabs/\n/\r
// e {TEXT_NEWLINE} (case-insensitive, qualquer quantidade) com que o jogo
// alinha o texto na tela. Qualquer outra tag ({TIME:00}, {ICON:…}) encerra o
// padding — não é alinhamento, é conteúdo.
var leadingPaddingRe = regexp.MustCompile(`(?i)^(?:\s|\{TEXT_NEWLINE\})+`)

// splitLeadingPadding separa o "padding de tela" do início de um texto
// canônico. Só o prefixo importa: a indentação de linhas DEPOIS do primeiro
// conteúdo é corpo visível (o editor a mostra ao tradutor), fora deste
// conceito. O valor canônico serializado nunca perde o padding — aqui é só
// leitura, para o import reconciliar texto novo com o padding da store.
func splitLeadingPadding(raw string) (padding, body string) {
	if raw == "" {
		return "", ""
	}
	loc := leadingPaddingRe.FindStringIndex(raw)
	if loc == nil {
		return "", raw
	}
	return raw[:loc[1]], raw[loc[1]:]
}

// keepStorePadding reconcilia o padding de tela de um texto importado com o
// da row vigente na store — o "padding oculto" do editor:
//
//   - store COM padding: o padding da store se mantém e só o corpo visível
//     do import entra. O padding que o import trouxer — a menos ou a mais —
//     é descartado: o tradutor nunca vê o padding (o editor o esconde),
//     então não há intenção dele em mudá-lo.
//   - store SEM padding: o texto importado vence integral.
//
// Nunca consulta o original — a store é a única fonte, e ela já está na mão
// no casamento por hash. Diferença só de padding não é texto alterado.
func keepStorePadding(storeText, importedText string) string {
	sp, _ := splitLeadingPadding(storeText)
	if sp == "" {
		return importedText
	}
	_, ib := splitLeadingPadding(importedText)
	return sp + ib
}

// reconcileImportPadding aplica keepStorePadding na row importada casada e
// devolve a row com o TARGET no texto 'us' — é esse valor que o merge grava
// no binário. Sem target, a row volta intacta (nada a importar).
func reconcileImportPadding(storeRow, importedRow dto.TextRow) dto.TextRow {
	t := importText(importedRow)
	if t == "" {
		return importedRow
	}
	rt := keepStorePadding(usText(storeRow), t)
	text := make(map[string]string, len(importedRow.Text)+1)
	for k, v := range importedRow.Text {
		text[k] = v
	}
	text[common.DefaultLocalization] = rt
	importedRow.Text = text
	return importedRow
}
