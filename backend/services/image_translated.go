package services

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
)

// ps3DataPrefix é o trecho do caminho interno a partir do qual o id da
// textura é válido — ddsphyre.ValidID exige exatamente esse prefixo.
const ps3DataPrefix = "gamedata/ps3data/"

// imageCandidate é um .dds.phyre da raiz traduzida, já resolvido para o id do
// jogo e conferido contra o container que está em mods/.
type imageCandidate struct {
	id      string
	subdir  string
	srcPath string
	differs bool
}

// translateImageID converte o caminho RELATIVO à raiz traduzida no id da
// textura (tudo a partir de gamedata/ps3data/, sem o sufixo) e devolve o
// diretório logo abaixo de ps3data (events, map, menu…), que é o que o
// diálogo mostra para o usuário marcar. Caminho fora do padrão é ignorado.
func translateImageID(relSlash string) (id, subdir string, ok bool) {
	idx := strings.Index(relSlash, ps3DataPrefix)
	if idx < 0 {
		return "", "", false
	}
	withoutSuffix := strings.TrimSuffix(relSlash[idx:], ddsphyre.Suffix)
	if withoutSuffix == relSlash[idx:] {
		return "", "", false
	}
	rest := strings.TrimPrefix(withoutSuffix, ps3DataPrefix)
	slash := strings.Index(rest, "/")
	if slash <= 0 || slash == len(rest)-1 {
		return "", "", false
	}
	return withoutSuffix, rest[:slash], true
}

// imageDiffers confere se o .dds.phyre de src difere do container que já está
// em mods/. Sem container em mods/ a textura é nova por definição.
func imageDiffers(version common.GameVersion, id, src string) bool {
	rel := ddsphyre.RelPath(version, id)
	acc, err := common.NewFileAccessorFrom(rel, common.SourceMods)
	if err != nil || !acc.Exists {
		return true
	}
	current, err := acc.ReadBytes()
	if err != nil {
		return true
	}
	source, err := os.ReadFile(src)
	if err != nil {
		return true
	}
	return !bytes.Equal(source, current)
}

// scanTranslatedImages devolve os subdiretorios disponiveis logo apos
// ps3data e os candidatos do escopo pedido, ja conferidos contra mods/.
func scanTranslatedImages(root string, version common.GameVersion, picked []string) ([]string, []imageCandidate, error) {
	want := make(map[string]bool, len(picked))
	for _, d := range picked {
		if d = strings.TrimSpace(d); d != "" {
			want[d] = true
		}
	}

	avail := map[string]bool{}
	var cands []imageCandidate

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d == nil || d.IsDir() || !strings.HasSuffix(d.Name(), ddsphyre.Suffix) {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		id, subdir, ok := translateImageID(filepath.ToSlash(rel))
		if !ok {
			return nil
		}
		avail[subdir] = true
		if len(want) > 0 && !want[subdir] {
			return nil
		}
		cands = append(cands, imageCandidate{
			id:      id,
			subdir:  subdir,
			srcPath: path,
			differs: imageDiffers(version, id, path),
		})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}

	out := make([]string, 0, len(avail))
	for k := range avail {
		out = append(out, k)
	}
	sort.Strings(out)
	sort.Slice(cands, func(i, j int) bool { return cands[i].id < cands[j].id })
	return out, cands, nil
}

// PreviewImportImages varre <TranslateLocation>/images e resume o lote SEM
// gravar: devolve os subdiretorios disponiveis (mesmo sem selecao) e quantos
// candidatos diferem do que ja esta em mods/.
func (s *MetadataService) PreviewImportImages(version common.GameVersion, subdirs []string) (dto.ImageBatch, error) {
	return s.importImagesBatch(version, subdirs, false)
}

// ImportImages grava sobre mods/ apenas os candidatos diferentes, um por um.
func (s *MetadataService) ImportImages(version common.GameVersion, subdirs []string) (dto.ImageBatch, error) {
	return s.importImagesBatch(version, subdirs, true)
}

func (s *MetadataService) importImagesBatch(version common.GameVersion, subdirs []string, apply bool) (dto.ImageBatch, error) {
	root := TranslatedImagesRoot()
	batch := dto.ImageBatch{Root: root, Picked: append([]string(nil), subdirs...)}

	avail, cands, err := scanTranslatedImages(root, version, subdirs)
	if err != nil {
		return batch, err
	}
	batch.Subdirs = avail
	batch.Max = len(cands)
	for _, c := range cands {
		if c.differs {
			batch.NewCount++
		}
	}
	if !apply || batch.NewCount == 0 {
		return batch, nil
	}

	// O rotulo carrega "N de MAX N novas" (a leitura pedida na UI): o total
	// e o numero de candidatos varridos, entao a barra cobre o lote inteiro,
	// nao so os que mudam.
	prog, step := s.progressStep(
		fmt.Sprintf("Importando imagens: %d novas de %d", batch.NewCount, batch.Max),
		batch.Max)
	defer prog.End()

	imported := 0
	for _, c := range cands {
		step.Step(c.id)
		if !c.differs {
			continue
		}
		raw, rerr := os.ReadFile(c.srcPath)
		if rerr != nil {
			batch.Errors = append(batch.Errors, fmt.Sprintf("%s: %s", c.id, rerr.Error()))
			continue
		}
		if ierr := ddsphyre.ImportContainer(version, c.id, raw); ierr != nil {
			batch.Errors = append(batch.Errors, fmt.Sprintf("%s: %s", c.id, ierr.Error()))
			continue
		}
		imported++
		// Sem re-extrair, os artefatos de trabalho em mods/images ficariam
		// defasados: Resolve prefere o .dds em disco e serviria a textura
		// ANTIGA, mascarando a recem-importada. ExtractEffective é IMUNE ao
		// toggle EnableMods — o refresh comum, com mods desligados, gravaria
		// artefato pristine sobre o container recem-importado.
		if _, eerr := ddsphyre.ExtractEffective(version, c.id); eerr != nil {
			common.LogWarning("images %s/%s: container gravado, mas nao atualizei os .dds/.png extraidos: %v", version, c.id, eerr)
		}
	}

	// Varios containers mudaram numa leva so: o indice de duplicatas em
	// cache nao serve mais.
	ddsphyre.InvalidateIndex(version)

	batch.NewCount = imported
	return batch, nil
}
