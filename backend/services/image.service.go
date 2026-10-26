package services

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/ddsphyre"
	"ffxresources/backend/uilog"
)

// IMAGENS (.dds.phyre)
//
// O kind KindImages não é texto: não tem rows, não participa de
// ExportStrings/ExportJSON/ImportFile e não entra no preload de coleções.
// A leitura é só o scan da árvore + um decode por textura, então cada
// operação é independente e cacheless — o "store" do frontend guarda um
// único ImageEntry por vez (a textura visível).
//
// A imagem servida vem SEMPRE nesta ordem de preferência (determinada pelo
// pacote ddsphyre.Resolve, não aqui):
//
//	1. .dds em disco (mods/images/... — extração anterior);
//	2. .png em disco (idem);
//	3. decode do próprio .dds.phyre EM MEMÓRIA (nada é gravado).
//
// Com o config EnableMods desligado a árvore mods/ fica invisível para a
// leitura (o mesmo toggle do texto): container direto da fonte original
// (data/ ou .vbf), artefatos de mods/images ignorados — sempre o ramo 3.
//
// Exportar (.dds + .png) é uma ação separada (ExtractImage /
// ExtractImageGroup) — abrir a textura não suja mods/.

// GetImage carrega a textura e devolve a pré-visualização pronta (data URL
// PNG) junto dos metadados do container e do DDS da fonte escolhida.
func (s *MetadataService) GetImage(kind, id string, version common.GameVersion) (dto.ImageEntry, error) {
	return s.getImage(kind, id, version, common.SourceData, true)
}

// GetImageFrom é GetImage com a fonte do ORIGINAL explícita — SourceVbf
// quando a textura está sendo aberta a partir da raiz do .vbf. Nesse caso o
// índice de duplicatas NÃO é anexado: ele é montado varrendo a árvore data/,
// que é justamente a que pode não existir por completo.
func (s *MetadataService) GetImageFrom(kind, id string, version common.GameVersion, originalSrc common.FileSource) (dto.ImageEntry, error) {
	return s.getImage(kind, id, version, originalSrc, false)
}

func (s *MetadataService) getImage(kind, id string, version common.GameVersion, originalSrc common.FileSource, withDupes bool) (dto.ImageEntry, error) {
	if !isImageKind(kind) {
		return dto.ImageEntry{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.ImageEntry{}, err
	}
	if !ddsphyre.ValidID(id) {
		return dto.ImageEntry{}, fmt.Errorf("textura desconhecida: %s", id)
	}
	r, err := ddsphyre.ResolveFrom(version, id, originalSrc)
	if err != nil {
		return dto.ImageEntry{}, err
	}
	entry := dto.ImageEntry{
		Metadata:       dto.NewImageMetadata(id, version),
		Source:         r.Source,
		Format:         r.Format,
		Width:          r.Width,
		Height:         r.Height,
		MipmapCount:    r.MipmapCount,
		MaxMipmapLevel: r.MaxMipmapLvl,
		AssetPath:      r.AssetPath,
		PNGData:        dataURL("image/png", r.PNG),
		Flipped:        true,
		DDSData:        dataURL("application/octet-stream", r.DDS),
		DDSPath:        r.DDSPath,
		PNGPath:        r.PNGPath,
		Modded:         r.InMods,
	}
	if withDupes {
		s.fillDuplicates(&entry, id, version)
	}
	return entry, nil
}

// fillDuplicates anexa o grupo de cópias de mesma imagem (payload original
// igual — as cópias da otimização do DVD, pareadas em data/ e sempre dentro
// da mesma versão).
//
// O índice é memoizado no pacote ddsphyre; a primeira consulta da versão
// varre a árvore (~1-2 s) e as seguintes são O(1). Falha aqui não derruba a
// textura: duplicatas são informação suplementar, loga e segue.
func (s *MetadataService) fillDuplicates(entry *dto.ImageEntry, id string, version common.GameVersion) {
	entry.Duplicates = []dto.ImageDuplicate{}
	dupes, err := duplicatesFor(id, version)
	if err != nil {
		common.LogWarning("images %s: índice de duplicatas indisponível: %v", version, err)
		return
	}
	entry.Duplicates = dupes.Duplicates
	entry.DupPayload = dupes.DupPayload
}

// duplicatesFor devolve as cópias da textura SEM decodificar imagem alguma
// (só o índice memoizado) — é o que os diálogos de extrair/replicar/deletar
// e o menu de contexto usam para montar os alvos.
func duplicatesFor(id string, version common.GameVersion) (dto.ImageDuplicates, error) {
	ix, err := ddsphyre.IndexFor(version)
	if err != nil {
		return dto.ImageDuplicates{Duplicates: []dto.ImageDuplicate{}}, err
	}
	copies, payload := ix.Copies(id)
	out := dto.ImageDuplicates{
		Duplicates: make([]dto.ImageDuplicate, 0, len(copies)),
		DupPayload: payload,
	}
	for _, c := range copies {
		out.Duplicates = append(out.Duplicates, dto.ImageDuplicate{
			ID:        c.ID,
			Key:       dto.NewImageMetadata(c.ID, version).Key,
			Modded:    c.InMods,
			Identical: c.Identical,
		})
	}
	return out, nil
}

// ImageDuplicates é o binding leve das cópias: idem fillDuplicates, mas sem
// carregar a imagem. Existe porque o menu de contexto age sobre o nó
// clicado, que pode NÃO ser a entry selecionada.
func (s *MetadataService) ImageDuplicates(kind, id string, version common.GameVersion) (dto.ImageDuplicates, error) {
	if !isImageKind(kind) {
		return dto.ImageDuplicates{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.ImageDuplicates{}, err
	}
	if !ddsphyre.ValidID(id) {
		return dto.ImageDuplicates{}, fmt.Errorf("textura desconhecida: %s", id)
	}
	out, err := duplicatesFor(id, version)
	if err != nil {
		return dto.ImageDuplicates{}, fmt.Errorf("índice de duplicatas: %w", err)
	}
	return out, nil
}

// hideImageDuplicates devolve só os REPRESENTANTES de cada grupo de cópias
// — a árvore de imagens não enche de N linhas iguais: um grupo = uma linha,
// e a lista "Repetidas" do painel é quem fala das cópias (e da propagação).
//
// Representante = 1º em ordem alfabética do grupo que esteja visível (o
// índice pareia só data/, então grupo ⊆ árvore regra 4; o laço só se
// protege de milagre fora disso). Id fora do índice passa direto — e se o
// índice nem existir, a árvore sai COMPLETA com um WARN: nunca vazia.
func hideImageDuplicates(entries []EntrySummary, version common.GameVersion) []EntrySummary {
	ix, err := ddsphyre.IndexFor(version)
	if err != nil {
		common.LogWarning(
			"images %s: índice de duplicatas indisponível — árvore exibindo TODAS as cópias: %v",
			version, err,
		)
		return entries
	}
	visible := make(map[string]bool, len(entries))
	for _, e := range entries {
		visible[e.ID] = true
	}
	out := make([]EntrySummary, 0, len(entries))
	for _, e := range entries {
		leader := e.ID
		for _, g := range ix.OriginalGroup(e.ID) {
			if visible[g] {
				// OriginalGroup devolve o grupo ordenado: o primeiro visível
				// É o representante (1º alfabético do grupo).
				leader = g
				break
			}
		}
		if e.ID == leader {
			out = append(out, e)
		}
	}
	return out
}

// batchTargets monta a lista final de um lote e valida o grupo ANTES de
// qualquer gravação: id primeiro, sem repetições, e todo alvo precisa ser
// cópia de mesmo payload ORIGINAL (pareado em data/) de id.
//
// Id fora do índice ⇒ lote restrito ao próprio id: sem grupo confiável não
// há cópias para tocar (modo fallback, data/ vazio — o seguro é não
// propagar nada), e o log denuncia.
func batchTargets(id string, targets []string, version common.GameVersion) ([]string, error) {
	ix, err := ddsphyre.IndexFor(version)
	if err != nil {
		return nil, fmt.Errorf("índice de duplicatas: %w", err)
	}
	group := ix.OriginalGroup(id)
	if len(group) == 0 {
		common.LogWarning(
			"images %s/%s: fora do índice de duplicatas (sem data/ ou payload ilegível) — lote restrito ao próprio id",
			version, id,
		)
		group = []string{id}
	}
	return groupTargets(id, group, targets)
}

// ExtractImage grava .dds e .png em mods/images (a cópia de trabalho
// que o translator edita) e devolve os caminhos escritos.
func (s *MetadataService) ExtractImage(kind, id string, version common.GameVersion) ([]string, error) {
	if !isImageKind(kind) {
		return nil, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}
	if !ddsphyre.ValidID(id) {
		return nil, fmt.Errorf("textura desconhecida: %s", id)
	}

	uilog.Info("Export", "Iniciando extração de 1 imagem(ns) de data/")

	paths, err := ddsphyre.Extract(version, id)
	if err != nil {
		return nil, err
	}

	for _, p := range paths {
		if strings.HasSuffix(strings.ToLower(p), ".dds") {
			uilog.File("Export", p, uilog.StatusOk, "extraído como dds")
		} else if strings.HasSuffix(strings.ToLower(p), ".png") {
			uilog.File("Export", p, uilog.StatusOk, "extraído como png")
		}
	}

	// Atualiza o view (fonte passa a ser o .dds em disco).
	if s != nil && s.notifier != nil {
		s.notifier.NotifyInfo(fmt.Sprintf("Imagem exportada: ddsphyre/%s", id))
	}
	return paths, nil
}

// ImportImage reempacota o .dds escolhido sobre o container pristine e grava
// o resultado em mods/ (mesmo formato, mesma dimensão — realocar fixups é
// iteração futura, ver o TODO do pacote ddsphyre).
func (s *MetadataService) ImportImage(kind, id, ddsPath string, version common.GameVersion) error {
	if !isImageKind(kind) {
		return fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return err
	}
	if !ddsphyre.ValidID(id) {
		return fmt.Errorf("textura desconhecida: %s", id)
	}

	uilog.Info("Import", "%s: importando", id)

	if err := ddsphyre.Import(version, id, ddsPath); err != nil {
		return err
	}
	// O payload efetivo desta textura mudou: o índice de duplicatas em
	// cache está desatualizado (as cópias deixaram de ser idênticas).
	ddsphyre.InvalidateIndex(version)
	// Mantém os artefatos de trabalho em sincronia com o .dds.phyre recém-
	// gravado em mods/: sem isso a próxima abertura serviria o .dds
	// extraído ANTIGO (Resolve prefere .dds em disco), mascarando a textura
	// importada. ExtractEffective é IMUNE ao toggle EnableMods: um refresh
	// comum, com mods desligados, gravaria artefato pristine sobre a
	// importação. Nada disso toca no .vbf nem em data/ — só mods/.
	if _, err := ddsphyre.ExtractEffective(version, id); err != nil {
		common.LogWarning("images %s/%s: import ok, mas não atualizei os .dds/.png extraídos: %v", version, id, err)
	}

	// Log the written path in mods/
	modsPath := ddsphyre.RelPath(version, id)
	uilog.File("Import", modsPath, uilog.StatusOk, "importado de um dds")
	return nil
}

// ImportImageGroup reempacota o MESMO .dds sobre vários containers de uma
// vez — a propagação para as cópias idênticas (otimização do DVD): edita-se
// uma textura e as N iguais voltam a ficar sincronizadas.
//
// A validação é ANTES de qualquer gravação: todos os alvos precisam estar
// no grupo de mesmo payload ORIGINAL de id (alvo de outro grupo é recusado
// com erro, nada é escrito). Falhas individuais entram no resultado sem
// abortar o lote.
func (s *MetadataService) ImportImageGroup(kind, id, ddsPath string, targets []string, version common.GameVersion) (dto.ImageImportResult, error) {
	if !isImageKind(kind) {
		return dto.ImageImportResult{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.ImageImportResult{}, err
	}
	if !ddsphyre.ValidID(id) {
		return dto.ImageImportResult{}, fmt.Errorf("textura desconhecida: %s", id)
	}
	list, err := batchTargets(id, targets, version)
	if err != nil {
		return dto.ImageImportResult{}, err
	}

	res := dto.ImageImportResult{
		Updated: []string{},
		Failed:  []string{},
		Total:   len(list),
	}
	for _, t := range list {
		if ierr := ddsphyre.Import(version, t, ddsPath); ierr != nil {
			res.Failed = append(res.Failed, t+": "+ierr.Error())
			continue
		}
		// ExtractEffective: refresh imune ao toggle (ver ImportImage).
		if _, eerr := ddsphyre.ExtractEffective(version, t); eerr != nil {
			common.LogWarning("images %s/%s: import ok, mas não atualizei os .dds/.png extraídos: %v", version, t, eerr)
		}
		res.Updated = append(res.Updated, t)
	}
	if len(res.Updated) > 0 {
		// Vários payloads efetivos mudaram: o índice é reconstruído na
		// próxima consulta.
		ddsphyre.InvalidateIndex(version)
	}
	// O aviso é do frontend ("Importadas N de M texturas.", com a lista de
	// falhas na descrição) — ver reportBatch.
	return res, nil
}

// ExtractImageGroup extrai .dds + .png de uma textura e das cópias
// escolhidas — o diálogo de extração pergunta "só esta ou todas as cópias",
// e o resultado sai com os arquivos lado a lado em mods/images (é o
// que confirma a duplicata, olhando os dois arquivos).
//
// Extração só grava artefatos derivados: não muda payload algum, então o
// índice de duplicatas continua válido (sem invalidação).
func (s *MetadataService) ExtractImageGroup(kind, id string, targets []string, version common.GameVersion) (dto.BatchResult, error) {
	if !isImageKind(kind) {
		return dto.BatchResult{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.BatchResult{}, err
	}
	if !ddsphyre.ValidID(id) {
		return dto.BatchResult{}, fmt.Errorf("textura desconhecida: %s", id)
	}
	list, err := batchTargets(id, targets, version)
	if err != nil {
		return dto.BatchResult{}, err
	}
	res := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(list)}
	prog, step := s.progressStep(fmt.Sprintf("Extraindo %d textura(s)", len(list)), len(list))
	defer prog.End()
	for _, t := range list {
		if _, eerr := ddsphyre.Extract(version, t); eerr != nil {
			res.Failed = append(res.Failed, t+": "+eerr.Error())
			step.Issue(t, eerr.Error())
			continue
		}
		res.Done = append(res.Done, t)
		step.Step(t)
	}
	// O aviso é do frontend (reportBatch → "N texturas extraídas.").
	return res, nil
}

// ExtractImageSelection extrai somente os ids explicitamente marcados na
// árvore. Ao contrário de ExtractImageGroup, não expande duplicatas.
func (s *MetadataService) ExtractImageSelection(kind string, ids []string, version common.GameVersion) (dto.BatchResult, error) {
	if !isImageKind(kind) {
		return dto.BatchResult{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.BatchResult{}, err
	}
	list, err := uniqueImageSelection(ids)
	if err != nil {
		return dto.BatchResult{}, err
	}
	if len(list) == 0 {
		return dto.BatchResult{}, fmt.Errorf("nenhuma imagem selecionada")
	}

	res := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(list)}
	prog, step := s.progressStep(fmt.Sprintf("Extraindo %d imagem(ns)", len(list)), len(list))
	defer prog.End()
	for _, id := range list {
		if _, err := ddsphyre.Extract(version, id); err != nil {
			res.Failed = append(res.Failed, id+": "+err.Error())
			step.Issue(id, err.Error())
			continue
		}
		res.Done = append(res.Done, id)
		step.Step(id)
	}
	return res, nil
}

// ReplicateImage reempacota a IMAGEM ABERTA em mods/ de cada cópia do grupo
// — o "dupe" que o app faz sozinho, sem diálogo de arquivo: a fonte é o
// próprio conteúdo servido ao painel, seguindo o config EnableMods (com
// mods desligados replica o pristine visível, como o texto).
//
// A origem NÃO é tocada: ela já mostra essa imagem (data/ ou mods/), só as
// cópias recebem o arquivo — por isso Total = cópias, não o grupo inteiro.
func (s *MetadataService) ReplicateImage(kind, id string, targets []string, version common.GameVersion) (dto.BatchResult, error) {
	if !isImageKind(kind) {
		return dto.BatchResult{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.BatchResult{}, err
	}
	if !ddsphyre.ValidID(id) {
		return dto.BatchResult{}, fmt.Errorf("textura desconhecida: %s", id)
	}
	list, err := batchTargets(id, targets, version)
	if err != nil {
		return dto.BatchResult{}, err
	}
	// list[0] é sempre a origem (groupTargets a põe na frente) — replicar
	// é só as cópias.
	if len(list) < 2 {
		return dto.BatchResult{}, fmt.Errorf("textura %s não tem cópias para replicar", id)
	}
	res := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(list) - 1}
	// Resolve segue o config EnableMods: a origem carrega a VERSÃO VISÍVEL
	// (importada com mods ligados, pristine com mods desligados).
	// Pré-requisito do repack é o container pristine de data/, exigido por
	// ImportPayload — nesse caso já vem embutido no erro.
	r, err := ddsphyre.Resolve(version, id)
	if err != nil {
		return dto.BatchResult{}, fmt.Errorf("lendo imagem %s: %w", id, err)
	}
	dds := r.DDS
	if dds == nil {
		if r.Texture == nil {
			return dto.BatchResult{}, fmt.Errorf("textura %s: sem .dds para replicar", id)
		}
		if dds, err = r.Texture.ExtractToDDS(); err != nil {
			return dto.BatchResult{}, fmt.Errorf("textura %s: %w", id, err)
		}
	}

	for _, t := range list[1:] {
		if ierr := ddsphyre.ImportPayload(version, t, dds); ierr != nil {
			res.Failed = append(res.Failed, t+": "+ierr.Error())
			continue
		}
		// Mesmo cuidado do import: sem extrair agora, a próxima abertura
		// serviria o .dds órfão ANTIGO de mods/images. ExtractEffective é
		// imune ao toggle — o refresh espelha o container recém-gravado.
		if _, eerr := ddsphyre.ExtractEffective(version, t); eerr != nil {
			common.LogWarning("images %s/%s: replicate ok, mas não atualizei os .dds/.png extraídos: %v", version, t, eerr)
		}
		res.Done = append(res.Done, t)
	}
	if len(res.Done) > 0 {
		ddsphyre.InvalidateIndex(version)
	}
	// O aviso é do frontend (reportBatch → "N texturas replicadas.").
	return res, nil
}

// DeleteImages apaga os containers da textura e das cópias confirmadas no
// escopo escolhido (data|mods|both) e SEMPRE os artefatos derivados — sem
// isso Resolve passaria a servir um .dds órfão.
//
// Nunca é cascata implícita: só o que o frontend confirmou (cópias entram
// por targets, marcadas no diálogo).
func (s *MetadataService) DeleteImages(kind, id string, targets []string, scope string, version common.GameVersion) (dto.BatchResult, error) {
	if !isImageKind(kind) {
		return dto.BatchResult{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.BatchResult{}, err
	}
	if !ddsphyre.ValidID(id) {
		return dto.BatchResult{}, fmt.Errorf("textura desconhecida: %s", id)
	}
	switch scope {
	case ddsphyre.DeleteData, ddsphyre.DeleteMods, ddsphyre.DeleteBoth:
	default:
		return dto.BatchResult{}, fmt.Errorf("escopo %q inválido (esperado data, mods ou both)", scope)
	}
	list, err := batchTargets(id, targets, version)
	if err != nil {
		return dto.BatchResult{}, err
	}
	res := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(list)}
	for _, t := range list {
		_, derr := ddsphyre.Delete(version, t, scope)
		if derr != nil {
			res.Failed = append(res.Failed, t+": "+derr.Error())
			continue
		}
		// "Já não existia neste escopo" também é estado desejado: conta
		// como concluído (sem alarmar o usuário com falso negativo).
		res.Done = append(res.Done, t)
	}
	if len(res.Done) > 0 {
		// Apagar em data/ muda quem tem original: o índice e a própria
		// árvore passam a enxergar grupos diferentes.
		ddsphyre.InvalidateIndex(version)
	}
	// O aviso é do frontend (reportBatch): ele distingue sucesso de falha
	// parcial e lista os erros. NotifyInfo aqui emitia um SEGUNDO toast
	// ("Imagem(ns) apagada(s)… escopo …") sobre o mesmo resultado.
	return res, nil
}

// DeleteImageSelection aplica o mesmo escopo e a mesma opção de cópias a
// cada imagem marcada. Cópias sobrepostas são deduplicadas antes de qualquer
// remoção; sem withCopies, apenas os ids marcados são apagados.
func (s *MetadataService) DeleteImageSelection(kind string, ids []string, withCopies bool, scope string, version common.GameVersion) (dto.BatchResult, error) {
	if !isImageKind(kind) {
		return dto.BatchResult{}, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return dto.BatchResult{}, err
	}
	switch scope {
	case ddsphyre.DeleteData, ddsphyre.DeleteMods, ddsphyre.DeleteBoth:
	default:
		return dto.BatchResult{}, fmt.Errorf("escopo %q inválido (esperado data, mods ou both)", scope)
	}
	selected, err := uniqueImageSelection(ids)
	if err != nil {
		return dto.BatchResult{}, err
	}
	if len(selected) == 0 {
		return dto.BatchResult{}, fmt.Errorf("nenhuma imagem selecionada")
	}
	list := selected
	if withCopies {
		index, err := ddsphyre.IndexFor(version)
		if err != nil {
			return dto.BatchResult{}, fmt.Errorf("índice de duplicatas: %w", err)
		}
		seen := make(map[string]struct{}, len(selected))
		list = make([]string, 0, len(selected))
		for _, id := range selected {
			group := index.OriginalGroup(id)
			if len(group) == 0 {
				group = []string{id}
			}
			for _, member := range group {
				if _, exists := seen[member]; exists {
					continue
				}
				seen[member] = struct{}{}
				list = append(list, member)
			}
		}
	}

	res := dto.BatchResult{Done: []string{}, Failed: []string{}, Total: len(list)}
	for _, id := range list {
		if _, err := ddsphyre.Delete(version, id, scope); err != nil {
			res.Failed = append(res.Failed, id+": "+err.Error())
			continue
		}
		res.Done = append(res.Done, id)
	}
	if len(res.Done) > 0 {
		ddsphyre.InvalidateIndex(version)
	}
	// Um só toast por delete: quem reporta é o frontend (reportBatch),
	// como no delete de textura única — ver DeleteImages.
	return res, nil
}

// ImageSelectionCopies devolve as cópias adicionais únicas dos ids marcados,
// sem carregar imagens, para o modal mostrar a contagem antes da confirmação.
func (s *MetadataService) ImageSelectionCopies(kind string, ids []string, version common.GameVersion) ([]string, error) {
	if !isImageKind(kind) {
		return nil, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return nil, err
	}
	selected, err := uniqueImageSelection(ids)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return []string{}, nil
	}
	index, err := ddsphyre.IndexFor(version)
	if err != nil {
		return nil, fmt.Errorf("índice de duplicatas: %w", err)
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	copies := make(map[string]struct{})
	for _, id := range selected {
		for _, copyID := range index.OriginalGroup(id) {
			if _, included := selectedSet[copyID]; !included {
				copies[copyID] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(copies))
	for id := range copies {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func uniqueImageSelection(ids []string) ([]string, error) {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if !ddsphyre.ValidID(id) {
			return nil, fmt.Errorf("textura desconhecida: %s", id)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

// RefreshImageDuplicates descarta o índice de duplicatas em cache — a
// próxima consulta de textura reconstrói lendo a árvore.
//
// Existe por causa de edição EXTERNA (hex editor): o cache só detecta
// mudança que passa por aqui (import).
func (s *MetadataService) RefreshImageDuplicates(kind string, version common.GameVersion) error {
	if !isImageKind(kind) {
		return fmt.Errorf("kind desconhecido: %s", kind)
	}
	ddsphyre.InvalidateIndex(version)
	return nil
}

// ImageExists responde se a textura ainda existe (data/ OU mods/). O frontend
// usa na revalidação da seleção: id apagado NÃO deve ser re-carregado — o
// Resolve erroaria "não encontrada em data/ nem em mods/" para uma imagem
// que o delete acabou de remover com sucesso (erro confuso).
func (s *MetadataService) ImageExists(kind, id string, version common.GameVersion) (bool, error) {
	if !isImageKind(kind) {
		return false, fmt.Errorf("kind desconhecido: %s", kind)
	}
	if !ddsphyre.ValidID(id) {
		return false, nil
	}
	inData, inMods := ddsphyre.Exists(version, id)
	return inData || inMods, nil
}

// groupTargets monta a lista final do lote e valida o grupo ANTES de
// qualquer gravação: id primeiro, sem repetições, preservando a ordem
// escolhida pelo frontend — e recusando qualquer alvo que não seja cópia de
// mesmo payload original de id (grupo vindo do índice de duplicatas).
func groupTargets(id string, group []string, targets []string) ([]string, error) {
	inGroup := make(map[string]bool, len(group))
	for _, g := range group {
		inGroup[g] = true
	}
	if !inGroup[id] {
		return nil, fmt.Errorf("textura %s fora do seu próprio grupo", id)
	}
	seen := map[string]bool{id: true}
	out := []string{id}
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		if !inGroup[t] {
			return nil, fmt.Errorf(
				"importação em lote recusada: %s não é cópia idêntica de %s", t, id)
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, nil
}

// SaveImage grava .dds ou .png no caminho escolhido pelo usuário, sem
// obrigar a ter extraído antes (é o "salvar em disco" do painel).
func (s *MetadataService) SaveImage(kind, id, format, destPath string, version common.GameVersion) error {
	if !isImageKind(kind) {
		return fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return err
	}
	if !ddsphyre.ValidID(id) {
		return fmt.Errorf("textura desconhecida: %s", id)
	}

	formatLower := strings.ToLower(strings.TrimSpace(format))
	uilog.Info("Export", "%s: exportando", id)

	// Trava de gravação: export escolhido pelo usuário, mas nunca por cima
	// do .vbf (fonte somente leitura) nem em data/ dentro da árvore.
	if err := common.CheckWritablePath(destPath); err != nil {
		return err
	}
	if err := ddsphyre.Save(version, id, format, destPath); err != nil {
		return err
	}

	uilog.File("Export", destPath, uilog.StatusOk, fmt.Sprintf("exportado como %s", formatLower))
	return nil
}

// ImageWorkDir devolve o diretório de trabalho da textura em
// mods/images/<raiz>/<caminho interno> — o mesmo onde ExportPaths grava os
// .dds/.png extraídos. É por onde os diálogos nativos de ARQUIVO devem
// abrir (escolher .dds para importar / Salvar como), para a imagem nunca
// cair solta na raiz de mods/images.
//
// Cria o diretório se faltar (mods/ é gravável); id inválido cai na raiz de
// imagens.
func ImageWorkDir(version common.GameVersion, id string) string {
	root := filepath.Join(common.GameFilesRoot, common.ModsFolder, common.ModsImagesDir)
	if !ddsphyre.ValidID(id) {
		_ = common.EnsurePathExists(root)
		return root
	}
	ddsPath, _ := ddsphyre.ExportPaths(version, id)
	dir := filepath.Dir(ddsPath)
	if err := common.EnsurePathExists(dir); err != nil {
		common.LogWarning("images %s/%s: destino %s não pôde ser criado (%v)", version, id, dir, err)
		return root
	}
	return dir
}

// ImageExportPath devolve o caminho padrão de exportação (.dds ou .png) em
// mods/images/<raiz>/<id> para o formato pedido. É o mesmo caminho que o
// Extract usa, mas só para UM formato — usado pelo "Exportar" direto sem diálogo.
func (s *MetadataService) ImageExportPath(kind, id, format string, version common.GameVersion) (string, error) {
	if !isImageKind(kind) {
		return "", fmt.Errorf("kind desconhecido: %s", kind)
	}
	if err := ensureVersionReady(version); err != nil {
		return "", err
	}
	if !ddsphyre.ValidID(id) {
		return "", fmt.Errorf("textura desconhecida: %s", id)
	}
	formatLower := strings.ToLower(strings.TrimSpace(format))
	if formatLower != "dds" && formatLower != "png" {
		return "", fmt.Errorf("formato %q inválido (esperado dds ou png)", format)
	}
	ddsPath, pngPath := ddsphyre.ExportPaths(version, id)
	if formatLower == "dds" {
		return ddsPath, nil
	}
	return pngPath, nil
}

func isImageKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), KindImages)
}

// dataURL empacota bytes como data URI para o <img> do frontend; vazio
// permanece vazio (o componente trata "sem cópia extraída").
func dataURL(media string, payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	return "data:" + media + ";base64," + base64.StdEncoding.EncodeToString(payload)
}
