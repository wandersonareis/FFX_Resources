package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/common"
	jsonfmt "ffxresources/backend/formatters/json"

	"ffxresources/testData/tempdir"
)

// A seleção completa de um kind na árvore (checkbox da raiz) produz o
// artefato bulk ÚNICO, com o prefixo tree_ — mesmo conteúdo do export
// completo, nome diferente (a origem é que muda). A regra que já valia
// para os subconjuntos continua: náo pode colidir com o canônico.
func TestTreeBulkPrefixSeparatesFromCanonical(t *testing.T) {
	seedEventsTempRoot(t)

	canonical, err := eventsExportPath(common.GameVersionFFX2, nil, "")
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	tree, err := eventsExportPath(common.GameVersionFFX2, nil, treeBulkPrefix)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if filepath.Base(canonical) != "events_all_localizations_ffx2.json" {
		t.Fatalf("canonical inesperado: %s", canonical)
	}
	if filepath.Base(tree) != "tree_events_all_localizations_ffx2.json" {
		t.Fatalf("tree inesperado: %s", tree)
	}
	if tree == canonical {
		t.Fatal("o artefato da árvore não pode sobrescrever o canônico")
	}

	// O prefixo é o ÚNICO caso que carrega marca: subconjunto e entrada
	// isolada ficam iguais com ou sem ele.
	sub, err := eventsExportPath(common.GameVersionFFX2, []string{"a", "b"}, treeBulkPrefix)
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	if strings.Contains(filepath.Base(sub), treeBulkPrefix) {
		t.Fatalf("subconjunto não deve carregar prefixo: %s", sub)
	}
}

// Todos os kinds com artefato bulk aceitam o prefixo tree_, e o kind sem
// caminho bulk (lockit) NÃO é marcado como completo — a seleção dele
// continua por entrada.
func TestTreeBulkPrefixKinds(t *testing.T) {
	if !hasBulkArtifact(KindLockit) {
		// esperado: lockit NÃO tem artefato bulk
	} else {
		t.Fatal("lockit não deveria ter artefato bulk")
	}
	for _, kind := range []string{
		KindEvents, KindMacro, KindHelp, KindObjects,
		KindBattleText, KindCloud, KindTutorial, KindMenuMain,
	} {
		if !hasBulkArtifact(kind) {
			t.Fatalf("%s deveria ter artefato bulk", kind)
		}
	}
}

// Os artefatos bulk por kind (tabela, objects, macro, help) também saem
// com o prefixo quando a seleção é do kind inteiro.
func TestTreeBulkPrefixAllBulkPaths(t *testing.T) {
	seedEventsTempRoot(t)

	bt, err := tableBulkPath(KindBattleText, common.GameVersionFFX, treeBulkPrefix)
	if err != nil {
		t.Fatalf("battletext: %v", err)
	}
	if filepath.Base(bt) != "tree_battletext_all_localizations_ffx.json" {
		t.Fatalf("battletext: %s", bt)
	}

	ob, err := objectsBulkPath(common.GameVersionFFX, treeBulkPrefix)
	if err != nil {
		t.Fatalf("objects: %v", err)
	}
	if filepath.Base(ob) != "tree_objects_all_localizations_ffx.json" {
		t.Fatalf("objects: %s", ob)
	}

	mc, err := macroExportPath(common.GameVersionFFX2, nil, treeBulkPrefix)
	if err != nil {
		t.Fatalf("macro: %v", err)
	}
	if filepath.Base(mc) != "tree_macro_dictionary_all_localizations_ffx2.json" {
		t.Fatalf("macro: %s", mc)
	}

	hp, err := jsonfmt.HelpJSONPath(common.GameVersionFFX, treeBulkPrefix)
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if filepath.Base(hp) != "tree_help_all_localizations_ffx.json" {
		t.Fatalf("help: %s", hp)
	}
}

// O round-trip do artefato tree_ passa pelo MESMO import: o kind é
// detectado pelo metadata.key das entradas, nunca pelo nome do arquivo.
// É o que garante que o prefixo é cosmético — o arquivo importa igual ao
// canônico.
func TestTreeArtifactImportIsNameIndependent(t *testing.T) {
	seedEventsTempRoot(t)

	exported, err := eventsExportPath(common.GameVersionFFX2, nil, treeBulkPrefix)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	// Mesmo diretório, nome diferente do canônico: é só a marca da origem.
	// Se a detecção de kind no import dependesse do nome, esse arquivo não
	// seria reconhecido — mas detectImportKind lê metadata.key.
	canonical, err := eventsExportPath(common.GameVersionFFX2, nil, "")
	if err != nil {
		t.Fatalf("canonical path: %v", err)
	}
	if filepath.Base(exported) == filepath.Base(canonical) {
		t.Fatalf("nomes deveriam diferir: %s vs %s", exported, canonical)
	}

	// O diretório do artefato é o mesmo dos canônicos (mods/edits) — só o
	// nome muda.
	if filepath.Dir(exported) != filepath.Dir(canonical) {
		t.Fatalf("destino deveria ser o mesmo: %s vs %s", exported, canonical)
	}
	if _, err := os.Stat(filepath.Dir(exported)); err != nil {
		t.Fatalf("edits/ não criado: %v", err)
	}
}

// O ciclo completo pela árvore: seleção completa do kind gera o artefato
// tree_ (um arquivo) e o IMPORT dele reconhece o kind pelo conteúdo —
// prova de que o nome não participa do import. Sem edição nada muda (o
// artefato espelha a store), com edição muda exatamente uma row.
func TestTreeSelectionExportImportRoundtrip(t *testing.T) {
	for _, format := range []string{"json", "strings"} {
		t.Run(format, func(t *testing.T) {
			tableBulkFixtures(t, common.GameVersionFFX, []string{
				"battle/btl/bika00_10/bika00_10.bin",
				"battle/btl/zzzz03_19/zzzz03_19.bin",
			})
			if err := ensureVersionReady(common.GameVersionFFX); err != nil {
				t.Fatalf("prepare version: %v", err)
			}
			svc := NewMetadataService(nil)

			// (1) Seleção completa do kind: UM artefato, nome tree_.
			paths, err := svc.ExportArtifactBatch(
				[]ExportPlanItem{{Kind: KindBattleText, Complete: true}},
				common.GameVersionFFX, nil, format,
			)
			if err != nil {
				t.Fatalf("export %s: %v", format, err)
			}
			if len(paths) != 1 {
				t.Fatalf("export %s: %d arquivo(s) (%v), queria o bulk único",
					format, len(paths), paths)
			}
			artifact := paths[0]
			if base := filepath.Base(artifact); base != "tree_battletext_all_localizations_ffx."+format {
				t.Fatalf("nome do artefato: %s", base)
			}

			// (2) Import sem edição: kind reconhecido pelo conteúdo, nada
			// a regravar (o artefato espelha a store).
			changed, err := svc.ImportFile(artifact, common.GameVersionFFX)
			if err != nil {
				t.Fatalf("import %s sem edição: %v", format, err)
			}
			if changed != 0 {
				t.Fatalf("import sem edição: changed=%d, queria 0", changed)
			}

			// (3) Edita uma row e reimporta do MESMO caminho tree_.
			edited := filepath.Join(tempdir.TempDir(t), "edited."+format)
			imported := parseBulkArtifact(t, format, artifact)
			id, index, _, newText := pickBulkEditableRow(t, imported)
			for i := range imported[id].Rows {
				if imported[id].Rows[i].Index == index {
					imported[id].Rows[i].Target = newText
					break
				}
			}
			if err := os.WriteFile(edited, marshalBulkArtifact(t, format, imported), 0o644); err != nil {
				t.Fatalf("gravar editado: %v", err)
			}
			changed, err = svc.ImportFile(edited, common.GameVersionFFX)
			if err != nil {
				t.Fatalf("import %s editado: %v", format, err)
			}
			if changed != 1 {
				t.Fatalf("import editado: changed=%d, queria 1", changed)
			}
		})
	}
}
