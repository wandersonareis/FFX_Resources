'use client';

import { useCallback, useMemo } from 'react';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { useEditDraft, rowKey } from '@/lib/ffx/edit-draft';
import { SOURCE_LANG } from '@/lib/ffx/save-all';
import { linkedRowText } from '@/lib/ffx/hash-ref';
import { locFromVbfPath } from '@/lib/ffx/tree-data';
import type { EntryRow } from '@/lib/ffx/tree-data';
import type { EntryKind } from '@/lib/ffx/display-names';
import type { GameTextMark } from '@/lib/ffx/game-text-parser';
import type { EntryView } from '../entry-view-store';
import { columnHelper } from './table-types';
import { buildIndexColumn } from './columns/index-column';
import { buildNameColumn } from './columns/name-column';
import { buildOriginalColumn } from './columns/original-column';
import { buildTranslatedColumn } from './columns/translated-column';

/**
 * Colunas da tabela e os dados derivados que só existem por causa delas:
 * o texto da célula linkada, a numeração própria do lockit e a língua do
 * binário clicado.
 */
export function useEntryTableColumns({
  rows,
  activeKind,
  selectedEntry,
  refLinks,
  version,
  actions,
  matchesByRow,
}: {
  rows: dto.TextRow[];
  activeKind: EntryKind;
  selectedEntry: EntryRow | null;
  refLinks: Record<string, dto.RefLink>;
  version: EntryView['version'];
  actions: EntryView['actions'];
  /** Destaques da busca local (rowKey → intervalos em <mark>). */
  matchesByRow: Map<string, GameTextMark[]>;
}) {
  const { store: drafts, snapshot } = useEditDraft();

  // Texto da célula LINKADA (row de ref): estado atual da def — rascunho
  // dela inclusive. O "$hash" do backend nunca é pintado.
  const linkedValueOf = useCallback(
    (row: dto.TextRow): string | undefined => {
      const link = refLinks[rowKey(row)];
      const entry = selectedEntry;
      if (!link) return undefined;
      if (!entry) return link.text ?? '';
      return linkedRowText(entry.id, link, (defId, defKey) =>
        drafts.editTextOf(
          version,
          entry.kind,
          defId,
          defKey,
          SOURCE_LANG,
          entry.vbf?.root ?? ''
        )
      );
    },
    [refLinks, selectedEntry, drafts, version]
  );

  // lockit: numeração própria por grupo (game/utf8), sem expor o índice
  // técnico do backend.
  const lockitSeq = useMemo(() => {
    const counters = new Map<string, number>();
    const map = new Map<string, number>();
    for (const r of rows) {
      const k = r.name ?? '';
      const n = (counters.get(k) ?? 0) + 1;
      counters.set(k, n);
      map.set(rowKey(r), n);
    }
    return map;
  }, [rows]);

  // Entrada .vbf de idioma não-us: a coluna Original/Traduzido mostra o
  // texto DESTE binário do idioma clicado (não o 'us').
  const textLoc = selectedEntry?.vbf?.path
    ? (locFromVbfPath(selectedEntry.vbf.path) ?? SOURCE_LANG)
    : SOURCE_LANG;

  const columns = useMemo(
    () =>
      columnHelper.columns([
        buildIndexColumn(activeKind, lockitSeq),
        ...(activeKind === 'objects' || activeKind === 'lockit'
          ? [buildNameColumn(activeKind)]
          : []),
        buildOriginalColumn(textLoc),
        buildTranslatedColumn({
          selectedEntry,
          version,
          drafts,
          actions,
          refLinks,
          textLoc,
          linkedValueOf,
          matchesByRow,
        }),
      ]),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeKind, selectedEntry, version, snapshot.revision, drafts, actions, lockitSeq, refLinks, linkedValueOf, matchesByRow]
  );

  return columns;
}
