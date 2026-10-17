package services

import (
	"fmt"
	"path/filepath"
	"sync"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/datastore"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/fileFormats/eventtable"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/fileFormats/macrodic"
	"ffxresources/backend/fileFormats/objectsfile"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/formatters/hash"
)

// ORIGINAL: a coluna "Original" da tabela vem SEMPRE da árvore data/
// (fonte da verdade, imutável); a coluna "Traduzido" (Text) continua
// mods-first (último save). Este arquivo concentra a leitura e a
// mesclagem do original em GetEntry.
//
// Regras:
//   - presença (arquivo/linha/idioma) é definida por data/;
//   - o que existe só em mods/ é IGNORADO (LogWarning) — não há original
//     contra o qual revisar;
//   - falha de leitura de data/ nunca quebra o view: loga e entrega a
//     entrada sem `original` (o frontend mostra "—");
//   - o ponteiro (Hash) da row casada passa a ser o hash do ORIGINAL de
//     data/ (imutável, uma vez na leitura pristine): o Traduzido (Text)
//     reusa o MESMO ponteiro — hash(Text) == hash(Original) ⇔ célula
//     ainda não traduzida; divergência de estrutura degrada por row
//     (sem reescrita nessa row).

// originalSource é a fonte de TODA leitura do original: data/ puro.
const originalSource = common.SourceData

type originalValue struct {
	// exists: o binário da entrada EXISTE em data/ (presença).
	exists bool
	// entry: conteúdo pristine de data/ (vazio quando !exists).
	entry dto.FileEntry
}

var (
	originalMu    sync.Mutex
	originalCache = map[string]originalValue{}
	originalMacro = map[common.GameVersion]dto.Collection{}

	// refOriginalsCache memoiza o pristine POR IDIOMA (RowKey → lang →
	// texto) para o tooltip de repetição e para o editor abre-pela-ref
	// (RefLink.Originals). nil = def sem original em data/ (ausência
	// memoizada: não vira re-leitura por clique).
	refOriginalsMu    sync.Mutex
	refOriginalsCache = map[string]map[string]map[string]string{}
)

func originalCacheKey(kind string, version common.GameVersion, id string) string {
	return kind + "|" + version.String() + "|" + id
}

// originalRelPath devolve o caminho (na raiz usada pelo respectivo leitor)
// do binário pristine da entrada na localização padrão. ok=false significa
// "a entrada não tem arquivo associado" (id desconhecido) — quem trata é o
// chamador.
func originalRelPath(kind, id string, version common.GameVersion) (string, bool) {
	return originalRelPathLoc(kind, id, version, common.DefaultLocalization)
}

// originalRelPathLoc é originalRelPath para UMA localização explícita. É o
// que o escopo do .vbf usa para montar o overlay com todas as variantes
// (eventos/help/macro têm um arquivo por localização).
func originalRelPathLoc(kind, id string, version common.GameVersion, loc string) (string, bool) {
	switch kind {
	case KindEvents:
		rel, err := event.EventRelPath(id)
		if err != nil {
			return "", false
		}
		return filepath.Join(common.GetLocalizationRootForVersion(version, loc), rel), true
	case KindObjects:
		key, ok := objectKeyForID(version, id)
		if !ok {
			return "", false
		}
		layout, ok := objectsfile.FileLayoutFor(version, key)
		if !ok {
			layout = objectsfile.FileLayouts[key]
		}
		return filepath.Join(
			common.GetLocalizationRootForVersion(version, loc),
			layout.PatternPath(),
		), true
	case KindMacro:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			// Expansões sem dicionário próprio (macrodic do jogo-pai).
			return "", false
		}
		return filepath.Join(
			common.GetLocalizationRootForVersion(version, loc),
			macrodic.MacroRelPath(),
		), true
	case KindLockit:
		for _, l := range lockit.LayoutsForVersion(version) {
			if l.ID() == id {
				return l.RelPath(loc), true
			}
		}
		return "", false
	case KindHelp:
		if version != common.GameVersionFFX || !helpfile.IsHelpEntry(id) {
			return "", false
		}
		return helpfile.HelpPathForVersion(version, loc, id), true
	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		if !eventtableKindUsable(kind, version) {
			return "", false
		}
		if rel, ok := eventtable.RelPathLoc(version, loc, kind, id); ok {
			return rel, true
		}
		return "", false
	case KindImages:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm || !ddsphyre.ValidID(id) {
			// Expansões sem texturas próprias (.dds.phyre do jogo-pai).
			return "", false
		}
		return ddsphyre.RelPath(version, id), true
	}
	return "", false
}

// originalTrees informa em qual árvore o binário da ENTRADA está.
//
//	inData  → data/ tem o arquivo: há original (fonte da verdade);
//	inMods  → só mods/ tem: regra 4, a entrada é ignorada;
//	nenhum  → o arquivo não está em árvore nenhuma (store sintético ou
//	          entrada sem arquivo associado): entrega sem `original`,
//	          sem quebrar o view.
func originalTrees(kind, id string, version common.GameVersion) (inData, inMods bool) {
	// events: a ACHAGEM do evento já é feita em data/ (o store só recebe o
	// que veio da árvore original), então o registro no store É a presença —
	// stat adicional aqui só tornaria o view frágil a atraso de disco.
	if kind == KindEvents {
		return event.GetEvent(version, id) != nil, false
	}

	rel, ok := originalRelPath(kind, id, version)
	if !ok {
		return false, false
	}
	if acc, err := common.NewFileAccessorFrom(rel, common.SourceData); err == nil && acc.Exists {
		return true, false
	}
	if acc, err := common.NewFileAccessorFrom(rel, common.SourceMods); err == nil && acc.Exists {
		return false, true
	}
	return false, false
}

// hasModsFile informa se o binário da entrada existe em mods/. É o gatilho
// do custo de leitura do original na construção da ordem global de ponteiros:
// arquivo sem mods não está traduzido, e o ponteiro de mods JÁ É o ponteiro
// do original (mesmo conteúdo ⇒ mesmo XXH64).
func hasModsFile(kind, id string, version common.GameVersion) bool {
	rel, ok := originalRelPath(kind, id, version)
	if !ok {
		return false
	}
	acc, err := common.NewFileAccessorFrom(rel, common.SourceMods)
	return err == nil && acc.Exists
}

// originalExists é o teste de presença usado na árvore lateral: só data/
// define quem aparece (regra 4 esconde o que só existe em mods/).
func originalExists(kind, id string, version common.GameVersion) bool {
	inData, _ := originalTrees(kind, id, version)
	return inData
}

// originalFor devolve a entrada pristine de data/ para (kind, version, id),
// com cache. Inexistência é exists=false, sem erro; erro de leitura/parse
// NÃO é memoizado (pode ser transitória) e o chamador degrada.
func (s *MetadataService) originalFor(kind, id string, version common.GameVersion) (dto.FileEntry, bool, error) {
	// Escopo do .vbf em curso: o cache guarda o pristine de data/ e o
	// conteúdo do container é OUTRA coisa — nem lê nem grava aqui.
	if common.VbfSourceActive() {
		return s.loadOriginalFrom(kind, id, version, originalSource)
	}
	if kind == KindMacro {
		return s.originalMacroEntry(id, version)
	}

	key := originalCacheKey(kind, version, id)
	originalMu.Lock()
	if v, ok := originalCache[key]; ok {
		originalMu.Unlock()
		return v.entry, v.exists, nil
	}
	originalMu.Unlock()

	entry, exists, err := s.loadOriginalFrom(kind, id, version, originalSource)
	if err == nil {
		originalMu.Lock()
		originalCache[key] = originalValue{exists: exists, entry: entry}
		originalMu.Unlock()
	}
	return entry, exists, err
}

// originalMacroEntry lê o macrodic de data/ uma vez por versão (o binário é
// único: todos os chunks saem do mesmo arquivo) e recorta a entrada pedida.
func (s *MetadataService) originalMacroEntry(id string, version common.GameVersion) (dto.FileEntry, bool, error) {
	originalMu.Lock()
	if c, ok := originalMacro[version]; ok {
		originalMu.Unlock()
		entry, found := c[id]
		return entry, found, nil
	}
	originalMu.Unlock()

	c, err := builders.BuildMacroDTOFromSource(version, originalSource, nil)
	if err != nil {
		// Falha de leitura/interpretação: não memoiza (pode ser transitória).
		return dto.FileEntry{}, false, err
	}
	originalMu.Lock()
	originalMacro[version] = c
	originalMu.Unlock()

	entry, found := c[id]
	return entry, found, nil
}

// macroEntryFrom monta a entrada de um chunk do dicionário lendo da fonte
// indicada, SEM cache — é o caminho do .vbf, onde o binário vem do overlay
// e varia a cada escopo (o cache é só do pristine de data/).
func (s *MetadataService) macroEntryFrom(id string, version common.GameVersion, src common.FileSource) (dto.FileEntry, bool, error) {
	c, err := builders.BuildMacroDTOFromSource(version, src, nil)
	if err != nil {
		return dto.FileEntry{}, false, err
	}
	entry, found := c[id]
	return entry, found, nil
}

// loadOriginalFrom monta a entrada pristine da FONTE dada para o kind,
// sem registrar nada em store nenhum (os stores continuam refletindo a
// visão traduzida/normal). originalSource = coluna Original de data/.
func (s *MetadataService) loadOriginalFrom(kind, id string, version common.GameVersion, src common.FileSource) (dto.FileEntry, bool, error) {
	if kind == KindMacro {
		return s.macroEntryFrom(id, version, src)
	}
	switch kind {
	case KindEvents:
		strs, err := event.ReadLocalizedEventStringsFrom(id, version, src)
		if err != nil {
			return dto.FileEntry{}, false, err
		}
		if len(strs) == 0 {
			return dto.FileEntry{}, false, nil
		}
		entry, ok := builders.BuildEventEntryDTOFrom(id, version, strs)
		if !ok {
			return dto.FileEntry{}, false, nil
		}
		return entry, true, nil

	case KindObjects:
		layouts, err := s.resolveObjectLayouts(version, []string{id})
		if err != nil {
			return dto.FileEntry{}, false, fmt.Errorf("objects %s: %w", id, err)
		}
		if len(layouts) == 0 {
			return dto.FileEntry{}, false, fmt.Errorf("objects %s: layout não encontrado", id)
		}
		layout := layouts[0]
		key := objectsfile.FileLayoutKey(version, layout.PatternPath())
		// Fluxo único: chunkmap. O schema registrado carrega da fonte
		// indicada com os MESMOS nomes de campo da coleção de exibição.
		loadFrom, ok := chunkmap.SchemaSourceLoaders[key]
		if !ok {
			return dto.FileEntry{}, false, fmt.Errorf("objects %s: schema chunkmap não registrado", key)
		}
		c, cerr := loadFrom(version, layout.PatternPath(), src)
		if cerr != nil {
			return dto.FileEntry{}, false, fmt.Errorf("objects %s: %w", id, cerr)
		}
		entry, ok := c[id]
		if !ok {
			return dto.FileEntry{}, false, nil
		}
		return entry, true, nil
	case KindLockit:
		for _, l := range lockit.LayoutsForVersion(version) {
			if l.ID() != id {
				continue
			}
			f, err := lockit.LoadFrom(l, src)
			if err != nil {
				return dto.FileEntry{}, false, fmt.Errorf("lockit %s: %w", id, err)
			}
			c, err := builders.BuildLockitDTOFrom([]*lockit.LockitFile{f}, nil)
			if err != nil {
				return dto.FileEntry{}, false, fmt.Errorf("lockit %s: %w", id, err)
			}
			entry, ok := c[id]
			if !ok {
				return dto.FileEntry{}, false, nil
			}
			return entry, true, nil
		}
		return dto.FileEntry{}, false, nil

	case KindHelp:
		panel := helpfile.ReadHelpPanelFrom(version, id, src)
		if panel == nil {
			return dto.FileEntry{}, false, nil
		}
		entry, ok := builders.BuildHelpEntryDTOFrom(id, version, panel)
		if !ok {
			return dto.FileEntry{}, false, nil
		}
		return entry, true, nil

	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		strs, err := eventtable.ReadLocalizedStringsFrom(kind, id, version, src)
		if err != nil {
			return dto.FileEntry{}, false, err
		}
		entry, ok := builders.BuildTableEntryDTOFrom(kind, id, version, strs)
		if !ok {
			return dto.FileEntry{}, false, nil
		}
		return entry, true, nil
	}
	return dto.FileEntry{}, false, fmt.Errorf("unknown kind: %s", kind)
}

// fillRefOriginals preenche o Original vazio das anotações de ref (tooltip
// "Repetição de:" na UI). O HashOrder do dedup de display nasce da coleção
// RAW (normalizeCollection), onde a coluna Original só é merged para
// entradas COM binário em mods/ — a def NÃO traduzida (o caso comum do
// dedup: a 1ª ocorrência ainda sem edição) chegava sem pristine e o tooltip
// saía truncado ("Repetição de: " sem texto). Aqui o original da def sai
// do pristine de data/ (originalFor), com memo de sessão.
//
// Ausência/falha do original degrada e fica memoizada (não vira re-leitura
// por clique): a anotação segue sem Original e o frontend cai no texto
// atual da def. id é a entrada servida — fallback quando a anotação não
// carrega SourceID.
func (s *MetadataService) fillRefOriginals(entry dto.FileEntry, kind, id string, version common.GameVersion) dto.FileEntry {
	if len(entry.Refs) == 0 {
		return entry
	}
	for key, ref := range entry.Refs {
		if ref.Original != "" && ref.Originals != nil {
			continue
		}
		defID := ref.SourceID
		if defID == "" {
			defID = id
		}
		defKey := dto.RowKey(dto.TextRow{Index: ref.SourceIndex, Name: ref.SourceName})
		if ref.Original == "" {
			if orig, ok := s.refOriginalRows(kind, defID, version)[defKey]; ok {
				ref.Original = orig
			}
		}
		// Mapa de idiomas da def: é o que o editor abre-pela-ref mostra
		// na coluna de idiomas extras. Sem ele, a ref cuja def vive em
		// OUTRO arquivo perdia os extras e mostrava só 'us'.
		if ref.Originals == nil {
			if byLang, ok := s.refOriginalsMap(kind, defID, version)[defKey]; ok {
				ref.Originals = byLang
			}
		}
		if ref.Original != "" || ref.Originals != nil {
			entry.Refs[key] = ref
		}
	}
	return entry
}

// refOriginalRows devolve o texto 'us' do pristine de data/ por RowKey — o
// recorte de um idioma do refOriginalsMap (fonte única do pristine memoizado
// em sessão). O pristine não muda com apply (mods/ é outra árvore), então o
// memo só cai no clearOriginalCache.
func (s *MetadataService) refOriginalRows(kind, id string, version common.GameVersion) map[string]string {
	byLang := s.refOriginalsMap(kind, id, version)
	if byLang == nil {
		return nil
	}
	rows := make(map[string]string, len(byLang))
	for key, langs := range byLang {
		rows[key] = langs[common.DefaultLocalization]
	}
	return rows
}

// refOriginalsMap devolve o pristine POR IDIOMA (RowKey → lang → texto) com
// memo de sessão — o espelho multi-idioma de refOriginalRows. Alimenta o
// RefLink.Originals, que é o que o editor aberto pela ref mostra na coluna
// de idiomas extras (sem isto, a ref cuja def vive em OUTRO arquivo perdia
// os extras e mostrava só 'us').
func (s *MetadataService) refOriginalsMap(kind, id string, version common.GameVersion) map[string]map[string]string {
	key := originalCacheKey(kind, version, id)
	refOriginalsMu.Lock()
	if rows, ok := refOriginalsCache[key]; ok {
		refOriginalsMu.Unlock()
		return rows
	}
	refOriginalsMu.Unlock()

	orig, exists, err := s.originalFor(kind, id, version)
	if err != nil || !exists {
		refOriginalsMu.Lock()
		refOriginalsCache[key] = nil
		refOriginalsMu.Unlock()
		return nil
	}
	rows := make(map[string]map[string]string, len(orig.Rows))
	for _, r := range orig.Rows {
		cp := make(map[string]string, len(r.Text))
		for lang, t := range r.Text {
			cp[lang] = t
		}
		rows[dto.RowKey(r)] = cp
	}
	refOriginalsMu.Lock()
	refOriginalsCache[key] = rows
	refOriginalsMu.Unlock()
	return rows
}

// withOriginal devolve uma CÓPIA de current com `Original` preenchido e o
// PONTEIRO de dupe (Hash) reescrito a partir de data/, casando as rows por
// (Index, Name) — e devolve o relatório de divergência de estrutura.
//
// A tabela é a UNIÃO dos dois lados: row só na tradução fica sem `Original`
// e marcada MissingInOriginal; row só no original entra no fim com Text
// vazio e marcada MissingInTranslated. É o frontend quem pinta em vermelho
// o lado que falta — o app não decide quem está "certo".
//
// O hash de exibição é o hash do ORIGINAL (imutável, criado uma vez na
// leitura pristine): coluna Traduzido (Text) reusa o MESMO ponteiro, então
// hash(Text) == hash(Original) ⇔ célula ainda não traduzida — a comparação
// dos dois é o flag "pendente de revisão" e decide o dupe.
//
// A emissão do aviso é do CHAMADOR (logDivergence), em ordem — os workers
// paralelos apenas coletam.
func withOriginal(ref entryRef, current, orig dto.FileEntry) (dto.FileEntry, divergeDiag) {
	if len(current.Rows) == 0 || len(orig.Rows) == 0 {
		// Um dos lados sem rows: a união é o outro lado inteiro, marcado
		// como ausente do lado que faltou (sem relatório — o chamador já
		// decide o que fazer com um original inexistente).
		return unionSingleSide(current, orig), divergeDiag{
			ref:      ref,
			dataRows: len(orig.Rows),
			modsRows: len(current.Rows),
		}
	}

	type rowKey struct {
		index int
		name  string
	}
	byKey := make(map[rowKey]map[string]string, len(orig.Rows))
	seenOrig := make(map[rowKey]bool, len(orig.Rows))
	for _, r := range orig.Rows {
		byKey[rowKey{r.Index, r.Name}] = r.Text
	}

	rows := make([]dto.TextRow, len(current.Rows))
	copy(rows, current.Rows)

	matched := 0
	var onlyInMods []DiagRow
	for i := range rows {
		k := rowKey{rows[i].Index, rows[i].Name}
		src, ok := byKey[k]
		if !ok || len(src) == 0 {
			// Só na tradução: coluna Original fica vazia e é ela que o
			// frontend pinta em vermelho.
			rows[i].MissingInOriginal = true
			onlyInMods = append(onlyInMods, diagRowOf(k.index, k.name, rows[i].Text[common.DefaultLocalization]))
			continue
		}
		cp := make(map[string]string, len(src))
		for kk, v := range src {
			cp[kk] = v
		}
		rows[i].Original = cp
		seenOrig[k] = true
		matched++
	}

	var onlyInData []DiagRow
	for _, r := range orig.Rows {
		k := rowKey{r.Index, r.Name}
		if seenOrig[k] {
			continue
		}
		onlyInData = append(onlyInData, diagRowOf(k.index, k.name, r.Text[common.DefaultLocalization]))
		// Só no original: entra na união com a coluna Traduzido vazia.
		// O ponteiro é preenchido adiante, junto das rows casadas.
		rows = append(rows, dto.TextRow{
			Index:               r.Index,
			Name:                r.Name,
			Original:            copyTextMap(r.Text),
			MissingInTranslated: true,
		})
	}

	diag := divergeDiag{
		ref:        ref,
		dataRows:   len(orig.Rows),
		modsRows:   len(rows) - len(onlyInData),
		onlyInMods: onlyInMods,
		onlyInData: onlyInData,
	}

	// Ponteiro estrutural por row casada: hash do original por idioma.
	// Idioma sem texto original mantém o hash atual (nada para agrupar por
	// ali). Idempotente: célula não traduzida já tem os hashes coincidentes.
	// As rows acrescidas da união entram aqui também (Text vazio ⇒ o
	// ponteiro é só o do original).
	for i := range rows {
		if rows[i].Original == nil {
			continue
		}
		for lang, o := range rows[i].Original {
			if o == "" {
				continue
			}
			if rows[i].Hash == nil {
				rows[i].Hash = map[string]string{}
			}
			rows[i].Hash[lang] = hash.Sum64Hex(o)
		}
	}
	// A união pode ter acrescentado rows fora de ordem: ordenar AQUI, e não
	// só no dedup de display, para que a ordem canônica (HashOrder) seja
	// construída sobre a mesma sequência que a view serve.
	dto.SortRows(rows)
	current.Rows = rows
	return current, diag
}

// unionSingleSide devolve a união quando um dos lados não tem rows: o lado
// existente inteiro, marcado como ausente do outro.
func unionSingleSide(current, orig dto.FileEntry) dto.FileEntry {
	if len(current.Rows) == 0 && len(orig.Rows) == 0 {
		return current
	}
	if len(current.Rows) == 0 {
		rows := make([]dto.TextRow, 0, len(orig.Rows))
		for _, r := range orig.Rows {
			cp := r
			cp.Original = copyTextMap(r.Text)
			cp.Text = nil
			cp.Hash = nil
			cp.MissingInTranslated = true
			rows = append(rows, cp)
		}
		orig.Rows = rows
		return orig
	}
	rows := make([]dto.TextRow, len(current.Rows))
	copy(rows, current.Rows)
	for i := range rows {
		rows[i].MissingInOriginal = true
	}
	current.Rows = rows
	return current
}

// copyTextMap copia o mapa de texto de uma row: os entries vêm do cache do
// original e não podem ser compartilhados com a view.
func copyTextMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	cp := make(map[string]string, len(src))
	for k, v := range src {
		cp[k] = v
	}
	return cp
}

// clearOriginalCache descarta o original em memória (a árvore data/ mudou —
// ou os caches de view estão sendo invalidados por higiene).
func clearOriginalCache() {
	originalMu.Lock()
	originalCache = map[string]originalValue{}
	originalMacro = map[common.GameVersion]dto.Collection{}
	originalMu.Unlock()
	refOriginalsMu.Lock()
	refOriginalsCache = map[string]map[string]map[string]string{}
	refOriginalsMu.Unlock()
}

// InvalidateViewCaches descarta tudo que foi DERIVADO da árvore de
// gamefiles (DTOs deduplicados, original pristine e os stores de formato).
// Chamado ao trocar GameFilesLocation: os binários carregados pertencem à
// árvore anterior e sem isso a view serviria texto de outro diretório.
func InvalidateViewCaches() {
	treeGeneration.Add(1)
	preloadMu.Lock()
	preloadedVersions = map[string]bool{}
	preloadMu.Unlock()
	clearDedupViewCache()
	event.ClearAllEvents()
	helpfile.ClearAllHelp()
	lockit.DataStore.Clear()
	eventtable.ClearStore()
	datastore.Instance.ClearAllMacros()
	// A preparação por versão (charsets + macros) leu a árvore anterior:
	// zera para que a próxima chamada recarregue do novo diretório.
	readyMu.Lock()
	readyVersions = map[common.GameVersion]error{}
	readyMu.Unlock()
}
