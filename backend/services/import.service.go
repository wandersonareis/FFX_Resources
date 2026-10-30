package services

// Importação em lote (.json/.strings/.csv/.xlsx/.po) do artefato de CAT:
// parse → detecção de kind/versão → store → casamento por COORDENADA →
// merge do 'us' → capacidade uint16.
//
// O artefato traz source/target: `source` é a referência (us de data/) e
// `target` é o texto a aplicar no slot `us` de mods/ — o ÚNICO texto que o
// import lê. `text`/`hash` continuam a referência do segmento; o hash de
// `us` valida staleness contra data/ (mudou desde o export ⇒ row rejeitada,
// não se sobrescreve tradução de outro segmento).
//
// Casamento: (Index, Name) — data/ e mods/ são o MESMO binário em estados
// diferentes, então a posição é estável e independe do hash do texto
// traduzido. Sem data/ disponível, o casamento por coordenada segue sozinho
// (a checagem de tags também é pulada).
//
// Divisão de erros:
//   - arquivo (ilegível, extensão, key ausente/inválida, kind ou versão
//     divergentes, kinds misturados, formato antigo sem source/target) →
//     error (toast no frontend);
//   - entrada inexistente na versão, erro de medição e capacidade uint16
//     → summary.Warnings: a ENTRADA é excluída do import (a granularidade
//     é o arquivo — um erro em qualquer row exclui a entrada inteira) e o
//     restante importa; o modal avisa o que será ignorado;
//   - bloqueadores globais (estouro do dicionário macro, binário único
//     compartilhado por todas as entradas) → summary.Errors (modal
//     desabilita Importar).
//
// NÃO bloqueiam: origem não encontrada e tag de controle divergente. A row é
// pulada e o resto importa; o texto completo sai no log (LogWarning) e o
// resumo carrega o espelho curto em Skipped — o modal de detalhes mostra
// ao revisor o que ficou de fora (botão pulsante) sem poluir a confirmação.
//
// Capacidade uint16 SEMPRE após a conversão das tags de controle em bytes:
//   - events: rebuild real em cópias (RebuildFieldStrings deduplica offsets);
//   - objects: texto medido direto por converter.StringToByteSize (mesma
//     conversão do FillByteList do rebuild, sem instâncias auxiliares);
//   - macro: rebuild real do dicionário inteiro (layout do container).
//
// A store nunca é mutada: events trabalham em cópias de FieldString.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/fileFormats/eventtable"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/fileFormats/objectsfile"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/formatters"
	"ffxresources/backend/formatters/hash"
	"ffxresources/backend/progress"
	"ffxresources/backend/uilog"
)

// importUint16Limit é o teto de bytes por binário: os offsets são uint16
// (0..65535) e a string table começa em 0 → 65536 bytes é o máximo gravável.
const importUint16Limit = 65536

// importPlan é o resultado do prepareImport: entradas válidas mescladas e o
// resumo para o modal.
type importPlan struct {
	kind    string
	merged  dto.Collection // store clonada + 'us' do import (alvo do apply)
	summary dto.ImportSummary
}

// ---- helpers de rows ---------------------------------------------------------

// binaryRowKey é a coordenada da row NO BINÁRIO: (Index, Name) — a mesma
// chave "index\x00name" do strings parser. É a identidade do casamento de
// import: data/ e mods/ são o mesmo binário em estados diferentes, então a
// posição é estável e não depende do hash do texto traduzido.
func binaryRowKey(r dto.TextRow) string {
	return fmt.Sprintf("%d\x00%s", r.Index, r.Name)
}

// usText devolve o texto 'us' da row da STORE (estado atual do binário).
func usText(r dto.TextRow) string {
	return r.Text[common.DefaultLocalization]
}

// importText devolve o texto que o artefato pede para aplicar: o campo
// `target` (projeção de export). Vazio = a row não pede alteração nenhuma
// (o segmento ainda não tem tradução em mods/).
func importText(r dto.TextRow) string {
	return r.Target
}

// artifactHasTarget informa se o artefato traz a projeção source/target (o
// formato atual). Artefato antigo (só text) não é importável: o texto do
// jogo vive no binário, e a conversão é reimportar com a versão anterior do
// app e exportar de novo.
func artifactHasTarget(c dto.Collection) bool {
	for _, entry := range c {
		for _, r := range entry.Rows {
			if r.Target != "" || r.Source != "" {
				return true
			}
		}
	}
	return false
}

// isBlankText diz se um texto não é traduzível: vazio, só espaço, o "-" que
// é o próprio texto do jogo (mais da metade do data/ pristine) ou só tags de
// formatação ({TEXT_NEWLINE}, {TEXT_ITALIC}, color…) — linha que o tradutor
// não teria o que editar. O filtro é de casamento e de export — a store
// nunca é filtrada, porque é ela que guarda a posição da row no binário e o
// rebuild posicional.
func isBlankText(t string) bool {
	s := strings.TrimSpace(stripTextTags(t))
	return s == "" || s == "-"
}

// stripTextTags remove do texto todas as tags classificadas como text-tag
// (ausência/quantidade delas % não é conteúdo). Chave
// explícita: tags de CONTROLE (icone, pausa…) NÃO saem — elas preservam
// significado no segmento. As duas regras batem: tag sem fechamento não é
// tag, vira texto literal.
func stripTextTags(t string) string {
	runes := []rune(t)
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if runes[i] != '{' {
			out = append(out, runes[i])
			continue
		}
		end := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == '}' {
				end = j
				break
			}
		}
		if end < 0 {
			out = append(out, runes[i])
			continue
		}
		if converter.IsTextTag(string(runes[i+1 : end])) {
			i = end
			continue
		}
		out = append(out, runes[i])
	}
	return string(out)
}

// sourceHash devolve o hash da REFERÊNCIA da row (o `us` de data/ no momento
// do export). Artefato feito à mão sem hash cai no hash do campo source.
func sourceHash(r dto.TextRow) string {
	if h := r.Hash[common.DefaultLocalization]; h != "" {
		return h
	}
	return hash.Sum64Hex(r.Source)
}

// countCandidates conta as rows do artefato com target utilizável — o
// denominador de "N inserido de N". Row sem target não pede alteração
// (segmento ainda não traduzido em mods/) e não é candidata.
func countCandidates(rows []dto.TextRow) int {
	n := 0
	for _, r := range rows {
		if !isBlankText(importText(r)) {
			n++
		}
	}
	return n
}

// rowIndexCount conta índices distintos (index_count do resumo).
func rowIndexCount(rows []dto.TextRow) int {
	seen := make(map[int]bool, len(rows))
	for _, r := range rows {
		seen[r.Index] = true
	}
	return len(seen)
}

// importLanguages devolve (ordem estável) os idiomas com algum texto no arquivo.
func importLanguages(c dto.Collection) []string {
	seen := make(map[string]bool)
	for _, id := range c.SortedKeys() {
		for _, r := range c[id].Rows {
			for lang, t := range r.Text {
				if t != "" {
					seen[lang] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for lang := range seen {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

// ---- casamento por coordenada ------------------------------------------------

// As mensagens do import. Todas vão para o console colorido e para o arquivo
// JSON de log (common/system.go). Origem não encontrada e tag divergente
// NUNCA entram em summary.Warnings/Errors: a row é bloqueada, a importação
// segue, e quem corrige é o revisor no artefato.
const (
	logImportUnmatched = "import %s: origem não encontrada — hash %q target %q"
	logImportTagLost   = "import %s: tag de controle divergente — hash %q target %q %v"
	logImportInserted  = "import %s: %d/%d linha(s) inserida(s)"

	// Reasons do ImportRowIssue: rótulos curtos exibidos no modal de
	// detalhes — o mesmo diagnóstico dos LogWarning, sem o texto longo.
	reasonUnmatched = "origem não encontrada"
	reasonTagLost   = "tag de controle divergente"
)

// truncateRowIssue encurta o texto da row para o resumo do modal: linhas de
// tutorial passam fácil de 200 runes e o detalhe inteiro vive no log. Corte
// por rune (nunca quebra UTF-8) com elipse explícita quando há mais.
func truncateRowIssue(s string) string {
	const max = 120
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// rowPair é o casamento entre uma row do artefato e a row da store. A
// posição vem SEMPRE da store; o texto novo vem do target do artefato.
type rowPair struct {
	store    dto.TextRow
	imported dto.TextRow
}

// tagViolation é uma row cuja tradução divergiu das tags de controle do
// original (data/ ou VBF).
type tagViolation struct {
	row    dto.TextRow
	issues []converter.TagIssue
}

// matchRows casa as rows do artefato com as da store pela COORDENADA
// (Index, Name): data/ e mods/ são o MESMO binário em estados diferentes,
// então a posição é estável e independe do hash do texto traduzido (que é o
// hash da tradução, não da origem).
//
// A entrada de origem (data/) entra como validação de staleness: o segmento
// na coordenada precisa ter o mesmo hash da referência gravada no artefato.
// Origem ausente = checagem pulada (degradação); origem divergente ou
// coordenada sem célula na store = unmatched.
//
//  1. row do artefato sem target → pulada (nada a importar);
//  2. coordenada duplicada no artefato → unmatched na segunda (não sobrescreve);
//  3. origem presente: hash(us de data/) != sourceHash → unmatched;
//  4. célula alvo = row da store na MESMA coordenada; ausente → unmatched.
func matchRows(importedEntry, storeEntry dto.FileEntry, orig *dto.FileEntry) (pairs []rowPair, unmatched []dto.TextRow) {
	storeByKey := make(map[string]dto.TextRow, len(storeEntry.Rows))
	for _, r := range storeEntry.Rows {
		storeByKey[binaryRowKey(r)] = r
	}
	var origByKey map[string]dto.TextRow
	if orig != nil {
		origByKey = make(map[string]dto.TextRow, len(orig.Rows))
		for _, r := range orig.Rows {
			origByKey[binaryRowKey(r)] = r
		}
	}

	claimed := make(map[string]bool, len(importedEntry.Rows))
	for _, r := range importedEntry.Rows {
		if isBlankText(importText(r)) {
			continue // nada a importar: segmento ainda não traduzido
		}
		key := binaryRowKey(r)
		if claimed[key] {
			unmatched = append(unmatched, r)
			continue
		}
		if origByKey != nil && (r.Source != "" || r.Hash[common.DefaultLocalization] != "") {
			o, ok := origByKey[key]
			if !ok || hash.Sum64Hex(usText(o)) != sourceHash(r) {
				unmatched = append(unmatched, r)
				continue
			}
		}
		storeRow, ok := storeByKey[key]
		if !ok {
			unmatched = append(unmatched, r)
			continue
		}
		claimed[key] = true
		// O padding de tela da store vence (keepStorePadding): o par já nasce
		// com o target reconciliado no 'us', então a validação de tags, a
		// contagem de alterados (diff só de padding não conta) e o merge usam
		// o mesmo valor final sem nenhuma passada extra.
		pairs = append(pairs, rowPair{store: storeRow, imported: reconcileImportPadding(storeRow, r)})
	}
	return pairs, unmatched
}

// importMatch é o resultado do casamento de uma entrada.
type importMatch struct {
	candidates int            // rows do artefato com target utilizável
	inserted   int            // casaram E passaram na validação de tags
	changed    int            // destas, com texto diferente da store
	pairs      []rowPair      // a gravar
	unmatched  []dto.TextRow  // sem origem/coordenada na store
	tagLost    []tagViolation // tags de controle divergentes
}

// matchImportEntry casa por coordenada e, quando orig for fornecido, valida o
// hash da referência e as tags de controle do original. Sem orig, as duas
// checagens são puladas.
//
// A filtragem acontece AQUI (e não no merge) para que a row rejeitada nem
// chegue em mergeImportUs: é por isso que ela não aparece em merged[id] e a
// função de uso nunca a vê.
func matchImportEntry(imp, se dto.FileEntry, orig *dto.FileEntry) importMatch {
	m := importMatch{candidates: countCandidates(imp.Rows)}
	pairs, unmatched := matchRows(imp, se, orig)
	m.unmatched = unmatched
	if len(pairs) == 0 {
		return m
	}

	origByText := originalRowTexts(orig)
	for _, p := range pairs {
		// Row cujo target é igual ao texto vigente da store é NO-OP: nada é
		// gravado, então não há tag para validar. Sem isto, uma divergência
		// de tag PRÉ-EXISTENTE na tradução (ex.: MCR de speaker que o
		// original EN não tem) vira aviso em todo re-import sem edição.
		changed := usText(p.imported) != usText(p.store)
		if changed && origByText != nil {
			if want, ok := origByText[binaryRowKey(p.store)]; ok {
				if issues := converter.ControlTagDiff(want, usText(p.imported)); len(issues) > 0 {
					m.tagLost = append(m.tagLost, tagViolation{row: p.imported, issues: issues})
					continue
				}
			}
		}
		m.pairs = append(m.pairs, p)
		if changed {
			m.changed++
		}
	}
	m.inserted = len(m.pairs)
	return m
}

// originalRowTexts devolve (coordenada binário → texto 'us' original) da
// entrada em data/ — ou no .vbf ativo, que é o que originalFor resolve.
//
// Nil quando não há referência: sem original não há o que comparar, e
// travar a importação por data/ incompleto seria pior do que seguir sem
// checagem. Rows originais em branco ficam de fora pelo mesmo motivo — não
// tem tag a preservar em "-".
func originalRowTexts(orig *dto.FileEntry) map[string]string {
	if orig == nil {
		return nil
	}
	out := make(map[string]string, len(orig.Rows))
	for _, r := range orig.Rows {
		if t := usText(r); !isBlankText(t) {
			out[binaryRowKey(r)] = t
		}
	}
	return out
}

// filterExportRows remove do ARTEFATO as rows sem referência nem tradução:
// fica a row cujo 'us' de data/ é utilizável OU cujo target (mods/) tem
// texto — linha em branco no original mas traduzida não pode sumir do
// artefato (é justamente o que o import precisa devolver ao binário).
//
// Só para EXPORT. A store (carga, merge, save, apply, VBF) continua
// completa, porque é ela que guarda a posição da row no binário. Recalcula
// Metadata.RowCount, que os builders preenchem com o total.
func filterExportRows(c dto.Collection) dto.Collection {
	out := make(dto.Collection, len(c))
	for id, entry := range c {
		rows := make([]dto.TextRow, 0, len(entry.Rows))
		for _, r := range entry.Rows {
			if isBlankText(usText(r)) && isBlankText(importText(r)) {
				continue
			}
			rows = append(rows, r)
		}
		if len(rows) == 0 {
			continue // entrada só com row em branco: nada a traduzir
		}
		entry.Metadata = entry.Metadata.WithRowCount(len(rows))
		entry.Rows = rows
		out[id] = entry
	}
	return out
}

// mergeImportUs clona a store e sobrescreve o 'us' nos pares casados.
//
// A posição e a ordem vêm SEMPRE da store — o artefato só diz qual texto
// novo entra em qual célula, e o que ele trouxer além disso é descartado.
// O hash NUNCA é reescrito: a store é derivada do binário, e os builders
// recalculam o hash na próxima carga.
//
// Idiomas não-us e placeholders vêm integralmente da store: o import só
// contribui com inglês (os demais idiomas do arquivo são descartados).
func mergeImportUs(store dto.Collection, matched map[string][]rowPair) dto.Collection {
	out := make(dto.Collection, len(store))
	for id, se := range store {
		pairs := matched[id]
		if len(pairs) == 0 {
			out[id] = se
			continue
		}
		byStore := make(map[string]dto.TextRow, len(pairs))
		for _, p := range pairs {
			byStore[binaryRowKey(p.store)] = p.imported
		}
		rows := make([]dto.TextRow, len(se.Rows))
		for i, r := range se.Rows {
			nr := r
			if ir, ok := byStore[binaryRowKey(r)]; ok {
				if t := usText(ir); !isBlankText(t) {
					text := make(map[string]string, len(r.Text)+1)
					for k, v := range r.Text {
						text[k] = v
					}
					text[common.DefaultLocalization] = t
					nr.Text = text
				}
				// Parsers infer Divergent when an external editor replaces
				// $hash in a copy with a different literal under the same
				// original hash. Preserve that apply-only intent through the
				// store/import merge.
				nr.Divergent = ir.Divergent
			}
			rows[i] = nr
		}
		out[id] = dto.FileEntry{Metadata: se.Metadata, Rows: rows}
	}
	return out
}

// ---- detecção ----------------------------------------------------------------

// kindFromKey decide o kind pela metadata.key: macrodic.dcp → macro,
// KindFromKey é kindFromKey exposto ao acesso externo (menu/CLI
// arquivo→arquivo): resolve o kind lógico de uma metadata.key canônica.
// "" = entrada fora do escopo do app (lista de skip / padrão desconhecido).
func KindFromKey(key string, version common.GameVersion) string {
	return kindFromKey(key, version)
}

// /event/ → events; /gamedata/ps3data/lockit/ → lockit; help/…/*.sps2 → help;
// demais → objects.
func kindFromKey(key string, version common.GameVersion) string {
	if common.IsSkippedFilePath(key, version) {
		return ""
	}
	lower := strings.ToLower(key)
	if strings.HasSuffix(lower, "macrodic.dcp") {
		return KindMacro
	}
	if strings.Contains(lower, "/event/") {
		return KindEvents
	}
	if lockit.IsLockitKey(key) {
		return KindLockit
	}
	if strings.Contains(lower, "/battle/btl/tuto0000/") {
		return KindTutorial
	}
	if strings.Contains(lower, "/battle/btl/menumain/") {
		return KindMenuMain
	}
	if strings.Contains(lower, "/battle/btl/") {
		return KindBattleText
	}
	if strings.Contains(lower, "/cloudsave/") {
		return KindCloud
	}
	if strings.HasSuffix(lower, "tutorial.msb") {
		return KindTutorial
	}
	if strings.HasSuffix(lower, "/menu/menumain.bin") {
		return KindMenuMain
	}
	if strings.Contains(lower, "/help/") && strings.HasSuffix(lower, ".sps2") {
		return KindHelp
	}
	return KindObjects
}

// detectImportKind infere o kind e valida a versão pelas metadata.key.
// A versão passa por VersionPathName: ffx2 e lastmiss dividem a árvore
// ("ffx2") e ffx e eternalcalm também ("ffx"), então um export de lastmiss
// importa na aba lastmiss (e vice-versa) — idem eternalcalm/ffx, limitado
// pela régua de events de cada aba. Falha aqui é de arquivo inteiro (toast);
// erros por entrada vão no resumo.
func detectImportKind(active common.GameVersion, imported dto.Collection) (string, error) {
	kind := ""
	for _, id := range imported.SortedKeys() {
		key := strings.TrimSpace(imported[id].Metadata.Key)
		if key == "" {
			return "", fmt.Errorf("entrada %q sem metadata.key (arquivo não gerado por este app?)", id)
		}
		p, ok := dto.ParseKey(key)
		if !ok {
			return "", fmt.Errorf("metadata.key inválida na entrada %q: %q", id, key)
		}
		fv, verr := common.ParseGameVersionStrict(p.Version)
		if verr != nil {
			return "", fmt.Errorf("versão desconhecida no arquivo: %q", p.Version)
		}
		if common.VersionPathName(fv) != common.VersionPathName(active) {
			return "", fmt.Errorf("arquivo é da versão %s, mas a aba ativa é %s", p.Version, active.String())
		}
		if common.IsSkippedFilePath(key, active) {
			continue // silencioso: lista estática, nada a fazer
		}
		k := kindFromKey(key, active)
		if k == "" {
			return "", fmt.Errorf("entrada %q sem kind reconhecido", id)
		}
		if kind == "" {
			kind = k
		} else if k != kind {
			return "", fmt.Errorf("arquivo mistura kinds diferentes (%s e %s)", kind, k)
		}
	}
	if kind == "" {
		return "", fmt.Errorf("arquivo sem entradas")
	}
	return kind, nil
}

// importKnownIDs filtra os ids importados que existem estruturalmente nesta
// versão (régua de events, layout de objects, chunks do macro). Devolve também
// o DTO completo de macro (a capacidade mede o arquivo inteiro).
func (s *MetadataService) importKnownIDs(kind string, version common.GameVersion, imported dto.Collection) (map[string]bool, dto.Collection, error) {
	known := make(map[string]bool)
	var macroFull dto.Collection
	switch kind {
	case KindEvents:
		if err := s.ensureEventsLoaded(version); err != nil {
			return nil, nil, err
		}
		visible := make(map[string]bool)
		for _, id := range filterEventIDsForVersion(event.GetAllEventIDs(version), version) {
			visible[id] = true
		}
		for _, id := range imported.SortedKeys() {
			if visible[id] && event.GetEvent(version, id) != nil {
				known[id] = true
			}
		}
	case KindObjects:
		for _, id := range imported.SortedKeys() {
			if _, ok := objectKeyForID(version, id); ok {
				known[id] = true
			}
		}
	case KindLockit:
		for _, id := range imported.SortedKeys() {
			if _, ok := lockit.LayoutForID(version, id); ok {
				known[id] = true
			}
		}
	case KindMacro:
		if version == common.GameVersionLastMiss || version == common.GameVersionEternalCalm {
			// Expansões sem dicionário próprio: store vazia e todas
			// as entradas importadas entram no resumo como desconhecidas.
			return known, nil, nil
		}
		full, err := builders.BuildMacroDTO(version, nil)
		if err != nil {
			return nil, nil, err
		}
		macroFull = full
		for _, id := range imported.SortedKeys() {
			if _, ok := full[id]; ok {
				known[id] = true
			}
		}
	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		for _, id := range imported.SortedKeys() {
			if _, ok := eventtable.RelPath(kind, id); ok && originalExists(kind, id, version) {
				known[id] = true
			}
		}
	case KindHelp:
		if err := ensureHelpLoaded(version); err != nil {
			return nil, nil, err
		}
		for _, id := range imported.SortedKeys() {
			if helpfile.GetHelp(version, id) != nil {
				known[id] = true
			}
		}
	}
	return known, macroFull, nil
}

// loadImportStore carrega a Collection da store só para os ids conhecidos
// (ids vazio significaria "tudo" no GetCollection — hence o guarda).
func (s *MetadataService) loadImportStore(kind string, version common.GameVersion, known map[string]bool, macroFull dto.Collection) (dto.Collection, error) {
	if len(known) == 0 {
		return dto.Collection{}, nil
	}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if kind == KindMacro {
		out := make(dto.Collection, len(ids))
		for _, id := range ids {
			if e, ok := macroFull[id]; ok {
				out[id] = e
			}
		}
		return out, nil
	}
	return s.GetCollection(kind, version, ids, progress.Nop())
}

// ---- capacidade uint16 (pós-rebuild, só leitura) ------------------------------

// importEventsUsage mede o binário do evento em cópias: espelha
// extractFieldStringsForLanguage (pula obj nil, placeholder p/ us nil), aplica
// o 'us' importado nas cópias e SEMPRE roda RebuildFieldStrings — que converte
// tags em bytes e deduplica offsets — só descartando o resultado. A store
// nunca é tocada.
func importEventsUsage(version common.GameVersion, id string, imp dto.FileEntry) (dto.ImportUsage, error) {
	usage := dto.ImportUsage{ID: id, Kind: KindEvents, Limit: importUint16Limit}
	ev := event.GetEvent(version, id)
	if ev == nil {
		return usage, fmt.Errorf("evento não encontrado: %s", id)
	}
	charset := ffxencoding.GetCharsetForLanguage(common.DefaultLocalization)
	importUS := make(map[int]string, len(imp.Rows))
	for _, r := range imp.Rows {
		if r.Name != "" {
			continue // events não têm Name; linha estranha já é bloqueada na validação
		}
		if t := r.Text[common.DefaultLocalization]; t != "" {
			importUS[r.Index] = t
		}
	}
	copies := make([]*event.FieldString, 0, len(ev.Strings))
	for i, obj := range ev.Strings {
		if obj == nil {
			continue
		}
		var c *event.FieldString
		if fs := obj.GetLocalizedContent(common.DefaultLocalization); fs != nil {
			cp := *fs
			c = &cp
		} else {
			c = event.NewEmptyFieldString(charset, version)
		}
		if t, ok := importUS[i]; ok {
			c.SetRegularString(t)
		}
		copies = append(copies, c)
	}
	content := event.RebuildFieldStrings(copies, charset, version)
	header := len(copies) * 8 // first = count*8, precisa caber em uint16
	usage.Used = header + len(content)
	over := header > 65535 || header+len(content) > importUint16Limit
	for _, c := range copies {
		if c.RegularOffset > 65535 || c.SimplifiedOffset > 65535 {
			over = true
			break
		}
	}
	usage.Over = over
	return usage, nil
}

// importEventTableUsage é o importEventsUsage das famílias eventtable (btl,
// cloud/cloudv, tutorial.msb): mesma tabela dos events → mesma régua uint16.
func importEventTableUsage(kind string, version common.GameVersion, id string, imp dto.FileEntry) (dto.ImportUsage, error) {
	usage := dto.ImportUsage{ID: id, Kind: kind, Limit: importUint16Limit}
	f := eventtable.Get(kind, version, id)
	if f == nil {
		return usage, fmt.Errorf("%s: %s não encontrado", kind, id)
	}
	charset := ffxencoding.GetCharsetForLanguage(common.DefaultLocalization)
	importUS := make(map[int]string, len(imp.Rows))
	for _, r := range imp.Rows {
		if r.Name != "" {
			continue
		}
		if t := r.Text[common.DefaultLocalization]; t != "" {
			importUS[r.Index] = t
		}
	}
	copies := make([]*event.FieldString, 0, len(f.Strings))
	for i, obj := range f.Strings {
		if obj == nil {
			continue
		}
		var c *event.FieldString
		if fs := obj.GetLocalizedContent(common.DefaultLocalization); fs != nil {
			cp := *fs
			c = &cp
		} else {
			c = event.NewEmptyFieldString(charset, version)
		}
		if t, ok := importUS[i]; ok {
			c.SetRegularString(t)
		}
		copies = append(copies, c)
	}
	content := event.RebuildFieldStrings(copies, charset, version)
	header := len(copies) * 8
	usage.Used = header + len(content)
	over := header > 65535 || header+len(content) > importUint16Limit
	for _, c := range copies {
		if c.RegularOffset > 65535 || c.SimplifiedOffset > 65535 {
			over = true
			break
		}
	}
	usage.Over = over
	return usage, nil
}

// importObjectsUsage mede a string table do arquivo com o import aplicado:
// o schema chunkmap aplica as rows em memória e devolve o tamanho da tabela
// reconstruída (dedup inclusive). Sem carregar wrappers legados e sem
// gravar arquivo.
func importObjectsUsage(version common.GameVersion, id string, imp dto.FileEntry) (dto.ImportUsage, error) {
	usage := dto.ImportUsage{ID: id, Kind: KindObjects, Limit: importUint16Limit}
	key, ok := objectKeyForID(version, id)
	if !ok {
		return usage, fmt.Errorf("objeto desconhecido: %s", id)
	}
	measure, ok := chunkmap.SchemaMeasurers[key]
	if !ok {
		return usage, fmt.Errorf("objects %s: schema chunkmap não registrado", key)
	}
	layout, ok := objectsfile.FileLayouts[key]
	if !ok {
		return usage, fmt.Errorf("layout não encontrado: %s", key)
	}
	used, merr := measure(version, layout.PatternPath(), imp.Rows)
	if merr != nil {
		return usage, merr
	}
	usage.Used = used
	usage.Over = used > importUint16Limit
	return usage, nil
}

// importMacroUsage reconstrói o dicionário inteiro (todos os chunks, todos os
// idiomas) com o 'us' mesclado e mede o container 'us'. Um uso só para o
// arquivo: o limite é por binário. O layout (header + chunks) não é derivável
// do texto puro, por isso o rebuild real.
//
// O merge é contra macroFull (o dicionário COMPLETO) e não contra a store
// parcial: medir store+store ignorava o texto importado, e a capacidade saía
// igual à de antes do import.
func importMacroUsage(version common.GameVersion, macroFull dto.Collection, matched map[string][]rowPair) (dto.ImportUsage, error) {
	usage := dto.ImportUsage{Kind: KindMacro, Limit: importUint16Limit}
	containers, err := builders.RebuildMacroContainers(version, mergeImportUs(macroFull, matched))
	if err != nil {
		return usage, err
	}
	us := containers[common.DefaultLocalization]
	if us == nil || len(us.Bytes) == 0 {
		return usage, fmt.Errorf("rebuild do dicionário sem conteúdo 'us'")
	}
	usage.Used = len(us.Bytes)
	usage.Over = usage.Used > importUint16Limit
	return usage, nil
}

// skipImportEntry exclui uma entrada do import por problema DELA: o motivo
// vira aviso do resumo (a entrada será ignorada se o usuário confirmar), a
// linha da tabela ganha o erro e a collection mesclada perde a entrada — o
// apply nunca a vê. merged pode ser nil (entrada que nunca chegou à store).
// A granularidade é o ARQUIVO: um erro em qualquer row exclui a entrada
// inteira; o restante do arquivo importa normalmente.
func skipImportEntry(summary *dto.ImportSummary, merged dto.Collection, id, reason string) {
	summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", id, reason))
	for i := range summary.Entries {
		if summary.Entries[i].ID != id {
			continue
		}
		summary.Entries[i].Error = reason
		// Os contadores do resumo refletem só o que vai de fato importar —
		// a entrada ignorada some dos três (entradas, índices e alterados).
		summary.ChangedTexts -= summary.Entries[i].ChangedTexts
		if summary.ChangedTexts < 0 {
			summary.ChangedTexts = 0
		}
		summary.TotalIndices -= summary.Entries[i].IndexCount
		if summary.TotalIndices < 0 {
			summary.TotalIndices = 0
		}
		summary.EntryCount--
		if summary.EntryCount < 0 {
			summary.EntryCount = 0
		}
		break
	}
	if merged != nil {
		delete(merged, id)
	}
}

// ---- pipeline -----------------------------------------------------------------

// prepareImport executa o pipeline SEM aplicar: parse → detecção → store →
// validação por entrada → merge do 'us' → capacidades. Erros de arquivo voltam
// como error (toast); problemas por entrada viram AVISO e excluem só a entrada
// (summary.Warnings); bloqueadores globais ficam em summary.Errors.
func (s *MetadataService) prepareImport(path string, version common.GameVersion) (*importPlan, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("nenhum arquivo selecionado")
	}
	raw, err := common.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("arquivo vazio: %s", filepath.Base(path))
	}

	var imported dto.Collection
	var format string
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".json":
		format = "json"
	case ".strings":
		format = "strings"
	case ".csv", ".xlsx":
		format = strings.TrimPrefix(ext, ".")
	case ".po":
		format = "po"
	default:
		return nil, fmt.Errorf("extensão não suportada: %s (use .json, .strings, .csv, .xlsx ou .po)", ext)
	}
	f, ok := formatters.Get(format)
	if !ok {
		return nil, fmt.Errorf("formatter %s não registrado", format)
	}
	imported, err = f.Unmarshal(raw)
	if err != nil {
		return nil, fmt.Errorf("falha ao analisar o arquivo: %w", err)
	}
	if len(imported) == 0 {
		return nil, fmt.Errorf("arquivo sem entradas: %s", filepath.Base(path))
	}

	kind, err := detectImportKind(version, imported)
	if err != nil {
		return nil, err
	}
	if !artifactHasTarget(imported) {
		return nil, fmt.Errorf("artefato no formato antigo (sem source/target): re-exporte no formato atual")
	}
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}
	known, macroFull, err := s.importKnownIDs(kind, version, imported)
	if err != nil {
		return nil, err
	}
	store, err := s.loadImportStore(kind, version, known, macroFull)
	if err != nil {
		return nil, err
	}

	summary := dto.ImportSummary{
		Path:      path,
		Format:    format,
		Kind:      kind,
		Version:   version.String(),
		Languages: importLanguages(imported),
		// Toda importação confirmada reconstrói e grava os binários em mods/;
		// o aviso no modal deixa o efeito explícito antes de aplicar.
		SavesBinary: true,
		Entries:     make([]dto.ImportEntryInfo, 0, len(imported)),
		Usages:      make([]dto.ImportUsage, 0, len(known)),
		Warnings:    []string{},
		Skipped:     make([]dto.ImportRowIssue, 0),
		Errors:      []string{},
	}

	valid := make(dto.Collection, len(known))
	matched := make(map[string][]rowPair, len(known))
	var totalChanged, totalIndices int
	for _, id := range imported.SortedKeys() {
		imp := imported[id]
		info := dto.ImportEntryInfo{
			ID:         id,
			Key:        imp.Metadata.Key,
			IndexCount: rowIndexCount(imp.Rows),
		}
		totalIndices += info.IndexCount

		se, ok := store[id]
		if !ok {
			// Problema de uma entrada só: vira aviso, a entrada é ignorada
			// e o resto do arquivo importa.
			summary.Entries = append(summary.Entries, info)
			skipImportEntry(&summary, nil, id, "entrada não existe nesta versão")
			continue
		}
		info.StoreIndexCount = rowIndexCount(se.Rows)

		// Referência de origem/tags = data/ (ou o .vbf ativo). Nil dentro de
		// matchImportEntry pula as duas checagens (data/ incompleto).
		m := matchImportEntry(imp, se, s.originalEntryFor(kind, id, version))
		info.ChangedTexts = m.changed
		totalChanged += m.changed

		// Origem não encontrada e tag divergente: LogWarning mantém o texto
		// COMPLETO no log; Skipped espelha a row no resumo do modal (texto
		// truncado) para o revisor ver o que ficou fora sem abrir o log.
		skippedRows := 0
		for _, r := range m.unmatched {
			common.LogWarning(logImportUnmatched, id, sourceHash(r), importText(r))
			skippedRows++
			summary.Skipped = append(summary.Skipped, dto.ImportRowIssue{
				ID:     id,
				Index:  r.Index,
				Hash:   sourceHash(r),
				Text:   truncateRowIssue(importText(r)),
				Reason: reasonUnmatched,
			})
		}
		for _, v := range m.tagLost {
			common.LogWarning(logImportTagLost, id, sourceHash(v.row), importText(v.row), v.issues)
			skippedRows++
			summary.Skipped = append(summary.Skipped, dto.ImportRowIssue{
				ID:     id,
				Index:  v.row.Index,
				Hash:   sourceHash(v.row),
				Text:   truncateRowIssue(importText(v.row)),
				Reason: reasonTagLost + fmt.Sprint(v.issues),
			})
		}
		info.SkippedRows = skippedRows
		common.LogInfo(logImportInserted, id, m.inserted, m.candidates)

		valid[id] = se
		matched[id] = m.pairs
		summary.Entries = append(summary.Entries, info)
	}
	summary.EntryCount = len(imported)
	summary.TotalIndices = totalIndices
	summary.ChangedTexts = totalChanged

	merged := mergeImportUs(valid, matched)

	// Capacidades sobre as entradas que serão gravadas: problema de uma
	// entrada a exclui (aviso — o resto importa); o estouro do dicionário
	// macro é global (binário único compartilhado) e bloqueia a importação.
	switch kind {
	case KindMacro:
		if len(matched) > 0 && macroFull != nil {
			usage, uerr := importMacroUsage(version, macroFull, matched)
			if uerr != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("dicionário: %s", uerr.Error()))
			} else {
				summary.Usages = append(summary.Usages, usage)
				if usage.Over {
					summary.Errors = append(summary.Errors, fmt.Sprintf(
						"dicionário: texto excede o limite binário (%d > %d bytes)",
						usage.Used, usage.Limit))
				}
			}
		}
	case KindLockit, KindHelp:
		// O lockit é uma lista CRLF sem offsets uint16 nem cabeçalho; não há
		// limite de capacidade a medir (o import grava apenas o us).
		// O help tem ponteiros u32 com textEnd/footer recalculados no
		// rebuild — nenhum limite uint16 a medir; a validação estrutural
		// acontece no próprio rebuild (reader revalida ao aplicar).
	default:
		for _, id := range merged.SortedKeys() {
			var usage dto.ImportUsage
			var uerr error
			switch {
			case kind == KindEvents:
				usage, uerr = importEventsUsage(version, id, merged[id])
			case kind == KindBattleText || kind == KindCloud || kind == KindTutorial || kind == KindMenuMain:
				usage, uerr = importEventTableUsage(kind, version, id, merged[id])
			default:
				usage, uerr = importObjectsUsage(version, id, merged[id])
			}
			if uerr != nil {
				skipImportEntry(&summary, merged, id, uerr.Error())
				continue
			}
			if usage.Over {
				skipImportEntry(&summary, merged, id, fmt.Sprintf(
					"texto excede o limite binário (%d > %d bytes)", usage.Used, usage.Limit))
				continue
			}
			summary.Usages = append(summary.Usages, usage)
		}
	}

	return &importPlan{kind: kind, merged: merged, summary: summary}, nil
}

// PreviewImport parseia e valida o arquivo (sem aplicar) e devolve o resumo
// para o modal de confirmação.
func (s *MetadataService) PreviewImport(path string, version common.GameVersion) (dto.ImportSummary, error) {
	plan, err := s.prepareImport(path, version)
	if err != nil {
		return dto.ImportSummary{}, err
	}
	return plan.summary, nil
}

// ImportFile reexecuta o pipeline e aplica o 'us' mesclado no binário.
// Entradas com problema ficam fora do merge (avisos do resumo) — só um
// bloqueador GLOBAL (Errors não vazio, como o estouro do dicionário macro)
// aborta a importação inteira. Devolve quantos textos em inglês mudaram em
// relação à store.
//
// O uilog narra o fluxo para a janela de logs DEPOIS da ação, e a
// granularidade é POR BINÁRIO: o gravador de gravações de common informa
// quais binários foram recompilados e em que destino de mods/ (ver
// import_log.go) — nenhum binário escrito ou falhado fica de fora do log.
func (s *MetadataService) ImportFile(path string, version common.GameVersion) (int, error) {
	plan, err := s.prepareImport(path, version)
	if err != nil {
		uilog.Error("Import", "%s: %v", filepath.Base(path), err)
		return 0, err
	}
	if len(plan.summary.Errors) > 0 {
		err := fmt.Errorf("importação bloqueada: %v", plan.summary.Errors)
		uilog.Error("Import", "%s: %v", filepath.Base(path), err)
		return 0, err
	}
	if len(plan.merged) == 0 {
		uilog.Warn("Import", "%s: nada a importar", filepath.Base(path))
		return 0, fmt.Errorf("nada a importar")
	}

	// Detectar formato do artefato para o log (json/strings)
	format := detectImportFormat(path)

	// Linha de início + linha por arquivo (padrão export)
	uilog.Info("Import", "Iniciando import de 1 arquivo de um %s", format)
	uilog.Info("Import", "%s: importando", filepath.Base(path))

	sum, err := applyRecorded(version, "Import", "importado de um "+format, func() error {
		return s.ApplyTextCollection(plan.kind, version, plan.merged)
	})
	if err != nil {
		uilog.Error("Import", "%s: falha ao aplicar: %v", filepath.Base(path), err)
		return 0, err
	}
	if sum.Written == 0 {
		uilog.Success("Import", "%s — %d texto(s), nenhum binário alterado (nada gravado)",
			filepath.Base(path), plan.summary.ChangedTexts)
		return plan.summary.ChangedTexts, nil
	}
	uilog.Success("Import", "%s — %d texto(s) em %d binário(s) gravado(s) em mods/",
		filepath.Base(path), plan.summary.ChangedTexts, sum.Written)
	return plan.summary.ChangedTexts, nil
}

// detectImportFormat extrai o formato do artefato pela extensão.
func detectImportFormat(path string) string {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".strings":
		return "strings"
	case ".po":
		return "po"
	}
	return "json" // .json, .csv, .xlsx → json (formato base)
}

// ExportJSON dobrou em ExportArtifactBatch (export_formats.go): a
// extração compartilhada com strings/csv/xlsx, um step por kind.

// CountChangedTexts devolve quantas rows têm 'us' diferente entre import e
// store (utilitário de conferência para o frontend). Sem kind/version não há
// como achar o original em data/, então a contagem é só por hash — a
// validação de tags acontece em prepareImport, que alimenta o resumo.
func (s *MetadataService) CountChangedTexts(imported, store dto.Collection) int {
	total := 0
	for _, id := range imported.SortedKeys() {
		se, ok := store[id]
		if !ok {
			continue
		}
		total += matchImportEntry(imported[id], se, nil).changed
	}
	return total
}
