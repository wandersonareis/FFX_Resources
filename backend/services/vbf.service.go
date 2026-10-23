package services

// NAVEGADOR DE .vbf — SOMENTE LEITURA.
//
// O container (~20 GB) nunca é lido inteiro nem pré-extraído: a árvore da
// sidebar sai do ÍNDICE (cabeçalho, ~10-18 MB) e só o arquivo clicado é
// decodificado, sob demanda, em memória. Nada aqui escreve no .vbf — nem o
// export, que lê do container (ou de um temp no dir de execução) e grava
// sempre em mods/.
//
// Fluxo por clique:
//
//	ListVbfRoots   → descobre os .vbf perto do executável do jogo;
//	ListVbfDir     → filhos imediatos de um diretório (só paths/contagens);
//	ListVbfMacroChunks → filhos virtuais de macrodic.dcp (chunk_XX);
//	GetVbfTextEntry    → decodifica o arquivo e devolve a tabela;
//	GetVbfImageEntry   → decodifica a textura e devolve a pré-visualização.
//
// Os dois Get* rodam DENTRO de common.WithVbfSourceReader: o overlay fica
// vivo só durante a chamada (escopo por chamada, serializado) e some no fim,
// quaisquer que sejam retorno ou pânico.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/fileFormats/eventtable"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/fileFormats/objectsfile"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/fileFormats/vbf"
	"ffxresources/backend/interactions"
	"ffxresources/backend/loggingService"
)

// ---------------------------------------------------------------------------
// descoberta das raízes
// ---------------------------------------------------------------------------

// gameExePath devolve o executável do jogo configurado ("" quando ausente).
// É dele que sai o diretório onde os .vbf moram — um campo de ARQUIVO no
// config, fora do map Locations (que é de diretórios e faz MkdirAll).
func gameExePath() string {
	svc := interactions.NewInteractionService()
	if svc == nil {
		return ""
	}
	cfg := svc.FFXAppConfig()
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.GetGameExeLocation())
}

// vbfSearchDirs devolve os diretórios varridos por *.vbf: a pasta do
// executável e a pasta data/ ao lado dele.
func vbfSearchDirs() []string {
	exe := gameExePath()
	if exe == "" {
		return nil
	}
	dir := filepath.Dir(exe)
	return []string{dir, filepath.Join(dir, common.DirData)}
}

// discoverVbfRoots varre os diretórios de busca. Sem executável configurado
// devolve vazio (não é erro: ainda não há o que navegar).
func discoverVbfRoots() []dto.VbfRoot {
	seen := map[string]bool{}
	roots := make([]dto.VbfRoot, 0, 2)

	for _, dir := range vbfSearchDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // diretório inexistente/ilegível: segue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".vbf") {
				continue
			}
			abs, aerr := filepath.Abs(filepath.Join(dir, e.Name()))
			if aerr != nil {
				continue
			}
			key := strings.ToLower(abs)
			if seen[key] {
				continue
			}
			info, ierr := e.Info()
			if ierr != nil {
				continue
			}
			seen[key] = true
			roots = append(roots, dto.VbfRoot{
				Name:    e.Name(),
				Path:    abs,
				Version: vbfVersionOf(e.Name(), "").String(),
				Size:    info.Size(),
			})
		}
	}

	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return roots
}

// ---------------------------------------------------------------------------
// registro de containers abertos
// ---------------------------------------------------------------------------

var (
	vbfArchMu   sync.Mutex
	vbfArchives = map[string]*vbf.Archive{}
)

// vbfArchiveFor abre (uma única vez) e devolve o .vbf pedido. Só aceita
// caminhos que a descoberta encontrou perto do executável: o argumento vem
// da UI e não pode virar leitura arbitrária de qualquer arquivo do disco.
//
// Manter o handle aberto evita reler o cabeçalho (10-18 MB) a cada clique;
// a descoberta é barata, então é refeita a cada chamada — o exe pode ter
// mudado de lugar.
func vbfArchiveFor(p string) (*vbf.Archive, error) {
	abs, err := filepath.Abs(strings.TrimSpace(p))
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(abs)

	vbfArchMu.Lock()
	defer vbfArchMu.Unlock()

	if a, ok := vbfArchives[key]; ok {
		return a, nil
	}
	known := false
	for _, r := range discoverVbfRoots() {
		if strings.EqualFold(r.Path, abs) {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf(".vbf não está na pasta do jogo: %s", filepath.Base(abs))
	}
	a, err := vbf.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", filepath.Base(abs), err)
	}
	vbfArchives[key] = a
	return a, nil
}

// CloseVbfArchives fecha os containers abertos (chamado no shutdown do app).
func CloseVbfArchives() {
	vbfSessionResetAll() // a sessão descreve os containers: cai junto
	vbfArchMu.Lock()
	defer vbfArchMu.Unlock()
	for key, a := range vbfArchives {
		_ = a.Close()
		delete(vbfArchives, key)
	}
}

// ---------------------------------------------------------------------------
// bindings de árvore
// ---------------------------------------------------------------------------

// ListVbfRoots devolve as raízes .vbf da sidebar, com o total de arquivos do
// índice (informação de cabeçalho — a árvore em si nunca é pré-extraída).
func (s *MetadataService) ListVbfRoots() ([]dto.VbfRoot, error) {
	if gameExePath() == "" {
		return []dto.VbfRoot{}, nil
	}
	found := discoverVbfRoots()
	out := make([]dto.VbfRoot, 0, len(found))
	for _, r := range found {
		if !isContentVbf(r.Name) {
			// A pasta do jogo tem outros .vbf (ex.: metamenu.vbf) que não
			// são árvore de conteúdo traduzível: a sidebar mostra UM
			// container por versão, e este não é um deles.
			continue
		}
		a, err := vbfArchiveFor(r.Path)
		if err != nil {
			// Não é um container legível: some da árvore. Só paths e
			// contagens no diagnóstico — nunca conteúdo de arquivo.
			loggingService.DiagInfo("vbf", "ignorando "+r.Name,
				map[string]any{"path": r.Path, "erro": err.Error()})
			continue
		}
		r.Entries = a.Len()
		out = append(out, r)
	}
	// Uma vez com a lista completa: mods/ é compartilhado entre FFX e
	// FFX-2, então um arquivo só é órfão quando não existe em NENHUM
	// container (checar por container marcaria como erro o arquivo que só
	// existe no outro jogo).
	s.checkVbfSync(out)
	return out, nil
}

// ---------------------------------------------------------------------------
// sincronismo mods/ ↔ container
// ---------------------------------------------------------------------------

// isContentVbf diz se o container é um dos de CONTEÚDO do jogo: um por
// versão (FFX_Data.vbf na aba FFX, FFX2_Data.vbf na FFX-2, nenhum na Last
// Mission, que reusa a árvore do FFX-2 sem container próprio). Os nomes
// casam com vbfVersionOf, então a filtragem por versão no frontend cai
// sempre em exatamente um.
func isContentVbf(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ffx_data.vbf", "ffx2_data.vbf":
		return true
	default:
		return false
	}
}

// modsTreeIgnored são subdiretórios de mods/ que NÃO são arquivos do jogo:
// artefatos de export (edits/), imagens extraídas (images/) e a árvore de
// tradução reimportável (translated/). Tudo o resto em mods/ é o espelho de
// um caminho do jogo.
var modsTreeIgnored = map[string]bool{
	"edits":              true,
	common.ModsImagesDir: true,
	"translated":         true,
	"extracted":          true,
	"reimported":         true,
}

// vbfSyncSeen guarda a assinatura (lista ordenada dos órfãos) já avisada
// por container: a listagem de raízes roda a cada refresh da sidebar e o
// toast não pode repetir para o mesmo estado.
var (
	vbfSyncMu   sync.Mutex
	vbfSyncSeen = map[string]string{}
)

// checkVbfSync compara o que existe em mods/ com o ÍNDICE DE TODOS os
// containers descobertos JUNTOS e avisa quando há arquivo em mods/ sem
// contraparte em nenhum deles: o .vbf é a fonte da verdade, então mods/
// órfão é erro — aquele arquivo nunca pode ser aplicado ao jogo e passaria
// despercebido no export/import.
//
// mods/ é compartilhado entre FFX e FFX-2, por isso a comparação é com a
// união dos índices: existir no outro container já resolve.
//
// Arquivo ausente em mods/ NÃO é problema (mods/ guarda só o que já foi
// tocado). E o aviso não exibe conteúdo de arquivo: só caminhos e contagens
// (toast curto no topo direito; lista completa no diagnóstico).
func (s *MetadataService) checkVbfSync(roots []dto.VbfRoot) {
	if len(roots) == 0 {
		return
	}
	archives := make([]*vbf.Archive, 0, len(roots))
	names := make([]string, 0, len(roots))
	keys := make([]string, 0, len(roots))
	for _, r := range roots {
		a, err := vbfArchiveFor(r.Path)
		if err != nil {
			continue
		}
		archives = append(archives, a)
		names = append(names, r.Name)
		keys = append(keys, r.Path)
	}
	if len(archives) == 0 {
		return // nenhum container legível: nada a comparar
	}
	scope := strings.Join(keys, "\n")

	modsRoot := filepath.Join(common.GameFilesRoot, common.ModsFolder)
	if info, err := os.Stat(modsRoot); err != nil || !info.IsDir() {
		return // mods/ ainda não existe: nada a comparar
	}

	var missing []string
	_ = filepath.WalkDir(modsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // um item ilegível não aborta a varredura
		}
		rel, rerr := filepath.Rel(modsRoot, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		seg := rel
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			seg = rel[:i]
		}
		if d.IsDir() {
			if modsTreeIgnored[seg] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil // arquivos de sistema, não do jogo
		}
		if !hasInAnyArchive(archives, rel) {
			missing = append(missing, rel)
		}
		return nil
	})
	if len(missing) > 0 {
		sort.Strings(missing)
	}
	signature := strings.Join(missing, "\n")

	vbfSyncMu.Lock()
	changed := vbfSyncSeen[scope] != signature
	vbfSyncSeen[scope] = signature
	vbfSyncMu.Unlock()
	if !changed {
		return // mesmo estado já comunicado
	}

	if len(missing) == 0 {
		loggingService.DiagInfo("vbf", "mods/ em dia com os containers",
			map[string]any{"containers": names})
		return
	}

	// Toast: contagem + poucos caminhos (a lista completa é do log).
	preview := missing
	const maxShown = 5
	if len(preview) > maxShown {
		preview = preview[:maxShown]
	}
	msg := fmt.Sprintf(
		"%d arquivo(s) em mods/ sem contraparte em nenhum .vbf (os .vbf são a fonte da verdade): %s",
		len(missing), strings.Join(preview, ", "),
	)
	if len(missing) > len(preview) {
		msg += fmt.Sprintf(" e mais %d", len(missing)-len(preview))
	}
	loggingService.DiagWarn("vbf", msg, map[string]any{
		"containers": names,
		"contagem":   len(missing),
		"orfãos":     missing,
	})
	if s != nil && s.notifier != nil {
		s.notifier.NotifyWarn(msg)
	}
}

// hasInAnyArchive reporta se o caminho (relativo a um container) existe no
// índice de algum dos containers carregados.
func hasInAnyArchive(archives []*vbf.Archive, rel string) bool {
	for _, a := range archives {
		if a.Has(rel) {
			return true
		}
	}
	return false
}

// ListVbfDir devolve os filhos IMEDIATOS de um diretório do .vbf.
//
// Diretórios vêm primeiro, depois arquivos, ambos em ordem alfabética — e
// cada arquivo carrega kind/id/versão QUANDO o app sabe servi-lo (o caminho
// bate com o que o próprio app resolveria em data/). Vazio = formato fora do
// escopo (áudio, vídeo, executável…): o nó continua na árvore, mas abrir avisa.
func (s *MetadataService) ListVbfDir(vbfPath, dir string) ([]dto.VbfNode, error) {
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(vbfPath)

	nodes := a.List(dir)
	out := make([]dto.VbfNode, 0, len(nodes))
	for _, n := range nodes {
		dn := dto.VbfNode{Name: n.Name, Path: n.Path, IsDir: n.IsDir, Size: n.Size}
		if !n.IsDir {
			if t, ok := matchVbfPath(base, n.Path); ok {
				dn.Kind = t.Kind
				dn.ID = t.ID
				dn.Version = t.Version.String()
				dn.Macro = t.Kind == KindMacro
				dn.Image = t.Kind == KindImages
			}
		}
		out = append(out, dn)
	}
	return out, nil
}

// ListVbfMacroChunks devolve os filhos VIRTUAIS de macrodic.dcp: o
// dicionário é UM arquivo por localização e o app o trata como um grupo com
// um filho por chunk (chunk_00, chunk_01, …) — o mesmo id da árvore data/.
func (s *MetadataService) ListVbfMacroChunks(vbfPath, macroPath string) ([]dto.VbfNode, error) {
	a, t, err := s.vbfTargetOf(vbfPath, macroPath)
	if err != nil {
		return nil, err
	}
	if t.Kind != KindMacro {
		return nil, fmt.Errorf("não é macrodic.dcp: %s", cleanVbfPath(macroPath))
	}
	if err := ensureVersionReady(t.Version); err != nil {
		return nil, err
	}

	keys, err := vbfRun(a, t, macroPath, func() ([]string, error) {
		c, cerr := builders.BuildMacroDTOFromSource(t.Version, common.SourceVbf, nil)
		if cerr != nil {
			return nil, cerr
		}
		out := make([]string, 0, len(c))
		for k := range c {
			out = append(out, k)
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)

	out := make([]dto.VbfNode, 0, len(keys))
	for _, k := range keys {
		out = append(out, dto.VbfNode{
			Name:    k,
			Path:    macroPath,
			Kind:    KindMacro,
			ID:      k,
			Version: t.Version.String(),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// bindings de conteúdo
// ---------------------------------------------------------------------------

// GetVbfTextEntry devolve a tabela (rows + coluna Original) de um arquivo de
// TEXTO aberto pelo navegador de .vbf. id é exigido só para macro (o chunk);
// nos demais kinds o id sai do próprio caminho.
//
// Diferente de GetEntry, nada aqui é data-driven: o estado ATUAL vem do
// container (mods-first) e o ORIGINAL do container puro — os dois do mesmo
// .vbf. O dedup de display é o da SESSÃO de cliques (vbfSessionStore): o
// def é a 1ª carga da sessão — com o texto linkado na UI, editar "na cópia"
// é editar a def. Divergência de estrutura continua sendo reportada
// (paths/contagens, sem conteúdo).
func (s *MetadataService) GetVbfTextEntry(vbfPath, innerPath, id string) (dto.FileEntry, error) {
	a, t, err := s.vbfTargetOf(vbfPath, innerPath)
	if err != nil {
		return dto.FileEntry{}, err
	}
	if t.Kind == KindMacro {
		if strings.TrimSpace(id) == "" {
			return dto.FileEntry{}, fmt.Errorf("macro: chunk não informado em %s", cleanVbfPath(innerPath))
		}
		t.ID = id
	}
	if err := ensureVersionReady(t.Version); err != nil {
		return dto.FileEntry{}, err
	}

	return vbfRun(a, t, innerPath, func() (dto.FileEntry, error) {
		// Sessão: arquivo já carregado e mods intocado serve do store, sem
		// re-decodificar o container. O dedupe acontece a cada inserção
		// (upsert na sessão) — clicou na cópia depois da def e ela não
		// está traduzida, sai como ref "$hash" da própria sessão.
		if out, ok := vbfSessionCached(vbfPath, t); ok {
			return s.fillRefOriginals(out, t.Kind, t.ID, t.Version), nil
		}
		// Todos os kinds de texto decodificam o MESMO objeto que embute o
		// DTO: é ele que o apply vai mutar e que o save persiste em mods/.
		estado, current, exists, err := s.decodeVbfEstado(t, common.SourceVbfPreferred)
		if err != nil {
			return dto.FileEntry{}, err
		}
		if !exists {
			return dto.FileEntry{}, fmt.Errorf("%s/%s indisponível no .vbf e em mods/", t.Kind, t.ID)
		}
		orig, okOrig, oerr := s.loadOriginalFrom(t.Kind, t.ID, t.Version, common.SourceVbf)
		if oerr != nil || !okOrig {
			common.LogWarning(
				"original de %s/%s/%s ausente no .vbf — entregando sem a coluna Original",
				t.Version, t.Kind, t.ID,
			)
			// Registra mesmo assim: sem upsert a sessão não teria o estado
			// do arquivo e o save recusaria na hora de editar.
			return s.fillRefOriginals(vbfSessionUpsertEstado(vbfPath, t, current, estado), t.Kind, t.ID, t.Version), nil
		}
		merged, diverged := withOriginal(entryRef{kind: t.Kind, id: t.ID, version: t.Version}, current, orig)
		logDivergence(diverged)
		return s.fillRefOriginals(vbfSessionUpsertEstado(vbfPath, t, merged, estado), t.Kind, t.ID, t.Version), nil
	})
}

// decodeVbfEstado decodifica o arquivo de texto do .vbf na fonte dada e
// devolve (estado, entry): o MESMO objeto que embute o DTO — a sessão guarda
// ele, o apply muta ele e o save grava ele em mods/. Devolve exists=false
// quando a fonte não tem o arquivo (sem erro: o chamador degrada).
func (s *MetadataService) decodeVbfEstado(t vbfTarget, src common.FileSource) (*vbfEstado, dto.FileEntry, bool, error) {
	switch t.Kind {
	case KindEvents:
		strs, err := event.ReadLocalizedEventStringsFrom(t.ID, t.Version, src)
		if err != nil {
			return nil, dto.FileEntry{}, false, err
		}
		if len(strs) == 0 {
			return nil, dto.FileEntry{}, false, nil
		}
		entry, ok := builders.BuildEventEntryDTOFrom(t.ID, t.Version, strs)
		if !ok {
			return nil, dto.FileEntry{}, false, nil
		}
		return &vbfEstado{strings: strs}, entry, true, nil

	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		strs, err := eventtable.ReadLocalizedStringsFrom(t.Kind, t.ID, t.Version, src)
		if err != nil {
			return nil, dto.FileEntry{}, false, err
		}
		if len(strs) == 0 {
			return nil, dto.FileEntry{}, false, nil
		}
		entry, ok := builders.BuildTableEntryDTOFrom(t.Kind, t.ID, t.Version, strs)
		if !ok {
			return nil, dto.FileEntry{}, false, nil
		}
		return &vbfEstado{strings: strs}, entry, true, nil

	case KindHelp:
		// O painel vem com todas as localizações que a fonte tem; o mesmo
		// objeto vira o DTO (coluna Traduzido) e o alvo do save (mods/).
		panel := helpfile.ReadHelpPanelFrom(t.Version, t.ID, src)
		if panel == nil {
			return nil, dto.FileEntry{}, false, nil
		}
		entry, ok := builders.BuildHelpEntryDTOFrom(t.ID, t.Version, panel)
		if !ok {
			return nil, dto.FileEntry{}, false, nil
		}
		return &vbfEstado{painel: panel}, entry, true, nil

	case KindLockit:
		for _, l := range lockit.LayoutsForVersion(t.Version) {
			if l.ID() != t.ID {
				continue
			}
			f, err := lockit.LoadFrom(l, src)
			if err != nil {
				return nil, dto.FileEntry{}, false, err
			}
			c, err := builders.BuildLockitDTOFrom([]*lockit.LockitFile{f}, nil)
			if err != nil {
				return nil, dto.FileEntry{}, false, err
			}
			entry, ok := c[t.ID]
			if !ok {
				return nil, dto.FileEntry{}, false, nil
			}
			return &vbfEstado{lockitFile: f}, entry, true, nil
		}
		return nil, dto.FileEntry{}, false, nil

	case KindObjects:
		// Chunkmap: o schema registrado carrega da FONTE pedida (o overlay
		// do .vbf entra via SourceVbfPreferred) com os nomes de campo do
		// fluxo novo. Sem estado de sessão: o save é o applyObjectsCollection
		// do fluxo normal (mods-first, grava em mods/).
		layouts, err := s.resolveObjectLayouts(t.Version, []string{t.ID})
		if err != nil {
			return nil, dto.FileEntry{}, false, fmt.Errorf("objects %s: %w", t.ID, err)
		}
		if len(layouts) == 0 {
			return nil, dto.FileEntry{}, false, fmt.Errorf("objects %s: layout não encontrado", t.ID)
		}
		layout := layouts[0]
		key := objectsfile.FileLayoutKey(t.Version, layout.PatternPath())
		loadFrom, ok := chunkmap.SchemaSourceLoaders[key]
		if !ok {
			return nil, dto.FileEntry{}, false, fmt.Errorf("objects %s: schema chunkmap não registrado", key)
		}
		c, err := loadFrom(t.Version, layout.PatternPath(), src)
		if err != nil {
			return nil, dto.FileEntry{}, false, fmt.Errorf("objects %s: %w", t.ID, err)
		}
		entry, ok := c[t.ID]
		if !ok {
			return nil, dto.FileEntry{}, false, nil
		}
		return nil, entry, true, nil

	case KindMacro:
		// O dicionário é UM artefato por localização: o estado guardado é o
		// dicionário INTEIRO (base do rebuild — os outros chunks não podem
		// sumir quando só um foi aberto).
		full, err := builders.BuildMacroDTOFromSource(t.Version, src, nil)
		if err != nil {
			return nil, dto.FileEntry{}, false, err
		}
		entry, ok := full[t.ID]
		if !ok {
			return nil, dto.FileEntry{}, false, nil
		}
		return &vbfEstado{macro: full}, entry, true, nil
	}
	return nil, dto.FileEntry{}, false, fmt.Errorf("kind sem fluxo de sessão: %s", t.Kind)
}

// ApplyVbfTextCollection aplica (no ESCOPO da sessão do container) os edits
// vindos das tabelas abertas no navegador de .vbf: o MESMO applier de data/,
// com a limitação imposta ao .vbf — o arquivo precisa ter sido aberto no
// clique (a propagação alcança só as cópias carregadas). Binários gravados =
// os tocados (lote ou propagação), em mods/<rel>; arquivo que só existe no
// container cria o mods/ na gravação. Qualquer escrita invalida os caches
// de view (a sessão incluída: a próxima carga re-decodifica pelo mods novo).
func (s *MetadataService) ApplyVbfTextCollection(vbfPath, kind string, version common.GameVersion, c dto.Collection) error {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if _, err := vbfArchiveFor(vbfPath); err != nil {
		return fmt.Errorf(".vbf indisponível: %w", err)
	}
	if err := ensureVersionReady(version); err != nil {
		return err
	}
	if err := applyVbfCollectionInSession(vbfPath, kind, version, c); err != nil {
		return err
	}
	clearDedupViewCache() // mods mudou: cru data/, pristine e sessão de .vbf caem
	return nil
}

// applyVbfCollectionInSession roda o applier do kind com o escopo montado
// da sessão de cliques do .vbf. A fonte do estado é SEMPRE a sessão (o
// objeto decodificado no clique); a gravação de todos os appliers vai para
// mods/ — o container não tem API de escrita e o guard de escrita recusa
// qualquer alvo fora de mods/ dentro da árvore do jogo.
func applyVbfCollectionInSession(vbfPath, kind string, version common.GameVersion, c dto.Collection) error {
	base := filepath.Base(vbfPath)
	switch kind {
	case KindEvents, KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		scope, ok := vbfSessionApplyScope(vbfPath, kind)
		if !ok {
			return fmt.Errorf("nenhum arquivo %s aberto na sessão de %s", kind, base)
		}
		return builders.ApplyTextDTOWithScope(kind, c, scope)

	case KindHelp:
		scope, ok := vbfSessionHelpScope(vbfPath)
		if !ok {
			return fmt.Errorf("nenhum painel help aberto na sessão de %s", base)
		}
		return builders.ApplyHelpDTOWithScope(version, c, c.SortedKeys(), scope)

	case KindLockit:
		scope, ok := vbfSessionLockitScope(vbfPath)
		if !ok {
			return fmt.Errorf("nenhum arquivo lockit aberto na sessão de %s", base)
		}
		return builders.ApplyLockitDTOWithScope(version, c, scope)

	case KindObjects:
		// Chunkmap puro: o apply lê mods-first (o container entra como fonte
		// de leitura via o overlay de clique) e grava em mods/ — o mesmo
		// fluxo do editor normal. Sem estado de sessão para objects.
		return applyObjectsCollection(version, c)

	case KindMacro:
		full, ok := vbfSessionMacro(vbfPath)
		if !ok {
			return fmt.Errorf("nenhum chunk de macro aberto na sessão de %s", base)
		}
		return applyMacroCollection(version, full, c)

	default:
		return fmt.Errorf("unknown kind: %s", kind)
	}
}

// GetVbfImageEntry devolve a textura (.dds.phyre) pedida a partir do .vbf,
// com pré-visualização PNG em data URL. Sem índice de duplicatas: ele é
// montado varrendo data/, que pode não ter sido extraída por completo.
func (s *MetadataService) GetVbfImageEntry(vbfPath, innerPath string) (dto.ImageEntry, error) {
	a, t, err := s.vbfTargetOf(vbfPath, innerPath)
	if err != nil {
		return dto.ImageEntry{}, err
	}
	if t.Kind != KindImages {
		return dto.ImageEntry{}, fmt.Errorf("não é uma textura do app: %s", cleanVbfPath(innerPath))
	}

	entry, err := vbfRun(a, t, innerPath, func() (dto.ImageEntry, error) {
		return s.GetImageFrom(t.Kind, t.ID, t.Version, common.SourceVbf)
	})
	if err != nil {
		return dto.ImageEntry{}, err
	}
	if raw, readErr := a.Read(cleanVbfPath(innerPath)); readErr == nil {
		effectiveRaw := raw
		if mods, modsErr := common.NewFileAccessorFrom(ddsphyre.RelPath(t.Version, t.ID), common.SourceMods); modsErr == nil && mods.Exists {
			if bytes, bytesErr := mods.ReadBytes(); bytesErr == nil {
				effectiveRaw = bytes
			}
		}
		entry.Duplicates, entry.DupPayload = vbfImageDuplicatesFor(vbfPath, innerPath, t, raw, effectiveRaw)
	}
	return entry, nil
}

// ImportVbfImage lê o container original do .vbf, repacka o DDS escolhido e
// grava o resultado em mods/. O .vbf permanece somente leitura.
func (s *MetadataService) ImportVbfImage(vbfPath, innerPath, ddsPath string) error {
	a, target, err := s.vbfTargetOf(vbfPath, innerPath)
	if err != nil {
		return err
	}
	if target.Kind != KindImages {
		return fmt.Errorf("não é uma textura do app: %s", cleanVbfPath(innerPath))
	}
	if !ddsphyre.ValidID(target.ID) {
		return fmt.Errorf("textura desconhecida: %s", target.ID)
	}
	if err := ensureVersionReady(target.Version); err != nil {
		return err
	}
	original, err := a.Read(cleanVbfPath(innerPath))
	if err != nil {
		return fmt.Errorf("lendo textura do .vbf: %w", err)
	}
	dds, err := os.ReadFile(ddsPath)
	if err != nil {
		return fmt.Errorf("lendo %s: %w", filepath.Base(ddsPath), err)
	}
	if err := ddsphyre.ImportPayloadFrom(target.Version, target.ID, original, dds); err != nil {
		return err
	}
	ddsphyre.InvalidateIndex(target.Version)
	// ExtractEffective: refresh imune ao toggle EnableMods — os artefatos
	// seguem o container recém-gravado, mesmo com mods desligados.
	if _, err := ddsphyre.ExtractEffective(target.Version, target.ID); err != nil {
		common.LogWarning("images %s/%s: import do .vbf ok, mas não atualizei os .dds/.png extraídos: %v", target.Version, target.ID, err)
	}
	vbfSessionResetAll()
	return nil
}

// SaveVbfImage salva DDS ou PNG da imagem atualmente servida pelo .vbf, sem
// escrever no container nem depender de a árvore data/ estar extraída.
func (s *MetadataService) SaveVbfImage(vbfPath, innerPath, format, destPath string) error {
	a, target, err := s.vbfTargetOf(vbfPath, innerPath)
	if err != nil {
		return err
	}
	if target.Kind != KindImages {
		return fmt.Errorf("não é uma textura do app: %s", cleanVbfPath(innerPath))
	}
	if !ddsphyre.ValidID(target.ID) {
		return fmt.Errorf("textura desconhecida: %s", target.ID)
	}
	resolved, err := vbfRun(a, target, innerPath, func() (*ddsphyre.Resolved, error) {
		return ddsphyre.ResolveFrom(target.Version, target.ID, common.SourceVbf)
	})
	if err != nil {
		return err
	}
	var payload []byte
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "dds":
		payload = resolved.DDS
		if payload == nil && resolved.Texture != nil {
			payload, err = resolved.Texture.ExtractToDDS()
		}
	case "png":
		payload = resolved.PNG
	default:
		return fmt.Errorf("formato %q inválido (esperado dds ou png)", format)
	}
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return fmt.Errorf("imagem %s: conteúdo %s indisponível", target.ID, format)
	}
	if err := common.CheckWritablePath(destPath); err != nil {
		return err
	}
	if err := common.EnsurePathExists(destPath); err != nil {
		return err
	}
	return os.WriteFile(destPath, payload, 0o644)
}

// ReplicateVbfImage propaga o DDS visível da imagem aberta no .vbf para as
// cópias que também foram abertas e identificadas na sessão. O container é
// apenas lido; cada repack é gravado em mods/.
func (s *MetadataService) ReplicateVbfImage(vbfPath, sourcePath string, targetPaths []string) (dto.BatchResult, error) {
	a, source, err := s.vbfTargetOf(vbfPath, sourcePath)
	if err != nil {
		return dto.BatchResult{}, err
	}
	if source.Kind != KindImages || !ddsphyre.ValidID(source.ID) {
		return dto.BatchResult{}, fmt.Errorf("não é uma textura válida do .vbf: %s", cleanVbfPath(sourcePath))
	}

	archiveKey := filepath.Clean(vbfPath)
	type vbfReplicaTarget struct {
		target vbfTarget
		path   string
	}
	selection := make([]vbfReplicaTarget, 0, len(targetPaths))
	seen := make(map[string]struct{}, len(targetPaths))
	vbfImageDedup.Lock()
	opened := vbfImageDedup.byArchive[archiveKey]
	sourceOpened, sourceWasOpened := opened[cleanVbfPath(sourcePath)]
	if !sourceWasOpened {
		vbfImageDedup.Unlock()
		return dto.BatchResult{}, fmt.Errorf("abra a imagem do .vbf antes de replicá-la")
	}
	for _, path := range targetPaths {
		key := cleanVbfPath(path)
		if key == cleanVbfPath(sourcePath) {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		candidate, wasOpened := opened[key]
		if !wasOpened || candidate.originalHash != sourceOpened.originalHash {
			vbfImageDedup.Unlock()
			return dto.BatchResult{}, fmt.Errorf("replicação recusada: %s não é uma cópia já identificada de %s", key, source.ID)
		}
		target, ok := matchVbfPath(filepath.Base(vbfPath), key)
		if !ok || target.Kind != KindImages || target.Version != source.Version || !ddsphyre.ValidID(target.ID) {
			vbfImageDedup.Unlock()
			return dto.BatchResult{}, fmt.Errorf("alvo de replicação inválido no .vbf: %s", key)
		}
		seen[key] = struct{}{}
		selection = append(selection, vbfReplicaTarget{target: target, path: key})
	}
	vbfImageDedup.Unlock()
	if len(selection) == 0 {
		return dto.BatchResult{}, fmt.Errorf("textura %s não tem cópias abertas para replicar", source.ID)
	}

	resolved, err := vbfRun(a, source, sourcePath, func() (*ddsphyre.Resolved, error) {
		return ddsphyre.ResolveFrom(source.Version, source.ID, common.SourceVbf)
	})
	if err != nil {
		return dto.BatchResult{}, fmt.Errorf("lendo imagem %s do .vbf: %w", source.ID, err)
	}
	dds := resolved.DDS
	if dds == nil && resolved.Texture != nil {
		dds, err = resolved.Texture.ExtractToDDS()
	}
	if err != nil || dds == nil {
		if err == nil {
			err = fmt.Errorf("DDS indisponível")
		}
		return dto.BatchResult{}, fmt.Errorf("textura %s: %w", source.ID, err)
	}

	result := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(selection)}
	prog, step := s.progressStep(fmt.Sprintf("Replicando para %d cópia(s)", len(selection)), len(selection))
	defer prog.End()
	for _, replica := range selection {
		target := replica.target
		original, readErr := a.Read(replica.path)
		if readErr == nil {
			readErr = ddsphyre.ImportPayloadFrom(target.Version, target.ID, original, dds)
		}
		if readErr != nil {
			result.Failed = append(result.Failed, target.ID+": "+readErr.Error())
			step.Issue(target.ID, readErr.Error())
			continue
		}
		// ExtractEffective: refresh imune ao toggle EnableMods — os
		// artefatos seguem o container recém-gravado (ver ImportVbfImage).
		if _, extractErr := ddsphyre.ExtractEffective(target.Version, target.ID); extractErr != nil {
			common.LogWarning("vbf images %s/%s: replicação ok, mas falhou a atualização dos .dds/.png: %v", target.Version, target.ID, extractErr)
		}
		result.Done = append(result.Done, target.ID)
		step.Step(target.ID)
	}
	if len(result.Done) > 0 {
		ddsphyre.InvalidateIndex(source.Version)
		vbfSessionResetAll()
	}
	// O aviso é do frontend (reportBatch → "N texturas replicadas.").
	return result, nil
}

type openedVbfImage struct {
	id            string
	path          string
	originalHash  string
	effectiveHash string
}

var vbfImageDedup = struct {
	sync.Mutex
	byArchive map[string]map[string]openedVbfImage
}{byArchive: make(map[string]map[string]openedVbfImage)}

// Dedupe do .vbf é incremental: só compara imagens que já foram abertas por
// clique nesta sessão/container. Não força uma varredura completa do arquivo.
func vbfImageDuplicatesFor(vbfPath, innerPath string, target vbfTarget, raw, effectiveRaw []byte) ([]dto.ImageDuplicate, int64) {
	hash, size, err := ddsphyre.PayloadHash(raw)
	if err != nil {
		common.LogWarning("vbf images %s: payload inválido para dedupe: %v", target.ID, err)
		return []dto.ImageDuplicate{}, 0
	}
	effectiveHash, _, err := ddsphyre.PayloadHash(effectiveRaw)
	if err != nil {
		common.LogWarning("vbf images %s: payload efetivo inválido para dedupe: %v", target.ID, err)
		return []dto.ImageDuplicate{}, size
	}
	archiveKey := filepath.Clean(vbfPath)
	pathKey := cleanVbfPath(innerPath)
	vbfImageDedup.Lock()
	defer vbfImageDedup.Unlock()
	opened := vbfImageDedup.byArchive[archiveKey]
	if opened == nil {
		opened = make(map[string]openedVbfImage)
		vbfImageDedup.byArchive[archiveKey] = opened
	}
	opened[pathKey] = openedVbfImage{
		id:            target.ID,
		path:          pathKey,
		originalHash:  hash,
		effectiveHash: effectiveHash,
	}

	duplicates := make([]dto.ImageDuplicate, 0)
	for path, candidate := range opened {
		if path == pathKey || candidate.originalHash != hash {
			continue
		}
		_, modded := ddsphyre.Exists(target.Version, candidate.id)
		duplicates = append(duplicates, dto.ImageDuplicate{
			ID:        candidate.id,
			Key:       dto.NewImageMetadata(candidate.id, target.Version).Key,
			VbfPath:   candidate.path,
			Modded:    modded,
			Identical: candidate.effectiveHash == effectiveHash,
		})
	}
	sort.Slice(duplicates, func(i, j int) bool { return duplicates[i].ID < duplicates[j].ID })
	return duplicates, size
}

func clearVbfImageDedup() {
	vbfImageDedup.Lock()
	vbfImageDedup.byArchive = make(map[string]map[string]openedVbfImage)
	vbfImageDedup.Unlock()
}

// ---------------------------------------------------------------------------
// internos
// ---------------------------------------------------------------------------

// vbfTargetOf resolve o caminho interno para (kind, id, versão) e devolve o
// container aberto. ok=false = formato que o app não serve.
func (s *MetadataService) vbfTargetOf(vbfPath, innerPath string) (*vbf.Archive, vbfTarget, error) {
	a, err := vbfArchiveFor(vbfPath)
	if err != nil {
		return nil, vbfTarget{}, err
	}
	t, ok := matchVbfPath(filepath.Base(vbfPath), innerPath)
	if !ok {
		return nil, vbfTarget{}, fmt.Errorf("arquivo fora do escopo do app: %s", cleanVbfPath(innerPath))
	}
	return a, t, nil
}

// vbfRun monta o overlay do alvo (todas as localizações da entrada — eventos,
// help, macro e objects têm UM ARQUIVO POR IDIOMA) e roda fn dentro do
// escopo de leitura do .vbf. O overlay cobre o conjunto candidato (decodifica
// uma vez); o decodificador de reserva atende o que faltar, sem custo.
func vbfRun[T any](a *vbf.Archive, t vbfTarget, clicked string, fn func() (T, error)) (T, error) {
	var zero T
	overlay := make(map[string][]byte)
	for _, p := range vbfOverlayPaths(t, clicked) {
		if data, err := a.Read(p); err == nil {
			overlay[p] = data
		}
	}
	decode := func(p string) ([]byte, bool) {
		data, err := a.Read(p)
		if err != nil {
			return nil, false
		}
		return data, true
	}

	var out T
	err := common.WithVbfSourceReader(overlay, decode, func() error {
		v, err := fn()
		if err != nil {
			return err
		}
		out = v
		return nil
	})
	if err != nil {
		return zero, err
	}
	return out, nil
}
