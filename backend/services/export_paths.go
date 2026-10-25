// Caminhos de artefato por escopo de export: o dedup do marshal é por
// arquivo (mapa `seen` local à serialização), então o NOME do artefato
// precisa refletir o escopo do pedido — um subconjunto nunca pode colidir
// com o bulk completo (sobrescreveria o arquivo completo do tradutor).
package services

import (
	"fmt"
	"path/filepath"

	"ffxresources/backend/common"
	jsonfmt "ffxresources/backend/formatters/json"
)

// treeBulkPrefix prefixa o base do artefato bulk gerado pela SELEÇÃO
// completa de um kind na árvore (checkbox da raiz). Separa o artefato
// vindo da árvore do canônico do export completo (mesmo conteúdo, outra
// origem) sem colidir com ele — e sem depender do nome no import, que
// casa o kind pelo metadata.key das entradas.
const treeBulkPrefix = "tree_"

// hasBulkArtifact informa se o kind tem artefato bulk ÚNICO (ids vazios =
// um arquivo por kind). lockit não tem: é sempre por entrada, então a
// marcação "kind completo" nele segue o caminho normal da seleção.
func hasBulkArtifact(kind string) bool {
	switch kind {
	case KindEvents, KindMacro, KindHelp, KindObjects,
		KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		return true
	}
	return false
}

// eventsExportPath resolve o caminho do artefato JSON de events:
//   - ids vazio (tudo)     → [<prefix>]events_all_localizations[_<v>].json
//   - 1 entrada            → event_<id>_all_localizations[_<v>].json
//   - subconjunto          → events_sel_<n>_<id1>_all_localizations[_<v>].json
//
// prefix só vale no ramo bulk: subconjunto nunca carrega prefixo.
func eventsExportPath(version common.GameVersion, ids []string, prefix string) (string, error) {
	editsPath := filepath.Join(common.GameFilesRoot, common.ModsFolder, "edits")
	if err := common.EnsurePathExists(editsPath); err != nil {
		return "", fmt.Errorf("error creating edits directory: %w", err)
	}
	var name string
	switch len(ids) {
	case 0:
		name = prefix + "events_all_localizations.json"
	case 1:
		name = "event_" + ids[0] + "_all_localizations.json"
	default:
		name = fmt.Sprintf("events_sel_%d_%s_all_localizations.json", len(ids), ids[0])
	}
	return filepath.Join(editsPath, common.WithVersionSuffixFor(name, version)), nil
}

// objectsBulkPath resolve o caminho do artefato JSON ÚNICO de objects
// (export completo): [<prefix>]objects_all_localizations[_<v>].json em edits/.
func objectsBulkPath(version common.GameVersion, prefix string) (string, error) {
	editsPath := filepath.Join(common.GameFilesRoot, common.ModsFolder, "edits")
	if err := common.EnsurePathExists(editsPath); err != nil {
		return "", fmt.Errorf("error creating edits directory: %w", err)
	}
	return filepath.Join(editsPath, common.WithVersionSuffixFor(prefix+"objects_all_localizations.json", version)), nil
}

// tableBulkNames mapeia kind de tabela → base do artefato bulk em edits/.
// O nome é da FAMÍLIA FÍSICA, nunca de um id de entrada — é a régua do
// topo deste arquivo: o bulk completo não pode colidir com o artefato
// por-entrada, senão a seleção sobrescreveria o arquivo do tradutor:
//   - battletext → battletext (nenhuma entrada tem esse id);
//   - cloud      → cloudsave (diretório real do jogo; a entrada "cloud"
//     já usa cloud_all_localizations);
//   - menumain   → mainmenu (a entrada "menumain" já usa
//     menumain_all_localizations);
//   - tutorial   → tuto (as entradas "tutorial"/"tuto0000" já usam
//     tutorial_all_localizations/tuto0000_all_localizations).
var tableBulkNames = map[string]string{
	KindBattleText: "battletext",
	KindCloud:      "cloudsave",
	KindMenuMain:   "mainmenu",
	KindTutorial:   "tuto",
}

// tableBulkPath resolve o caminho do artefato JSON ÚNICO do export
// completo (ids vazio) dos kinds de tabela: edits/<base>_all_localizations
// [_<versão>].json — irmão do bulk de objects (objectsBulkPath), mesmo
// padrão de dedup global entre entradas no WriteObjectsFile.
func tableBulkPath(kind string, version common.GameVersion, prefix string) (string, error) {
	base, ok := tableBulkNames[kind]
	if !ok {
		return "", fmt.Errorf("sem nome bulk registrado para o kind: %s", kind)
	}
	editsPath := filepath.Join(common.GameFilesRoot, common.ModsFolder, "edits")
	if err := common.EnsurePathExists(editsPath); err != nil {
		return "", fmt.Errorf("error creating edits directory: %w", err)
	}
	return filepath.Join(editsPath, common.WithVersionSuffixFor(prefix+base+"_all_localizations.json", version)), nil
}

// macroExportPath resolve o caminho do artefato JSON do dicionário:
//   - ids vazio (tudo)  → edits/macrodic/macro_dictionary_all_localizations
//     (artefato canônico, mesmo do import padrão);
//   - 1 chunk           → macro_chunk_NN_all_localizations
//   - subconjunto       → macro_sel_<n>_<chunk1>_all_localizations
func macroExportPath(version common.GameVersion, ids []string, prefix string) (string, error) {
	canonical, err := jsonfmt.MacroJSONPath(version)
	if err != nil {
		return "", err
	}
	var name string
	switch len(ids) {
	case 0:
		name = prefix + filepath.Base(canonical)
	case 1:
		name = "macro_" + ids[0] + "_all_localizations.json"
	default:
		name = fmt.Sprintf("macro_sel_%d_%s_all_localizations.json", len(ids), ids[0])
	}
	return filepath.Join(filepath.Dir(canonical), common.WithVersionSuffixFor(name, version)), nil
}
