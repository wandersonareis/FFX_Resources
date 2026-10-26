package services

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/interactions"
	"ffxresources/backend/uilog"
)

// Subpastas sob a raiz de tradução (TranslateLocation): textos em text/,
// imagens em images/ — o padrão é <game>/mods/translated.
const (
	TranslatedTextSubdir   = "text"
	TranslatedImagesSubdir = "images"
)

// TranslatedRoot devolve a raiz de tradução configurada no app
// (TranslateLocation, padrão <game>/mods/translated).
func TranslatedRoot() string {
	if svc := interactions.NewInteractionService(); svc != nil && svc.TranslateLocation != nil {
		if root := strings.TrimSpace(svc.TranslateLocation.GetTargetDirectory()); root != "" {
			return root
		}
	}
	return common.DefaultTranslatedDir(filepath.Join(common.GetExecDir(), common.DirData))
}

// TranslatedTextRoot é a raiz de TEXTOS traduzidos: <TranslateLocation>/text.
func TranslatedTextRoot() string {
	return filepath.Join(TranslatedRoot(), TranslatedTextSubdir)
}

// TranslatedImagesRoot é a raiz de IMAGENS traduzidas:
// <TranslateLocation>/images.
func TranslatedImagesRoot() string {
	return filepath.Join(TranslatedRoot(), TranslatedImagesSubdir)
}

// walkTranslatedCandidates varre root (recursivo) devolvendo os .json e
// .strings em ordem canônica, relativos a root. Raiz inexistente é lote
// vazio e não erro: o diretório pode ainda não ter sido criado.
func walkTranslatedCandidates(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d == nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".json" && ext != ".strings" {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// PreviewImportTranslated varre <TranslateLocation>/text e monta o resumo do
// lote SEM aplicar nada — é o que alimenta o diálogo de confirmação.
func (s *MetadataService) PreviewImportTranslated(version common.GameVersion, kinds []string) (dto.ImportBatch, error) {
	return s.importTranslatedBatch(version, kinds, false)
}

// ImportTranslated reexecuta o lote aplicando arquivo a arquivo. Um arquivo
// com erro global é pulado individualmente (o dos demais segue); um arquivo
// com problema em uma ENTRADA aplica as demais entradas e a problemática é
// ignorada. Falhas e avisos voltam agregados em batch.Errors.
func (s *MetadataService) ImportTranslated(version common.GameVersion, kinds []string) (dto.ImportBatch, error) {
	return s.importTranslatedBatch(version, kinds, true)
}

func (s *MetadataService) importTranslatedBatch(version common.GameVersion, kinds []string, apply bool) (dto.ImportBatch, error) {
	root := TranslatedTextRoot()
	batch := dto.ImportBatch{Root: root, Errors: []string{}, Warnings: []string{}}

	files, err := walkTranslatedCandidates(root)
	if err != nil {
		if apply {
			uilog.Error("Import", "lote translated/ falhou ao varrer %s: %v", root, err)
		}
		return batch, err
	}

	want := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			want[k] = true
		}
	}
	if len(want) == 0 {
		return batch, fmt.Errorf("nenhum kind pedido")
	}

	if apply {
		// Detectar formato (assumir strings se tiver .strings, senão json)
		format := "json"
		if len(files) > 0 && strings.HasSuffix(strings.ToLower(files[0]), ".strings") {
			format = "strings"
		}
		uilog.Info("Import", "Iniciando import de %d arquivo(s) de um %s", len(files), format)
	}

	found := make(map[string]bool, len(want))
	for _, name := range files {
		// O plano é montado AQUI, arquivo a arquivo: prepareImport mede
		// contra a store do momento, então o 2º arquivo precisa enxergar o
		// que o 1º acabou de gravar — um plano pré-montado para todos os
		// arquivos ficaria defasado e regravaria o que o anterior alterou.
		plan, perr := s.prepareImport(filepath.Join(root, name), version)
		if perr != nil {
			batch.Errors = append(batch.Errors, fmt.Sprintf("%s: %s", name, perr.Error()))
			if apply {
				uilog.File("Import", name, uilog.StatusFailed, perr.Error())
			}
			continue
		}
		if !want[plan.kind] {
			continue
		}
		found[plan.kind] = true
		batch.Files = append(batch.Files, name)
		batch.EntryCount += plan.summary.EntryCount
		batch.ChangedTexts += plan.summary.ChangedTexts
		for _, e := range plan.summary.Errors {
			batch.Errors = append(batch.Errors, fmt.Sprintf("%s: %s", name, e))
		}
		// Avisos de entrada (entrada inexistente/medida/limite): só aquela
		// entrada fica de fora — as demais entradas do arquivo importam.
		for _, w := range plan.summary.Warnings {
			batch.Warnings = append(batch.Warnings, fmt.Sprintf("%s: %s", name, w))
		}

		// Erros globais (dicionário macro) pulam o arquivo inteiro; sem
		// entrada mesclada não há o que aplicar.
		if len(plan.summary.Errors) > 0 {
			if apply {
				uilog.File("Import", name, uilog.StatusFailed, strings.Join(plan.summary.Errors, "; "))
			}
			continue
		}
		if !apply || len(plan.merged) == 0 {
			continue
		}

		// Detectar formato do primeiro arquivo (todos têm mesmo formato no lote)
		format := detectImportFormat(name)

		// Linha por arquivo (padrão export)
		uilog.Info("Import", "%s: importando", name)

		// applyRecorded loga uma linha por binário com detalhe "importado de um <format>"
		sum, aerr := applyRecorded(version, "Import", "importado de um "+format, func() error {
			return s.ApplyTextCollection(plan.kind, version, plan.merged)
		})
		if aerr != nil {
			batch.Errors = append(batch.Errors, fmt.Sprintf("%s: %s", name, aerr.Error()))
			if apply {
				uilog.File(importAction, name, uilog.StatusFailed, aerr.Error())
			}
			continue
		}
		if sum.Written == 0 {
			uilog.File(importAction, name, uilog.StatusOk, "sem alteração — nenhum binário gravado")
		}
	}

	// Kinds pedidos sem arquivo nenhum não são erro (dá para marcar mais
	// kinds do que a pasta tem): entram só como achados, para o diálogo
	// mostrar o que de fato será importado.
	for k := range want {
		if found[k] {
			batch.Kinds = append(batch.Kinds, k)
		}
	}
	sort.Strings(batch.Kinds)

	if apply {
		if len(batch.Errors) > 0 {
			uilog.Warn("Import", "lote translated/ concluído com %d erro(s) — %d texto(s) em %d arquivo(s)",
				len(batch.Errors), batch.ChangedTexts, len(batch.Files))
		} else {
			uilog.Success("Import", "lote translated/ concluído — %d texto(s) em %d arquivo(s)",
				batch.ChangedTexts, len(batch.Files))
		}
	}
	return batch, nil
}
