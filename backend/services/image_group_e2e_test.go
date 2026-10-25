package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/ddsphyre"
)

// Fluxo real de propagação de import, de ponta a ponta:
//
// duas cópias idênticas em data/ (a otimização do DVD) + uma imagem outra,
// extrair o .dds de trabalho, importá-lo no lote e conferir que:
//
//   - o índice de duplicatas enxerga as cópias ANTES do import;
//   - os dois containers viram mods/ DEPOIS (mesmo payload efetivo);
//   - um alvo de OUTRO grupo é recusado sem gravar nada;
//   - a cópia passa a contar como modded e o cache é invalidado.

// realSampleWithPayload localiza na árvore de runtime dois .dds.phyre com
// payloads DISTINTOS (usa o próprio PayloadHash, que é o que o índice usa).
func realSampleWithPayload(t *testing.T) (first, other []byte) {
	t.Helper()
	root := filepath.Join("..", "..", "build", "bin", "data")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Skip("árvore build/bin/data indisponível")
	}
	var want string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".dds.phyre") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		hash, _, herr := ddsphyre.PayloadHash(raw)
		if herr != nil {
			return nil
		}
		if first == nil {
			first, want = raw, hash
			return nil
		}
		if hash != want {
			other = raw
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		t.Fatalf("varredura: %v", err)
	}
	if first == nil {
		t.Skip("nenhum .dds.phyre na árvore de runtime")
	}
	if other == nil {
		t.Skip("só um payload distinto na árvore de runtime")
	}
	return first, other
}

// writePhyre grava um container em <root>/ffx_data/<id>.dds.phyre.
func writePhyre(t *testing.T, root, id string, raw []byte) {
	t.Helper()
	dest := filepath.Join(root, "ffx_data", filepath.FromSlash(id)+".dds.phyre")
	if err := common.EnsurePathExists(dest); err != nil {
		t.Fatalf("criando árvore: %v", err)
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatalf("gravando %s: %v", dest, err)
	}
}

func TestImportImageGroupEndToEnd(t *testing.T) {
	sampleA, sampleC := realSampleWithPayload(t)

	root := tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		ddsphyre.InvalidateIndexes()
	})

	const (
		idA = "gamedata/ps3data/zzz_group/a"
		idB = "gamedata/ps3data/zzz_group/b" // cópia idêntica de a
		idC = "gamedata/ps3data/zzz_other/c" // imagem outra
	)
	writePhyre(t, root, idA, sampleA)
	writePhyre(t, root, idB, append([]byte(nil), sampleA...))
	writePhyre(t, root, idC, sampleC)

	svc := NewMetadataService(nil)

	// 1) O índice enxerga a cópia antes de qualquer import.
	entry, err := svc.GetImage(KindImages, idA, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if entry.Modded {
		t.Error("a pristine apareceu como importada")
	}
	if len(entry.Duplicates) != 1 || entry.Duplicates[0].ID != idB {
		t.Fatalf("duplicatas = %+v, esperado só %s", entry.Duplicates, idB)
	}
	if !entry.Duplicates[0].Identical || entry.Duplicates[0].Modded {
		t.Errorf("cópia pristine deveria ser idêntica e sem mods/: %+v", entry.Duplicates[0])
	}
	if entry.DupPayload <= 0 {
		t.Errorf("dupPayload = %d, esperado o tamanho do payload", entry.DupPayload)
	}

	// 2) Extrai o .dds de trabalho e importa no lote (a e b).
	if _, err := svc.ExtractImage(KindImages, idA, common.GameVersionFFX); err != nil {
		t.Fatalf("ExtractImage: %v", err)
	}
	extracted, err := svc.GetImage(KindImages, idA, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage(após extrair): %v", err)
	}
	if extracted.DDSPath == "" {
		t.Fatal("ExtractImage não gerou .dds de trabalho")
	}
	// A extração não pode ter passado por mods/: continua pristine.
	if extracted.Modded {
		t.Error("extrair .dds não deveria marcar a textura como importada")
	}

	res, err := svc.ImportImageGroup(
		KindImages, idA, extracted.DDSPath, []string{idB}, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("ImportImageGroup: %v", err)
	}
	if res.Total != 2 || len(res.Updated) != 2 || len(res.Failed) != 0 {
		t.Fatalf("resultado = %+v, esperado 2 atualizadas e 0 falhas", res)
	}

	// 3) Os dois containers estão em mods/ com o mesmo payload efetivo.
	for _, id := range []string{idA, idB} {
		modsPath := filepath.Join(root, common.ModsFolder, ddsphyre.RelPath(common.GameVersionFFX, id))
		if _, err := os.Stat(modsPath); err != nil {
			t.Errorf("%s não foi para mods/: %v", id, err)
		}
	}
	after, err := svc.GetImage(KindImages, idA, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage(após import): %v", err)
	}
	if !after.Modded {
		t.Error("a textura importada não ficou modded")
	}
	if len(after.Duplicates) != 1 || !after.Duplicates[0].Identical || !after.Duplicates[0].Modded {
		t.Errorf("cópia depois do lote = %+v, esperado idêntica e modded", after.Duplicates)
	}

	// 4) Alvo de OUTRO grupo: recusado e nada gravado.
	if _, err := svc.ImportImageGroup(
		KindImages, idA, extracted.DDSPath, []string{idC}, common.GameVersionFFX); err == nil {
		t.Fatal("esperava recusa para alvo de outro grupo")
	} else if !strings.Contains(err.Error(), "não é cópia idêntica") {
		t.Errorf("erro sem o motivo: %v", err)
	}
	foreignMods := filepath.Join(root, common.ModsFolder, ddsphyre.RelPath(common.GameVersionFFX, idC))
	if _, err := os.Stat(foreignMods); !os.IsNotExist(err) {
		t.Errorf("alvo recusado foi gravado em mods/ (%v)", foreignMods)
	}

	// 5) Textura única não oferece propagação.
	unique, err := svc.GetImage(KindImages, idC, common.GameVersionFFX)
	if err != nil {
		t.Fatalf("GetImage(c): %v", err)
	}
	if len(unique.Duplicates) != 0 {
		t.Errorf("imagem única com duplicatas: %+v", unique.Duplicates)
	}
}
