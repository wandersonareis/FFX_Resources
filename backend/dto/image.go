package dto

import "ffxresources/backend/common"

// ImageEntry é a textura .dds.phyre servida ao frontend para exibição.
//
// A imagem chega pronta em base64 (data URL) — o navegador renderiza direto
// em <img>, sem decoder JS. `Source` diz de onde veio o conteúdo, na ordem
// de preferência acordada:
//
//	dds   → cópia extraída em disco (mods/images);
//	png   → idem, formato .png;
//	phyre → decodificado do próprio .dds.phyre EM MEMÓRIA, sem gravar nada.
type ImageEntry struct {
	Metadata Metadata `json:"metadata"`

	// Source é a fonte servida: "dds", "png" ou "phyre".
	Source string `json:"source"`
	// Format/Width/Height/mips vêm do container .dds.phyre (fonte de tudo).
	Format         string `json:"format"`
	Width          uint32 `json:"width"`
	Height         uint32 `json:"height"`
	MipmapCount    uint32 `json:"mipmapCount"`
	MaxMipmapLevel uint32 `json:"maxMipmapLevel"`

	// AssetPath é o caminho interno embutido no namespace do container
	// (ex.: "PS3Data/help_es/helpgra/help10.dds") — o caminho que o jogo
	// usa na leitura, útil inclusive para um .dds.phyre avulso, sem árvore.
	// Só INFO de exibição (a UI tira o idioma dele) e descartado em
	// seguida: Import/Save/encode nunca leem este campo.
	AssetPath string `json:"assetPath,omitempty"`

	// PNGData é a pré-visualização (data:image/png;base64,...) — sempre
	// preenchida, é o que o frontend exibe no lugar da tabela.
	PNGData string `json:"pngData"`
	// Flipped documenta o contrato da pré-visualização: o backend já a
	// espelhou ao decodificar (o payload do DDS é gravado de baixo para
	// cima), então ela sai NA ORIENTAÇÃO DO JOGO. O botão de flip do
	// frontend é puramente visual — gira o <img>, sem rebuscar nada aqui.
	Flipped bool `json:"flipped"`
	// DDSData é o DDS da fonte escolhida (data:application/octet-stream);
	// vazio quando a fonte é um .png em disco.
	DDSData string `json:"ddsData"`

	// DDSPath/PNGPath apontam para as cópias já extraídas ("" = nenhuma).
	DDSPath string `json:"ddsPath,omitempty"`
	PNGPath string `json:"pngPath,omitempty"`

	// Modded indica que mods/ tem o .dds.phyre (textura já substituída).
	Modded bool `json:"modded"`

	// Duplicates lista as OUTRAS texturas cujo payload original é idêntico
	// (as cópias da otimização do DVD, pareadas em data/ e sempre dentro
	// da mesma versão). Vazio = imagem única.
	Duplicates []ImageDuplicate `json:"duplicates"`
	// DupPayload é o tamanho do payload em bytes: o frontend mostra o
	// desperdício do grupo como payload × (nº de cópias).
	DupPayload int64 `json:"dupPayload"`
}

// ImageDuplicate é uma cópia de mesma imagem (payload original igual).
type ImageDuplicate struct {
	// ID/Key são o id e a key canônica da cópia (navegação na árvore).
	ID  string `json:"id"`
	Key string `json:"key"`
	// VbfPath identifica a entrada no container quando a duplicata foi
	// descoberta pelo navegador .vbf (data/ não precisa deste campo).
	VbfPath string `json:"vbfPath,omitempty"`
	// Modded indica que a cópia já foi importada (existe em mods/).
	Modded bool `json:"modded"`
	// Identical indica que a cópia ainda está SINCRONIZADA com a textura
	// atual (o payload que o jogo carrega é o mesmo). Falso + Modded =
	// cópia divergente: foi editada em separado e convém reimportar junto.
	Identical bool `json:"identical"`
}

// ImageImportResult é o resultado de uma importação em lote: o que entrou,
// o que falhou (id: motivo) e o total considerado.
type ImageImportResult struct {
	Updated []string `json:"updated"`
	Failed  []string `json:"failed"`
	Total   int      `json:"total"`
}

// BatchResult é o resultado genérico de uma operação em lote sobre imagens
// (extrair / replicar / deletar): o que deu certo, o que falhou (id:
// motivo) e o total considerado.
type BatchResult struct {
	Done   []string `json:"done"`
	Failed []string `json:"failed"`
	Total  int      `json:"total"`
}

// ImageDuplicates é o resultado do binding leve das cópias — sem a imagem
// em base64, é o que os diálogos e o menu de contexto pedem para montar a
// lista de alvos.
type ImageDuplicates struct {
	Duplicates []ImageDuplicate `json:"duplicates"`
	// DupPayload é o tamanho do payload em bytes (desperdício do grupo).
	DupPayload int64 `json:"dupPayload"`
}

// NewImageMetadata monta a metadata canônica de uma textura: a key segue o
// padrão dos demais kinds (<versão>/<relPath>), o id é o caminho relativo
// da árvore sem o sufixo .dds.phyre.
func NewImageMetadata(id string, version common.GameVersion) Metadata {
	return Metadata{
		Key: common.VersionPathName(version) + "/" + id + ".dds.phyre",
		ID:  id,
	}
}

// ImageImportPreviewCopy descreve uma cópia AUSENTE na pasta traduzida
// para o collapse do modal: mesmo payload original, mas sem arquivo
// traduzido — o import pode propagar os bytes da imagem para ela.
type ImageImportPreviewCopy struct {
	// ID é o id da cópia (caminho sem sufixo .dds.phyre).
	ID string `json:"id"`
	// DisplayPath é o caminho relativo à raiz traduzida, COM extensão
	// (ex.: "gamedata/ps3data/events/evt_01.dds.phyre").
	DisplayPath string `json:"display_path"`
	// InMods indica que a cópia tem substituição em mods/.
	InMods bool `json:"in_mods"`
	// Identical indica que o payload efetivo (o que o jogo carrega) é o
	// mesmo da imagem — cópia ainda sincronizada.
	Identical bool `json:"identical"`
	// Selected é o estado do checkbox no modal. Default: acompanha a
	// imagem (marcado quando a imagem é nova).
	Selected bool `json:"selected"`
}

// ImageImportPreviewEntry descreve uma imagem candidata ao import vinda de
// mods/translated/images, com detalhes para o modal de confirmação.
type ImageImportPreviewEntry struct {
	// ID da textura (caminho relativo sem sufixo .dds.phyre).
	ID string `json:"id"`
	// DisplayPath é o caminho relativo à raiz traduzida
	// (mods/translated/images), COM extensão — semelhante ao caminho
	// interno do .vbf e sem o prefixo absoluto, que alongaria o modal
	// (ex.: "gamedata/ps3data/event/obj/bl/bltz0201/tex_us/d3d11/461728_256_1.dds.phyre").
	DisplayPath string `json:"display_path"`
	// Differs indica se o .dds.phyre difere do que já está em mods/.
	// False = igual ao binário (hoje pulada): vem desmarcada e com cor de
	// texto distinta no modal — marcar força a regravação do binário.
	Differs bool `json:"differs"`
	// CopyCount é o número de OUTRAS cópias de mesmo payload original (do
	// índice de duplicatas). 0 = imagem única.
	CopyCount int `json:"copy_count"`
	// CopyMissingCount quantas cópias do grupo original estão AUSENTES na
	// pasta traduzida (info diz N, pasta tem M, faltam N-M).
	CopyMissingCount int `json:"copy_missing_count"`
	// Copies são as cópias AUSENTES na pasta traduzida — o conteúdo do
	// collapse. Só vem preenchido na PRIMEIRA entrada exibida do grupo.
	Copies []ImageImportPreviewCopy `json:"copies,omitempty"`
	// ValidationError erro de validação que impede o import (formato DX,
	// dimensões, mipmaps, IO, formato inválido). Vazio = sem erro.
	ValidationError string `json:"validation_error,omitempty"`
	// Selected é o estado do checkbox da imagem (estado do modal): só as
	// novas e válidas nascem marcadas.
	Selected bool `json:"selected"`
}

// ImageImportPreview é o resumo devolvido pelo preview do import de
// imagens traduzidas — uma entrada por .dds.phyre encontrado na raiz.
type ImageImportPreview struct {
	// Root é a varredura (ex.: "C:/.../mods/translated/images").
	Root string `json:"root"`
	// Entries são TODAS as imagens candidatas (uma por .dds.phyre encontrado).
	Entries []ImageImportPreviewEntry `json:"entries"`
	// Max é o total de candidatos.
	Max int `json:"max"`
	// NewCount quantos diferem do que já está em mods/ (nascem marcados).
	NewCount int `json:"new_count"`
	// IdenticalCount quantos são IGUAIS ao binário em mods/ (nascem
	// desmarcados, com cor distinta — marcar regrava o binário).
	IdenticalCount int `json:"identical_count"`
	// Errors agrega as falhas de validação por textura (formato DX,
	// dimensões, mipmaps, IO, formato inválido) — estas não importam.
	Errors []string `json:"errors"`
}

// ImageImportSelectionEntry é a decisão do modal para UMA imagem da
// seleção: gravar (ou regravar) o container dela e/ou propagar os bytes
// para as cópias marcadas.
type ImageImportSelectionEntry struct {
	// ID da imagem de origem (o .dds.phyre da pasta traduzida).
	ID string `json:"id"`
	// Image grava o container da própria imagem. Falso = só as cópias.
	Image bool `json:"image"`
	// Copies são os ids das cópias a receber os mesmos bytes.
	Copies []string `json:"copies,omitempty"`
}

// ImageImportSelection é a seleção do modal: só o que está aqui é
// aplicado — nunca a pasta traduzida inteira.
type ImageImportSelection struct {
	Entries []ImageImportSelectionEntry `json:"entries"`
}
