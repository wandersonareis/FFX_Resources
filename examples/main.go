package main

import (
	"ffxresources/backend/common"
	"ffxresources/backend/core/reader"
	"ffxresources/backend/core/writer"
	"ffxresources/backend/interactions"
)

/*
MENU PRINCIPAL DE EXEMPLOS
=========================

Este arquivo contém o main() principal que organiza e executa todas as funções de exemplo
dos editores de eventos JSON.
*/
func main() {
	common.LogInfo("=== EXEMPLOS DO SISTEMA DE EVENTOS FFX ===")

	// Texturas .dds.phyre (PhyreEngine/DX11): scan da árvore, extração em
	// DDS/PNG (com flip de orientação), save e reimport. Auto-contido:
	// trabalha em cópias temporárias, sem tocar em data/ nem mods/.
	runDDSPHYREExample()

	// ===== FFX (v1) =====
	interactions.NewInteractionService().FFXAppConfig().SetGameVersion(common.GameVersionFFX)
	if err := interactions.NewInteractionService().GameLocation.SetTargetDirectory("build\\bin\\data"); err != nil {
		// Defina o caminho correto para os arquivos do jogo
		common.LogError("Erro ao definir diretório de destino: %v\n", err)
		return
	}
	common.SetVerboseMode(false) // Ativa o modo verboso para depuração

	// Inicialização obrigatória
	common.LogInfo("=== FFX (v1) ===")
	common.LogInfo("Inicializando sistema (FFX v1)...")
	if err := reader.InitializeInternals(common.GameVersionFFX); err != nil {
		common.LogError("Erro ao inicializar sistema: %v\n", err)
		return
	}
	// Mapeamento struct <-> binário (uma struct Go por chunk, sem tags - o
	// tipo do campo diz se é texto ou dado). Auto-contido: não depende do
	// diretório do jogo nem da inicialização.
	runChunkMappingExample()
}

func showMainMenu() {
	// ===== EXEMPLOS BÁSICOS =====
	ExportMacroDictionaryExample()
	writer.ExportAllLocalizationsToJSON()
	writer.ExportMacroDictionaryToJSON()
}
