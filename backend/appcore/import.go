// ImportText: artefato de texto APONTADO (.json/.strings/.csv/.xlsx/.po) +
// binários APONTADOS (multi-seleção) → binários reconstruídos em imported/
// ao lado de CADA binário. O import é ON THE FLY: tudo acontece apenas ali —
// nenhum store do frontend é mutado, nada vai para mods/.
//
// O casamento texto↔binário é por NOME (basename): as operações do menu
// não se preocupam com caminho — o binário pode estar em qualquer pasta,
// inclusive fora das conhecidas do .vbf. Binário sem correspondência nas
// keys do artefato é IGNORADO sem aviso (só log); binário de outro formato
// (ex.: events num JSON de Sistema) é erro que nomeia texto e binário.
//
// CLI futuro: `import <game> <formato> <texto> <binário> <saída>` parseia
// argv para este mesmo request.
package appcore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/services"
)

// ImportRequest é o pedido de import de texto.
type ImportRequest struct {
	// Text é o artefato escolhido (.json/.strings).
	Text string
	// Binaries são os binários apontados (multi-seleção do dialog); vazio
	// usa os caminhos declarados nas keys do artefato (com OutDir).
	Binaries []string
	// Kind é o kind esperado (submenu); "" aceita o kind das keys.
	Kind string
	// Version: Known=false usa a versão das metadata.keys do artefato.
	Version VersionRequest
	// OutDir muda o destino ("" = imported/ ao lado de cada binário).
	OutDir string
	// Collision é a política de sobrescrita do binário de saída.
	Collision CollisionPolicy
	// MacrodicPath aponta o dicionário sob demanda (ErrMacrodicUnavailable).
	MacrodicPath string
}

// ImportText aplica o artefato em cada binário casado. A versão vem da
// metadata.key do artefato (decisão registrada no plano: sem perguntar —
// o backend reprova key inexistente/mista/incompatível).
func ImportText(ctx context.Context, req ImportRequest) (ImportResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		return ImportResult{}, fmt.Errorf("nenhum arquivo de texto selecionado")
	}

	art, err := readArtifact(req.Text)
	if err != nil {
		return ImportResult{}, err
	}
	return importCollection(ctx, req, art)
}

// importCollection aplica uma Collection já parseada nos binários do pedido.
// É o caminho ÚNICO de import por arquivo: ImportText (JSON/CSV/XLSX/PO) lê o
// artefato e chama aqui. O DTO do artefato traz o texto alvo em Target — os
// appliers avulsos consomem Text[us], daí a projeção ApplyTargetAsText.
func importCollection(ctx context.Context, req ImportRequest, art dto.Collection) (ImportResult, error) {
	if len(req.Binaries) == 0 {
		return ImportResult{}, fmt.Errorf("nenhum binário selecionado")
	}

	art = builders.ApplyTargetAsText(art)

	kind, version, err := artifactTarget(art, req.Kind)
	if err != nil {
		return ImportResult{}, err
	}

	if _, err := prepareTextRuntime(version, kind, req.MacrodicPath); err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{Outcomes: make([]FileOutcome, 0, len(req.Binaries))}
	for _, binary := range req.Binaries {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		result.Outcomes = append(result.Outcomes, importOne(req, art, kind, version, binary))
	}
	return result, nil
}

// importOne processa UM binário do lote.
func importOne(req ImportRequest, art dto.Collection, kind string, version common.GameVersion, binary string) FileOutcome {
	outcome := FileOutcome{Input: binary, Kind: kind, Version: version.String(), Status: StatusFailed}

	d, err := DetectFile(binary)
	if err != nil {
		outcome.Status = StatusMismatch
		outcome.Reason = err.Error()
		return outcome
	}

	// Formato incorreto nomeia os DOIS arquivos: texto e binário.
	if !equalKind(kind, d.Kind) {
		inc := Incompatible(binary,
			fmt.Sprintf("%s (%s)", kindLabel(kind), filepath.Base(req.Text)),
			kindLabel(d.Kind),
			"binário não corresponde ao formato do texto")
		outcome.Status = StatusMismatch
		outcome.Reason = inc.Error()
		return outcome
	}

	// Casamento por NOME: a(s) entrada(s) do artefato cujo arquivo da key
	// tem o mesmo basename. Sem correspondência → ignorado SEM aviso.
	matched, matchedKey, ok := matchEntries(kind, art, binary)
	if !ok {
		outcome.Status = StatusIgnored
		loggingService.Info("import: %s não encontrado no texto %s (ignorado)", filepath.Base(binary), filepath.Base(req.Text))
		return outcome
	}
	_ = matchedKey

	data, err := os.ReadFile(binary)
	if err != nil {
		outcome.Reason = fmt.Sprintf("lendo binário: %v", err)
		return outcome
	}

	lang := detectFileLanguage(binary)
	rebuilt, err := applyAvulso(d, version, binary, data, lang, matched)
	if err != nil {
		outcome.Status = StatusMismatch
		outcome.Reason = err.Error()
		return outcome
	}

	// Validação de reconstrução (a paridade de validação com o frontend,
	// no nível mais forte que existe): decodifica a SAÍDA e confere o
	// texto do idioma do arquivo — cada row reconstruída precisa ser o
	// texto ORIGINAL (não tocado) ou um dos textos APLICADOS do artefato.
	if err := verifyRebuilt(d, version, binary, rebuilt, data, matched, lang); err != nil {
		outcome.Reason = fmt.Sprintf("reconstrução inválida: %v", err)
		return outcome
	}

	target, existed := ImportOutputPath(binary, req.Collision)
	if req.OutDir != "" {
		target = filepath.Join(req.OutDir, filepath.Base(binary))
	}
	if existed && req.Collision == CollisionFail {
		outcome.Status = StatusCollision
		outcome.Reason = target
		return outcome
	}
	if err := writeChecked(target, rebuilt); err != nil {
		outcome.Reason = err.Error()
		return outcome
	}

	outcome.Output = target
	if existed {
		outcome.Status = StatusOverwritten
	} else {
		outcome.Status = StatusWritten
	}
	return outcome
}

// readArtifact parseia o artefato pelo formato detectado (extensão +
// conteúdo — o sniff pega o artefato trocado antes do parse genérico).
// O formatter vem do registry (chave canônica do formato); o parse é o
// MESMO para todo kind — os bytes JSON/strings são idênticos.
func readArtifact(path string) (dto.Collection, error) {
	d, err := DetectFile(path)
	if err != nil {
		return nil, err
	}
	format := d.Format.artifactRegistryKey()
	if format == "" {
		return nil, Incompatible(path, "artefato de texto", d.Format.String(), "arquivo não é .json, .strings, .csv, .xlsx nem .po")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, ok := formatters.Get(format)
	if !ok {
		return nil, fmt.Errorf("formatter %s não registrado", format)
	}
	c, err := f.Unmarshal(raw)
	if err != nil {
		return nil, fmt.Errorf("falha ao analisar %s: %w", format, err)
	}
	return c, nil
}

// artifactTarget valida as keys do artefato: kind único, versão presente e
// coerente (a versão é o primeiro segmento da metadata.key).
func artifactTarget(art dto.Collection, wantKind string) (string, common.GameVersion, error) {
	kind := ""
	version := common.GameVersion{}
	for _, id := range art.SortedKeys() {
		key := strings.TrimSpace(art[id].Metadata.Key)
		if key == "" {
			return "", version, Incompatible(id, "", "", "entrada sem metadata.key (arquivo não gerado por este app?)")
		}
		p, ok := dto.ParseKey(key)
		if !ok {
			return "", version, Incompatible(id, "", "", fmt.Sprintf("metadata.key inválida: %q", key))
		}
		v, err := common.ParseGameVersionStrict(p.Version)
		if err != nil {
			return "", version, Incompatible(id, "", "", fmt.Sprintf("versão desconhecida na key: %q", p.Version))
		}
		if version.String() == "" {
			version = v
		} else if common.VersionPathName(version) != common.VersionPathName(v) {
			return "", version, Incompatible(id, "", "",
				fmt.Sprintf("arquivo mistura versões (%s e %s)", version, v))
		}
		if k := services.KindFromKey(key, v); k == "" {
			return "", version, Incompatible(id, "", "", "entrada fora do escopo do app (lista de skip)")
		} else if kind == "" {
			kind = k
		} else if k != kind {
			return "", version, Incompatible(id, "", "",
				fmt.Sprintf("arquivo mistura kinds diferentes (%s e %s)", kind, k))
		}
	}
	if kind == "" {
		return "", version, Incompatible("", "", "", "arquivo sem entradas")
	}
	if wantKind != "" && !equalKind(wantKind, kind) {
		return "", version, Incompatible("", kindLabel(wantKind), kindLabel(kind),
			"artefato não corresponde ao submenu escolhido")
	}
	return kind, version, nil
}

// matchEntries casa o binário apontado com as entradas do artefato pelo
// NOME do arquivo declarado na key (basename). Lockit é especial: o
// arquivo físico é <stem>_<lang>.bin e a key declara o nome lógico
// <stem>.bin; macrodic traz TODAS as entradas (o container é um artefato
// só por localização).
func matchEntries(kind string, art dto.Collection, binary string) (dto.Collection, string, bool) {
	base := strings.ToLower(filepath.Base(binary))

	if kind == services.KindMacro {
		// O dicionário é um arquivo só: todas as entradas do artefato.
		return art, "macrodic.dcp", len(art) > 0
	}

	out := make(dto.Collection, len(art))
	matched := ""
	for _, id := range art.SortedKeys() {
		p, ok := dto.ParseKey(art[id].Metadata.Key)
		if !ok {
			continue
		}
		name := strings.ToLower(p.FileName)
		if kind == services.KindLockit {
			// nome físico <stem>_<lang>.bin ↔ nome lógico <stem>.bin:
			// o sufixo de idioma do arquivo físico é descartado no match.
			trimmed := base
			if lang := lockitLanguageSuffix(strings.TrimSuffix(base, filepath.Ext(base))); lang != "" {
				stem := strings.TrimSuffix(base, filepath.Ext(base))
				trimmed = stem[:len(stem)-len(lang)-1] + filepath.Ext(base)
			}
			if strings.EqualFold(trimmed, name) || strings.EqualFold(base, name) {
				out[id] = art[id]
				matched = id
			}
			continue
		}
		if strings.EqualFold(base, name) {
			out[id] = art[id]
			matched = id
		}
	}
	return out, matched, len(out) > 0
}

// verifyRebuilt decodifica o binário reconstruído e o ORIGINAL, e confere
// row a row: o texto do idioma do arquivo precisa ser o original (não
// tocado) ou um dos textos aplicados do artefato. Capacidade estourada,
// offsets quebrados ou codificação errada aparecem como texto fora das
// duas casas — é a validação mais forte de reconstrução que existe.
func verifyRebuilt(d Detection, version common.GameVersion, path string, rebuilt, original []byte, matched dto.Collection, lang string) error {
	decoded, err := decodeAvulso(d, version, path, rebuilt, lang)
	if err != nil {
		return fmt.Errorf("saída não decodifica: %w", err)
	}
	base, err := decodeAvulso(d, version, path, original, lang)
	if err != nil {
		return fmt.Errorf("original não decodifica: %w", err)
	}
	for _, id := range matched.SortedKeys() {
		want, ok := decoded[id]
		if !ok {
			return fmt.Errorf("entrada %s sumiu na reconstrução", id)
		}
		origEntry, ok := base[id]
		if !ok {
			return fmt.Errorf("entrada %s ausente no original", id)
		}
		applied := appliedTexts(matched[id], lang)
		if len(want.Rows) != len(origEntry.Rows) {
			return fmt.Errorf("%s: row count divergiu (%d vs %d)", id, len(want.Rows), len(origEntry.Rows))
		}
		orig := make(map[int]string, len(origEntry.Rows))
		for _, row := range origEntry.Rows {
			orig[row.Index] = row.Text[lang]
		}
		for _, row := range want.Rows {
			text := row.Text[lang]
			if text == orig[row.Index] {
				continue
			}
			if applied[text] {
				continue
			}
			return fmt.Errorf("%s: row %d reconstruída com texto inesperado", id, row.Index)
		}
	}
	return nil
}

// appliedTexts indexa os textos do idioma aplicados pela entrada do artefato
// (o conjunto permitido além do original — inclui propagação: cópias
// idênticas recebem exatamente um dos textos do lote).
func appliedTexts(entry dto.FileEntry, lang string) map[string]bool {
	out := make(map[string]bool, len(entry.Rows))
	for _, row := range entry.Rows {
		if t := row.Text[lang]; t != "" {
			out[t] = true
		}
	}
	return out
}
