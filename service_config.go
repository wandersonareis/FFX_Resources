package main

import (
	"ffxresources/backend/appupdater"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/interactions"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ConfigService reúne a configuração do app: localizações do jogo, preferência
// mods-first e escolha do executável — tudo o que o diálogo de configuração
// toca. O equivalente vivia no App (o "god object" do v2).
type ConfigService struct {
	notifier services.INotificationService
}

// snapshot monta o espelho completo do config (locais resolvidos + toggle
// + executável + versão) para o frontend.
func (c *ConfigService) snapshot() dto.AppConfigSnapshot {
	service := interactions.NewInteractionService()
	config := service.FFXAppConfig()
	return dto.AppConfigSnapshot{
		GameFilesLocation: service.GameLocation.GetTargetDirectory(),
		TranslateLocation: service.TranslateLocation.GetTargetDirectory(),
		GameExeLocation:   config.GetGameExeLocation(),
		EnableMods:        config.GetEnableMods(),
		GameVersion:       config.GetGameVersion().String(),
	}
}

// GetConfigSnapshot devolve o config inteiro em UMA consulta: é a carga
// inicial do store do frontend — as atualizações vêm pelo evento
// AppConfigChanged, dispensando re-fetch ao reabrir o diálogo.
func (c *ConfigService) GetConfigSnapshot() dto.AppConfigSnapshot {
	return c.snapshot()
}

// emitConfigChanged publica o snapshot pós-mudança no bus do Wails. É o
// "eco" do config: o frontend mantém um único store espelhando o que o
// backend realmente persistiu — o caminho Abs normalizado, o executável
// recusado em silêncio (o persistido continua) e o translated derivado
// do gamefiles novo chegam juntos, sem consulta extra.
func (c *ConfigService) emitConfigChanged() {
	services.EmitEvent("AppConfigChanged", c.snapshot())
}

func (c *ConfigService) SelectDirectory(title string) string {
	return pickDirectory(title, common.GetExecDir())
}

// GetGameFilesLocation devolve o diretório dos arquivos do jogo
// (config ou default <exec>/data). Consulta sob demanda para o
// diálogo de config não depender do evento de startup.
func (c *ConfigService) GetGameFilesLocation() string {
	return interactions.NewInteractionService().GameLocation.GetTargetDirectory()
}

// GetTranslateLocation devolve o diretório de tradução usado no
// reimport (<game>/mods/translated por padrão). Consulta sob demanda.
func (c *ConfigService) GetTranslateLocation() string {
	return interactions.NewInteractionService().TranslateLocation.GetTargetDirectory()
}

// GetEnableMods devolve a preferência mods-first da carga de binários
// (config EnableMods). Consulta sob demanda para o checkbox de config.
func (c *ConfigService) GetEnableMods() bool {
	return interactions.NewInteractionService().FFXAppConfig().GetEnableMods()
}

// SetEnableMods persiste e aplica o toggle mods-first imediatamente.
//
// A preferência muda a RESOLUÇÃO de caminhos de tudo que já foi carregado
// (DTOs deduplicados, stores de events/help/lockit/eventtable, macros,
// original pristine): os caches derivados da árvore caem aqui — sem isso o
// frontend recarregaria e o backend continuaria servindo conteúdo lido com
// a preferência ANTIGA. O eco AppConfigChanged sai DEPOIS da invalidação,
// então o reload que ele dispara nunca pega cache velho.
func (c *ConfigService) SetEnableMods(enabled bool) error {
	interactions.NewInteractionService().FFXAppConfig().SetEnableMods(enabled)
	services.InvalidateViewCaches()
	c.emitConfigChanged()
	return nil
}

// ResolveEnableModsChange publica a decisão da guarda de edições não
// salvas do checkbox de mods. Os rascunhos de tradução moram na janela
// principal (o editDraft do frontend é POR JANELA — na janela filha de
// config ele é uma instância vazia), então salvar/descartar só pode
// acontecer lá: este evento é o pedido que o AppShell da janela principal
// executa (save/discard + apply), vindo o clique do modal OU da janela
// filha. decision: "save" | "discard".
func (c *ConfigService) ResolveEnableModsChange(enabled bool, decision string) {
	services.EmitEvent("EnableModsChange", map[string]any{
		"enabled":  enabled,
		"decision": decision,
	})
}

// GetTheme devolve o tema persistido ("dark"|"light") — o frontend lê no
// boot para alinhar o backend ao que está no localStorage.
func (c *ConfigService) GetTheme() string {
	return interactions.NewInteractionService().FFXAppConfig().GetTheme()
}

// SetTheme persiste o tema escolhido no toggle e repassa ao updater: a
// janela de atualização é HTML estático (webview separada, sem
// localStorage) e renderiza o valor que o Go guarda na abertura — ver
// docs/updater.md §5.
func (c *ConfigService) SetTheme(theme string) error {
	cfg := interactions.NewInteractionService().FFXAppConfig()
	cfg.SetTheme(theme) // valida dark/light; ignora o resto
	appupdater.SetTheme(cfg.GetTheme())
	return nil
}

// GetGameExeLocation devolve o executável do jogo configurado — é dele que
// sai a pasta onde os .vbf do container moram (FFX_Data.vbf, FFX2_Data.vbf).
// "" = o usuário ainda não escolheu (sem raízes VBF na sidebar).
func (c *ConfigService) GetGameExeLocation() string {
	return interactions.NewInteractionService().FFXAppConfig().GetGameExeLocation()
}

// SetGameExeLocation persiste o caminho do executável do jogo. Os containers
// abertos são fechados: a pasta de busca mudou e os handles antigos não são
// mais alcançáveis (a próxima ListVbfRoots reabre os da nova pasta).
func (c *ConfigService) SetGameExeLocation(path string) {
	interactions.NewInteractionService().FFXAppConfig().SetGameExeLocation(path)
	services.CloseVbfArchives()
	// Eco SEMPRE (o backend recusa caminho fora de FFX.exe/FFX-2.exe em
	// silêncio e mantém o anterior): o snapshot devolve a UI ao estado
	// realmente persistido.
	c.emitConfigChanged()
}

// SetGameVersion persiste a versão do jogo.
func (c *ConfigService) SetGameVersion(version string) error {
	v, err := common.ParseGameVersionStrict(version)
	if err != nil {
		return err
	}
	interactions.NewInteractionService().FFXAppConfig().SetGameVersion(v)
	c.emitConfigChanged()
	return nil
}

// SetGameLocation persiste o diretório dos arquivos do jogo, invalida caches
// e acompanha o translated se ele era derivado.
func (c *ConfigService) SetGameLocation(path string) error {
	service := interactions.NewInteractionService()
	oldGameDir := service.GameLocation.GetTargetDirectory()
	oldDefaultTranslate := common.DefaultTranslatedDir(oldGameDir)

	service.GameLocation.SetTargetDirectory(path)
	services.InvalidateViewCaches()

	// Se o translated ainda apontava para o default derivado do gamefiles anterior,
	// acompanha o novo gamefiles.
	if service.TranslateLocation.GetTargetDirectory() == oldDefaultTranslate {
		newDefaultTranslate := common.DefaultTranslatedDir(path)
		_ = service.TranslateLocation.SetTargetDirectory(newDefaultTranslate)
	}
	// Eco DEPOIS da derivação: o translated acompanhado chega no mesmo
	// snapshot, sem segunda consulta.
	c.emitConfigChanged()
	return nil
}

// SetTranslateLocation persiste o diretório de tradução.
func (c *ConfigService) SetTranslateLocation(path string) error {
	if err := interactions.NewInteractionService().TranslateLocation.SetTargetDirectory(path); err != nil {
		return err
	}
	c.emitConfigChanged()
	return nil
}

// SelectGameExeFile abre o seletor nativo para escolher o executável do
// jogo. O filtro é LITERAL — só FFX.exe e FFX-2.exe, sem curinga *.exe —
// e a escolha é conferida de novo aqui: o diálogo do sistema aceita digitar
// qualquer nome no campo de arquivo. Devolve "" no cancelamento e numa
// escolha inválida.
func (c *ConfigService) SelectGameExeFile() string {
	selection := pickFile(
		"Selecionar o executável do jogo (FFX.exe ou FFX-2.exe)", "",
		application.FileFilter{
			DisplayName: "Executável do jogo (FFX.exe; FFX-2.exe)",
			Pattern:     "FFX.exe;FFX-2.exe",
		},
	)
	if selection == "" {
		return ""
	}
	normalized, ok := interactions.NormalizeGameExe(selection)
	if !ok {
		// Só o caminho vai para o diagnóstico — nunca conteúdo de arquivo.
		loggingService.DiagWarn("vbf",
			"executável fora de FFX.exe/FFX-2.exe recusado",
			map[string]any{"path": selection})
		if c.notifier != nil {
			c.notifier.NotifyWarn(
				"Escolha um dos executáveis do jogo: FFX.exe ou FFX-2.exe.")
		}
		return ""
	}
	return normalized
}
