package services

// Export com projeção source/target: `text`/`source` vêm SEMPRE de data/
// (ou do .vbf, quando o chamador passa a fonte) e `target` vem de mods/ —
// independente do toggle EnableMods (common.DisableMods). O `target` é a
// tradução que vive no slot `us` de mods/ e é o único texto que o import
// aplica de volta no binário.
//
// A leitura por fonte (loadOriginalFrom) já é a mesma dos originais da view:
// nenhum builder muda, e a store mods-first do editor não participa do
// export (senão `text` sairia traduzido).

import (
	"fmt"
	"path/filepath"
	"strings"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/event"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/progress"
)

// ExportTextCollection devolve a Collection de export do kind pronta para
// os formatos que vivem FORA do batch do frontend (menu/CLI): source de
// data/ + target de mods/ (exportCollection), com o mesmo filtro de rows em
// branco dos demais artefatos e o universo de events respeitando a aba da
// versão. ids vazio = tudo do kind.
func (s *MetadataService) ExportTextCollection(kind string, version common.GameVersion, ids []string, r progress.Reporter) (dto.Collection, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == KindEvents && len(ids) == 0 {
		// Bulk: precisa do universo completo (a árvore carrega por página).
		if err := s.ensureAllEventsLoaded(version); err != nil {
			return nil, err
		}
		ids = filterEventIDsForVersion(event.GetAllEventIDs(version), version)
	}
	c, err := s.exportCollection(kind, version, helpExportIDs(kind, ids), r)
	if err != nil {
		return nil, err
	}
	return filterExportRows(c), nil
}

// exportCollection monta o DTO do export para o kind: extrai `text`/`source`
// da origem (data/) e projeta o `target` de mods/.
func (s *MetadataService) exportCollection(kind string, version common.GameVersion, ids []string, r progress.Reporter) (dto.Collection, error) {
	c, err := s.collectionFromSource(kind, version, ids, originalSource, r)
	if err != nil {
		return nil, err
	}
	return s.projectTargets(kind, version, c), nil
}

// projectTargets preenche Source/Target na collection já montada a partir da
// origem. A leitura de mods/ é opcional: ausência/erro degrada para target
// vazio (a exportação continua, mostrando só a referência).
func (s *MetadataService) projectTargets(kind string, version common.GameVersion, c dto.Collection) dto.Collection {
	if len(c) == 0 {
		return c
	}
	mods, err := s.collectionFromSource(kind, version, c.SortedKeys(), common.SourceMods, nil)
	if err != nil {
		common.LogVerbose("export %s: sem target em mods/: %v", kind, err)
		mods = dto.Collection{}
	}
	return builders.ProjectSourceTarget(c, mods)
}

// collectionFromSource monta a Collection do kind lendo da FONTE indicada
// (data/, mods/ ou .vbf), sem tocar na store. ids vazio = universo do kind
// (mesmo contrato do GetCollection); ids dado = só os pedidos.
func (s *MetadataService) collectionFromSource(kind string, version common.GameVersion, ids []string, src common.FileSource, r progress.Reporter) (dto.Collection, error) {
	r = progress.OrNop(r)
	kind = strings.ToLower(strings.TrimSpace(kind))
	targets, err := s.exportIDs(kind, version, ids)
	if err != nil {
		return nil, err
	}

	// Dicionário: um arquivo por localização — monta o container UMA vez e
	// recorta os chunks pedidos (loadOriginalFrom por chunk releria o binário
	// inteiro para cada um).
	if kind == KindMacro {
		c, err := builders.BuildMacroDTOFromSource(version, src, r)
		if err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			return c, nil
		}
		out := make(dto.Collection, len(targets))
		for _, id := range targets {
			if e, ok := c[id]; ok {
				out[id] = e
			}
		}
		return out, nil
	}

	r.SetTotal(len(targets))
	out := make(dto.Collection, len(targets))
	for _, id := range targets {
		r.Step(id)
		entry, ok, err := s.loadOriginalFrom(kind, id, version, src)
		if err != nil {
			if src != common.SourceMods {
				common.LogVerbose("export %s: %s ausente em %s: %v", kind, id, src, err)
			}
			continue
		}
		if !ok {
			continue
		}
		out[id] = entry
	}
	return out, nil
}

// exportIDs resolve o universo de ids do export (mesma régua do
// GetCollection) para a fonte pedida. Para macro o dicionário é único e a
// lista fica com o chamador.
func (s *MetadataService) exportIDs(kind string, version common.GameVersion, ids []string) ([]string, error) {
	switch kind {
	case KindEvents:
		if len(ids) > 0 {
			return ids, nil
		}
		// Bulk sem lista: o universo do datastore já filtrado pela aba
		// (exportKind carrega tudo antes de chamar).
		return filterEventIDsForVersion(event.GetAllEventIDs(version), version), nil

	case KindObjects:
		layouts, err := s.resolveObjectLayouts(version, ids)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(layouts))
		for _, layout := range layouts {
			out = append(out, strings.TrimSuffix(layout.FileName, filepath.Ext(layout.FileName)))
		}
		return out, nil

	case KindMacro:
		return ids, nil

	case KindLockit:
		if len(ids) == 0 {
			for _, l := range lockit.LayoutsForVersion(version) {
				ids = append(ids, l.ID())
			}
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if _, ok := lockit.LayoutForID(version, id); ok {
				out = append(out, id)
			}
		}
		return out, nil

	case KindHelp:
		if len(ids) == 0 {
			return helpfile.HelpEntryNames(), nil
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if helpfile.IsHelpEntry(id) {
				out = append(out, id)
			}
		}
		return out, nil

	case KindBattleText, KindCloud, KindTutorial, KindMenuMain:
		if !eventtableKindUsable(kind, version) {
			return nil, nil
		}
		return builders.ResolveTableTargets(kind, version, ids)
	}
	return nil, fmt.Errorf("unknown kind: %s", kind)
}
