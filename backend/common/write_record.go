package common

// Gravador de gravações: TODO escritor de binário do app passa por
// WriteBytesToFile/WriteBytesToFileAnywhere (events, eventtable, objects,
// macrodic, lockit, help), e é aí que uma operação precisa saber O QUE foi
// para o disco e ONDE. A janela de logs narra o import por binário
// ("caminho — gravado: 8 localizações" / "caminho — falha: <motivo>") em
// vez de um resumo genérico, e o apply não devolve os destinos — ele
// devolve um erro ou nada.
//
// O gravador é OPT-IN e não muda comportamento nenhum da escrita: sem
// ninguém gravando, recordWrite custa um cheque de pilha vazia.

import (
	"path/filepath"
	"strings"
	"sync"
)

// WriteRecord é UMA tentativa de gravação observada: o destino, os bytes e
// o erro (nil = gravado). Entram também os alvos RECUSADOS antes de criar
// diretório (CheckWritablePath) — o destino é conhecido e é justamente o
// que a mensagem de falha precisa carregar (permissão, data/ somente
// leitura).
type WriteRecord struct {
	// Path é o destino como foi passado ao escritor.
	Path string
	// Bytes é o tamanho do payload.
	Bytes int
	// Err é o erro da tentativa (nil = gravado).
	Err error
}

var (
	writeRecordMu sync.Mutex
	// writeStack tem um buffer por gravador ativo (RecordWrites aninhado).
	// Uma gravação é entregue a TODOS os gravadores ativos: o externo precisa
	// ver o que aconteceu dentro dele, inclusive o que o interno narrou.
	writeStack [][]WriteRecord
)

// RecordWrites roda fn com o gravador ligado e devolve o que foi gravado
// (ou tentado) dentro dela, na ordem. O buffer é devolvido mesmo que fn
// entre em pânico (defer) — o gravador nunca fica ocupado depois.
func RecordWrites(fn func()) (records []WriteRecord) {
	writeRecordMu.Lock()
	writeStack = append(writeStack, nil)
	idx := len(writeStack) - 1
	writeRecordMu.Unlock()

	defer func() {
		writeRecordMu.Lock()
		records = writeStack[idx]
		writeStack = append(writeStack[:idx], writeStack[idx+1:]...)
		writeRecordMu.Unlock()
	}()

	fn()
	return nil
}

// recordWrite entrega a tentativa de gravação a todos os gravadores ativos.
func recordWrite(path string, n int, err error) {
	writeRecordMu.Lock()
	defer writeRecordMu.Unlock()
	if len(writeStack) == 0 {
		return
	}
	rec := WriteRecord{Path: path, Bytes: n, Err: err}
	for i := range writeStack {
		writeStack[i] = append(writeStack[i], rec)
	}
}

// ModsRelative tira a raiz de localização (<gamefiles>/mods/<locRoot>) de
// um destino gravado e devolve o idioma da raiz encontrada mais o padrão
// relativo do binário (event/obj_ps3/ev/ev001/ev001.bin, sempre com "/").
// Caminho fora das raízes de idioma da versão volta com loc vazio e o
// caminho inteiro como padrão — não é binário de jogo localizado e não
// entra em agrupamento.
//
// É o que permite a linha de log ser POR BINÁRIO: o apply grava o MESMO
// padrão nas 8 raízes de idioma (e twin bins de eventtable também), e sem
// tirar a raiz cada localização viraria uma linha própria.
func ModsRelative(version GameVersion, path string) (loc, pattern string) {
	for _, code := range SupportedLanguageCodes() {
		prefix := filepath.Join(GameFilesRoot, ModsFolder,
			filepath.FromSlash(GetLocalizationRootForVersion(version, code)))
		if rel, ok := trimPathPrefix(path, prefix); ok {
			if rel == "" {
				return code, ""
			}
			// Barra normalizada: a chave de agrupamento é a mesma em qualquer
			// plataforma (o destino em si segue o separador do sistema).
			return code, filepath.ToSlash(rel)
		}
	}
	return "", path
}

// trimPathPrefix corta prefix de path quando casa (componente inteiro, não
// um prefixo de nome) e devolve o resto sem separador inicial. A comparação
// é case-insensitive: no Windows o volume/raiz varia em maiúsculas e o
// destino foi montado pela própria GetLocalizationRootForVersion.
func trimPathPrefix(path, prefix string) (string, bool) {
	if len(path) < len(prefix) || !strings.EqualFold(path[:len(prefix)], prefix) {
		return "", false
	}
	rest := path[len(prefix):]
	if rest == "" {
		return "", true
	}
	if rest[0] != filepath.Separator && rest[0] != '/' {
		return "", false
	}
	return strings.TrimLeft(rest, `/\`), true
}
