// Package tempdir é o t.TempDir() com limpeza resiliente.
//
// Por que não usar t.TempDir() direto: no Windows o RemoveAll do diretório
// temporário falha de forma INTERMITENTE com "The directory is not empty".
// A causa é externa ao código: o antivírus (Defender) abre cada arquivo
// recém-criado em %TEMP% para escaneá-lo — e o faz com FILE_SHARE_DELETE,
// então o arquivo já deletado continua ocupando uma entrada do diretório
// até o handle do scanner fechar. Se a varredura sobrevive ao orçamento de
// retries do RemoveAll (acontece sob carga: `go test ./...` roda dezenas de
// pacotes em paralelo e o disco enfileira o scan), o diretório some pouco
// depois — mas o teste já foi marcado como falho. Foi assim que o build
// quebrou 3 vezes em testes diferentes (vbf, services, appcore), sempre na
// LIMPEZA, nunca numa asserção.
//
// A correção: nossa limpeza roda ANTES da do testing (t.Cleanup é LIFO) e
// insiste por alguns segundos. Quando ela consegue, o RemoveAll do
// t.TempDir() não encontra nada e passa direto.
//
// Este pacote é filho de testData DE PROPÓSITO: testData/mock_notifier.go
// importa backend/services, então os testes de pacotes que o services
// consome (common, ddsphyre, eventtable...) não podem importar testData —
// viraria ciclo de importação no teste. Um subpaciente sem dependências
// resolve isso para todo mundo.
package tempdir

import (
	"os"
	"testing"
	"time"
)

// TempDir cria um diretório temporário para o teste e registra uma limpeza
// que insiste até ~5s. Equivalente a t.TempDir() no que diz respeito ao
// caminho devolvido.
func TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { _ = removeAllRetry(dir) })
	return dir
}

// removeAllRetry tenta RemoveAll por até ~5s. Não classifica o erro de
// propósito: num helper de teste, a distinção entre "diretório não vazio
// ainda", "arquivo em uso" e "divergência de permissão" não compensa — o
// retry é barato e o erro só sobrevive se for REAL (aí o RemoveAll do
// próprio t.TempDir() ainda roda depois e reporta a falha).
func removeAllRetry(dir string) error {
	const budget = 5 * time.Second
	deadline := time.Now().Add(budget)
	delay := 25 * time.Millisecond
	for {
		err := os.RemoveAll(dir)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		if delay < 500*time.Millisecond {
			delay *= 2
		}
	}
}
