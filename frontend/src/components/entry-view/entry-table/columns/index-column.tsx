import type { EntryKind } from '@/lib/ffx/display-names';
import { rowKey } from '@/lib/ffx/edit-draft';
import { columnHelper } from '../table-types';

/**
 * Coluna "#": o índice técnico do backend, ou — no lockit — a numeração
 * própria por grupo (game/utf8), que não expõe o índice do backend.
 */
export function buildIndexColumn(activeKind: EntryKind, lockitSeq: Map<string, number>) {
  return columnHelper.accessor('index', {
    header: '#',
    cell: (info) => {
      const r = info.row.original;
      if (activeKind === 'lockit') {
        const n = lockitSeq.get(rowKey(r)) ?? '';
        return <span className="text-muted-foreground">{n}</span>;
      }
      return <span className="text-muted-foreground">{info.getValue()}</span>;
    },
  });
}
