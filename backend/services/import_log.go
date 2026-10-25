package services

// Narração do import para a janela de logs (backend/uilog).
//
// O apply não devolve os binários que gravou — devolve um erro ou nada, e
// um artefato não escreve um binário só: a propagação por hash recompila
// TODOS os binários do escopo que carregam um texto editado (centenas de
// eventos num import completo). Quem sabe o que foi para o disco é o
// gravador de gravações de common (todo escritor de binário passa por
// common.WriteBytesToFile), e é com esses registros que a janela ganha a
// linha por binário:
//
//	[Import] mods/ffx_ps2/ffx2/master/new_uspc/event/obj_ps3/ev/ev001/ev001.bin — gravado: 8 localizações
//
// Um binário lógico existe em uma linha por localização (o mesmo padrão nas
// 8 raízes de idioma; twin bins de eventtable também dividem o padrão por
// raiz). A linha é POR BINÁRIO: o destino exibido é o da localização padrão
// ('us') e o detalhe conta as demais — um import completo de FFX-2 gera
// ~400 linhas em vez de ~3200.

import (
	"fmt"
	"sort"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/uilog"
)

// importAction é o rótulo da operação nas linhas do log — o mesmo do menu.
const importAction = "Import"

// Rótulos de status das linhas de arquivo. uilog deriva a cor deles
// (levelForStatus): "gravado" é sucesso (verde), "falha" é erro (vermelho).
const (
	writeStatusGravado = "gravado"
	writeStatusFalha   = "falha"
)

// writeSummary é o resultado da narração: quantos binários foram gravados e
// quantos falharam (uma localização que falha marca o binário inteiro).
// Alimenta a linha de resumo da operação. Não é exportado de propósito: é
// detalhe interno do fluxo de import, não contrato com o frontend.
type writeSummary struct {
	Written int
	Failed  int
}

// applyRecorded roda o apply do import sob o gravador de gravações, narra o
// resultado por binário e devolve o erro do apply intacto.
func applyRecorded(version common.GameVersion, apply func() error) (writeSummary, error) {
	var err error
	records := common.RecordWrites(func() { err = apply() })
	return narrateImportWrites(version, records), err
}

// narrateImportWrites emite uma linha por binário gravado (ou tentado)
// durante o import e devolve a contagem para a linha de resumo.
func narrateImportWrites(version common.GameVersion, records []common.WriteRecord) writeSummary {
	var summary writeSummary
	for _, bin := range groupWritesByBinary(version, records) {
		status, detail := bin.result()
		uilog.File(importAction, bin.path, status, detail)
		if status == writeStatusGravado {
			summary.Written++
			continue
		}
		summary.Failed++
	}
	return summary
}

// binaryWrite é um binário lógico: o padrão dele (caminho sem a raiz de
// localização) e uma entrada por localização gravada.
type binaryWrite struct {
	// pattern é o caminho do binário sem a raiz de idioma — a chave do grupo.
	pattern string
	// path é o destino exibido na linha: o da localização padrão ('us')
	// quando ela foi gravada, senão o primeiro registro do grupo.
	path    string
	entries []writeEntry
}

// writeEntry é uma localização do binário com o registro da gravação dela.
type writeEntry struct {
	loc    string
	record common.WriteRecord
}

// groupWritesByBinary agrupa os registros por binário lógico e ordena pelo
// destino exibido — a saída do log fica determinística. Caminhos fora das
// raízes de idioma da versão ficam em grupo próprio, sem agrupar nada.
func groupWritesByBinary(version common.GameVersion, records []common.WriteRecord) []binaryWrite {
	groups := make(map[string]*binaryWrite, len(records))
	for _, rec := range records {
		loc, pattern := common.ModsRelative(version, rec.Path)
		g, ok := groups[pattern]
		if !ok {
			g = &binaryWrite{pattern: pattern}
			groups[pattern] = g
		}
		g.entries = append(g.entries, writeEntry{loc: loc, record: rec})
	}

	out := make([]binaryWrite, 0, len(groups))
	for _, g := range groups {
		for _, e := range g.entries {
			if e.loc == common.DefaultLocalization {
				g.path = e.record.Path
				break
			}
		}
		if g.path == "" {
			g.path = g.entries[0].record.Path
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// result resume o grupo no status e no detalhe da linha do log.
func (g binaryWrite) result() (status, detail string) {
	var failed, bytes int
	var firstErr error
	var failedPath string
	for _, e := range g.entries {
		bytes += e.record.Bytes
		if e.record.Err == nil {
			continue
		}
		failed++
		if firstErr == nil {
			firstErr, failedPath = e.record.Err, e.record.Path
		}
	}

	switch {
	case failed == 0:
		if len(g.entries) > 1 {
			return writeStatusGravado,
				fmt.Sprintf("%s · %s", locCount(len(g.entries)), humanBytes(bytes))
		}
		return writeStatusGravado, humanBytes(bytes)
	case failed == len(g.entries):
		return writeStatusFalha, failureDetail(firstErr, failedPath, g.path)
	default:
		return writeStatusFalha, fmt.Sprintf("%d de %s falhou: %s",
			failed, locCount(len(g.entries)), failureDetail(firstErr, failedPath, g.path))
	}
}

// failureDetail é o motivo da falha com o DESTINO garantido: o erro de
// permissão do Windows já nomeia o arquivo, mas mensagem embrulhada pode
// não ter — nesse caso o destino entra antes do motivo. Quando o destino
// que falhou é o que a própria linha já mostra, ele não se repete.
func failureDetail(err error, path, shown string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if path == "" || path == shown || strings.Contains(msg, path) {
		return msg
	}
	return fmt.Sprintf("destino %s: %s", path, msg)
}

// locCount conta as localizações do binário no plural do português.
func locCount(n int) string {
	if n == 1 {
		return "1 localização"
	}
	return fmt.Sprintf("%d localizações", n)
}

// humanBytes formata o total gravado (linha de log técnica, sem locale).
func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	case n == 1:
		return "1 byte"
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
