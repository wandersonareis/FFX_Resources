package ffxencoding

import "ffxresources/backend/common"

// localizationMap mapeia só os idiomas com tabela de encoding dedicada.
// O jogo tem jp, en, fr, sp, de, it, kr, ch (e cn carregado como charset),
// mas as tabelas sjistbl existem apenas para ch/jp/kr/us: os idiomas
// ocidentais compartilham tabelas — us (a-til, font do us modificado) e
// default (a-trema, fonts de sp/fr/de/it com o glifo do trema intacto).
var localizationMap = map[string]string{
	"ch": "ch",
	"kr": "kr",
	"jp": "jp",
}

// defaultTableLanguages são os idiomas ocidentais cujo font preserva o
// a-trema (a troca trema→til foi feita apenas no font do us).
var defaultTableLanguages = map[string]bool{
	"sp": true,
	"fr": true,
	"de": true,
	"it": true,
}

// GetCharsetForLanguage resolve o charset de um idioma do jogo:
//   - jp/kr/ch têm tabela dedicada;
//   - sp/fr/de/it usam a tabela default (a-trema);
//   - us/en e desconhecidos usam a tabela us (a-til).
func GetCharsetForLanguage(languageCode string) string {
	if charset, ok := localizationMap[languageCode]; ok {
		return charset
	}
	if defaultTableLanguages[languageCode] {
		return "default"
	}
	return "us"
}

// ResolveCharset devolve o bucket de charset para o parâmetro que os codecs
// de texto recebem: um NOME de charset válido (us, default, cn, ...) passa
// direto; um código de idioma (sp, fr, de, it, en) é resolvido como sempre.
//
// Sem isso, "default" — o charset de sp/fr/de/it — decodificava pela tabela
// us: o font desses idiomas tem o trema intacto (alemão usa ä/ö/ü de
// verdade), e o texto decodificava com o til do font us. O encode já usava
// o bucket direto; só o decode re-resolvia.
func ResolveCharset(localization string) string {
	for _, cs := range common.Charsets {
		if localization == cs {
			return localization
		}
	}
	return GetCharsetForLanguage(localization)
}
