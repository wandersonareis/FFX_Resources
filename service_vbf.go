package main

import (
	"fmt"
	"path/filepath"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/services"
)

// VbfService cobre o navegador de .vbf (somente leitura).
//
// O container nunca é materializado inteiro em memória: a árvore sai do
// índice e a extração selecionada decodifica/grava um arquivo por vez. O
// .vbf nunca é escrito; export de texto vai para mods/edits, e extração de
// binários vai para data/ ou para a pasta escolhida pelo usuário.
type VbfService struct {
	meta *services.MetadataService
}

// ListVbfRoots devolve os .vbf encontrados ao lado do executável do jogo
// (raízes da sidebar), com o total de arquivos do índice. Lista vazia =
// executável não configurado.
func (s *VbfService) ListVbfRoots() ([]dto.VbfRoot, error) {
	if s.meta == nil {
		return nil, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ListVbfRoots()
}

// ListVbfDir devolve os filhos imediatos de um diretório do .vbf — diretórios
// primeiro, depois arquivos, cada um com kind/id quando o app sabe servi-lo.
func (s *VbfService) ListVbfDir(vbfPath, dir string) ([]dto.VbfNode, error) {
	if s.meta == nil {
		return nil, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ListVbfDir(vbfPath, dir)
}

// ListVbfMacroChunks devolve os filhos virtuais de macrodic.dcp (chunk_00,
// chunk_01, …): o dicionário é um arquivo por localização e o app o trata
// como um grupo.
func (s *VbfService) ListVbfMacroChunks(vbfPath, macroPath string) ([]dto.VbfNode, error) {
	if s.meta == nil {
		return nil, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ListVbfMacroChunks(vbfPath, macroPath)
}

// GetVbfTextEntry decodifica um arquivo de TEXTO do .vbf e devolve a tabela
// (rows + coluna Original). id é exigido só para macro (o chunk).
func (s *VbfService) GetVbfTextEntry(vbfPath, innerPath, id string) (dto.FileEntry, error) {
	if s.meta == nil {
		return dto.FileEntry{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.GetVbfTextEntry(vbfPath, innerPath, id)
}

// GetVbfImageEntry decodifica uma textura (.dds.phyre) do .vbf e devolve a
// pré-visualização PNG em data URL.
// ---- extração/export da SELEÇÃO da árvore do .vbf --------------------------
//
// A seleção é uma lista de caminhos internos (arquivos e diretórios);
// diretório = toda a subárvore, expandida contra o índice no backend.
// Extrair preserva a estrutura de caminhos no destino (default data/ do
// jogo); exportar grava JSON/.strings em mods/edits, como o export de
// data/. O container continua somente leitura.

func (s *VbfService) GetVbfImageEntry(vbfPath, innerPath string) (dto.ImageEntry, error) {
	if s.meta == nil {
		return dto.ImageEntry{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.GetVbfImageEntry(vbfPath, innerPath)
}

func (s *VbfService) ImportVbfImage(vbfPath, innerPath, ddsPath string) error {
	if s.meta == nil {
		return fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ImportVbfImage(vbfPath, innerPath, ddsPath)
}

func (s *VbfService) SaveVbfImage(vbfPath, innerPath, format, destPath string) error {
	if s.meta == nil {
		return fmt.Errorf("metadata service not initialized")
	}
	return s.meta.SaveVbfImage(vbfPath, innerPath, format, destPath)
}

func (s *VbfService) ReplicateVbfImage(vbfPath, sourcePath string, targets []string) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ReplicateVbfImage(vbfPath, sourcePath, targets)
}

// PreviewVbfExtraction devolve o resumo da seleção (arquivos, bytes e
// quantos já existem no destino) para o diálogo de confirmação — destRoot
// vazio significa data/ do jogo.
func (s *VbfService) PreviewVbfExtraction(vbfPath string, paths []string, destRoot string) (dto.VbfExtractPreview, error) {
	if s.meta == nil {
		return dto.VbfExtractPreview{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.PreviewVbfExtraction(vbfPath, paths, destRoot)
}

// ExtractVbfSelection extrai os binários da seleção para destRoot (vazio =
// data/ do jogo), preservando a estrutura interna de caminhos.
func (s *VbfService) ExtractVbfSelection(vbfPath string, paths []string, destRoot string) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ExtractVbfSelection(vbfPath, paths, destRoot)
}

// ExtractVbfImagesSelection extrai DDS/PNG das imagens marcadas, preservando
// os caminhos internos do .vbf sob destRoot (vazio = mods/images).
func (s *VbfService) ExtractVbfImagesSelection(vbfPath string, paths []string, destRoot string) (dto.BatchResult, error) {
	if s.meta == nil {
		return dto.BatchResult{}, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ExtractVbfImagesSelection(vbfPath, paths, destRoot)
}

// SelectVbfExtractDir abre o seletor nativo de pasta para o "Extrair
// para…", começando em data/ do jogo. Devolve "" no cancelamento.
func (s *VbfService) SelectVbfExtractDir() string {
	return pickDirectory("Extrair para…",
		filepath.Join(common.GameFilesRoot, common.DirData))
}

// SelectVbfImageExtractDir escolhe a RAIZ onde DDS/PNG extraídos do .vbf
// serão gravados — mods/images por padrão; abaixo dela o caminho interno do
// container é somado na gravação (por isso o picker abre na raiz e não já
// com o caminho interno, que duplicaria a raiz).
func (s *VbfService) SelectVbfImageExtractDir() string {
	defaultDir := filepath.Join(common.GameFilesRoot, common.ModsFolder, common.ModsImagesDir)
	// O delete poda ramos vazios até GameFilesRoot: garante a raiz para o
	// picker não abrir em pasta fallback (ImageWorkDir faz o mesmo nos
	// outros dois pickers de arquivo).
	_ = common.EnsurePathExists(defaultDir)
	return pickDirectory("Escolher destino das imagens extraídas", defaultDir)
}

// ExportVbfSelection decodifica os kinds de TEXTO da seleção e grava os
// artefatos em mods/edits no formato pedido ("json" | "strings").
func (s *VbfService) ExportVbfSelection(vbfPath, format string, paths []string, langs []string) ([]string, error) {
	if s.meta == nil {
		return nil, fmt.Errorf("metadata service not initialized")
	}
	return s.meta.ExportVbfSelection(vbfPath, format, paths, langs)
}

// VbfImageExportPath devolve o caminho padrão de exportação (.dds ou .png) em
// mods/images/<raiz>/<id> para a textura do .vbf — usado pelo "Exportar"
// direto sem diálogo.
func (s *VbfService) VbfImageExportPath(vbfPath, innerPath, format string) (string, error) {
	if s.meta == nil {
		return "", fmt.Errorf("metadata service not initialized")
	}
	return s.meta.VbfImageExportPath(vbfPath, innerPath, format)
}
