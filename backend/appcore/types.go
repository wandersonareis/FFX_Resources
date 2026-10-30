// Package appcore é o NÚCLEO de operações arquivo→arquivo do acesso externo
// (menu nativo hoje, CLI no mesmo app amanhã). Ele não conhece Wails, nem
// dialogs, nem menu: recebe requests tipados e devolve resultados tipados.
//
// A divisão de responsabilidades é a régua do projeto:
//
//   - appmenu: coleta argumentos (dialogs nativos) e renderiza resultados
//     (dialogs Info/Warning/Error). Zero regra de negócio.
//   - appcore: TODO o cálculo — detectar formato/versão, validar skip,
//     decodificar, aplicar, reconstruir e gravar. Zero UI.
//
// O binário de trabalho pode estar em QUALQUER diretório (avulso): nada aqui
// depende da árvore do jogo, exceto o runtime de texto (charsets/macrodic —
// ver runtime.go). O casamento texto↔binário é por NOME de arquivo
// (basename), não por caminho.
//
// TODO (granularidade de dependências, apontar/detectar macrodic):
//
//	A necessidade de encoding e macrodic hoje é CARREGADA EM BLOCO por
//	todas as operações de texto (ver runtime.go), mas ela é granular:
//
//	  - imagem (.dds.phyre) NÃO precisa de encoding;
//	  - lockit precisa apenas de encoding (charsets);
//	  - o restante (events, objects, macro, help, tabelas) precisa do
//	    macrodic — as tags MCR do texto resolvem pelo dicionário.
//
//	Aqui presumimos que o macrodic SEMPRE existe e está ao alcance
//	(árvore padrão, pasta menu/ da localização). No uso do menu ou do CLI
//	ele pode NÃO estar (usuário apontando binários avulsos, sem árvore):
//	será necessário APONTAR no menu (sob demanda, quando não encontrado
//	na pasta padrão) ou DETECTAR no CLI. O erro ErrMacrodicUnavailable
//	marca esse ponto; MacrodicPath no request é o gancho já preparado.
package appcore

import (
	"fmt"

	"ffxresources/backend/common"
)

// ArtifactFormat é o formato do artefato de texto (export/import).
type ArtifactFormat uint8

const (
	FormatJSON ArtifactFormat = iota
	FormatStrings
	FormatCSV
	FormatXLSX
	FormatPO
)

// String devolve o rótulo curto ("json" / "strings" / "csv" / "xlsx" / "po").
func (f ArtifactFormat) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatStrings:
		return "strings"
	case FormatCSV:
		return "csv"
	case FormatXLSX:
		return "xlsx"
	case FormatPO:
		return "po"
	default:
		return "unknown"
	}
}

// Ext devolve a extensão de arquivo do formato.
func (f ArtifactFormat) Ext() string {
	switch f {
	case FormatJSON:
		return ".json"
	case FormatStrings:
		return ".strings"
	case FormatCSV:
		return ".csv"
	case FormatXLSX:
		return ".xlsx"
	case FormatPO:
		return ".po"
	default:
		return ""
	}
}

// ImageArtifact é o artefato de imagem pedido na extração.
type ImageArtifact uint8

const (
	ImageDDS ImageArtifact = iota
	ImagePNG
)

// String devolve o rótulo curto ("dds" / "png").
func (a ImageArtifact) String() string {
	switch a {
	case ImageDDS:
		return "dds"
	case ImagePNG:
		return "png"
	default:
		return "unknown"
	}
}

// Ext devolve a extensão do artefato (".dds" / ".png").
func (a ImageArtifact) Ext() string {
	switch a {
	case ImageDDS:
		return ".dds"
	case ImagePNG:
		return ".png"
	default:
		return ""
	}
}

// CollisionPolicy decide o que fazer quando o arquivo de SAÍDA já existe.
type CollisionPolicy uint8

const (
	// CollisionFail não grava: o outcome volta StatusCollision e o
	// chamador decide (menu pergunta; CLI poderia ter flag).
	CollisionFail CollisionPolicy = iota
	// CollisionPrefix grava com o prefixo da operação (export_/extract_).
	CollisionPrefix
	// CollisionOverwrite grava por cima do existente.
	CollisionOverwrite
)

// Status é o resultado de UM arquivo dentro do lote.
type Status uint8

const (
	StatusWritten     Status = iota // gravado (destino novo)
	StatusOverwritten               // gravado por cima de destino existente
	StatusIgnored                   // sem correspondência (import: não achou no texto)
	StatusMismatch                  // formato incompatível com a operação
	StatusCollision                 // destino existia e a política era CollisionFail
	StatusFailed                    // falha individual (Reason carrega o erro)
)

// String devolve o rótulo curto do status.
func (s Status) String() string {
	switch s {
	case StatusWritten:
		return "gravado"
	case StatusOverwritten:
		return "sobrescrito"
	case StatusIgnored:
		return "ignorado"
	case StatusMismatch:
		return "incompatível"
	case StatusCollision:
		return "já existe"
	case StatusFailed:
		return "falha"
	default:
		return "unknown"
	}
}

// FileOutcome é o resultado por arquivo de entrada do lote.
type FileOutcome struct {
	// Input é o caminho apontado pelo usuário.
	Input string
	// Output é o caminho gravado ("" quando nada foi gravado).
	Output string
	// Kind é o kind lógico do arquivo (events, objects, macro, lockit,
	// help, battletext, cloud, tutorial, menumain, images) — "" quando
	// desconhecido.
	Kind string
	// Version é a versão do jogo detectada ("" quando ambígua).
	Version string
	// Status é o resultado da operação para este arquivo.
	Status Status
	// Reason carrega o motivo (erro, formato detectado, caminho em
	// colisão) quando Status != StatusWritten/StatusOverwritten.
	Reason string
}

// ExportResult resume o lote de export/extrai.
type ExportResult struct {
	Outcomes []FileOutcome
}

// ImportResult resume o lote de import.
type ImportResult struct {
	Outcomes []FileOutcome
}

// Count devolve quantos outcomes tem o status pedido.
func (r ExportResult) Count(s Status) int { return countStatus(r.Outcomes, s) }

// Count devolve quantos outcomes tem o status pedido.
func (r ImportResult) Count(s Status) int { return countStatus(r.Outcomes, s) }

func countStatus(outcomes []FileOutcome, s Status) int {
	n := 0
	for _, o := range outcomes {
		if o.Status == s {
			n++
		}
	}
	return n
}

// ErrMacrodicUnavailable sinaliza que o dicionário de macros NÃO foi
// encontrado no lugar padrão (árvore do jogo). O chamador deve apontar um
// macrodic.dcp (menu: dialog sob demanda; CLI: flag/detecção) e repetir a
// operação com Request.MacrodicPath preenchido.
//
// TODO (granular): ver o TODO do pacote — imagem não precisa de encoding;
// lockit só precisa de encoding; o resto precisa do macrodic. Hoje a
// preparação carrega os dois em bloco por segurança.
var ErrMacrodicUnavailable = fmt.Errorf("macrodic.dcp não encontrado na pasta padrão: aponte o dicionário de macros")

// VersionRequest é a versão do jogo para a operação: conhecida (Known=true)
// ou a ser adivinhada pela detecção (Known=false — AutoVersion).
type VersionRequest struct {
	Known bool
	V     common.GameVersion
}

// AutoVersion pede a detecção de versão (caminho, nome, headers).
var AutoVersion = VersionRequest{}

// VersionOf devolve a versão efetiva, já adivinhada quando necessário.
// unknown=true significa "não deu para adivinhar" — o chamador pergunta.
func (r VersionRequest) VersionOf(d Detection) (common.GameVersion, bool) {
	if r.Known {
		return r.V, true
	}
	return d.Version, d.VersionKnown
}

// IncompatibleError é o erro tipado de arquivo incompatível: formato
// desconhecido, extensão fora da lista, entrada da lista de skip, ou kind
// detectado diferente do esperado pela operação. O menu o transforma em
// dialog de aviso; um CLI o transformaria em mensagem + exit code.
type IncompatibleError struct {
	Path     string
	Expected string // formato/kind esperado pela operação ("" = nenhum)
	Got      string // formato/kind detectado ("" = nada reconhecido)
	Reason   string
}

// Error implementa error.
func (e *IncompatibleError) Error() string {
	switch {
	case e.Expected != "" && e.Got != "":
		return fmt.Sprintf("%s: formato incorreto — esperado %s, detectado %s (%s)",
			e.Path, e.Expected, e.Got, e.Reason)
	case e.Got != "":
		return fmt.Sprintf("%s: %s (detectado %s)", e.Path, e.Reason, e.Got)
	default:
		return fmt.Sprintf("%s: %s", e.Path, e.Reason)
	}
}

// Incompatible constrói o erro tipado.
func Incompatible(path, expected, got, reason string) *IncompatibleError {
	return &IncompatibleError{Path: path, Expected: expected, Got: got, Reason: reason}
}
