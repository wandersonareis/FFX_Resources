//go:build windows

package services

import (
	"ffxresources/testData/tempdir"
	"os"
	"path/filepath"
	"testing"
)

// O caminho inexistente precisa virar ERRO visível (o toast do frontend), não
// uma janela do Explorer na pasta padrão da conta. Este é o ramo que pegava o
// caminho errado em silêncio — e é coberto sem efeito colateral: a shell não
// acha o PIDL, logo nenhuma janela é aberta.
func TestRevealFileOnOSRetornaErroParaCaminhoInexistente(t *testing.T) {
	abs := filepath.Join(tempdir.TempDir(t), "nao-existe.dds.phyre")

	if err := revealFileOnOS(abs); err == nil {
		t.Fatalf("revealFileOnOS(%s) = nil, esperado erro para caminho inexistente", abs)
	}
}

// Caminho com espaços e `&` (o Steam usa "FINAL FANTASY FFX&FFX-2 HD
// Remaster") precisa chegar inteiro à shell. Só roda quando pedido: ABRE UMA
// JANELA do Explorer para conferência manual da seleção.
func TestRevealFileOnOSDestacaArquivoComEspacosECasosEspeciais(t *testing.T) {
	if os.Getenv("FFX_REVEAL_DEMO") == "" {
		t.Skip("defina FFX_REVEAL_DEMO=1 para abrir o Explorer e conferir a seleção manualmente")
	}

	abs := filepath.Join(tempdir.TempDir(t), "FINAL FANTASY FFX&FFX-2 (HD) arquivo de teste.txt")
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatalf("criando arquivo de teste: %v", err)
	}

	if err := revealFileOnOS(abs); err != nil {
		t.Fatalf("revealFileOnOS(%s): %v", abs, err)
	}
}
