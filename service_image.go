package main

import (
	"fmt"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ImageService reúne o ciclo de vida das texturas: leitura, extração,
// import/repack, réplicas, duplicatas, remoção e os seletores de arquivo.
type ImageService struct {
	meta *services.MetadataService
}

// GetImageEntry carrega a textura kind=images e devolve a pré-visualização
// (data URL PNG) com o DDS da fonte escolhida — ordem .dds em disco →
// .png em disco → decode do .dds.phyre em memória.
func (s *ImageService) GetImageEntry(kind, id string, version common.GameVersion) (dto.ImageEntry, error) {
	if s.meta == nil {
		return dto.ImageEntry{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.GetImage(kind, id, version)
}

// ExtractImage grava .dds e .png em mods/images (cópia de trabalho) e
// devolve os caminhos escritos.
func (s *ImageService) ExtractImage(kind, id string, version common.GameVersion) ([]string, error) {
	if s.meta == nil {
		return nil, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ExtractImage(kind, id, version)
}

// ImportImage reempacota o .dds escolhido sobre o container pristine e grava
// o resultado em mods/ (a árvore data/ nunca é alterada).
func (s *ImageService) ImportImage(kind, id, ddsPath string, version common.GameVersion) error {
	if s.meta == nil {
		return fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImportImage(kind, id, ddsPath, version)
}

// ImportImageGroup reempacota o mesmo .dds sobre a textura e as cópias
// idênticas dela (payload original igual — as réplicas da otimização do
// DVD). O lote é validado antes de gravar: alvo fora do grupo é recusado.
func (s *ImageService) ImportImageGroup(kind, id, ddsPath string, targets []string, version common.GameVersion) (dto.ImageImportResult, error) {
	if s.meta == nil {
		return dto.ImageImportResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImportImageGroup(kind, id, ddsPath, targets, version)
}

// RefreshImageDuplicates descarta o índice de duplicatas em cache (rebuild
// na próxima consulta) — para edição externa em hex editor.
func (s *ImageService) RefreshImageDuplicates(kind string, version common.GameVersion) error {
	if s.meta == nil {
		return fmt.Errorf("metadata service not initialized")
	}
	return s.meta.RefreshImageDuplicates(kind, version)
}

// ImageDuplicates devolve só as cópias da textura (sem a imagem em base64):
// é o que o menu de contexto e os diálogos pedem para montar os alvos de
// extrair / replicar / deletar sobre o NÓ CLICADO, que pode não ser a entry
// selecionada.
func (s *ImageService) ImageDuplicates(kind, id string, version common.GameVersion) (dto.ImageDuplicates, error) {
	if s.meta == nil {
		return dto.ImageDuplicates{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImageDuplicates(kind, id, version)
}

// ImageExists responde se a textura ainda existe (data/ OU mods/). O frontend
// usa na revalidação da seleção após reload (ex.: delete apagou o id — não
// re-carregar).
func (s *ImageService) ImageExists(kind, id string, version common.GameVersion) (bool, error) {
	if s.meta == nil {
		return false, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImageExists(kind, id, version)
}

// ExtractImageGroup extrai .dds + .png de uma textura e das cópias
// escolhidas (o "só esta ou todas as cópias" do diálogo de extração).
func (s *ImageService) ExtractImageGroup(kind, id string, targets []string, version common.GameVersion) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ExtractImageGroup(kind, id, targets, version)
}

// ExtractImageSelection extrai somente as imagens explicitamente selecionadas.
func (s *ImageService) ExtractImageSelection(kind string, ids []string, version common.GameVersion) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ExtractImageSelection(kind, ids, version)
}

// ReplicateImage reempacota a imagem ABERTA em mods/ das cópias escolhidas
// — o dupe sem diálogo de arquivo: fonte é o próprio conteúdo do painel.
func (s *ImageService) ReplicateImage(kind, id string, targets []string, version common.GameVersion) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ReplicateImage(kind, id, targets, version)
}

// DeleteImages apaga a textura e das cópias confirmadas no escopo pedido
// ("data" | "mods" | "both"), sempre com os artefatos derivados. O escopo e
// as cópias vêm EXPLICITAMENTE do diálogo — nunca é cascata implícita.
func (s *ImageService) DeleteImages(kind, id string, targets []string, scope string, version common.GameVersion) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.DeleteImages(kind, id, targets, scope, version)
}

// DeleteImageSelection deleta as imagens selecionadas no mesmo escopo. A
// opção withCopies inclui, uma vez cada, as cópias de payload de cada seleção.
func (s *ImageService) DeleteImageSelection(kind string, ids []string, withCopies bool, scope string, version common.GameVersion) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.DeleteImageSelection(kind, ids, withCopies, scope, version)
}

// ImageSelectionCopies retorna, sem decodificar imagens, a união única das
// cópias adicionais dos ids selecionados.
func (s *ImageService) ImageSelectionCopies(kind string, ids []string, version common.GameVersion) ([]string, error) {
	if s.meta == nil {
		return nil, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImageSelectionCopies(kind, ids, version)
}

// SaveImage grava .dds ou .png no caminho escolhido pelo usuário (o
// "salvar em disco" do painel de imagem).
func (s *ImageService) SaveImage(kind, id, format, destPath string, version common.GameVersion) error {
	if s.meta == nil {
		return fmt.Errorf("metadata service not initialized")
	}
	return s.meta.SaveImage(kind, id, format, destPath, version)
}

// ImageExportPath devolve o caminho padrão de exportação (.dds ou .png) em
// mods/images/<raiz>/<id> para o formato pedido — usado pelo "Exportar"
// direto sem diálogo.
func (s *ImageService) ImageExportPath(kind, id, format string, version common.GameVersion) (string, error) {
	if s.meta == nil {
		return "", fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImageExportPath(kind, id, format, version)
}

// SelectImageFile abre o seletor nativo para escolher um .dds a importar,
// começando no diretório de trabalho da textura (mods/images/<raiz>/<dir do
// id>) — o lugar onde o .dds extraído dela já mora. Só .dds: é o único
// formato aceito no importe (o repack usa os bytes crus — um PNG/PDV teria
// de ser re-encodado para DXT, com perda e sem mips).
// Devolve "" quando o usuário cancela.
func (s *ImageService) SelectImageFile(id string, version common.GameVersion) string {
	return pickFile(
		"Selecionar textura para importar",
		services.ImageWorkDir(version, id),
		application.FileFilter{DisplayName: "DDS (*.dds)", Pattern: "*.dds"},
	)
}

// SelectImageSavePath abre o diálogo "Salvar como" para .dds/.png já no
// diretório de trabalho da textura, para o arquivo nunca sair solto em
// mods/images. Devolve "" quando o usuário cancela.
func (s *ImageService) SelectImageSavePath(format, suggestedName, id string, version common.GameVersion) string {
	pattern := "*.dds"
	display := "DDS (*.dds)"
	if format == "png" {
		pattern, display = "*.png", "PNG (*.png)"
	}
	return pickSave("Salvar imagem", services.ImageWorkDir(version, id), suggestedName,
		application.FileFilter{DisplayName: display, Pattern: pattern},
		application.FileFilter{DisplayName: "Todos os arquivos (*.*)", Pattern: "*.*"},
	)
}

// PreviewImportImages varre a raiz de imagens traduzidas
// (<TranslateLocation>/images) e resume o lote SEM gravar: devolve os
// diretórios logo após ps3data (events, map, menu…) para o diálogo marcar e
// quantos candidatos diferem do que já está em mods/.
func (s *ImageService) PreviewImportImages(version common.GameVersion, subdirs []string) (dto.ImageBatch, error) {
	if s.meta == nil {
		return dto.ImageBatch{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.PreviewImportImages(version, subdirs)
}

// ImportImages grava sobre mods/ apenas os .dds.phyre diferentes dos já
// presentes (idênticos são pulados) e atualiza os artefatos de trabalho.
func (s *ImageService) ImportImages(version common.GameVersion, subdirs []string) (dto.ImageBatch, error) {
	if s.meta == nil {
		return dto.ImageBatch{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImportImages(version, subdirs)
}
