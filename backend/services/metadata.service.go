package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/core/reader"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/fileFormats/eventtable"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/fileFormats/macrodic"
	"ffxresources/backend/fileFormats/objectsfile"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/formatters"
	jsonfmt "ffxresources/backend/formatters/json"
	_ "ffxresources/backend/formatters/strings"
	"ffxresources/backend/loggingService"
	"ffxresources/backend/progress"
	"ffxresources/backend/uilog"
)

// Kinds lógicos servidos ao frontend: textos e imagens (.dds.phyre).
const (
	KindEvents     = "events"
	KindObjects    = "objects"
	KindMacro      = "macro"
	KindLockit     = "lockit"
	KindHelp       = "help"
	KindBattleText = "battletext"
	KindCloud      = "cloud"
	KindTutorial   = "tutorial"
	KindMenuMain   = "menumain"
	// KindImages é a árvore de texturas .dds.phyre — um kind de IMAGEM:
	// não tem rows de texto, não participa de export/import de texto e não
	// entra no preload de coleções.
	KindImages = "images"
)

// LastMissionShortened é o prefixo (eventID[:2]) do grupo de events da
// Last Mission, que mora na árvore do ffx2: "lm".
const LastMissionShortened = "lm"

// EternalCalmShortened é o prefixo (eventID[:2]) do grupo de events do
// Eternal Calm, que mora na árvore do ffx (diretório sc/scene*): "sc".
const EternalCalmShortened = "sc"

// EntrySummary é o índice leve para sidebar/tree: id + key, sem rows.
// row_count aparece nas entradas (GetEntry/GetCollection), nunca aqui.
type EntrySummary struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// MetadataService é o acesso do frontend (via Wails) a metadata e texto.
// Devolve DTO estruturado (dto.Collection/FileEntry) direto da memória —
// sem exportar para disco e reler o arquivo. O formato JSON de export
// continua intocado; este service é o "formato frontend".
type MetadataService struct {
	notifier INotificationService
	// progress é a ponte wails dos batches (export/extração). nil = sem
	// progresso: testes, CLI e appcore seguem silenciosos.
	progress *progress.Emitter
}

// NewMetadataService cria o serviço. O emitter é OPCIONAL (variadic): o
// app injeta o emitter real (batches publicam eventos do wails) e o
// resto do mundo (testes/appcore) constrói sem progresso.
func NewMetadataService(notifier INotificationService, em ...*progress.Emitter) *MetadataService {
	s := &MetadataService{notifier: notifier}
	if len(em) > 0 {
		s.progress = em[0]
	}
	return s
}

// progressStep cria o batch de UMA ação (padrão das extrações/replicações
// de imagem e .vbf): o título do modal é o label da ação. Sem emitter
// devolve (nil, nil) — todos os métodos são nil-safe.
func (s *MetadataService) progressStep(label string, total int) (*progress.Batch, *progress.Step) {
	batch := s.progress.NewBatch(label)
	step := batch.Step(label)
	step.Begin(label, total)
	return batch, step
}

// dedupViewKinds são os kinds cujo view (GetEntry) sai com dedup global
// da versão — os formatos que agrupam texto numa única extração, onde as
// repetições cruzam arquivos: help (arquivo único com os 6 painéis),
// events (eventos gêmeos com repetição maciça) e macro (dicionário único
// em um artefato). objects é extração 1:1 por arquivo — dedup intra-arquivo
// no GetEntry, refs não propagam entre objetos.
var dedupViewKinds = map[string]bool{
	KindHelp:       true,
	KindEvents:     true,
	KindMacro:      true,
	KindBattleText: true,
	KindCloud:      true,
	KindTutorial:   true,
	KindMenuMain:   true,
}

type rawView struct {
	collection dto.Collection
	order      *builders.HashOrder
}

// Cache do cru por (versão|kind): o dedup global exige construir a versão
// inteira (events: 45MB no ffx2) — construído uma única vez por carga e
// invalidado por qualquer escrita (apply/import altera o store).
//
// Guarda o estado CRU (sem refs) + a ordem global dos ponteiros (HashOrder):
// o dedup de display acontece POR ENTRADA em GetEntry — depois do merge com
// o original, que é quem decide "traduzido" (Text vs Original) e reescreve
// o ponteiro.
var (
	dedupViewMu    sync.Mutex
	dedupViewCache = map[string]rawView{}
	// Serializa a CONSTRUÇÃO do view cru. É um mutex à parte do
	// dedupViewMu de propósito: a build passa por GetCollection, que pode
	// recarregar o store (EnsureAllEventsFresh: binário mudou no disco, ou
	// o toggle EnableMods trocou a árvore resolvida pelos carimbos) e
	// chamar clearDedupViewCache NO MEIO do caminho — mutex não é
	// reentrante, e segurar o dedupViewMu através da build travava a
	// goroutine nela mesma (self-deadlock permanente: toda carga de events
	// a partir dali bloqueava no lock mantido).
	dedupViewBuildMu sync.Mutex
	// Geração do cache: sobe a cada clear. Uma build que atravessou um
	// clear leu um store anterior a ele — não grava no cache (o próximo
	// leitor reconstrói do store atual).
	dedupViewGeneration atomic.Uint64
)

func (s *MetadataService) rawViewOf(kind string, version common.GameVersion) (rawView, error) {
	key := version.String() + "|" + kind
	if v, ok := dedupViewCached(key); ok {
		return v, nil
	}
	dedupViewBuildMu.Lock()
	defer dedupViewBuildMu.Unlock()
	// Outra goroutine pode ter completado a build enquanto esperávamos a
	// trava — o view dela serve (mesma árvore, mesmo store).
	if v, ok := dedupViewCached(key); ok {
		return v, nil
	}
	generation := dedupViewGeneration.Load()
	// Build FORA do dedupViewMu (ver o bloco var acima): o clear interno
	// de GetCollection não esbarra no lock da própria build.
	full, err := s.GetCollection(kind, version, nil, progress.Nop())
	if err != nil {
		return rawView{}, err
	}
	normalized := s.normalizeCollection(kind, version, full)
	v := rawView{collection: normalized, order: builders.NewHashOrder(normalized)}
	// Ninguém limpou o cache durante a build: grava. Alguém limpou (apply,
	// import ou o próprio reload de frescura): o view construído é anterior
	// ao clear — entrega ao chamador atual e deixa o cache vazio para o
	// próximo leitor reconstruir do store que ficou.
	if dedupViewGeneration.Load() == generation {
		dedupViewMu.Lock()
		dedupViewCache[key] = v
		dedupViewMu.Unlock()
	}
	return v, nil
}

func dedupViewCached(key string) (rawView, bool) {
	dedupViewMu.Lock()
	defer dedupViewMu.Unlock()
	v, ok := dedupViewCache[key]
	return v, ok
}

// normalizeCollection devolve a Collection com os ponteiros normalizados
// para o domínio display: entradas com binário em mods/ são merged com o
// original (ponteiro = hash do original); as sem mods são não traduzidas
// e o ponteiro de mods JÁ É o do original (mesmo conteúdo ⇒ mesmo XXH64).
//
// O trabalho é PARALELO a nível de arquivo: um worker por entrada executa
// stat + leitura do original + merge + comparação de rows (todos os loops
// internos sincronos), coletando o relatório de divergência; a emissão dos
// avisos acontece DEPOIS do pool, em ordem canônica de chave.
func (s *MetadataService) normalizeCollection(kind string, version common.GameVersion, c dto.Collection) dto.Collection {
	keys := c.SortedKeys()
	type normalized struct {
		key   string
		entry dto.FileEntry
		diag  divergeDiag
	}
	results := make([]normalized, len(keys))
	parallelFor(len(keys), func(i int) {
		k := keys[i]
		ref := entryRef{kind: kind, id: k, version: version}
		entry := c[k]
		if !hasModsFile(kind, k, version) {
			results[i] = normalized{key: k, entry: entry}
			return
		}
		if inData, _ := originalTrees(kind, k, version); !inData {
			results[i] = normalized{key: k, entry: entry}
			return
		}
		orig, exists, oerr := s.originalFor(kind, k, version)
		if oerr != nil || !exists {
			results[i] = normalized{key: k, entry: entry}
			return
		}
		merged, diag := withOriginal(ref, entry, orig)
		results[i] = normalized{key: k, entry: merged, diag: diag}
	})

	out := make(dto.Collection, len(c))
	for i := range results {
		out[results[i].key] = results[i].entry
	}
	// Emissão em ordem canônica: os workers apenas coletam; o arquivo de
	// diagnóstico fica determinístico.
	for i := range results {
		logDivergence(results[i].diag)
	}
	return out
}

// clearDedupViewCache invalida o cru em cache: o store mudou (apply ou
// import), e os ponteiros/ordem precisam ser reconstruídos na próxima
// entrega. O original pristine (data/) acompanha: os dois caches descrevem
// a MESMA árvore de gamefiles, então caem juntos.
//
// A sessão de .vbf cai também: uma escrita em mods/ muda o texto que a
// sessão serviu (cópia traduzida vira literal na próxima carga) — o
// carimbo por arquivo pegaria o arquivo próprio, mas o def de outro
// arquivo carregado seguiria velho. Perder o decode cache é o preço da
// frescura; re-decodificar é o comportamento original do fluxo por clique.
func clearDedupViewCache() {
	clearTextSearchCache()
	// A geração sobe ANTES de limpar: uma build em curso (fora do lock, ver
	// rawViewOf) compara a geração no fim e se abstém de gravar o view que
	// leu antes do clear.
	dedupViewGeneration.Add(1)
	dedupViewMu.Lock()
	dedupViewCache = map[string]rawView{}
	dedupViewMu.Unlock()
	clearOriginalCache()
	vbfSessionResetAll()
}

func (s *MetadataService) notifyWarn(message string) {
	if s.notifier != nil {
		s.notifier.NotifyWarn(message)
	}
}

// reportEventSkips leva os eventos pulados na carga implícita ao
// diagnóstico + toast (cap anti-flood): a carga em si continua silenciosa
// — quem reporta é o chamador, não o loader.
const maxSkipToasts = 15

func (s *MetadataService) reportEventSkips(skips []event.EventSkip) {
	for i, skip := range skips {
		loggingService.DiagWarn("carga/"+skip.ID, skip.Reason, map[string]any{"item": skip.ID})
		if i < maxSkipToasts {
			s.notifyWarn(skip.Reason)
		}
	}
}

// Cache de inicialização por versão: charsets + macros carregados uma única
// vez (lazy), sem depender de chamada manual de InitializeInternals.
var (
	readyMu       sync.Mutex
	readyVersions = map[common.GameVersion]error{}
)

// ensureVersionReady garante charsets e macros da versão antes de qualquer
// leitura de texto. lastmiss reaproveita o bucket de ffx2 (CharsetVersion).
// Sob o viewLoadMu (leitura): a carga em voo não pode atravessar um
// InvalidateViewCaches e ressuscitar o estado da árvore anterior.
func ensureVersionReady(version common.GameVersion) error {
	viewLoadMu.RLock()
	defer viewLoadMu.RUnlock()
	v := common.CharsetVersion(version)
	readyMu.Lock()
	defer readyMu.Unlock()
	if err, done := readyVersions[v]; done {
		return err
	}
	err := reader.PrepareVersion(v)
	readyVersions[v] = err
	return err
}

// ---- metadata por key/id/path ---------------------------------------------

// GetMetadata resolve qualquer uma das 3 formas e devolve a metadata nova
// (key, id, is_dir; row_count omitido — só existe no export).
// Formas: metadata.key (ffx/...), id de collection (azit0000, command,
// chunk_00) ou caminho em disco (absoluto ou com ffx_ps2/).
func (s *MetadataService) GetMetadata(query string) (dto.Metadata, error) {
	key, id, isDir, err := s.Resolve(query)
	if err != nil {
		return dto.Metadata{}, err
	}
	return dto.Metadata{Key: key, ID: id, IsDir: isDir}, nil
}

// Resolve devolve (key, id, isDir) para as 3 formas de consulta.
func (s *MetadataService) Resolve(query string) (key, id string, isDir bool, err error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", "", false, fmt.Errorf("empty query")
	}
	slash := filepath.ToSlash(filepath.Clean(q))
	// Caminho em disco: absoluto, com ffx_ps2/, com backslash (Windows)
	// ou relativo que existe no filesystem.
	if strings.Contains(slash, "ffx_ps2/") || filepath.IsAbs(q) ||
		strings.Contains(q, "\\") || pathExists(q) {
		return s.resolvePath(q)
	}
	if strings.Contains(slash, "/") {
		return s.resolveKey(slash)
	}
	return s.resolveID(slash)
}

// pathExists informa se o caminho existe (arquivo ou diretório).
func pathExists(q string) bool {
	clean := filepath.Clean(q)
	if _, err := os.Stat(clean); err == nil {
		return true
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return false
	}
	_, err = os.Stat(abs)
	return err == nil
}

// resolveKey trata metadata.key canônica (version/pattern).
func (s *MetadataService) resolveKey(slash string) (string, string, bool, error) {
	p, ok := dto.ParseKey(slash)
	if !ok {
		return "", "", false, fmt.Errorf("invalid key: %s", slash)
	}
	id := p.Stem
	if strings.EqualFold(p.FileName, "macrodic.dcp") {
		// Key compartilhada por todos os chunks: sem id.
		id = ""
	}
	return slash, id, false, nil
}

// resolveID trata id de collection usando a versão ativa como default.
func (s *MetadataService) resolveID(id string) (string, string, bool, error) {
	cur := common.CurrentGameVersion()
	if _, ok := dto.ChunkIndexFromID(id); ok {
		return common.VersionPathName(cur) + "/menu/macrodic.dcp", id, false, nil
	}
	if helpfile.IsHelpEntry(id) {
		return dto.NewHelpMetadata(id, "help/"+helpfile.HelpEntryDir(id), cur).Key, id, false, nil
	}
	if key, ok := objectKeyForID(cur, id); ok {
		return key, id, false, nil
	}
	if l, ok := lockit.LayoutForID(cur, id); ok {
		return l.Key(), id, false, nil
	}
	if len(id) < 2 {
		return "", "", false, fmt.Errorf("unknown id: %s", id)
	}
	return dto.NewEventMetadata(id, cur).Key, id, false, nil
}

// resolvePath trata caminho em disco: deriva key + id + is_dir.
func (s *MetadataService) resolvePath(q string) (string, string, bool, error) {
	clean := filepath.Clean(q)
	abs, err := filepath.Abs(clean)
	if err != nil {
		abs = clean
	}
	isDir := false
	if st, serr := os.Stat(abs); serr == nil {
		isDir = st.IsDir()
	}
	if isDir {
		// Diretório: sem key de arquivo; id = basename.
		return "", filepath.Base(clean), true, nil
	}
	version := common.CurrentGameVersion()
	if v, verr := common.CheckFFXPath(abs); verr == nil {
		version = v
	}
	slash := filepath.ToSlash(abs)
	if lockit.IsLockitKey(slash) {
		base := filepath.Base(slash)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		for _, l := range lockit.LayoutsForVersion(version) {
			if strings.HasPrefix(stem, l.ID()) {
				return l.Key(), l.ID(), false, nil
			}
		}
		return "", "", false, fmt.Errorf("lockit path desconhecido: %s", q)
	}
	locPattern, ok := localizationPatternFromPath(slash)
	if !ok {
		return "", "", false, fmt.Errorf("path is not under a localization root: %s", q)
	}
	if strings.HasPrefix(locPattern, helpfile.HelpDirPrefix+"/") && strings.HasSuffix(locPattern, helpfile.HelpFileExt) {
		stem := strings.TrimSuffix(filepath.Base(locPattern), filepath.Ext(locPattern))
		return common.VersionPathName(version) + "/" + locPattern, stem, false, nil
	}
	key := common.VersionPathName(version) + "/" + locPattern
	id := strings.TrimSuffix(filepath.Base(locPattern), filepath.Ext(locPattern))
	if strings.EqualFold(filepath.Base(locPattern), "macrodic.dcp") {
		id = ""
	}
	return key, id, false, nil
}

// localizationPatternFromPath extrai o trecho após ffx_ps2/<v>/master/new_*pc/.
func localizationPatternFromPath(slashAbs string) (string, bool) {
	idx := strings.Index(slashAbs, "/master/")
	if idx < 0 {
		return "", false
	}
	rest := strings.Trim(slashAbs[idx+len("/master/"):], "/")
	segs := strings.Split(rest, "/")
	if len(segs) < 2 || !strings.HasPrefix(segs[0], "new_") {
		return "", false
	}
	return strings.Join(segs[1:], "/"), true
}

// exportProgress deprecou: a barra agora nasce na EXTRAÇÃO (builders
// fazem SetTotal + tick por entrada), dirigida pelo step do batch —
// ver ExportArtifactBatch/exportKind em export_formats.go.

// objectKeyForID procura nos layouts a key do basename (determinístico).
func objectKeyForID(version common.GameVersion, id string) (string, bool) {
	want := id + ".bin"
	var matches []string
	for key, layout := range objectsfile.FileLayouts {
		if layout.Version == version && strings.EqualFold(layout.FileName, want) {
			matches = append(matches, key)
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	sort.Strings(matches)
	return matches[0], true
}

// ExportEntry monta o DTO da entrada (kind/id/version) e escreve os artefatos
// JSON e .strings em mods/edits (caminhos padrão dos formatters). Devolve os
// caminhos escritos. langs nil/vazio = todos os idiomas. Progresso: batch de
// UMA ação — o step acompanha a extração (help carrega os 6 painéis).
func (s *MetadataService) ExportEntry(kind string, version common.GameVersion, id string, langs []string) (paths []string, retErr error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	// help: o artefato é único e SEMPRE cobre os 6 painéis (as defs do dedup
	// só resolvem com todos os arquivos no mesmo arquivo) — a seleção da
	// árvore é validada e ignorada.
	ids := []string{id}
	if kind == KindHelp {
		if !helpfile.IsHelpEntry(id) {
			return nil, fmt.Errorf("painel de ajuda desconhecido: %s", id)
		}
		ids = helpfile.HelpEntryNames()
	}

	label := id
	if kind == KindHelp || kind == KindMacro || kind == KindLockit {
		if l, ok := kindStepLabels[kind]; ok {
			label = l
		}
	}
	batch, step := s.progressStep(fmt.Sprintf("Exportando %s…", label), 0)
	defer func() {
		if retErr != nil {
			step.Fail(retErr.Error())
		} else {
			batch.SetSummary(len(paths), exportSummaryDestination(paths))
		}
		batch.End()
	}()

	c, err := s.exportCollection(kind, version, ids, step)
	if err != nil {
		return nil, err
	}
	// Menu de contexto exportou com hifen: o filtro de rows em branco
	// ('us' vazio/espaço/"-") vale para TODO artefato — mesmo contrato de
	// ExportArtifactBatch. Depois do filtro o step fecha cheio.
	c = filterExportRows(c)
	step.End()

	// O export de entrada grava o PAR json+strings (contrato do menu de
	// contexto) — camada de arquivo padronizada: formatters do registry e
	// naming canônico por kind/escopo (a mesma régua do ExportArtifactBatch).
	jsonF, ok := formatters.Get("json")
	if !ok {
		return paths, fmt.Errorf("formatter json não registrado")
	}
	stringsF, ok := formatters.Get("strings")
	if !ok {
		return paths, fmt.Errorf("formatter strings não registrado")
	}
	writePair := func(col dto.Collection, jsonPath string) error {
		jp, werr := formatters.NewWriter(jsonF, col, jsonPath, langs).Write()
		if werr != nil {
			return werr
		}
		sp, werr := formatters.NewWriter(stringsF, col, formatters.SwapExt(stringsF, jsonPath), langs).Write()
		if werr != nil {
			return werr
		}
		paths = append(paths, jp, sp)
		return nil
	}

	switch kind {
	case KindEvents:
		path, perr := eventsExportPath(version, []string{id}, "")
		if perr != nil {
			return nil, perr
		}
		if err := writePair(c, path); err != nil {
			return paths, err
		}
	case KindMacro:
		// Chunk selecionado não pode sobrescrever o artefato canônico
		// (macro_dictionary): o naming segue o escopo do pedido.
		path, perr := macroExportPath(version, []string{id}, "")
		if perr != nil {
			return nil, perr
		}
		if err := writePair(c, path); err != nil {
			return paths, err
		}
	case KindHelp:
		path, perr := jsonfmt.HelpJSONPath(version, "")
		if perr != nil {
			return nil, perr
		}
		if err := writePair(c, path); err != nil {
			return paths, err
		}
	case KindObjects, KindLockit, KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		// Um arquivo por entrada (mesmo nome do JSON por entrada; dedup
		// intra-arquivo = self-contained).
		for _, key := range c.SortedKeys() {
			jp, perr := jsonfmt.ObjectsJSONPath(key, version)
			if perr != nil {
				return nil, perr
			}
			if err := writePair(dto.Collection{key: c[key]}, jp); err != nil {
				return paths, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
	return paths, nil
}

// ImportEntry lê o artefato JSON padrão da entrada (mods/edits) e aplica o DTO
// de volta no binário, persistendo. Devolve o caminho lido. Escrita muda o
// store: o view dedupado (cache por versão) é invalidado.
//
// O log da janela é POR BINÁRIO, como no ImportFile: o gravador de gravações
// informa o que foi recompilado e em que destino de mods/ (import_log.go).
func (s *MetadataService) ImportEntry(kind string, version common.GameVersion, id string) ([]string, error) {
	clearDedupViewCache()
	kind = strings.ToLower(strings.TrimSpace(kind))
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}
	uilog.Info(importAction, "%s — aplicando artefato de %s (%s)", id, kind, version.String())

	// O artefato de CAT traz o texto alvo no campo target; os appliers
	// (builders.Apply*) consomem Text[us]. A substituição é feita no DTO
	// lido — a store e o arquivo em disco não são tocados.
	applyTarget := func(c dto.Collection) dto.Collection {
		return builders.ApplyTargetAsText(c)
	}

	switch kind {
	case KindEvents:
		if err := s.ensureEventsLoaded(version); err != nil {
			return nil, err
		}
		c, err := s.GetCollection(kind, version, []string{id}, progress.Nop())
		if err != nil {
			return nil, err
		}
		path, err := jsonfmt.EventsJSONPath(c, version)
		if err != nil {
			return nil, err
		}
		read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, path).Read()
		if err != nil {
			return nil, err
		}
		if _, err := applyRecorded(version, "Import", "importado de um json", func() error {
			return builders.ApplyEventsDTO(version, applyTarget(read), []string{id})
		}); err != nil {
			return nil, err
		}
		return []string{path}, nil

	case KindObjects:
		path, err := jsonfmt.ObjectsJSONPath(id, version)
		if err != nil {
			return nil, err
		}
		read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, path).Read()
		if err != nil {
			return nil, err
		}
		read = applyTarget(read)
		for _, key := range read.SortedKeys() {
			entry := read[key]
			if _, err := applyRecorded(version, "Import", "importado de um json", func() error {
				return s.applyObjectsEntry(version, key, entry)
			}); err != nil {
				return nil, err
			}
		}
		return []string{path}, nil

	case KindMacro:
		path, err := jsonfmt.DefaultMacroJSONPath(version)
		if err != nil {
			return nil, err
		}
		read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, path).Read()
		if err != nil {
			return nil, err
		}
		if _, err := applyRecorded(version, "Import", "importado de um json", func() error {
			return builders.ApplyMacroDTO(version, applyTarget(read))
		}); err != nil {
			return nil, err
		}
		return []string{path}, nil

	case KindLockit:
		path, err := jsonfmt.ObjectsJSONPath(id, version)
		if err != nil {
			return nil, err
		}
		read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, path).Read()
		if err != nil {
			return nil, err
		}
		if _, err := applyRecorded(version, "Import", "importado de um json", func() error {
			return builders.ApplyLockitDTO(version, applyTarget(read))
		}); err != nil {
			return nil, err
		}
		return []string{path}, nil

	case KindHelp:
		if err := ensureHelpLoaded(version); err != nil {
			return nil, err
		}
		// Artefato único de help: lê mods/edits e aplica as entradas nele
		// presentes — o arquivo é a unidade de import (sem agrupamento).
		if !helpfile.IsHelpEntry(id) {
			return nil, fmt.Errorf("painel de ajuda desconhecido: %s", id)
		}
		path, err := jsonfmt.HelpJSONPath(version, "")
		if err != nil {
			return nil, err
		}
		read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, path).Read()
		if err != nil {
			return nil, err
		}
		if _, err := applyRecorded(version, "Import", "importado de um json", func() error {
			return builders.ApplyHelpDTO(version, applyTarget(read), read.SortedKeys())
		}); err != nil {
			return nil, err
		}
		return []string{path}, nil

	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		path, err := jsonfmt.ObjectsJSONPath(id, version)
		if err != nil {
			return nil, err
		}
		read, err := formatters.NewReader(jsonfmt.JSONFormatter{}, path).Read()
		if err != nil {
			return nil, err
		}
		if _, err := applyRecorded(version, "Import", "importado de um json", func() error {
			return builders.ApplyTableDTO(kind, version, applyTarget(read))
		}); err != nil {
			return nil, err
		}
		return []string{path}, nil

	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}

// ApplyEntry aplica uma entrada editada (DTO) de volta no binário e persiste.
// É o "salvar" do editor in-memory (Ver/Editar). Escrita muda o store: o
// view dedupado (cache por versão) é invalidado.
func (s *MetadataService) ApplyEntry(kind string, version common.GameVersion, id string, entry dto.FileEntry) error {
	clearDedupViewCache()
	kind = strings.ToLower(strings.TrimSpace(kind))
	if err := ensureVersionReady(version); err != nil {
		return err
	}

	switch kind {
	case KindEvents:
		if err := s.ensureEventsLoaded(version); err != nil {
			return err
		}
		return builders.ApplyEventsDTO(version, dto.Collection{id: entry}, []string{id})
	case KindObjects:
		return s.applyObjectsEntry(version, id, entry)
	case KindMacro:
		// Aplica sobre o DTO completo para não perder os outros chunks.
		c, err := builders.BuildMacroDTO(version, nil)
		if err != nil {
			return err
		}
		c[id] = entry
		return builders.ApplyMacroDTO(version, c)
	case KindLockit:
		return builders.ApplyLockitDTO(version, dto.Collection{id: entry})
	case KindHelp:
		if err := ensureHelpLoaded(version); err != nil {
			return err
		}
		return builders.ApplyHelpDTO(version, dto.Collection{id: entry}, []string{id})
	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		if !eventtableKindUsable(kind, version) {
			return nil
		}
		return builders.ApplyTableDTO(kind, version, dto.Collection{id: entry})
	default:
		return fmt.Errorf("unknown kind: %s", kind)
	}
}

// macroChunkHasText informa se alguma row do chunk tem texto em algum idioma.
func macroChunkHasText(entry dto.FileEntry) bool {
	for _, row := range entry.Rows {
		for _, text := range row.Text {
			if strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

// ApplyTextCollection aplica um lote de entradas editadas (DTO) e persiste.
// É o "salvar" do editor do frontend: recebe só as entradas com edição, mas
// agrupa por arquivo para reconstruir cada binário uma única vez.
// Escrever muda o store: o view dedupado (cache por versão) é invalidado.
func (s *MetadataService) ApplyTextCollection(kind string, version common.GameVersion, c dto.Collection) error {
	clearDedupViewCache()
	kind = strings.ToLower(strings.TrimSpace(kind))
	if len(c) == 0 {
		return nil
	}
	if err := ensureVersionReady(version); err != nil {
		return err
	}

	// Log start (padrão export: "Kind: salvando")
	uilog.Info("Save", "%s: salvando", kind)

	// Poda das linhas que só existem em mods (sem par no original). O
	// dicionário (macro) faz a poda no arquivo inteiro dentro do branch; os
	// demais kinds podam só o lote que o save reconstrói. Lockit fica de
	// fora: não é row-table.
	if kind != KindMacro && kind != KindLockit {
		pruned, perr := s.prunePayload(kind, version, c)
		if perr != nil {
			return perr
		}
		c = pruned
	}

	// Wrap com applyRecorded para capturar gravações e narrar por binário
	// detailFormat = "salvo" (save não tem formato de artefato como import/export)
	_, err := applyRecorded(version, "Save", "salvo", func() error {
		switch kind {
		case KindEvents:
			if err := s.ensureEventsLoaded(version); err != nil {
				return err
			}
			return builders.ApplyEventsDTO(version, c, c.SortedKeys())

		case KindObjects:
			// Aplica as rows de todas as entradas antes de salvar cada binário:
			// entradas repetidas (mesmo arquivo) recebem todas as suas rows.
			return applyObjectsCollection(version, c)

		case KindMacro:
			// Aplica sobre o DTO completo para não perder os outros chunks.
			full, err := builders.BuildMacroDTO(version, nil)
			if err != nil {
				return err
			}
			original, err := builders.BuildMacroDTOFromSource(version, originalSource, nil)
			if err != nil {
				return err
			}
			return applyMacroCollection(version, full, c, original)

		case KindLockit:
			return builders.ApplyLockitDTO(version, c)

		case KindHelp:
			if err := ensureHelpLoaded(version); err != nil {
				return err
			}
			return builders.ApplyHelpDTO(version, c, c.SortedKeys())

		case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
			if !eventtableKindUsable(kind, version) {
				return nil
			}
			return builders.ApplyTableDTO(kind, version, c)

		default:
			return fmt.Errorf("unknown kind: %s", kind)
		}
	})
	return err
}

// applyObjectsCollection aplica um lote de entradas de objects e persiste
// cada binário em mods/. Agrupa por arquivo para reconstruir cada binário
// uma única vez. Fluxo único: chunkmap (SchemaAppliers).
//
// O payload do Save é o DTO de DISPLAY: cópias não traduzidas chegam como
// ref "$hash". O applier grava o texto da row direto no binário, então a ref
// tem de ser expandida (e a órfã descartada) ANTES — senão o binário recebe
// o hash como texto e a view passa a exibir o hash cru no lugar da tradução.
func applyObjectsCollection(version common.GameVersion, c dto.Collection) error {
	c = builders.ResolveApplyRefs(c)
	if len(c) == 0 {
		return nil
	}
	byKey := make(map[string][]dto.FileEntry, len(c))
	order := make([]string, 0, len(c))
	for _, id := range c.SortedKeys() {
		key, ok := objectKeyForID(version, id)
		if !ok {
			return fmt.Errorf("unknown object id: %s", id)
		}
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], c[id])
	}
	for _, key := range order {
		applier, ok := chunkmap.SchemaAppliers[key]
		if !ok {
			return fmt.Errorf("objects %s: schema chunkmap não registrado", key)
		}
		for _, entry := range byKey[key] {
			if err := applier(version, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyMacroCollection sobrepõe o lote no dicionário COMPLETO e reconstrói
// os binários (→ mods/). full vem da fonte da sessão: o dicionário inteiro
// do .vbf no fluxo de container, o do store em data/. Refs "$hash" do
// payload (view dedupado) resolvem contra as defs do dicionário inteiro —
// nunca são gravadas literais no rebuild.
//
// Antes do rebuild roda a PODA: rows sem par no original (ponteiro
// simplified errado no binário traduzido, por exemplo) saem do lote. É o
// caminho que impede o erro de se propagar a cada import/save — o binário
// não aceita menos linhas que o original, e as linhas originais são
// intocáveis (builders.PruneNotInOriginal aborta se faltar alguma).
//
// A base entra em uma cópia rasa: ela pode ser o dicionário GUARDADO na
// sessão de .vbf, e um rebuild que falhe no meio não pode deixar sujeira
// no estado (a próxima carga pegaria um dicionário pela metade).
func applyMacroCollection(version common.GameVersion, full, c, original dto.Collection) error {
	base := make(dto.Collection, len(full)+len(c))
	for id, entry := range full {
		base[id] = entry
	}
	for id, entry := range c {
		base[id] = entry
	}
	pruned, report, err := builders.PruneNotInOriginal(base, original)
	if err != nil {
		return err
	}
	for id, rows := range report {
		logPrunedRows(pruneViews(KindMacro, version, base[id], rows))
	}
	return builders.ApplyMacroDTO(version, builders.ResolveDedupRefs(pruned))
}

// prunePayload poda as rows do lote que não existem no original da entrada
// (data//container). Vale para os kinds cujo save só reconstrói as entradas
// sujas; o dicionário (macro) tem a poda própria sobre o arquivo inteiro.
// Lockit fica de fora: o formato não é row-table e não tem "linha órfã".
func (s *MetadataService) prunePayload(kind string, version common.GameVersion, c dto.Collection) (dto.Collection, error) {
	original := make(dto.Collection, len(c))
	for id := range c {
		entry, ok, err := s.originalFor(kind, id, version)
		if err != nil || !ok {
			continue
		}
		original[id] = entry
	}
	if len(original) == 0 {
		return c, nil
	}
	pruned, report, err := builders.PruneNotInOriginal(c, original)
	if err != nil {
		return nil, err
	}
	for id, rows := range report {
		logPrunedRows(pruneViews(kind, version, c[id], rows))
	}
	return pruned, nil
}

// applyObjectsEntry carrega o arquivo do layout via schema chunkmap,
// aplica a entrada e grava em mods/. Fluxo único: chunkmap.
//
// Mesma regra do lote: o applier grava o texto da row direto, então a ref
// "$hash" do DTO de display é expandida (e a órfã descartada) antes.
func (s *MetadataService) applyObjectsEntry(version common.GameVersion, id string, entry dto.FileEntry) error {
	resolved := builders.ResolveApplyRefs(dto.Collection{id: entry})
	if len(resolved) == 0 {
		return nil // nada a aplicar (só refs órfãs)
	}
	entry = resolved[id]
	key, ok := objectKeyForID(version, id)
	if !ok {
		return fmt.Errorf("unknown object id: %s", id)
	}
	applier, ok := chunkmap.SchemaAppliers[key]
	if !ok {
		return fmt.Errorf("objects %s: schema chunkmap não registrado", key)
	}
	return applier(version, entry)
}

// ---- texto em memória (DTO estruturado, sem disco) --------------------------

// filterEventIDsForVersion aplica a régua de events por versão, porque as
// expansões leem a MESMA árvore de eventos do jogo-pai:
//   - ffx2: esconde o grupo "lm" — é conteúdo de Last Mission;
//   - lastmiss: mostra só o grupo "lm" (lmdn*/lmev*/lmtuto*/lmys*);
//   - ffx: esconde o grupo "sc" (sc/scene*) — é conteúdo de Eternal Calm;
//   - eternalcalm: mostra só o grupo "sc" (scene1..scene6);
//   - ffx/fx2 fora desses grupos: inalterado (o "lm" do FFX, lmyt*, é
//     texto dessa versão).
func filterEventIDsForVersion(ids []string, version common.GameVersion) []string {
	hasRule := version == common.GameVersionFFX || version == common.GameVersionEternalCalm ||
		version == common.GameVersionFFX2 || version == common.GameVersionLastMiss
	if !hasRule {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if len(id) < 2 {
			continue
		}
		prefix := strings.ToLower(id[:2])
		switch version {
		case common.GameVersionEternalCalm:
			// Mostra só os events do Eternal Calm (diretório sc).
			if prefix == EternalCalmShortened {
				out = append(out, id)
			}
		case common.GameVersionFFX:
			// Esconde o diretório sc da árvore do FFX: é Eternal Calm.
			if prefix == EternalCalmShortened {
				continue
			}
			out = append(out, id)
		case common.GameVersionLastMiss:
			if prefix == LastMissionShortened {
				out = append(out, id)
			}
		case common.GameVersionFFX2:
			if prefix == LastMissionShortened {
				continue
			}
			out = append(out, id)
		}
	}
	return out
}

// ListEntries devolve o índice leve (id + key, sem rows) para montar
// sidebar/tree. Barato por design: não carrega binários de objects.
func (s *MetadataService) ListEntries(kind string, version common.GameVersion) ([]EntrySummary, error) {
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindEvents:
		if err := s.ensureEventsLoaded(version); err != nil {
			return nil, err
		}
		ids := filterEventIDsForVersion(event.GetAllEventIDs(version), version)
		sort.Strings(ids)
		out := make([]EntrySummary, 0, len(ids))
		for _, id := range ids {
			out = append(out, EntrySummary{ID: id, Key: dto.NewEventMetadata(id, version).Key})
		}

		// Inventário de arquivos em paralelo (stat por arquivo) + varredura
		// dos sobras em mods/ (evento só em mods é invisível na árvore: a
		// descoberta é data-driven — só o log denuncia).
		s.emitTreeDiag(KindEvents, version, ids, modsOnlyFilesForEvents(version))
		return out, nil
	case KindObjects:
		var keys []string
		for key, layout := range objectsfile.FileLayouts {
			if layout.Version == version {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		allIDs := make([]string, 0, len(keys))
		out := make([]EntrySummary, 0, len(keys))
		for _, key := range keys {
			layout := objectsfile.FileLayouts[key]
			id := strings.TrimSuffix(layout.FileName, filepath.Ext(layout.FileName))
			allIDs = append(allIDs, id)
			if !originalExists(KindObjects, id, version) {
				common.LogWarning(
					"objects %s/%s: sem original em data/ — omitido da árvore (arquivo só em mods/)",
					version, id,
				)
				continue
			}
			out = append(out, EntrySummary{ID: id, Key: key})
		}
		s.emitTreeDiag(KindObjects, version, allIDs, nil)
		return out, nil
	case KindMacro:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			// Last Mission e Eternal Calm não têm dicionário próprio: não
			// servir os chunks do macrodic do jogo-pai (vazio, sem erro).
			return []EntrySummary{}, nil
		}
		// A árvore é definida por data/: o dicionário existe em data/ ou
		// não existe. O que só estiver em mods/ não é exibido (regra 4).
		c, err := builders.BuildMacroDTOFromSource(version, common.SourceData, nil)
		if err != nil {
			common.LogWarning("macro %s: sem original em data/ — árvore vazia: %v", version, err)
			return []EntrySummary{}, nil
		}
		out := make([]EntrySummary, 0, len(c))
		for _, k := range c.SortedKeys() {
			if !macroChunkHasText(c[k]) {
				// Chunk sem texto: não interessa ao tradutor; oculto no frontend.
				continue
			}
			out = append(out, EntrySummary{ID: k, Key: c[k].Metadata.Key})
		}
		s.emitTreeDiag(KindMacro, version, c.SortedKeys(), nil)
		return out, nil
	case KindLockit:
		layouts := lockit.LayoutsForVersion(version)
		ids := make([]string, 0, len(layouts))
		out := make([]EntrySummary, 0, len(layouts))
		for _, l := range layouts {
			ids = append(ids, l.ID())
			if !originalExists(KindLockit, l.ID(), version) {
				common.LogWarning(
					"lockit %s/%s: sem original em data/ — omitido da árvore (arquivo só em mods/)",
					version, l.ID(),
				)
				continue
			}
			out = append(out, EntrySummary{ID: l.ID(), Key: l.Key()})
		}
		s.emitTreeDiag(KindLockit, version, ids, nil)
		return out, nil
	case KindImages:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			// Last Mission divide a árvore do ffx2 e Eternal Calm a do ffx,
			// mas nenhuma tem texturas próprias: os .dds.phyre vivem só nos
			// jogos-pai — vazio, como macro/lockit.
			return []EntrySummary{}, nil
		}
		ids, onlyMods, fallback, err := ddsphyre.Scan(version)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []EntrySummary{}, nil
		}
		if fallback {
			// data/ não foi extraído do FFX_Data.vbf: a árvore saiu de
			// mods/, então originalExists é sempre false — serve sem filtro
			// e denuncia uma única vez, em vez de 2 mil WARNs (regra 4).
			common.LogWarning(
				"images %s: nenhum .dds.phyre em data/ — árvore montada com %d texturas de mods/ (extraia o FFX_Data.vbf)",
				version, len(ids),
			)
		}
		out := make([]EntrySummary, 0, len(ids))
		kept := make([]string, 0, len(ids))
		for _, id := range ids {
			if !fallback && !originalExists(KindImages, id, version) {
				common.LogWarning(
					"images %s/%s: sem original em data/ — omitido da árvore (arquivo só em mods/)",
					version, id,
				)
				continue
			}
			kept = append(kept, id)
			out = append(out, EntrySummary{
				ID:  id,
				Key: dto.NewImageMetadata(id, version).Key,
			})
		}
		if !fallback {
			// Sobras em mods/ (textura nova ou só substituída) viram WARN —
			// ids já são o relatório legível (rel sem o sufixo).
			s.emitTreeDiag(KindImages, version, kept, onlyMods)
		}
		// Cópias idênticas somem da árvore: um grupo = UMA linha (o
		// representante), as cópias vivem na lista "Repetidas" do painel.
		return hideImageDuplicates(out, version), nil
	case KindHelp:
		// Painéis de ajuda são FFX-only: a árvore ffx2 (e as expansões
		// lastmiss/eternalcalm, que dividem essas árvores) não tem a pasta
		// help/. Sem erro — vazio, como o macro/lockit para lastmiss.
		if version != common.GameVersionFFX {
			return []EntrySummary{}, nil
		}
		if err := ensureHelpLoaded(version); err != nil {
			return nil, err
		}
		out := make([]EntrySummary, 0, len(helpfile.HelpEntries))
		for _, entry := range helpfile.HelpEntries {
			if helpfile.GetHelp(version, entry.Name) == nil {
				continue
			}
			if !originalExists(KindHelp, entry.Name, version) {
				common.LogWarning(
					"help %s/%s: sem original em data/ — omitido da árvore (arquivo só em mods/)",
					version, entry.Name,
				)
				continue
			}
			out = append(out, EntrySummary{
				ID:  entry.Name,
				Key: dto.NewHelpMetadata(entry.Name, "help/"+entry.Dir, version).Key,
			})
		}
		names := make([]string, 0, len(helpfile.HelpEntries))
		for _, entry := range helpfile.HelpEntries {
			names = append(names, entry.Name)
		}
		s.emitTreeDiag(KindHelp, version, names, nil)
		return out, nil
	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		// eventtable: mesma régua dos demais kinds — a árvore é definida
		// por data/; o que só existe em mods/ não é exibido (regra 4).
		if !eventtableKindUsable(kind, version) {
			return []EntrySummary{}, nil
		}
		ids := eventtable.List(kind, version)
		out := make([]EntrySummary, 0, len(ids))
		for _, id := range ids {
			if !originalExists(kind, id, version) {
				common.LogWarning(
					"%s %s/%s: sem original em data/ — omitido da árvore (arquivo só em mods/)",
					kind, version, id,
				)
				continue
			}
			key, ok := eventtable.Key(version, kind, id)
			if !ok {
				continue
			}
			out = append(out, EntrySummary{ID: id, Key: key})
		}
		s.emitTreeDiag(kind, version, ids, nil)
		return out, nil
	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}

// GetEntry devolve uma entrada completa (metadata + rows) por demanda.
// GetCollection continua RAW (export, preview e validação de import).
//
// Fluxo display, em três fases:
//  1. cru (ponteiro = hash do texto do arquivo, sem refs);
//  2. merge com o original de data/ — quando TODAS as rows casam em
//     (Index, Name), o ponteiro é reescrito para o hash do ORIGINAL
//     (imutável) e `Original` é anexado.hash(Text)==hash(Original) ⇔
//     célula ainda não traduzida;
//  3. dedup de display: colapsa em ref o que é repetição de um original
//     at e não é a 1ª ocorrência global do ponteiro. Dupe sobre texto já
//     traduzido não acontece (o texto traduzido permanece literal).
//
// O que só existe em mods/ é ignorado com warning; falha de leitura de
// data/ degrada (sem `original`) em vez de quebrar o view.
func (s *MetadataService) GetEntry(kind, id string, version common.GameVersion) (dto.FileEntry, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	// Frescura do store: binário modificado no disco (tradução copiada/
	// editada em mods/) recarrega antes de servir. O store mudou → o view
	// cru dedupado dos kinds dedupados também sai.
	if s.refreshEntryStore(kind, id, version) {
		clearDedupViewCache()
	}
	entry, base, order, err := s.currentEntry(kind, id, version)
	if err != nil {
		return dto.FileEntry{}, err
	}

	// Kinds sem dedup de display (lockit): entrega direta, sem collapse.
	_ = base
	_ = order

	inData, inMods := originalTrees(kind, id, version)
	if !inData {
		if inMods {
			// Regra 4: o que só existe em mods/ não é exibido — não há
			// original contra o qual revisar.
			common.LogWarning(
				"sem original em data/: ignorando %s/%s/%s (arquivo presente apenas em mods/)",
				version, kind, id,
			)
			return dto.FileEntry{}, fmt.Errorf("%s/%s não tem original em data/ (arquivo presente apenas em mods/)", kind, id)
		}
		// Nenhuma árvore tem o arquivo (store sem contraparte em disco).
		// Nunca quebrar o view por ausência do original: entrega sem ele
		// e SEM reescrever ponteiros — o colapso cai no ramo RAW do dedup
		// de display (por texto).
		common.LogWarning(
			"sem original em data/ para %s/%s/%s — entregando a entrada sem a coluna Original",
			version, kind, id,
		)
		return s.fillRefOriginals(builders.DedupDisplayDTO(entry, base, order), kind, id, version), nil
	}

	orig, exists, oerr := s.originalFor(kind, id, version)
	if oerr != nil {
		common.LogError(
			"falha ao ler o original de %s/%s/%s: %v — entregando a entrada sem a coluna Original",
			version, kind, id, oerr,
		)
		return s.fillRefOriginals(builders.DedupDisplayDTO(entry, base, order), kind, id, version), nil
	}
	if !exists {
		common.LogWarning(
			"original de %s/%s/%s sem rows em data/ — entregando a entrada sem a coluna Original",
			version, kind, id,
		)
		return s.fillRefOriginals(builders.DedupDisplayDTO(entry, base, order), kind, id, version), nil
	}
	merged, diverged := withOriginal(entryRef{kind: kind, id: id, version: version}, entry, orig)
	logDivergence(diverged)
	return s.fillRefOriginals(builders.DedupDisplayDTO(merged, base, order), kind, id, version), nil
}

// currentEntry monta a entrada CRU (estado atual, ponteiro = hash do texto
// do arquivo, sem refs): kinds com dedup global saem do cache de cru da
// versão, objects saem do GetCollection do escopo pedido e os demais
// passam pelo GetCollection direto. O dedup de display e o merge com o
// original acontecem em GetEntry.
func (s *MetadataService) currentEntry(kind, id string, version common.GameVersion) (dto.FileEntry, int, *builders.HashOrder, error) {
	if dedupViewKinds[kind] {
		raw, err := s.rawViewOf(kind, version)
		if err != nil {
			return dto.FileEntry{}, 0, nil, err
		}
		entry, ok := raw.collection[id]
		if !ok {
			return dto.FileEntry{}, 0, nil, fmt.Errorf("%s entry not found: %s", kind, id)
		}
		base, ok := raw.order.Offset(id)
		if !ok {
			base = 0
		}
		return entry, base, raw.order, nil
	}
	if kind == KindObjects {
		// Escopo do PRÓPRIO arquivo (refs não propagam entre objetos): a
		// ordem dos ponteiros cobre só este arquivo, self-contained como o
		// export por arquivo.
		c, err := s.GetCollection(kind, version, []string{id}, progress.Nop())
		if err != nil {
			return dto.FileEntry{}, 0, nil, err
		}
		entry, ok := c[id]
		if !ok {
			return dto.FileEntry{}, 0, nil, fmt.Errorf("%s entry not found: %s", kind, id)
		}
		order := builders.NewHashOrder(c)
		base, ok := order.Offset(id)
		if !ok {
			base = 0
		}
		return entry, base, order, nil
	}
	c, err := s.GetCollection(kind, version, []string{id}, progress.Nop())
	if err != nil {
		return dto.FileEntry{}, 0, nil, err
	}
	entry, ok := c[id]
	if !ok {
		return dto.FileEntry{}, 0, nil, fmt.Errorf("%s entry not found: %s", kind, id)
	}
	return entry, 0, nil, nil
}

// refreshEntryStore vigia a frescura do store da entrada antes de servir:
// binário modificado no disco (tradução copiada/editada manualmente em
// mods/) recarrega — mesmo vigia do lockit, estendido aos demais formatos.
// Devolve true quando o store foi atualizado (o chamador invalida caches
// derivados).
func (s *MetadataService) refreshEntryStore(kind, id string, version common.GameVersion) bool {
	switch kind {
	case KindEvents:
		if version == common.GameVersionLastMiss {
			// LastMiss divide a árvore do ffx2: os carimbos bulk foram
			// tomados com a versão pedida (lm) — mesma régua.
			return event.EnsureEventFresh(version, id)
		}
		return event.EnsureEventFresh(version, id)
	case KindObjects:
		// A reutilização vigiada roda em buildObjectsCollection (GetCollection);
		// GetEntry objects passa por lá.
		return false
	case KindMacro:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			return false
		}
		return macrodic.EnsureMacrosFresh(version)
	case KindLockit:
		// Vigiado dentro de LoadFromStore (lockitFiles/ApplyLockitDTO).
		return false
	case KindHelp:
		if version != common.GameVersionFFX {
			return false
		}
		return helpfile.EnsureHelpFresh(version)
	default:
		return false
	}
}

// ListLanguages devolve os idiomas disponíveis em formato chave/valor
// (Code para arquivos, Name para exibição). Todas as versões usam
// os mesmos idiomas.
func (s *MetadataService) ListLanguages() []common.Language {
	return common.AvailableLanguages()
}

// normalizeIDs aceita ids ou keys: item com "/" ou "\" (metadata.key ou
// caminho) é resolvido para id via Resolve; id puro passa direto.
// Uma metadata.key de macro (sem chunk) não seleciona nada e é ignorada.
func (s *MetadataService) normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		q := strings.TrimSpace(id)
		if q == "" {
			continue
		}
		if strings.Contains(q, "/") || strings.Contains(q, "\\") {
			_, rid, _, err := s.Resolve(q)
			if err != nil {
				out = append(out, q)
				continue
			}
			if rid == "" {
				continue
			}
			out = append(out, rid)
			continue
		}
		out = append(out, q)
	}
	return out
}

// GetCollection monta o DTO em memória via builders (sem gravar arquivo).
// ids vazio = tudo (bulk: 20MB FFX / 45MB FFX2+lastmiss em events —
// uso excepcional; o padrão do frontend é ListEntries + GetEntry).
// ids aceita ids ou keys (metadata.key/caminho resolvidos para id).
// GetCollection monta a Collection do kind (extração: leitura dos binários
// + montagem do DTO). O Reporter r recebe o progresso da extração — os
// builders fazem SetTotal e um tick por entrada; nil = sem progresso
// (view/import/apply não abrem modal).
func (s *MetadataService) GetCollection(kind string, version common.GameVersion, ids []string, r progress.Reporter) (dto.Collection, error) {
	r = progress.OrNop(r)
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}
	ids = s.normalizeIDs(ids)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindEvents:
		if err := s.ensureEventsLoaded(version); err != nil {
			return nil, err
		}
		// Bulk (export/lote): binário modificado no disco
		// recarrega antes de montar o DTO.
		if event.EnsureAllEventsFresh(version, ids) {
			clearDedupViewCache()
		}
		return builders.BuildEventsDTO(version, ids, r)
	case KindObjects:
		return s.buildObjectsCollection(version, ids, r)
	case KindMacro:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			// Expansões sem dicionário próprio (macrodic do jogo-pai).
			return dto.Collection{}, nil
		}
		c, err := builders.BuildMacroDTO(version, r)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return c, nil
		}
		filter := make(map[string]bool, len(ids))
		for _, id := range ids {
			filter[id] = true
		}
		out := make(dto.Collection, len(ids))
		for _, k := range c.SortedKeys() {
			if filter[k] {
				out[k] = c[k]
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("no matching macro chunks in DTO")
		}
		return out, nil
	case KindLockit:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			// Expansões sem lockit próprio (mesma régua do macro:
			// vazio, sem erro).
			return dto.Collection{}, nil
		}
		return builders.BuildLockitDTO(version, ids, r)
	case KindHelp:
		if version != common.GameVersionFFX {
			// A árvore ffx2/expansões não tem a pasta help/.
			return dto.Collection{}, nil
		}
		return builders.BuildHelpDTO(version, ids, r)
	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		if !eventtableKindUsable(kind, version) {
			return dto.Collection{}, nil
		}
		return builders.BuildTableDTO(kind, version, ids, r)
	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}

// helpExportIDs decide os ids do export: o artefato de help é único e
// SEMPRE cobre os 6 painéis (as defs do dedup só resolvem com todos os
// arquivos no mesmo arquivo); demais kinds passam os ids recebidos.
func helpExportIDs(kind string, ids []string) []string {
	if strings.EqualFold(strings.TrimSpace(kind), KindHelp) {
		return helpfile.HelpEntryNames()
	}
	return ids
}

// ExportStrings/ExportJSON/ExportArtifact dobraram em ExportArtifactBatch
// (export_formats.go): a extração é UMA (builders tick a tick), só o write
// muda por formato — sem triplicação de Begin/SetTotal por kind.

// buildObjectsCollection carrega cada arquivo por demanda e monta o DTO.
// ids vazio = todos os layouts da versão. Fluxo único: chunkmap. O
// Reporter r recebe um tick por layout visitado.
func (s *MetadataService) buildObjectsCollection(version common.GameVersion, ids []string, r progress.Reporter) (dto.Collection, error) {
	r = progress.OrNop(r)
	layouts, err := s.resolveObjectLayouts(version, ids)
	if err != nil {
		return nil, err
	}
	r.SetTotal(len(layouts))
	out := make(dto.Collection, len(layouts))
	for _, layout := range layouts {
		r.Step(layout.FileName)
		key := objectsfile.FileLayoutKey(version, layout.PatternPath())
		loader, ok := chunkmap.SchemaRegistry[key]
		if !ok {
			common.LogVerbose("skip objects %s: schema chunkmap não registrado", key)
			continue
		}
		c, cerr := loader(version, layout.PatternPath())
		if cerr != nil {
			common.LogVerbose("skip chunkmap %s: %v", key, cerr)
			continue
		}
		for k, v := range c {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no objects with text data found")
	}
	return out, nil
}

// resolveObjectLayouts traduz ids (basenames) em layouts; vazio = todos.
func (s *MetadataService) resolveObjectLayouts(version common.GameVersion, ids []string) ([]objectsfile.FileLayout, error) {
	if len(ids) == 0 {
		var all []objectsfile.FileLayout
		for _, layout := range objectsfile.FileLayouts {
			if layout.Version == version {
				all = append(all, layout)
			}
		}
		if len(all) == 0 {
			return nil, fmt.Errorf("no object layouts for version %s", version)
		}
		sort.Slice(all, func(a, b int) bool { return all[a].FileName < all[b].FileName })
		return all, nil
	}
	var out []objectsfile.FileLayout
	var missing []string
	for _, id := range ids {
		key, ok := objectKeyForID(version, id)
		if !ok {
			missing = append(missing, id)
			continue
		}
		layout, ok := objectsfile.FileLayoutFor(version, key)
		if !ok {
			// key veio do próprio registro: fallback pelo mapa direto.
			layout = objectsfile.FileLayouts[key]
		}
		out = append(out, layout)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("none of the %d requested object(s) found: %v", len(ids), missing)
	}
	return out, nil
}

// ensureEventsLoaded garante o store em memória (carga bulk única;
// chamadas seguintes usam o datastore). Sem isso, o DTO viria vazio
// onde o Extract direto do arquivo funcionaria.
// ensureEventsLoaded garante o store em memória (carga bulk única;
// método: precisa do notifier para reportar os eventos pulados).
// Sob o viewLoadMu (leitura): uma carga bulk em voo não pode atravessar
// o ClearAllEvents de um InvalidateViewCaches e repovoar o store com o
// conteúdo da árvore anterior (ver view_load_barrier.go).
func (s *MetadataService) ensureEventsLoaded(version common.GameVersion) error {
	viewLoadMu.RLock()
	defer viewLoadMu.RUnlock()
	if event.HasEvents(version) {
		return nil
	}
	b := event.NewEventsBinaryFile(version)
	if err := b.LoadFromBinary(); err != nil {
		return fmt.Errorf("failed to load events for version %s: %w", version, err)
	}
	// A carga é silenciosa (implícita), mas os eventos pulados AVISAM:
	// diagnóstico + toast com cap — ver reportEventSkips.
	s.reportEventSkips(b.Skipped)
	return nil
}

// ensureAllEventsLoaded carrega TODOS os eventos, mesmo com um subconjunto
// já no store (a árvore carrega só a primeira página). Export bulk (ids=[])
// precisa do universo completo. Mesma barreira do ensureEventsLoaded.
func (s *MetadataService) ensureAllEventsLoaded(version common.GameVersion) error {
	viewLoadMu.RLock()
	defer viewLoadMu.RUnlock()
	b := event.NewEventsBinaryFile(version)
	if err := b.LoadFromBinary(); err != nil {
		return fmt.Errorf("failed to load all events for version %s: %w", version, err)
	}
	s.reportEventSkips(b.Skipped)
	return nil
}

// ensureHelpLoaded garante os painéis de ajuda em memória (carga única).
// Mesma barreira dos events: a carga em voo não atravessa o ClearAllHelp.
func ensureHelpLoaded(version common.GameVersion) error {
	viewLoadMu.RLock()
	defer viewLoadMu.RUnlock()
	return helpfile.EnsureHelpLoaded(version)
}

// eventtableKindUsable informa se a família eventtable tem binário nesta
// versão: battletext e cloud existem em FFX e FFX-2; tutorial.msb é FFX-2
// only. LastMiss divide a árvore do FFX-2 e EternalCalm a do FFX — os
// artefatos seguem a régua: essas abas não exibem estas folhas (ficam no
// jogo-pai).
func eventtableKindUsable(kind string, version common.GameVersion) bool {
	switch kind {
	case KindBattleText, KindCloud:
		return version == common.GameVersionFFX || version == common.GameVersionFFX2
	case KindTutorial:
		return version == common.GameVersionFFX2
	case KindMenuMain:
		return version == common.GameVersionFFX
	}
	return false
}
