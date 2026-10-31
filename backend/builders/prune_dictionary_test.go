package builders_test

import (
	"encoding/binary"
	"os"
	"testing"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/macrodic"
)

// chave de identificação da row no relatório de teste (Index + Name).
func testRowKey(r dto.TextRow) string {
	return string(rune(r.Index)) + "/" + r.Name
}

// TestPruneDicionarioEliminaSimplifiedFantasma reproduz o caso do dicionário
// traduzido (macrodic com ponteiros "simplified" apontando para dentro do
// standard): o original (data/ shape: só rows "name") não tem essas linhas, a
// poda as remove e o rebuild grava o ponteiro simplified = standard.
func TestPruneDicionarioEliminaSimplifiedFantasma(t *testing.T) {
	raw, err := os.ReadFile("../../testData/FFX-2/binary/ffx_ps2/ffx2/master/new_uspc/menu/macrodic.dcp")
	if err != nil {
		t.Skipf("fixture macrodic indisponível: %v", err)
	}
	base, err := macrodic.NewMacroDictionaryBinaryFileFromBytes(raw, "us", common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("parse do fixture: %v", err)
	}
	full, err := builders.BuildMacroDTOFromContainers(
		common.GameVersionFFX2,
		map[string]*macrodic.MacroDictionaryBinaryFile{"us": base},
		nil,
	)
	if err != nil {
		t.Fatalf("DTO do traduzido: %v", err)
	}

	// Original no shape de data/: as mesmas entradas, só as rows "name".
	original := make(dto.Collection, len(full))
	fantasmas := 0
	for id, entry := range full {
		rows := make([]dto.TextRow, 0, len(entry.Rows))
		for _, r := range entry.Rows {
			if r.Name != "name" {
				fantasmas++
				continue
			}
			rows = append(rows, r)
		}
		original[id] = dto.FileEntry{Metadata: entry.Metadata, Rows: rows}
	}
	if fantasmas == 0 {
		t.Fatal("fixture sem simplified distinto — não reproduz o caso")
	}

	pruned, report, err := builders.PruneNotInOriginal(full, original)
	if err != nil {
		t.Fatalf("poda: %v", err)
	}
	total := 0
	for _, rows := range report {
		total += len(rows)
	}
	if total != fantasmas {
		t.Fatalf("relatório devia listar todas as %d órfãs, listou %d", fantasmas, total)
	}
	for _, entry := range pruned {
		for _, r := range entry.Rows {
			if r.Name != "name" {
				t.Fatalf("linha fantasma sobreviveu: %s/%d", r.Name, r.Index)
			}
		}
	}

	// Rebuild e releitura: nenhum ponteiro simplified pode continuar distinto.
	back, err := builders.RebuildMacroContainers(common.GameVersionFFX2, pruned)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	rc, ok := back["us"]
	if !ok || len(rc.Bytes) == 0 {
		t.Fatal("rebuild sem container us")
	}
	distintos := 0
	for i := range rc.ChunkOffsets {
		f, err := rc.FileAt(i)
		if err != nil {
			t.Fatalf("FileAt(%d): %v", i, err)
		}
		for j, seg := range f.Segments {
			if seg == nil {
				continue
			}
			if seg.SimplifiedNameOffset != seg.NameOffset {
				distintos++
			}
			_ = binary.LittleEndian
			_ = j
		}
	}
	if distintos != 0 {
		t.Fatalf("rebuild manteve %d ponteiro(s) simplified distinto(s)", distintos)
	}
}
