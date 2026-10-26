package services

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/uilog"
)

// ps3DataPrefix é o trecho do caminho interno a partir do qual o id da
// textura é válido — ddsphyre.ValidID exige exatamente esse prefixo.
const ps3DataPrefix = "gamedata/ps3data/"

// imageCandidate é um .dds.phyre da raiz traduzida, já resolvido para o id
// do jogo e conferido contra o container que está em mods/.
type imageCandidate struct {
	id          string
	srcPath     string
	displayPath string
	// groupKey é o representante canônico do grupo de payload original
	// (a primeira entrada do grupo) — é ele que decide QUAL linha exibe as
	// cópias ausentes: a primeira imagem exibida do grupo.
	groupKey string
	// copyTotal é o número de cópias do grupo (o que o "info" sabe):
	// total de texturas de mesmo payload original menos a própria.
	copyTotal int
	// copies são as cópias AUSENTES na pasta traduzida (sem arquivo
	// traduzido) — o conteúdo do collapse.
	copies          []ddsphyre.Copy
	differs         bool
	validationError string
}

// translateImageID converte o caminho RELATIVO à raiz traduzida no id da
// textura (tudo a partir de gamedata/ps3data/, sem o sufixo). Caminho fora
// do padrão é ignorado.
func translateImageID(relSlash string) (id string, ok bool) {
	idx := strings.Index(relSlash, ps3DataPrefix)
	if idx < 0 {
		return "", false
	}
	withoutSuffix := strings.TrimSuffix(relSlash[idx:], ddsphyre.Suffix)
	if withoutSuffix == relSlash[idx:] {
		return "", false
	}
	return withoutSuffix, true
}

// copyDisplayPath é o caminho de exibição de uma cópia: o id (relativo,
// sem sufixo) com a extensão — o mesmo formato do caminho interno do .vbf
// e do DisplayPath das entradas.
func copyDisplayPath(id string) string {
	return id + ddsphyre.Suffix
}

// imageDiffers confere se os bytes do .dds.phyre traduzido diferem do
// container que já está em mods/. Sem container em mods/ a textura é nova
// por definição.
func imageDiffers(version common.GameVersion, id string, raw []byte) bool {
	rel := ddsphyre.RelPath(version, id)
	acc, err := common.NewFileAccessorFrom(rel, common.SourceMods)
	if err != nil || !acc.Exists {
		return true
	}
	current, err := acc.ReadBytes()
	if err != nil {
		return true
	}
	return !bytes.Equal(raw, current)
}

// validateImageBytes valida os bytes de um .dds.phyre e retorna erro se
// inválido (formato DX, dimensões, mipmaps, IO, formato inválido).
func validateImageBytes(raw []byte) string {
	if len(raw) == 0 {
		return "arquivo vazio"
	}
	// Tenta parsear o container
	_, err := ddsphyre.Parse(raw)
	if err != nil {
		errStr := err.Error()
		// Classifica o erro para mensagem mais amigável
		switch {
		case strings.Contains(errStr, "magic"):
			return "formato inválido: magic PHYRE não reconhecido"
		case strings.Contains(errStr, "platformId"):
			return "formato DX não suportado (apenas DX11)"
		case strings.Contains(errStr, "size="):
			return "formato inválido: header DX11 com tamanho incorreto"
		case strings.Contains(errStr, "namespace"):
			return "formato inválido: namespace corrompido"
		case strings.Contains(errStr, "PTexture2D"):
			return "formato inválido: textura 2D não encontrada"
		case strings.Contains(errStr, "PTexture2DBase"):
			return "dimensões inválidas: width/height não encontrados"
		case strings.Contains(errStr, "PTextureCommonBase"):
			return "mipmaps inválidos: contagem de mipmap não encontrada"
		case strings.Contains(errStr, "formato de textura"):
			return "formato DX inválido: " + errStr
		case strings.Contains(errStr, "mudança de formato"):
			return "mudança de formato não suportada"
		default:
			return fmt.Sprintf("formato inválido: %v", err)
		}
	}
	return ""
}

// walkTranslatedImages varre a raiz traduzida e devolve:
//   - translated: os ids que TÊM arquivo .dds.phyre na pasta (base para
//     saber quais cópias do grupo estão ausentes);
//   - sources: id → caminho absoluto do arquivo.
//
// Só lê diretórios (sem conteúdo): é a metade rápida da varredura, feita
// uma vez antes do fan-out.
func walkTranslatedImages(root string) (map[string]bool, map[string]string) {
	translated := map[string]bool{}
	sources := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || !strings.HasSuffix(d.Name(), ddsphyre.Suffix) {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		if id, ok := translateImageID(filepath.ToSlash(rel)); ok {
			translated[id] = true
			sources[id] = path
		}
		return nil
	})
	return translated, sources
}

// buildImageCandidate processa UM candidato: lê o arquivo, confere contra
// mods/, valida o container e resolve as cópias ausentes do grupo. Cada
// imagem é independente (não há ordem nem estado compartilhado), então a
// função roda em paralelo — o que a torna segura para o fan-out.
func buildImageCandidate(root, srcPath string, version common.GameVersion, translated map[string]bool, ix *ddsphyre.Index) imageCandidate {
	rel, err := filepath.Rel(root, srcPath)
	if err != nil {
		return imageCandidate{srcPath: srcPath, validationError: fmt.Sprintf("caminho inválido: %v", err)}
	}
	relSlash := filepath.ToSlash(rel)
	id, _ := translateImageID(relSlash)
	c := imageCandidate{
		id: id,
		srcPath:     srcPath,
		// Exibição: caminho relativo à raiz traduzida, COM extensão —
		// semelhante ao interno do .vbf e sem o prefixo absoluto.
		displayPath: relSlash,
	}

	raw, rerr := os.ReadFile(srcPath)
	if rerr != nil {
		c.validationError = fmt.Sprintf("erro de leitura (IO): %v", rerr)
		c.differs = true
		return c
	}
	c.differs = imageDiffers(version, id, raw)
	c.validationError = validateImageBytes(raw)

	if ix != nil {
		group := ix.OriginalGroup(id)
		if len(group) > 0 {
			c.groupKey = group[0]
			copies, _ := ix.Copies(id)
			c.copyTotal = len(copies)
			for _, cp := range copies {
				if translated[cp.ID] {
					continue // tem arquivo traduzido: não é "cópia ausente"
				}
				c.copies = append(c.copies, cp)
			}
		}
	}
	return c
}

// scanTranslatedImages varre a raiz traduzida e devolve os candidatos já
// conferidos contra mods/, VALIDADOS e com as cópias ausentes do grupo.
//
// A validação de cada arquivo é independente (leitura, parse e comparação
// com mods/): o trabalho é distribuído em goroutines — um candidato por
// worker, resultado indexado, sem lock — e agregado na ordem canônica (id),
// que é a ordem que o modal exibe.
func scanTranslatedImages(root string, version common.GameVersion) ([]imageCandidate, error) {
	translated, sources := walkTranslatedImages(root)

	// Índice de duplicatas UMA vez: a construção varre a árvore inteira
	// (~1-2 s) e depois disso só consultas de leitura, seguras em paralelo.
	ix, ixErr := ddsphyre.IndexFor(version)
	if ixErr != nil {
		common.LogWarning("images %s: índice de duplicatas indisponível (%v) — sem info de cópias", version, ixErr)
	}

	paths := make([]string, 0, len(sources))
	for _, p := range sources {
		paths = append(paths, p)
	}

	results := make([]imageCandidate, len(paths))
	workers := runtime.NumCPU() * 4
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = buildImageCandidate(root, path, version, translated, ix)
		}(i, path)
	}
	wg.Wait()

	sort.Slice(results, func(a, b int) bool { return results[a].id < results[b].id })
	return results, nil
}

// PreviewImportImages varre <TranslateLocation>/images e devolve o resumo
// do import: uma entrada por .dds.phyre encontrado, com o estado de cada
// uma (nova / igual ao binário / inválida) e as cópias AUSENTES do grupo —
// anexadas só à PRIMEIRA entrada exibida de cada grupo. Nascem marcadas
// apenas as novas e válidas; as iguais ao binário nascem desmarcadas.
func (s *MetadataService) PreviewImportImages(version common.GameVersion) (dto.ImageImportPreview, error) {
	root := TranslatedImagesRoot()
	preview := dto.ImageImportPreview{Root: root, Errors: []string{}}

	cands, err := scanTranslatedImages(root, version)
	if err != nil {
		return preview, err
	}
	preview.Max = len(cands)

	entries := make([]dto.ImageImportPreviewEntry, 0, len(cands))
	claimed := map[string]bool{}
	for _, c := range cands {
		if c.id == "" {
			continue
		}
		if c.differs {
			preview.NewCount++
		} else {
			preview.IdenticalCount++
		}
		if c.validationError != "" {
			preview.Errors = append(preview.Errors, fmt.Sprintf("%s: %s", c.displayPath, c.validationError))
		}
		selected := c.differs && c.validationError == ""
		entry := dto.ImageImportPreviewEntry{
			ID:               c.id,
			DisplayPath:      c.displayPath,
			Differs:          c.differs,
			CopyCount:        c.copyTotal,
			CopyMissingCount: len(c.copies),
			ValidationError:  c.validationError,
			Selected:         selected,
		}
		// Cópias ausentes só na primeira entrada exibida do grupo: as
		// demais linhas do grupo ficam sem collapse (a mesma cópia não
		// pode aparecer em vários lugares).
		if c.groupKey != "" && len(c.copies) > 0 && !claimed[c.groupKey] {
			claimed[c.groupKey] = true
			entry.Copies = make([]dto.ImageImportPreviewCopy, 0, len(c.copies))
			for _, cp := range c.copies {
				entry.Copies = append(entry.Copies, dto.ImageImportPreviewCopy{
					ID:          cp.ID,
					DisplayPath: copyDisplayPath(cp.ID),
					InMods:      cp.InMods,
					Identical:   cp.Identical,
					// Default acompanha a imagem: marcado quando ela é nova.
					Selected: selected,
				})
			}
		}
		entries = append(entries, entry)
	}
	preview.Entries = entries
	return preview, nil
}

// ImportImages aplica a SELEÇÃO do modal: grava em mods/ apenas as imagens
// marcadas e as cópias marcadas de cada uma — nunca a pasta traduzida
// inteira. A imagem entra quando o modal pede (Image), inclusive a
// regravação de uma idêntica ao binário (o modal a marca para manter o
// grupo coerente quando só as cópias são marcadas).
func (s *MetadataService) ImportImages(version common.GameVersion, selection dto.ImageImportSelection) (dto.ImageImportResult, error) {
	root := TranslatedImagesRoot()
	result := dto.ImageImportResult{Updated: []string{}, Failed: []string{}, Total: 0}

	for _, e := range selection.Entries {
		if e.ID == "" {
			continue
		}
		if e.Image {
			result.Total++
		}
		result.Total += len(e.Copies)
	}
	if result.Total == 0 {
		return result, nil
	}

	// id → caminho do .dds.phyre na pasta traduzida (varredura de paths).
	_, sources := walkTranslatedImages(root)
	// Índice só para validar que a cópia é do grupo da imagem.
	ix, _ := ddsphyre.IndexFor(version)

	uilog.Info(importAction, "Iniciando import de %d imagem(ns) de %s", result.Total, root)

	prog, step := s.progressStep(
		fmt.Sprintf("Importando imagens: %d de %d", result.Total, result.Total),
		result.Total)
	defer prog.End()

	for _, e := range selection.Entries {
		if e.ID == "" || (!e.Image && len(e.Copies) == 0) {
			continue
		}
		src, ok := sources[e.ID]
		if !ok {
			result.Failed = append(result.Failed,
				fmt.Sprintf("%s: não encontrada na pasta traduzida", e.ID))
			continue
		}
		raw, rerr := os.ReadFile(src)
		if rerr != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %v", e.ID, rerr))
			continue
		}
		if verr := validateImageBytes(raw); verr != "" {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %s", e.ID, verr))
			uilog.File(importAction, imageModsPath(version, e.ID), uilog.StatusFailed, verr)
			continue
		}

		if e.Image {
			step.Step(e.ID)
			modsPath := imageModsPath(version, e.ID)
			if ierr := ddsphyre.ImportContainer(version, e.ID, raw); ierr != nil {
				uilog.File(importAction, modsPath, uilog.StatusFailed, ierr.Error())
				result.Failed = append(result.Failed, fmt.Sprintf("%s: %s", e.ID, ierr.Error()))
				continue
			}
			// Sem re-extrair, os artefatos de trabalho em mods/images
			// ficariam defasados (Resolve serviria a textura antiga).
			if _, eerr := ddsphyre.ExtractEffective(version, e.ID); eerr != nil {
				common.LogWarning("images %s/%s: container gravado, mas não atualizei os .dds/.png extraídos: %v", version, e.ID, eerr)
			}
			uilog.File(importAction, modsPath, uilog.StatusOk, "importado de "+src)
			result.Updated = append(result.Updated, e.ID)
		}

		for _, copyID := range e.Copies {
			if copyID == "" || copyID == e.ID {
				continue
			}
			if ix != nil {
				if !containsString(ix.OriginalGroup(e.ID), copyID) {
					result.Failed = append(result.Failed,
						fmt.Sprintf("%s: cópia fora do grupo de %s", copyID, e.ID))
					continue
				}
			}
			step.Step(copyID)
			modsPath := imageModsPath(version, copyID)
			if ierr := ddsphyre.ImportContainer(version, copyID, raw); ierr != nil {
				uilog.File(importAction, modsPath, uilog.StatusFailed, ierr.Error())
				result.Failed = append(result.Failed, fmt.Sprintf("%s: %s", copyID, ierr.Error()))
				continue
			}
			if _, eerr := ddsphyre.ExtractEffective(version, copyID); eerr != nil {
				common.LogWarning("images %s/%s: container gravado, mas não atualizei os .dds/.png extraídos: %v", version, copyID, eerr)
			}
			uilog.File(importAction, modsPath, uilog.StatusOk, "importado de "+src)
			result.Updated = append(result.Updated, copyID)
		}
	}

	// Vários containers mudaram numa leva só: o índice em cache não serve.
	ddsphyre.InvalidateIndex(version)
	return result, nil
}

// containsString diz se a lista tem o valor.
func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
