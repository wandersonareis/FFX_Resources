// Detecção de formato ANTES de processar: cada operação do appcore chama
// DetectFile/DetectBytes antes de qualquer decode — arquivo incompatível
// vira *IncompatibleError, nunca um parse quebrado no meio.
//
// Ordem das regras (barata → específica), como combinado:
//
//  1. extensão como LIMITE (whitelist conhecida);
//  2. lista de skip pelo NOME (binário avulso não tem caminho canônico —
//     common.IsSkippedBinaryAnyVersion confere as duas árvores);
//  3. extensão + conteúdo: .dds.phyre (IsPhyre), .json, .strings, .dcp
//     (header de macrodic), .sps2 (help), .vbf;
//  4. .bin: header de chunkmap (V1 20B ⇒ FFX; V2 32B ⇒ FFX-2 — é sinal de
//     versão), depois o padrão de PONTEIROS de events/eventtable
//     (ffxstr.PointerStatusAt: razão de ponteiros caindo em texto válido),
//     depois o padrão CRLF de lockit.
//
// A versão é adivinhada por todos os sinais disponíveis: caminho
// (ffx2/ffx-2/ffx_ps2/ffx2 — a mesma régua do .vbf), header de chunkmap,
// nomes exclusivos de cada versão (help ⇒ FFX, tutorial.msb/lm_* ⇒ FFX-2,
// stem do lockit) e a extensão. Ambiguidade vira VersionKnown=false — o
// chamador pergunta (menu) ou exige flag (CLI).
package appcore

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ffxresources/backend/common"
	ffxstr "ffxresources/backend/core/strings"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/services"
)

// Format é o formato físico detectado no arquivo.
type Format uint8

const (
	FormatUnknown      Format = iota
	FormatChunkmapV1          // binário de objects, header V1 (FFX, 20 bytes)
	FormatChunkmapV2          // binário de objects, header V2 (FFX-2/LastMission, 32 bytes)
	FormatMacroDic            // dicionário de macros (macrodic.dcp)
	FormatEvent               // events (padrão de ponteiros)
	FormatEventTable          // tabelas events (battletext/cloud/tutorial/menumain)
	FormatLockit              // loc kit (registros CRLF)
	FormatHelp                // painel de ajuda (.sps2)
	FormatPhyre               // textura (.dds.phyre)
	FormatArtifactJSON        // artefato de texto exportado (.json)
	FormatArtifactStrings
	FormatArtifactCSV
	FormatArtifactXLSX
	FormatVBF // container (somente leitura — não é entrada de menu)
)

// String devolve o rótulo do formato (usado nos dialogs/erros).
func (f Format) String() string {
	switch f {
	case FormatChunkmapV1:
		return "objects (header V1/FFX)"
	case FormatChunkmapV2:
		return "objects (header V2/FFX-2)"
	case FormatMacroDic:
		return "dicionário de macros"
	case FormatEvent:
		return "events"
	case FormatEventTable:
		return "events (tabela)"
	case FormatLockit:
		return "loc kit"
	case FormatHelp:
		return "painel de ajuda"
	case FormatPhyre:
		return "textura .dds.phyre"
	case FormatArtifactJSON:
		return "artefato JSON"
	case FormatArtifactStrings:
		return "artefato Strings"
	case FormatArtifactCSV:
		return "artefato CSV"
	case FormatArtifactXLSX:
		return "artefato XLSX"
	case FormatVBF:
		return "container .vbf"
	default:
		return "desconhecido"
	}
}

// Kind do formato de texto (services.Kind*), "" quando não é texto.
func (f Format) TextKind() string {
	switch f {
	case FormatChunkmapV1, FormatChunkmapV2:
		return services.KindObjects
	case FormatMacroDic:
		return services.KindMacro
	case FormatEvent:
		return services.KindEvents
	case FormatEventTable:
		return services.KindBattleText // refinado pelo caminho em detectEventKind
	case FormatLockit:
		return services.KindLockit
	case FormatHelp:
		return services.KindHelp
	default:
		return ""
	}
}

// artifactRegistryKey devolve a chave canônica do registry de formatters
// (formatters.Get) para o artefato detectado — "" quando não é artefato
// de texto (.json/.strings/.csv/.xlsx).
func (f Format) artifactRegistryKey() string {
	switch f {
	case FormatArtifactJSON:
		return "json"
	case FormatArtifactStrings:
		return "strings"
	case FormatArtifactCSV:
		return "csv"
	case FormatArtifactXLSX:
		return "xlsx"
	}
	return ""
}

// Detection é o resultado da sniff: formato, kind, versão (adivinhada) e o
// detalhe que explica a classificação (para dialogs e logs).
type Detection struct {
	Format       Format
	Kind         string
	Version      common.GameVersion
	VersionKnown bool
	Detail       string
}

// maxSniffBytes é quanto se lê para classificar (headers vivem no começo,
// mas events .bin maiores têm ponteiros espalhados por megabytes — 8MB
// cobre os eventos do jogo com folga; o arquivo inteiro é lido pela
// operação em si).
const maxSniffBytes = 8 << 20

// DetectFile lê o começo do arquivo e classifica. Erro é sempre
// *IncompatibleError (extensão fora da lista, skip, formato irreconhecível).
func DetectFile(path string) (Detection, error) {
	data, err := readHead(path, maxSniffBytes)
	if err != nil {
		return Detection{}, err
	}
	return DetectBytes(path, data)
}

// readHead lê até limit bytes do arquivo (a sniff só precisa do começo).
func readHead(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s: o caminho é um diretório", path)
	}
	if info.Size() <= limit {
		return os.ReadFile(path)
	}
	buf := make([]byte, limit)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// DetectBytes classifica o arquivo pelo caminho + conteúdo (head pode ser
// truncado — as regras só olham o começo do arquivo, exceto as checagens de
// limite que toleram o corte).
func DetectBytes(path string, head []byte) (Detection, error) {
	base := strings.ToLower(filepath.Base(path))
	stem := strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))

	// (1) Extensão como limite da operação.
	switch {
	case strings.HasSuffix(base, ".dds.phyre"):
		// ok — cai no case phyre abaixo
	case filepath.Ext(base) == ".phyre":
		// ".dds.phyre" já coberto; um ".phyre" solto não é textura do app.
		return Detection{}, Incompatible(path, "", "", "extensão não suportada pelo app (.phyre sem .dds)")
	}

	// (2) Lista de skip pelo NOME — antes de qualquer decode.
	if common.IsSkippedBinaryAnyVersion(base) {
		return Detection{}, Incompatible(path, "", "", "arquivo na lista de skip (formato conhecido, não servido)")
	}

	kind := ""
	detail := ""

	switch {
	case strings.HasSuffix(base, ".dds.phyre"):
		if !ddsphyre.IsPhyre(head) {
			return Detection{}, Incompatible(path, "textura .dds.phyre", "", "header PhyreEngine não reconhecido")
		}
		return Detection{
			Format:       FormatPhyre,
			Kind:         services.KindImages,
			Version:      guessVersion(path, FormatUnknown),
			VersionKnown: pathSaysVersion(path),
			Detail:       "container PhyreEngine válido",
		}, nil

	case filepath.Ext(base) == ".json":
		if !looksLikeJSON(head) {
			return Detection{}, Incompatible(path, "artefato JSON", "", "conteúdo não é JSON")
		}
		return Detection{Format: FormatArtifactJSON, Detail: "artefato JSON"}, nil

	case filepath.Ext(base) == ".strings":
		if !looksLikeStrings(head) {
			return Detection{}, Incompatible(path, "artefato Strings", "", "sem cabeçalho /*key=*/ do formato .strings")
		}
		return Detection{Format: FormatArtifactStrings, Detail: "artefato Strings"}, nil

	case filepath.Ext(base) == ".csv":
		if !looksLikeCSV(head) {
			return Detection{}, Incompatible(path, "artefato CSV", "", "conteúdo não parece CSV válido")
		}
		return Detection{Format: FormatArtifactCSV, Detail: "artefato CSV"}, nil

	case filepath.Ext(base) == ".xlsx":
		if !looksLikeXLSX(head) {
			return Detection{}, Incompatible(path, "artefato XLSX", "", "conteúdo não é XLSX válido (assinatura PK)")
		}
		return Detection{Format: FormatArtifactXLSX, Detail: "artefato XLSX"}, nil

	case filepath.Ext(base) == ".vbf":
		return Detection{}, Incompatible(path, "", "container .vbf", "container é somente leitura — use o navegador de .vbf do app")

	case filepath.Ext(base) == ".dcp":
		if !looksLikeMacroDic(head) {
			return Detection{}, Incompatible(path, "dicionário de macros", "", "header de offsets de chunks não reconhecido")
		}
		return Detection{
			Format:       FormatMacroDic,
			Kind:         services.KindMacro,
			Version:      guessVersion(path, FormatMacroDic),
			VersionKnown: pathSaysVersion(path) || lockitOrMacroSaysVersion(stem).String() != "",
			Detail:       "tabela de offsets de chunks válida",
		}, nil

	case filepath.Ext(base) == ".sps2":
		if !helpfile.IsHelpEntry(stem) {
			return Detection{}, Incompatible(path, "painel de ajuda", "", "nome não é um painel de help conhecido")
		}
		return Detection{
			Format:       FormatHelp,
			Kind:         services.KindHelp,
			Version:      common.GameVersionFFX,
			VersionKnown: true, // painéis .sps2 são exclusivos da árvore do FFX
			Detail:       "painel de help",
		}, nil

	case filepath.Ext(base) == ".msb":
		return Detection{
			Format:       FormatEventTable,
			Kind:         detectEventTableKind(path),
			Version:      common.GameVersionFFX2,
			VersionKnown: true, // tutorial.msb é exclusivo do FFX-2
			Detail:       "tabela events (tutorial.msb)",
		}, nil

	case filepath.Ext(base) == ".bin":
		// chunkmap primeiro: header declarativo, sem ambiguidade.
		if d, ok := detectChunkmap(path, head); ok {
			return d, nil
		}
		// events/eventtable: padrão de ponteiros.
		if d, ok := detectEventPattern(path, head); ok {
			return d, nil
		}
		// lockit: registros CRLF sem binário de controle.
		if d, ok := detectLockit(path, head); ok {
			return d, nil
		}
		kind, detail = "", "nenhum header conhecido (chunkmap, macrodic, ponteiros de events, CRLF de lockit)"

	default:
		kind, detail = "", "extensão não suportada pelo app"
	}

	return Detection{}, Incompatible(path, "", kind, detail)
}

// detectChunkmap aplica a régua do header V1/V2 (chunkmap.ReadFileHeader):
// V2 tem headerSize==32 em 0x1C; V1 tem 20 bytes com contadores u16.
// A versão VERSA do header: V1 ⇒ FFX, V2 ⇒ FFX-2 (LastMission divide a
// árvore do FFX-2).
func detectChunkmap(path string, head []byte) (Detection, bool) {
	if len(head) >= 32 {
		headerSize := int(binary.LittleEndian.Uint32(head[28:32]))
		if headerSize == 32 {
			min := int(binary.LittleEndian.Uint32(head[12:16]))
			max := int(binary.LittleEndian.Uint32(head[16:20]))
			individual := int(binary.LittleEndian.Uint32(head[20:24]))
			total := int(binary.LittleEndian.Uint32(head[24:28]))
			if individual > 0 && max >= min && max-min+1 > 0 && 32+total <= len(head) {
				return Detection{
					Format:       FormatChunkmapV2,
					Kind:         services.KindObjects,
					Version:      common.GameVersionFFX2,
					VersionKnown: true,
					Detail:       "header V2 (32 bytes, contadores u32)",
				}, true
			}
		}
	}
	if len(head) >= 20 {
		min := int(binary.LittleEndian.Uint16(head[8:10]))
		max := int(binary.LittleEndian.Uint16(head[10:12]))
		individual := int(binary.LittleEndian.Uint16(head[12:14]))
		total := int(binary.LittleEndian.Uint16(head[14:16]))
		if individual > 0 && max >= min && max-min+1 > 0 && 20+total <= len(head) {
			return Detection{
				Format:       FormatChunkmapV1,
				Kind:         services.KindObjects,
				Version:      common.GameVersionFFX,
				VersionKnown: true,
				Detail:       "header V1 (20 bytes, contadores u16)",
			}, true
		}
	}
	return Detection{}, false
}

// looksLikeMacroDic confere a tabela de 16 offsets u32 do macrodic.dcp
// (mapBytes): slots ZEROS são placeholders de chunk vazio (legítimos e
// comuns), os não-zero começam na base da tabela (64), são crescentes e
// ficam dentro do arquivo. Sem terminador 0xFFFFFFFF obrigatório — as
// fixtures reais não o trazem; a contagem/posição dos chunks varia por
// versão (TODO: medir e usar como sinal de versão quando as duas árvores
// tiverem fixtures).
func looksLikeMacroDic(head []byte) bool {
	const tableLen = 64
	if len(head) < tableLen {
		return false
	}
	prev := -1
	nonZero := 0
	hasFirst := false
	for i := 0; i < 16; i++ {
		off := int(binary.LittleEndian.Uint32(head[i*4 : i*4+4]))
		if off == 0 {
			continue // placeholder de chunk vazio
		}
		if off == -1 { // 0xFFFFFFFF: terminador — nada depois importa.
			break
		}
		if off < tableLen || off >= len(head) || off <= prev {
			return false
		}
		if off == tableLen {
			hasFirst = true
		}
		prev = off
		nonZero++
	}
	return nonZero > 0 && hasFirst
}

// detectEventPattern valida o header de events/eventtable: o u16 inicial é
// o tamanho da tabela de headers (múltiplo de 8; a primeira entrada também
// aponta para o começo do conteúdo — os offsets são relativos ao INÍCIO do
// arquivo), e os offsets u16 das entradas caem em textos VÁLIDOS
// (ffxstr.PointerStatusAt — PointerOK na fronteira de bloco, PointerEmpty
// no terminador). Exige razão alta para não colidir com outros .bin.
func detectEventPattern(path string, head []byte) (Detection, bool) {
	if len(head) < 10 {
		return Detection{}, false
	}
	first := int(binary.LittleEndian.Uint16(head[0:2]))
	if first < 8 || first%8 != 0 || first > len(head) {
		return Detection{}, false
	}
	count := first / 8
	if len(head) <= first {
		return Detection{}, false
	}
	sample := count
	if sample > 32 {
		sample = 32
	}
	ok, empty := 0, 0
	for i := 0; i < sample; i++ {
		regular := int(binary.LittleEndian.Uint16(head[i*8 : i*8+2]))
		simplified := int(binary.LittleEndian.Uint16(head[i*8+4 : i*8+6]))
		for _, off := range []int{regular, simplified} {
			if off < first {
				continue // aponta para a própria tabela de headers
			}
			switch ffxstr.PointerStatusAt(head, off) {
			case ffxstr.PointerOK:
				ok++
			case ffxstr.PointerEmpty:
				empty++
			}
		}
	}
	total := sample * 2
	if total == 0 || ok+empty < total*6/10 || ok == 0 {
		return Detection{}, false
	}
	kind := detectEventTableKind(path)
	format := FormatEvent
	detail := "padrão de ponteiros de events válido"
	if kind != services.KindEvents {
		format = FormatEventTable
		detail = "padrão de ponteiros de events válido (tabela " + kind + ")"
	}
	version := guessVersion(path, format)
	return Detection{
		Format:       format,
		Kind:         kind,
		Version:      version,
		VersionKnown: pathSaysVersion(path),
		Detail:       detail,
	}, true
}

// detectEventTableKind refina o kind das tabelas pelo CAMINHO/nome (a régua
// do matchVbfPath, aplicada ao que o caminho avulso revelar; sem dicas,
// events é o default da família).
func detectEventTableKind(path string) string {
	lower := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	base := strings.ToLower(filepath.Base(lower))
	switch {
	case strings.Contains(lower, "/battle/btl/tuto0000/"):
		return services.KindTutorial
	case strings.Contains(lower, "/battle/btl/menumain/"):
		return services.KindMenuMain
	case strings.Contains(lower, "/cloudsave/"):
		return services.KindCloud
	case base == "tutorial.msb":
		return services.KindTutorial
	case base == "menumain.bin" && strings.Contains(lower, "/menu/"):
		return services.KindMenuMain
	case strings.Contains(lower, "/battle/btl/"):
		return services.KindBattleText
	default:
		return services.KindEvents
	}
}

// detectLockit reconhece o loc kit: registros CRLF/LF, pelo menos dois, sem
// NUL (binário de controle) e o nome físico <stem>_<lang>.bin (o arquivo é
// único por idioma). A codificação game/utf8 é decidida no decode.
func detectLockit(path string, head []byte) (Detection, bool) {
	base := strings.ToLower(filepath.Base(path))
	stem := strings.TrimSuffix(base, ".bin")
	lang := lockitLanguageSuffix(stem)
	if lang == "" {
		return Detection{}, false
	}
	if len(head) == 0 || strings.ContainsRune(string(head), 0) {
		return Detection{}, false
	}
	records := strings.Count(string(head), "\n")
	if records < 1 { // 1 quebra = 2 registros (ou 1 terminado)
		return Detection{}, false
	}
	version := lockitOrMacroSaysVersion(stem[:len(stem)-3])
	known := version != (common.GameVersion{})
	return Detection{
		Format:       FormatLockit,
		Kind:         services.KindLockit,
		Version:      version,
		VersionKnown: known,
		Detail:       "registros CRLF por idioma (lockit)",
	}, true
}

// lockitLanguageSuffix devolve o idioma do sufixo _<lang> do lockit
// (ffx_loc_kit_ps3_us → "us"), "" quando o nome não tem o padrão.
func lockitLanguageSuffix(stem string) string {
	if idx := strings.LastIndex(stem, "_"); idx >= 0 && len(stem)-idx-1 == 2 {
		lang := stem[idx+1:]
		if _, ok := common.SupportedLanguages[lang]; ok {
			return lang
		}
	}
	return ""
}

// lockitOrMacroSaysVersion resolve a versão pelos arquivos exclusivos de
// cada árvore: stems ffx_loc_kit_ps3/ffx2_loc_kit_ps3 do lockit (o macrodic
// em si não é exclusivo — o mesmo nome existe nas duas árvores).
func lockitOrMacroSaysVersion(stem string) common.GameVersion {
	switch {
	case strings.HasPrefix(stem, "ffx2_"):
		return common.GameVersionFFX2
	case strings.HasPrefix(stem, "ffx_"):
		return common.GameVersionFFX
	default:
		return common.GameVersion{}
	}
}

// pathSaysVersion informa se o caminho revela a árvore (ffx2/ffx-2/ffx_ps2/
// ffx2 → FFX-2; ffx_data/ffx/ → FFX) — a mesma régua do vbfVersionOf.
func pathSaysVersion(path string) bool {
	return guessVersion(path, FormatUnknown) != (common.GameVersion{})
}

// guessVersion adivinha a versão pelos sinais do CAMINHO e do formato:
// ffx2/ffx-2/ffx_ps2/ffx2 ⇒ FFX-2; lm_* ⇒ Last Mission (árvore ffx2);
// help ⇒ FFX. events não tem header próprio nas versões (o binário é o
// mesmo) — vale o que o caminho disser.
func guessVersion(path string, format Format) common.GameVersion {
	lower := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	base := strings.ToLower(filepath.Base(lower))
	stem := strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))

	switch {
	case strings.Contains(lower, "ffx2"),
		strings.Contains(lower, "ffx-2"),
		strings.Contains(lower, "ffx_ps2/ffx2/"):
		return common.GameVersionFFX2
	case strings.Contains(lower, "ffx_data"), strings.Contains(lower, "/ffx/"):
		return common.GameVersionFFX
	case strings.HasPrefix(stem, "lm_"):
		// Kernel de Last Mission (lm_*): divide a árvore do FFX-2.
		return common.GameVersionLastMiss
	case format == FormatHelp, strings.HasSuffix(base, ".sps2"):
		return common.GameVersionFFX
	case base == "tutorial.msb":
		return common.GameVersionFFX2
	case base == "menumain.bin" && strings.Contains(lower, "/menu/"):
		return common.GameVersionFFX
	default:
		return common.GameVersion{}
	}
}

// looksLikeJSON confere o começo esperado de um artefato JSON (objeto {).
func looksLikeJSON(head []byte) bool {
	for _, b := range head {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

// looksLikeStrings confere o cabeçalho de bloco do formato .strings.
func looksLikeStrings(head []byte) bool {
	return strings.Contains(string(head), "/*key=")
}

// looksLikeCSV confere se o conteúdo parece CSV (tem vírgulas e linhas).
func looksLikeCSV(head []byte) bool {
	s := string(head)
	// CSV deve ter pelo menos uma vírgula e quebra de linha
	if !strings.Contains(s, ",") {
		return false
	}
	if !strings.Contains(s, "\n") && !strings.Contains(s, "\r") {
		return false
	}
	// Header esperado: version,id,name,index,lang,hash,text,divergent
	// Aceita se tem "version,id" ou "hash,text" nas primeiras linhas
	lines := strings.SplitN(s, "\n", 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "version") && strings.Contains(line, "id") {
			return true
		}
		if strings.Contains(line, "hash") && strings.Contains(line, "text") {
			return true
		}
	}
	// Se tem estrutura de CSV com múltiplas colunas, aceita
	fields := strings.Split(lines[0], ",")
	return len(fields) >= 7
}

// looksLikeXLSX confere a assinatura PK (ZIP) do XLSX.
func looksLikeXLSX(head []byte) bool {
	// XLSX é um ZIP com assinatura PK\x03\x04 ou PK\x05\x06 ou PK\x07\x08
	if len(head) < 4 {
		return false
	}
	return (head[0] == 'P' && head[1] == 'K' &&
		(head[2] == 0x03 && head[3] == 0x04 ||
			head[2] == 0x05 && head[3] == 0x06 ||
			head[2] == 0x07 && head[3] == 0x08))
}
