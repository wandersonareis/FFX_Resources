package ddsphyre

import (
	"strings"
	"testing"
)

// TestAssetPathOf cobre a leitura do caminho embutido sem árvore alguma:
// o caminho completo vence o basename, o basename sozinho é o fallback
// (container raro, ex.: AutoSaving.dds), e lixo binário que termina em
// ".dds" por acaso nunca é aceito (só ASCII imprimível).
func TestAssetPathOf(t *testing.T) {
	cases := []struct {
		name      string
		container []byte
		want      string
	}{
		{
			"caminho completo",
			[]byte("PTexture2D\x00PS3Data/help_es/helpgra/help10.dds\x00"),
			"PS3Data/help_es/helpgra/help10.dds",
		},
		{
			"só basename",
			[]byte("PTexture2D\x00AutoSaving.dds\x00"),
			"AutoSaving.dds",
		},
		{
			"basename antes do caminho vira caminho",
			[]byte("menu.dds\x00PS3Data/event/es/obj/hi/hiku2100/tex/x.dds\x00"),
			"PS3Data/event/es/obj/hi/hiku2100/tex/x.dds",
		},
		{
			"nenhuma string .dds",
			[]byte("PTexture2D\x00m_width\x00m_height\x00"),
			"",
		},
		{
			"lixo binário terminando em .dds",
			[]byte{0x00, 0xff, 0xfe, '.', 'd', 'd', 's', 0x00},
			"",
		},
		{"container vazio", nil, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assetPathOf(tc.container); got != tc.want {
				t.Errorf("assetPathOf() = %q, esperado %q", got, tc.want)
			}
		})
	}
}

// TestParsePreencheAssetPath valida o caminho embutido num container real
// (amostra da árvore do jogo): todo .dds.phyre declara o caminho interno
// do asset, sempre terminado em ".dds". Pula quando não há amostra.
func TestParsePreencheAssetPath(t *testing.T) {
	_, raw := samplePhyre(t)

	tex, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tex.AssetPath == "" {
		t.Fatal("AssetPath vazio: o container real deveria declarar o caminho do asset")
	}
	if !strings.HasSuffix(strings.ToLower(tex.AssetPath), ".dds") {
		t.Errorf("AssetPath = %q, esperado terminado em .dds", tex.AssetPath)
	}
}
