package dto

// Este arquivo define o resumo da importação devolvido ao modal de
// confirmação. Erros de arquivo (ilegível, kind/versão divergentes etc.)
// voltam como error (toast no frontend); problemas por entrada ficam em
// Warnings (a entrada é ignorada e o resto do arquivo importa) e
// bloqueadores globais em Errors (não vazio desabilita o botão Importar).
// Todos os slices são inicializados não-nil no backend (nil → JSON null).

// ImportEntryInfo compara uma entrada do arquivo importado com a store.
type ImportEntryInfo struct {
	ID              string `json:"id"`
	Key             string `json:"key"`
	IndexCount      int    `json:"index_count"`
	StoreIndexCount int    `json:"store_index_count"`
	ChangedTexts    int    `json:"changed_texts"`
	Error           string `json:"error,omitempty"`
}

// ImportUsage reporta o uso pós-rebuild do limite uint16 de um binário
// (Used em bytes; Over exclui a entrada da importação — vira aviso).
type ImportUsage struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Used  int    `json:"used"`
	Limit int    `json:"limit"`
	Over  bool   `json:"over"`
}

// ImportSummary é o resumo exibido no modal antes de aplicar a importação.
type ImportSummary struct {
	Path         string   `json:"path"`
	Format       string   `json:"format"` // "json" | "strings"
	Kind         string   `json:"kind"`
	Version      string   `json:"version"`
	Languages    []string `json:"languages"`
	EntryCount   int      `json:"entry_count"`
	TotalIndices int      `json:"total_indices"`
	ChangedTexts int      `json:"changed_texts"`
	// SavesBinary avisa no modal que a confirmação reconstrói e grava os
	// binários em mods/ — e que a carga seguinte passa a ler deles.
	SavesBinary bool              `json:"saves_binary"`
	Entries     []ImportEntryInfo `json:"entries"`
	Usages      []ImportUsage     `json:"usages"`
	// Warnings lista as entradas que têm problema e serão IGNORADAS se o
	// usuário confirmar (entrada inexistente nesta versão, erro de medição,
	// estouro do limite uint16). A granularidade é o ARQUIVO: um erro em
	// qualquer row exclui a entrada inteira — o restante importa normalmente.
	Warnings []string `json:"warnings"`
	// Errors lista os bloqueadores GLOBAIS — não atribuíveis a uma entrada,
	// como o estouro do dicionário macro (binário único compartilhado).
	// Não vazio desabilita o botão Importar.
	Errors []string `json:"errors"`
}

// ImportBatch descreve uma importação em LOTE a partir de um diretório
// (mods/translated/text): o que foi encontrado, o que mudaria e o que falhou.
// Diferente de ImportSummary, que descreve UM arquivo, este agrega N arquivos
// de kinds diferentes — por isso os kinds vêm como lista e não como escalar.
type ImportBatch struct {
	// Root é a varredura (a raiz de textos traduzidos).
	Root string `json:"root"`
	// Kinds são os kinds pedidos que de fato tiveram arquivo encontrado.
	Kinds []string `json:"kinds"`
	// Files são os arquivos varridos, relativos a Root (mantém o resumo legível).
	Files        []string `json:"files"`
	EntryCount   int      `json:"entry_count"`
	ChangedTexts int      `json:"changed_texts"`
	// Errors agrega os bloqueadores GLOBAIS por arquivo (estouro do
	// dicionário macro): o arquivo inteiro é pulado e o lote segue.
	Errors []string `json:"errors"`
	// Warnings agrega as entradas ignoradas por arquivo (entrada
	// inexistente, erro de medição, estouro do limite): só aquela entrada
	// fica de fora — as demais entradas do arquivo são importadas.
	Warnings []string `json:"warnings"`
}
