package interactions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
)

func TestDefaultLocationsDerivesTranslatedFromGameFiles(t *testing.T) {
	execDir := common.GetExecDir()
	locations := defaultLocations()

	wantGame := filepath.Join(execDir, common.DirData)
	wantTranslate := common.DefaultTranslatedDir(wantGame)

	if locations["GameFilesLocation"] != wantGame {
		t.Fatalf("GameFilesLocation = %q, want %q", locations["GameFilesLocation"], wantGame)
	}
	if locations["TranslateLocation"] != wantTranslate {
		t.Fatalf("TranslateLocation = %q, want %q", locations["TranslateLocation"], wantTranslate)
	}
}

func TestValidateConfigDerivesTranslatedFromCustomGameFiles(t *testing.T) {
	tmp := tempdir.TempDir(t)
	gameDir := filepath.Join(tmp, "jogo")

	config := &AppConfig{
		filePath: filepath.Join(tmp, "config.json"),
		locations: map[string]string{
			"GameFilesLocation": gameDir,
		},
		gameVersion: common.GameVersionFFX,
	}

	if err := config.validateConfig(); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}

	want := common.DefaultTranslatedDir(gameDir)
	if got := config.GetLocation("TranslateLocation"); got != want {
		t.Fatalf("TranslateLocation = %q, want %q", got, want)
	}
}

func TestValidateConfigKeepsCustomTranslated(t *testing.T) {
	tmp := tempdir.TempDir(t)
	custom := filepath.Join(tmp, "minha-traducao")

	config := &AppConfig{
		filePath: filepath.Join(tmp, "config.json"),
		locations: map[string]string{
			"GameFilesLocation": filepath.Join(tmp, "jogo"),
			"TranslateLocation": custom,
		},
		gameVersion: common.GameVersionFFX,
	}

	if err := config.validateConfig(); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}
	if got := config.GetLocation("TranslateLocation"); got != custom {
		t.Fatalf("TranslateLocation = %q, want custom %q", got, custom)
	}
}

func TestUnmarshalDerivesTranslatedWhenMissing(t *testing.T) {
	tmp := tempdir.TempDir(t)
	gameDir := filepath.Join(tmp, "jogo")

	payload, err := json.Marshal(appConfigJSON{
		Locations: map[string]string{
			"GameFilesLocation": gameDir,
		},
		GameVersion: common.GameVersionFFX,
	})
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}

	config := &AppConfig{}
	if err := json.Unmarshal(payload, config); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	want := common.DefaultTranslatedDir(gameDir)
	if got := config.GetLocation("TranslateLocation"); got != want {
		t.Fatalf("TranslateLocation = %q, want derived %q", got, want)
	}
}

func TestSetLocationPersistsImmediately(t *testing.T) {
	tmp := tempdir.TempDir(t)
	configPath := filepath.Join(tmp, "config.json")
	custom := filepath.Join(tmp, "traduzido-custom")

	config := &AppConfig{
		filePath:    configPath,
		locations:   defaultLocations(),
		gameVersion: common.GameVersionFFX,
	}

	config.SetLocation("TranslateLocation", custom)

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.json not persisted: %v", err)
	}

	var persisted appConfigJSON
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config error = %v", err)
	}
	if got := persisted.Locations["TranslateLocation"]; got != custom {
		t.Fatalf("persisted TranslateLocation = %q, want %q", got, custom)
	}
}

// O seletor de arquivo e o campo de configuração só aceitam os DOIS
// executáveis do jogo: sem curinga *.exe e sem qualquer .exe do disco. ""
// (limpar) continua válido e mantém a árvore VBF desligada.
func TestNormalizeGameExe_SoAceitaFFXeFFX2(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", "", true},
		{"   ", "", true},
		{`D:\Jogo\FFX.exe`, `D:\Jogo\FFX.exe`, true},
		{`D:\Jogo\ffx-2.exe`, `D:\Jogo\ffx-2.exe`, true},
		{`D:\Jogo\FFX-2.EXE`, `D:\Jogo\FFX-2.EXE`, true},
		{"FFX.exe", "FFX.exe", true},
		{`D:\Jogo\explorer.exe`, "", false},
		{`D:\Jogo\FFX2.exe`, "", false},
		{`D:\Jogo\FFX-2 (1).exe`, "", false},
		{`D:\Jogo\*.exe`, "", false},
		{`D:\Jogo\FFX.exe.bak`, "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeGameExe(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("NormalizeGameExe(%q) = (%q, %v), want (%q, %v)",
				c.in, got, ok, c.want, c.wantOK)
		}
	}
}
