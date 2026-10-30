package services

// EXTRAÇÃO E EXPORT DA SELEÇÃO DA ÁRVORE DO .vbf.
//
// A seleção do frontend é uma lista de CAMINHOS internos do container —
// arquivos OU diretórios (marcados com checkbox tri-state). Diretório
// marcado = "tudo abaixo dele": a expansão contra o índice acontece AQUI,
// no backend, porque a árvore do frontend é preguiçosa (só o diretório
// aberto tem filhos).
//
// Dois destinos, SEMPRE preservando a estrutura interna de caminhos:
//
//	Extrair (binário)   → bytes decomprimidos em <destRoot>/<caminho do
//	                      container>. "Extrair binário" usa data/ do jogo;
//	                      "Extrair para…" abre o seletor com data/ de
//	                      default. Nada aqui escreve em mods/ nem no .vbf.
//	Exportar (formato)  → kinds de TEXTO decodificados pela maquinaria de
//	                      builders e gravados em mods/edits (JSON/.strings),
//	                      mesmo lugar e convenção do export de data/.
//
// Nenhum dos dois lê o container inteiro: a expansão é um passe sobre o
// índice (em memória) e cada arquivo sai por blocos, streaming, sem o cap
// de 64 MiB do decode em memória (vídeos/executáveis saem inteiros).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/vbf"
	"ffxresources/backend/formatters"
	jsonfmt "ffxresources/backend/formatters/json"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/progress"
	"ffxresources/backend/uilog"
)

// vbfDefaultExtractRoot é o destino de "Extrair binário": a árvore data/
// do jogo, que o app lê como fonte da verdade. Os caminhos internos do
// container (ffx_ps2/…) reproduzem exatamente a estrutura de data/.
func vbfDefaultExtractRoot() string {
	return filepath.Join(common.GameFilesRoot, common.DirData)
}

// vbfExtractTarget monta o destino sem permitir que um caminho vindo do
// índice escape da pasta escolhida (absolute/drive/..). Também rejeita
// symlinks em componentes abaixo do destino: a extração sobrescreve arquivos
// e não deve seguir links preexistentes para gravar fora da pasta escolhida.
// createDirs=false é usado pelo preview, que não deve modificar o disco.
func vbfExtractTarget(destRoot, innerPath string, createDirs bool) (string, error) {
	if strings.TrimSpace(destRoot) == "" {
		destRoot = vbfDefaultExtractRoot()
	}
	innerPath = strings.ReplaceAll(innerPath, "\\", "/")
	if innerPath == "" || strings.HasPrefix(innerPath, "/") || filepath.IsAbs(filepath.FromSlash(innerPath)) {
		return "", fmt.Errorf("caminho interno inválido no .vbf: %q", innerPath)
	}
	parts := strings.Split(innerPath, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("caminho interno inválido no .vbf: %q", innerPath)
		}
	}
	root, err := filepath.Abs(destRoot)
	if err != nil {
		return "", fmt.Errorf("destino inválido %s: %w", destRoot, err)
	}
	target := filepath.Join(append([]string{root}, filepath.FromSlash(innerPath))...)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("caminho de extração sai do destino: %q", innerPath)
	}

	parent := root
	for _, part := range parts[:len(parts)-1] {
		parent = filepath.Join(parent, part)
		info, statErr := os.Lstat(parent)
		if os.IsNotExist(statErr) && createDirs {
			if err := os.Mkdir(parent, 0o755); err != nil && !os.IsExist(err) {
				return "", fmt.Errorf("criando diretório %s: %w", parent, err)
			}
			info, statErr = os.Lstat(parent)
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("verificando diretório %s: %w", parent, statErr)
		}
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("destino contém link simbólico: %s", parent)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("componente do destino não é diretório: %s", parent)
			}
		}
	}
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("arquivo de destino é link simbólico: %s", target)
		}
		if info.IsDir() {
			return "", fmt.Errorf("arquivo de destino é diretório: %s", target)
		}
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("verificando destino %s: %w", target, statErr)
	}
	return target, nil
}

// expandVbfSelection resolve a seleção (arquivos + prefixos de diretório)
// nas entries concretas do índice, sem duplicar (um arquivo alcançado por
// dois caminhos sai uma vez). macrodic.dcp marcado como grupo extrai o
// ARQUIVO do dicionário (os chunks são virtuais).
func vbfSelectionPath(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "\\", "/")
	if raw == "" {
		return "", nil // raiz do container
	}
	if strings.HasPrefix(raw, "/") || filepath.IsAbs(filepath.FromSlash(raw)) {
		return "", fmt.Errorf("caminho absoluto não permitido na seleção do .vbf: %q", raw)
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("caminho inválido na seleção do .vbf: %q", raw)
		}
	}
	clean := cleanVbfPath(raw)
	if clean == "" {
		return "", fmt.Errorf("caminho inválido na seleção do .vbf: %q", raw)
	}
	return clean, nil
}

func expandVbfSelection(a *vbf.Archive, paths []string) ([]vbf.Entry, error) {
	candidates := make(map[string]vbf.Entry, len(paths))
	rules := make(map[string]bool, len(paths))
	for _, p := range paths {
		excluded := strings.HasPrefix(p, "!")
		if excluded {
			p = strings.TrimPrefix(p, "!")
		}
		clean, err := vbfSelectionPath(p)
		if err != nil {
			return nil, err
		}
		if excluded && clean == "" {
			return nil, fmt.Errorf("exceção vazia inválida na seleção do .vbf")
		}
		rules[clean] = !excluded
		if excluded {
			continue
		}
		if clean != "" {
			// A path can be both a file and a directory prefix. The tree
			// uses the directory node (Archive.List's rule), so expansion
			// must prefer descendants before treating it as a single file.
			if a.IsDir(clean) {
				for _, e := range a.FilesUnder(clean) {
					candidates[cleanVbfPath(e.Path)] = e
				}
				continue
			}
			if e, ok := a.Entry(clean); ok {
				// É arquivo: extrai o próprio.
				candidates[cleanVbfPath(e.Path)] = e
				continue
			}
			continue
		}
		// Diretório (ou raiz vazia): os arquivos abaixo do prefixo.
		for _, e := range a.FilesUnder(clean) {
			candidates[cleanVbfPath(e.Path)] = e
		}
	}
	// Regra mais específica vence. Assim `!dir/file` exclui um arquivo
	// dentro de `dir`, e `dir/file` pode incluí-lo de volta sob uma exceção
	// `!dir`.
	includes := func(filePath string) bool {
		key := cleanVbfPath(filePath)
		for {
			if include, ok := rules[key]; ok {
				return include
			}
			i := strings.LastIndexByte(key, '/')
			if i < 0 {
				if include, ok := rules[""]; ok {
					return include
				}
				return false
			}
			key = key[:i]
		}
	}
	out := make([]vbf.Entry, 0, len(candidates))
	for path, e := range candidates {
		if includes(path) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// PreviewVbfExtraction devolve o resumo da seleção ANTES de extrair:
// quantos arquivos, quantos bytes e quantos já existem no destino (o
// aviso de sobrescrita). destRoot vazio = data/ do jogo.
func (s *MetadataService) PreviewVbfExtraction(vbfPath string, paths []string, destRoot string) (dto.VbfExtractPreview, error) {
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		return dto.VbfExtractPreview{}, err
	}
	if strings.TrimSpace(destRoot) == "" {
		destRoot = vbfDefaultExtractRoot()
	}
	entries, err := expandVbfSelection(a, paths)
	if err != nil {
		return dto.VbfExtractPreview{}, err
	}
	out := dto.VbfExtractPreview{Files: len(entries), DestRoot: destRoot}
	for _, e := range entries {
		out.Bytes += e.Size
		target, terr := vbfExtractTarget(destRoot, e.Path, false)
		if terr != nil {
			return dto.VbfExtractPreview{}, terr
		}
		if _, serr := os.Lstat(target); serr == nil {
			out.Existing++
		} else if !os.IsNotExist(serr) {
			return dto.VbfExtractPreview{}, fmt.Errorf("verificando destino %s: %w", target, serr)
		}
	}
	return out, nil
}

// ExtractVbfSelection extrai os arquivos da seleção para destRoot
// (vazio = data/ do jogo), preservando a estrutura interna de caminhos.
// Progresso: batch de UMA ação (modal do frontend, tick por arquivo). O
// .vbf nunca é escrito e data/ é tocado só aqui — cópia direta de bytes
// decomprimidos, sem passar pelos writers do app.
func (s *MetadataService) ExtractVbfSelection(vbfPath string, paths []string, destRoot string) (dto.BatchResult, error) {
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		return dto.BatchResult{}, err
	}
	if strings.TrimSpace(destRoot) == "" {
		destRoot = vbfDefaultExtractRoot()
	}
	entries, err := expandVbfSelection(a, paths)
	if err != nil {
		return dto.BatchResult{}, err
	}
	if len(entries) == 0 {
		return dto.BatchResult{}, fmt.Errorf("seleção vazia ou nenhum arquivo no container")
	}
	if err := common.EnsurePathExists(destRoot); err != nil {
		return dto.BatchResult{}, fmt.Errorf("criando destino %s: %w", destRoot, err)
	}

	out := dto.BatchResult{Total: len(entries)}
	prog, step := s.progressStep(fmt.Sprintf("Extraindo %s", filepath.Base(vbfPath)), len(entries))
	defer prog.End()

	for _, e := range entries {
		target, err := vbfExtractTarget(destRoot, e.Path, true)
		if err != nil {
			out.Failed = append(out.Failed, fmt.Sprintf("%s: %v", e.Path, err))
			step.Issue(e.Path, err.Error())
			continue
		}
		if err := a.ExtractEntry(e, target); err != nil {
			out.Failed = append(out.Failed, fmt.Sprintf("%s: %v", e.Path, err))
			step.Issue(e.Path, err.Error())
			continue
		}
		out.Done = append(out.Done, e.Path)
		step.Step(e.Path)
	}
	loggingService.Info("vbf: extraídos %d/%d arquivos para %s",
		len(out.Done), out.Total, destRoot)
	return out, nil
}

// ExtractVbfImagesSelection decodifica somente as imagens escolhidas (ou
// encontradas sob diretórios marcados) e grava DDS/PNG em
// destRoot/<caminho-interno-no-vbf>, sem alterar o container.
// destRoot vazio usa mods/images, o destino padrão dos artefatos de
// trabalho quando uma imagem é extraída da árvore data/.
func (s *MetadataService) ExtractVbfImagesSelection(vbfPath string, paths []string, destRoot string) (dto.BatchResult, error) {
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		return dto.BatchResult{}, err
	}
	entries, err := expandVbfSelection(a, paths)
	if err != nil {
		return dto.BatchResult{}, err
	}
	base := filepath.Base(vbfPath)
	images := make([]vbf.Entry, 0, len(entries))
	for _, entry := range entries {
		target, ok := matchVbfPath(base, entry.Path)
		if ok && target.Kind == KindImages {
			images = append(images, entry)
		}
	}
	out := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(images)}
	if len(images) == 0 {
		return out, nil
	}
	if strings.TrimSpace(destRoot) == "" {
		destRoot = filepath.Join(common.GameFilesRoot, common.ModsFolder, common.ModsImagesDir)
	}
	if err := common.EnsurePathExists(destRoot); err != nil {
		return dto.BatchResult{}, fmt.Errorf("criando destino %s: %w", destRoot, err)
	}

	uilog.Info("Export", "Iniciando extração de %d imagem(ns) de %s", len(images), base)

	prog, step := s.progressStep(fmt.Sprintf("Extraindo %d imagem(ns) de %s", len(images), base), len(images))
	defer prog.End()
	for _, entry := range images {
		raw, err := a.Read(entry.Path)
		if err != nil {
			out.Failed = append(out.Failed, entry.Path+": "+err.Error())
			step.Issue(entry.Path, err.Error())
			continue
		}
		texture, err := ddsphyre.Parse(raw)
		if err != nil {
			out.Failed = append(out.Failed, entry.Path+": "+err.Error())
			step.Issue(entry.Path, err.Error())
			continue
		}
		dds, err := texture.ExtractToDDS()
		if err != nil {
			out.Failed = append(out.Failed, entry.Path+": "+err.Error())
			step.Issue(entry.Path, err.Error())
			continue
		}
		png, err := ddsphyre.DDSToPNG(dds)
		if err != nil {
			out.Failed = append(out.Failed, entry.Path+": "+err.Error())
			step.Issue(entry.Path, err.Error())
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Path), strings.ToLower(ddsphyre.Suffix)) {
			err = fmt.Errorf("caminho de imagem inválido no .vbf: %s", entry.Path)
		} else {
			stem := entry.Path[:len(entry.Path)-len(ddsphyre.Suffix)]
			ddsPath, err := writeVbfImageArtifact(destRoot, stem+".dds", dds)
			if err == nil {
				pngPath, err := writeVbfImageArtifact(destRoot, stem+".png", png)
				if err == nil {
					uilog.File("Export", ddsPath, uilog.StatusOk, "extraído como dds")
					uilog.File("Export", pngPath, uilog.StatusOk, "extraído como png")
				}
			}
		}
		if err != nil {
			out.Failed = append(out.Failed, entry.Path+": "+err.Error())
			step.Issue(entry.Path, err.Error())
			continue
		}
		out.Done = append(out.Done, entry.Path)
		step.Step(entry.Path)
	}
	return out, nil
}

func writeVbfImageArtifact(destRoot, innerPath string, data []byte) (string, error) {
	target, err := vbfExtractTarget(destRoot, innerPath, true)
	if err != nil {
		return "", err
	}
	if err := common.CheckWritablePath(target); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", fmt.Errorf("gravando %s: %w", filepath.Base(target), err)
	}
	return target, nil
}

// ExportVbfSelection decodifica os kinds de TEXTO da seleção e grava os
// artefatos JSON ou .strings em mods/edits — mesmo lugar, mesma convenção
// de nomes do export de data/. Arquivos fora do escopo do app (áudio,
// vídeo, .exe, texturas) são ignorados. O DTO é decodificado diretamente
// do container (sem refs/dedup da tabela de visualização), para o artefato
// conter entradas completas e não apontar para dados ausentes em data/.
func (s *MetadataService) ExportVbfSelection(vbfPath, format string, paths []string, langs []string) ([]string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "" && format != "json" && format != "strings" {
		return nil, fmt.Errorf("formato de export desconhecido: %s", format)
	}
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(vbfPath)

	// (kind, id) → caminho clicado no container. É essencial usar
	// GetVbfTextEntry abaixo: ExportJSON/ExportStrings leem data/ e mods/,
	// enquanto esta operação deve exportar os bytes do próprio container.
	byKind := make(map[string]map[string]string)
	skipped := make([]string, 0)
	entries, err := expandVbfSelection(a, paths)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		t, ok := matchVbfPath(base, e.Path)
		if !ok {
			skipped = append(skipped, e.Path)
			continue
		}
		if t.Kind == KindImages {
			// Textura não é texto: extract cobre o .dds.phyre cru.
			skipped = append(skipped, e.Path)
			continue
		}
		entries := byKind[t.Kind]
		if entries == nil {
			entries = make(map[string]string)
			byKind[t.Kind] = entries
		}
		if t.Kind == KindMacro {
			// O dicionário físico é um arquivo por localização. Selecionar
			// macrodic.dcp/chunk equivale a exportar a coleção do dicionário.
			entries[""] = e.Path
		} else if _, exists := entries[t.ID]; !exists {
			entries[t.ID] = e.Path
		}
	}
	if len(skipped) > 0 {
		loggingService.DiagInfo("vbf", "export: arquivos fora do escopo ignorados",
			map[string]any{"count": len(skipped), "first": skipped[0]})
	}
	if len(byKind) == 0 {
		// Pastas e arquivos fora dos kinds de texto são seleções válidas; a
		// ação de texto simplesmente não produz saída para eles.
		return []string{}, nil
	}

	// Versão do container (um .vbf só tem conteúdo de uma árvore).
	version := vbfVersionOf(base, "")
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}

	// Ordem canônica dos kinds para o progresso sair previsível.
	order := []string{
		KindEvents, KindBattleText, KindCloud, KindTutorial, KindMenuMain,
		KindHelp, KindMacro, KindObjects, KindLockit,
	}
	// Batch com UM STEP POR KIND presente na seleção — o modal lista as
	// ações com a barra própria de cada uma (declara upfront).
	present := make([]string, 0, len(byKind))
	for _, kind := range order {
		if _, ok := byKind[kind]; ok {
			present = append(present, kind)
		}
	}
	labels := make([]string, len(present))
	for i, kind := range present {
		if l, ok := kindStepLabels[kind]; ok {
			labels[i] = l
		} else {
			labels[i] = kind
		}
	}
	batch := s.progress.NewBatch(fmt.Sprintf("Extraindo texto de %s", base))
	steps := batch.Steps(labels...)
	// Sem emitter: steps nil — um step nil (no-op) por kind.
	if steps == nil {
		steps = make([]*progress.Step, len(present))
	}
	defer batch.End()
	var written []string
	for ki, kind := range present {
		selected := byKind[kind]
		step := steps[ki]
		if kind == KindHelp {
			// Help é um único artefato com dedup/ref entre os seis painéis.
			// Como no export de data/, se um painel foi marcado o artefato
			// inclui todos os painéis existentes no container.
			for _, id := range helpfile.HelpEntryNames() {
				if _, exists := selected[id]; exists {
					continue
				}
				for _, lang := range common.SupportedLanguageCodes() {
					rel, ok := originalRelPathLoc(KindHelp, id, version, lang)
					if ok && a.Has(rel) {
						selected[id] = rel
						break
					}
				}
			}
		}
		ids := make([]string, 0, len(selected))
		for id := range selected {
			if id != "" {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		step.Begin("", len(ids))

		collection := make(dto.Collection)
		if kind == KindMacro {
			macroPath := selected[""]
			chunks, cerr := s.ListVbfMacroChunks(vbfPath, macroPath)
			if cerr != nil {
				step.Fail(fmt.Errorf("listando chunks de macro do .vbf: %w", cerr).Error())
				return written, fmt.Errorf("listando chunks de macro do .vbf: %w", cerr)
			}
			ids = ids[:0]
			for _, chunk := range chunks {
				ids = append(ids, chunk.ID)
			}
			sort.Strings(ids)
			step.SetTotal(len(ids))
			for _, id := range ids {
				entry, eerr := s.vbfExportEntry(a, kind, id, macroPath, version)
				if eerr != nil {
					step.Fail(fmt.Errorf("lendo macro %s do .vbf: %w", id, eerr).Error())
					return written, fmt.Errorf("lendo macro %s do .vbf: %w", id, eerr)
				}
				collection[id] = entry
				step.Step(id)
			}
		} else {
			for _, id := range ids {
				entry, eerr := s.vbfExportEntry(a, kind, id, selected[id], version)
				if eerr != nil {
					step.Fail(fmt.Errorf("lendo %s/%s do .vbf: %w", kind, id, eerr).Error())
					return written, fmt.Errorf("lendo %s/%s do .vbf: %w", kind, id, eerr)
				}
				collection[id] = entry
				step.Step(id)
			}
		}
		if len(collection) == 0 {
			step.End()
			continue
		}
		// text/source vêm do CONTAINER (é o export do .vbf); target vem de
		// mods/ — a tradução, quando existir.
		collection = s.projectTargets(kind, version, collection)
		collection = filterExportRows(collection)
		if len(collection) == 0 {
			step.End()
			continue
		}
		outputs, werr := writeVbfExport(kind, version, ids, collection, format, langs)
		if werr != nil {
			step.Fail(fmt.Errorf("export %s do .vbf: %w", kind, werr).Error())
			return written, fmt.Errorf("export %s do .vbf: %w", kind, werr)
		}
		written = append(written, outputs...)
		step.End()
	}
	return written, nil
}

// vbfExportEntry decodifica um DTO RAW diretamente da fonte VBF. Não usa
// GetVbfTextEntry, que aplica dedup de sessão para exibição na tabela e pode
// produzir refs a arquivos que não fazem parte da seleção do export. A
// leitura é do CONTAINER (SourceVbf): o export do .vbf não deve herdar o
// texto de mods/ — o target da tradução é projetado pelo chamador.
func (s *MetadataService) vbfExportEntry(a *vbf.Archive, kind, id, clicked string, version common.GameVersion) (dto.FileEntry, error) {
	target := vbfTarget{Kind: kind, ID: id, Version: version}
	return vbfRun(a, target, clicked, func() (dto.FileEntry, error) {
		_, entry, exists, err := s.decodeVbfEstado(target, common.SourceVbf)
		if err != nil {
			return dto.FileEntry{}, err
		}
		if !exists {
			return dto.FileEntry{}, fmt.Errorf("%s/%s indisponível no .vbf e em mods/", kind, id)
		}
		return entry, nil
	})
}

// writeVbfExport espelha os mesmos caminhos de ExportJSON e
// ExportStrings, mas recebe a collection já montada a partir do
// container. O formatter vem do registry; a gravação é a camada de
// arquivo padronizada (formatters.NewWriter) — o MESMO shape do
// exportKind, com a extensão trocada pelo formato pedido.
func writeVbfExport(kind string, version common.GameVersion, ids []string, c dto.Collection, format string, langs []string) ([]string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "":
		format = "json"
	case "json", "strings":
	default:
		return nil, fmt.Errorf("formato de export desconhecido: %s", format)
	}
	f, ok := formatters.Get(format)
	if !ok {
		return nil, fmt.Errorf("formatter %s não registrado", format)
	}
	write := func(col dto.Collection, jsonPath string) (string, error) {
		return formatters.NewWriter(f, col, formatters.SwapExt(f, jsonPath), langs).Write()
	}

	switch kind {
	case KindEvents:
		p, err := eventsExportPath(version, ids, "")
		if err != nil {
			return nil, err
		}
		written, werr := write(c, p)
		return []string{written}, werr
	case KindObjects, KindLockit, KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		// Um arquivo por entrada (mesmo nome do JSON por entrada; dedup
		// intra-arquivo = self-contained).
		var paths []string
		for _, key := range c.SortedKeys() {
			jp, perr := jsonfmt.ObjectsJSONPath(key, version)
			if perr != nil {
				return nil, perr
			}
			p, werr := write(dto.Collection{key: c[key]}, jp)
			if werr != nil {
				return nil, werr
			}
			paths = append(paths, p)
		}
		return paths, nil
	case KindMacro:
		p, err := macroExportPath(version, nil, "")
		if err != nil {
			return nil, err
		}
		written, werr := write(c, p)
		return []string{written}, werr
	case KindHelp:
		p, err := jsonfmt.HelpJSONPath(version, "")
		if err != nil {
			return nil, err
		}
		written, werr := write(c, p)
		return []string{written}, werr
	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}
