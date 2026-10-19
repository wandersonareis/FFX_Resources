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

// ImageBatch descreve um import em lote de .dds.phyre a partir da raiz de
// imagens traduzidas (<TranslateLocation>/images).
type ImageBatch struct {
	// Root é a varredura.
	Root string `json:"root"`
	// Subdirs são os diretórios disponíveis logo após ps3data (events, map,
	// menu…). Voltam sempre, mesmo sem seleção — é o que o diálogo oferece
	// para marcar.
	Subdirs []string `json:"subdirs"`
	// Picked são os subdiretórios efetivamente selecionados.
	Picked []string `json:"picked"`
	// Max é o total de candidatos no escopo; NewCount quantos diferem do que
	// já está em mods/ (no preview) ou foram gravados (na aplicação).
	Max      int `json:"max"`
	NewCount int `json:"new_count"`
	// Errors agrega as falhas por textura; o lote segue nos demais.
	Errors []string `json:"errors"`
}
