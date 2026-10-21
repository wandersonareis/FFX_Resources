package interactions

import (
	"bytes"
	"encoding/json"
	"ffxresources/backend/common"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type IGetAppConfig interface {
	GetGameVersion() common.GameVersion
	GetLocations() map[string]string
	GetLocation(name string) string
	GetEnableMods() bool
	GetTheme() string
}

type ISetAppConfig interface {
	SetGameVersion(version common.GameVersion)
	SetLocation(name, path string)
	SetEnableMods(enabled bool)
	SetTheme(theme string)
}

type IAppConfig interface {
	IGetAppConfig
	ISetAppConfig
	FromJson() error
	ToJson() error
	// GameExeLocation é o executável do jogo escolhido pelo usuário
	// (caminho de ARQUIVO, não diretório): é a partir dele que se
	// localizam os .vbf que abrem a árvore original completa.
	GetGameExeLocation() string
	SetGameExeLocation(path string)
}

type AppConfig struct {
	filePath    string
	locations   map[string]string
	gameVersion common.GameVersion
	enableMods  bool
	gameExeLoc  string
	// theme é "dark"|"light": espelho do next-themes no backend, para a
	// janela de atualização (HTML estático, sem localStorage) seguir o
	// tema do app. Ver docs/updater.md §5.
	theme string
}

type appConfigJSON struct {
	Locations       map[string]string  `json:"Locations"`
	GameVersion     common.GameVersion `json:"GameVersion"`
	EnableMods      *bool              `json:"EnableMods,omitempty"`
	GameExeLocation string             `json:"GameExeLocation,omitempty"`
	Theme           string             `json:"Theme,omitempty"`
}

func NewAppConfig() *AppConfig {
	filePath := filepath.Join(common.GetExecDir(), "config", "config.json")

	c := &AppConfig{
		filePath:    filePath,
		locations:   defaultLocations(),
		gameVersion: common.GameVersionFFX,
		enableMods:  true, // mods-first é o comportamento padrão
		theme:       "dark", // mesmo default do Providers (next-themes)
	}

	if err := c.FromJson(); err != nil {
		common.LogVerbose("Using default config (%v)", err)
	}

	common.SetModsEnabled(c.enableMods)

	if err := c.validateConfig(); err != nil {
		common.LogVerbose("Could not persist default config (%v)", err)
	}

	return c
}

// defaultLocations monta os diretórios padrão a partir do diretório do
// executável e dos nomes em common. GameFiles = <exec>/data;
// Translate é derivado: <gamefiles>/mods/translated — é a raiz do import
// de texto e de imagens.
func defaultLocations() map[string]string {
	execDir := common.GetExecDir()
	gameDir := filepath.Join(execDir, common.DirData)
	return map[string]string{
		"GameFilesLocation": gameDir,
		"TranslateLocation": common.DefaultTranslatedDir(gameDir),
	}
}

// ensureTranslateDerived preenche TranslateLocation derivando do
// GameFilesLocation atual quando a chave está ausente/vazia.
// Valores customizados do usuário nunca são sobrescritos.
func ensureTranslateDerived(locations map[string]string) {
	if strings.TrimSpace(locations["TranslateLocation"]) != "" {
		return
	}
	gameDir := strings.TrimSpace(locations["GameFilesLocation"])
	if gameDir == "" {
		gameDir = filepath.Join(common.GetExecDir(), common.DirData)
	}
	locations["TranslateLocation"] = common.DefaultTranslatedDir(gameDir)
}

func (c *AppConfig) validateConfig() error {
	changed := false

	if c.locations == nil {
		c.locations = defaultLocations()
		changed = true
	} else {
		// Preenche chaves ausentes sem sobrescrever as definidas pelo usuário.
		for key, defPath := range defaultLocations() {
			if key == "TranslateLocation" {
				continue
			}
			if strings.TrimSpace(c.locations[key]) == "" {
				c.locations[key] = defPath
				changed = true
			}
		}
		// Translate deriva do gamefiles atual, não de constante fixa.
		before := strings.TrimSpace(c.locations["TranslateLocation"])
		ensureTranslateDerived(c.locations)
		if strings.TrimSpace(c.locations["TranslateLocation"]) != before {
			changed = true
		}
	}

	// Garante que os diretórios padrão existam.
	for _, dir := range c.locations {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			common.LogVerbose("Could not create directory %s (%v)", dir, err)
		}
	}

	if changed {
		return c.ToJson()
	}

	return nil
}

func (c *AppConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(appConfigJSON{
		Locations:       c.locations,
		GameVersion:     c.gameVersion,
		EnableMods:      &c.enableMods,
		GameExeLocation: c.gameExeLoc,
		Theme:           c.theme,
	})
}

func (c *AppConfig) UnmarshalJSON(data []byte) error {
	var aux appConfigJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	// Mescla sobre os defaults: chaves ausentes/vazias no arquivo
	// mantêm os diretórios padrão em vez de zerarem a config.
	// Translate vazio deriva do gamefiles (<game>/mods/translated).
	merged := defaultLocations()
	// Remove o translate pré-derivado do default para que ele seja
	// recalculado a partir do GameFilesLocation efetivo (arquivo ou default).
	delete(merged, "TranslateLocation")
	for key, path := range aux.Locations {
		if key == "TranslateLocation" {
			continue
		}
		// Só entra o que o schema atual conhece (merged nasce com os
		// defaults): chaves de fluxos descontinuados — ExtractLocation,
		// ImportLocation — são descartadas aqui e somem do arquivo no
		// próximo ToJson() (roda a cada saída).
		if _, known := merged[key]; !known {
			continue
		}
		if strings.TrimSpace(path) != "" {
			merged[key] = path
		}
	}
	if p := strings.TrimSpace(aux.Locations["TranslateLocation"]); p != "" {
		merged["TranslateLocation"] = p
	} else {
		ensureTranslateDerived(merged)
	}
	c.locations = merged
	c.gameVersion = aux.GameVersion
	// Caminho do executável: vazio mantém "sem VBF configurado" (a árvore
	// vbf simplesmente não aparece na sidebar).
	// Valor que não seja FFX.exe/FFX-2.exe vem de versões antigas do
	// config e é descartado ("" mantém "sem VBF configurado").
	if exe, ok := NormalizeGameExe(aux.GameExeLocation); ok {
		c.gameExeLoc = exe
	}
	// EnableMods ausente no arquivo → mantém o default (true).
	if aux.EnableMods != nil {
		c.enableMods = *aux.EnableMods
	}
	// Theme ausente/inválido (não dark/light) → mantém o default ("dark").
	switch theme := strings.TrimSpace(aux.Theme); theme {
	case "dark", "light":
		c.theme = theme
	}
	return nil
}

func (c *AppConfig) ToJson() error {
	if c == nil {
		return fmt.Errorf("invalid configuration")
	}

	if err := os.MkdirAll(filepath.Dir(c.filePath), 0755); err != nil {
		return err
	}

	file, err := os.Create(c.filePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			common.LogVerbose("Error closing config file: %v", err)
		}
	}(file)

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(c)
	if err != nil {
		return err
	}

	return nil
}

func (c *AppConfig) FromJson() error {
	file, err := os.ReadFile(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return c.validateConfig()
		}
		return err
	}
	defer func() { file = file[:0] }()

	if len(bytes.TrimSpace(file)) == 0 {
		return c.validateConfig()
	}

	if err := json.Unmarshal(file, c); err != nil {
		return err
	}

	return c.validateConfig()
}

func (c *AppConfig) GetGameVersion() common.GameVersion {
	return c.gameVersion
}

func (c *AppConfig) SetGameVersion(version common.GameVersion) {
	c.gameVersion = version
	common.SetCurrentGameVersion(c.gameVersion)

	_ = c.ToJson()
}

func (c *AppConfig) GetLocations() map[string]string {
	return c.locations
}

func (c *AppConfig) GetLocation(name string) string {
	return c.locations[name]
}

func (c *AppConfig) SetLocation(name, path string) {
	if c.locations == nil {
		c.locations = make(map[string]string)
	}
	c.locations[name] = path
	_ = c.ToJson()
}

// GetEnableMods devolve a preferência mods-first (config EnableMods).
func (c *AppConfig) GetEnableMods() bool {
	return c.enableMods
}

// SetEnableMods persiste o toggle e aplica na resolução de caminhos.
func (c *AppConfig) SetEnableMods(enabled bool) {
	c.enableMods = enabled
	common.SetModsEnabled(enabled)
	_ = c.ToJson()
}

// GetTheme devolve o tema do app ("dark"|"light") — espelho do
// next-themes para o backend (janela de atualização e qualquer janela
// filha de HTML embutido, que não têm acesso ao localStorage).
func (c *AppConfig) GetTheme() string {
	return c.theme
}

// SetTheme persiste o tema escolhido no frontend. Valor fora de
// dark/light é ignorado (mantém o atual) — a janela de update só
// conhece esses dois seletores.
func (c *AppConfig) SetTheme(theme string) {
	theme = strings.TrimSpace(theme)
	if theme != "dark" && theme != "light" {
		return
	}
	c.theme = theme
	_ = c.ToJson()
}

// GetGameExeLocation devolve o executável do jogo configurado ("" quando o
// usuário ainda não escolheu — nesse caso não há raízes VBF na sidebar).
func (c *AppConfig) GetGameExeLocation() string {
	return c.gameExeLoc
}

// gameExeNames é o PAR de executáveis aceitos. O seletor de arquivo e o
// campo de configuração não podem virar seletor de qualquer .exe do disco:
// só FFX.exe e FFX-2.exe, sem curinga.
var gameExeNames = map[string]bool{
	"ffx.exe":   true,
	"ffx-2.exe": true,
}

// NormalizeGameExe valida e normaliza o caminho do executável do jogo.
//
// "" é aceito (limpar a configuração desliga as raízes VBF da sidebar).
// Qualquer outro caminho só passa se o nome base for FFX.exe ou
// FFX-2.exe, sem diferenciar maiúsculas — devolve o caminho como veio.
// Caminho rejeitado sai como ("", false) e NÃO é persistido.
func NormalizeGameExe(path string) (string, bool) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", true
	}
	if !gameExeNames[strings.ToLower(filepath.Base(p))] {
		return "", false
	}
	return p, true
}

// SetGameExeLocation persiste o caminho do executável do jogo (fonte da
// localização dos .vbf). Caminho que não seja FFX.exe/FFX-2.exe é
// recusado em silêncio — o valor guardado continua o anterior, então o
// eco do config (event GameExeLocation) devolve a UI ao estado válido.
func (c *AppConfig) SetGameExeLocation(path string) {
	normalized, ok := NormalizeGameExe(path)
	if !ok {
		common.LogVerbose("game exe rejeitado (só FFX.exe/FFX-2.exe): %q", path)
		return
	}
	c.gameExeLoc = normalized
	_ = c.ToJson()
}
