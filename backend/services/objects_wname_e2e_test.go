package services

import (
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
)

// TestObjectsWeaponNamesCarregaNoFluxoChunkmap: w_name.bin é registrado
// pelos weaponnames (Register via init de services) e carrega pelo MESMO
// caminho do frontend — buildObjectsCollection. Sem o registro, o arquivo
// era pulado com "skip objects" e o painel vinha vazio.
func TestObjectsWeaponNamesCarregaNoFluxoChunkmap(t *testing.T) {
	seedFFXHelpRoot(t)
	_ = ensureVersionReady(common.GameVersionFFX)

	const id = "w_name"
	s := NewMetadataService(nil)
	c, err := s.buildObjectsCollection(common.GameVersionFFX, []string{id}, nil)
	if err != nil {
		t.Fatalf("buildObjectsCollection: %v", err)
	}
	entry, ok := c[id]
	if !ok {
		t.Fatal("w_name ausente na coleção — schema chunkmap não registrado?")
	}
	if len(entry.Rows) == 0 {
		t.Fatalf("w_name sem rows (metadata=%+v)", entry.Metadata)
	}
	// Rows rotuladas pelo dono: "Tidus.Standard", "Yuna.Standard", ...
	var names []string
	for _, r := range entry.Rows {
		names = append(names, r.Name)
	}
	t.Logf("w_name: %d rows, ex: %v", len(entry.Rows), names[:min(4, len(names))])
	if entry.Metadata.RowCount != len(entry.Rows) {
		t.Fatalf("RowCount=%d, esperado %d", entry.Metadata.RowCount, len(entry.Rows))
	}
	_ = dto.RowKey(entry.Rows[0])
}

// TestObjectsWeaponNamesImportPreviewClean: o preview do import do artefato
// de w_name (rows rotuladas pelo dono, "Tidus.Standard") sai LIMPO — sem
// bloqueador global e sem aviso de entrada. Antes da correção do medidor, o
// GenericApply rejeitava o naming de dono e o erro `campo "Tidus.Standard"
// nao existe no chunk 0` bloqueava a importação inteira.
func TestObjectsWeaponNamesImportPreviewClean(t *testing.T) {
	seedFFXHelpRoot(t)
	_ = ensureVersionReady(common.GameVersionFFX)

	svc := NewMetadataService(nil)
	paths, err := svc.ExportArtifactBatch(
		[]ExportPlanItem{{Kind: KindObjects, IDs: []string{"w_name"}}},
		common.GameVersionFFX, nil, "json",
	)
	if err != nil {
		t.Fatalf("export w_name: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("export w_name: %d arquivo(s), queria 1 (%v)", len(paths), paths)
	}

	plan, err := svc.prepareImport(paths[0], common.GameVersionFFX)
	if err != nil {
		t.Fatalf("prepare import w_name: %v", err)
	}
	if len(plan.summary.Errors) != 0 {
		t.Fatalf("erros globais no preview: %v", plan.summary.Errors)
	}
	if len(plan.summary.Warnings) != 0 {
		t.Fatalf("avisos de entrada no preview: %v", plan.summary.Warnings)
	}
	if _, ok := plan.merged["w_name"]; !ok {
		t.Fatal("w_name ausente do merge — a entrada deveria ser importável")
	}
	// A medida de capacidade rodou de verdade: há barra de limite no resumo.
	if len(plan.summary.Usages) != 1 || plan.summary.Usages[0].ID != "w_name" {
		t.Fatalf("usages=%+v, queria a medida de w_name", plan.summary.Usages)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
