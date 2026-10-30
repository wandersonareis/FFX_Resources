package formatters

import (
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
)

// SourceTarget devolve o par source/target de uma linha (row, idioma) para os
// formatos tabulares/.strings: só o idioma default (`us`) carrega o par — é
// ali que vive a tradução em mods/. O `target` é o ÚNICO texto que o import
// aplica de volta no binário.
func SourceTarget(r dto.TextRow, lang string) (source, target string) {
	if lang == common.DefaultLocalization {
		return r.Source, r.Target
	}
	return "", ""
}

// HasSourceTarget informa se a row pede a linha de `us` mesmo com o texto
// original vazio: tradução em mods/ sem referência (ex.: original "-") não
// pode sumir do artefato.
func HasSourceTarget(r dto.TextRow, lang string) bool {
	return lang == common.DefaultLocalization && (r.Source != "" || r.Target != "")
}
