import type { EntryKind } from '@/lib/ffx/display-names';
import { resolveSegmentLabel } from '@/lib/ffx/display-names';
import { columnHelper } from '../table-types';

/**
 * Coluna de nome — só existe em objects ("Nome") e lockit ("Tipo", com o
 * rótulo do segmento em vez do valor bruto).
 */
export function buildNameColumn(activeKind: EntryKind) {
  return columnHelper.accessor('name', {
    header: activeKind === 'lockit' ? 'Tipo' : 'Nome',
    cell: (info) =>
      activeKind === 'lockit' ? (
        <span className="block whitespace-normal wrap-break-word text-muted-foreground">
          {resolveSegmentLabel(info.getValue())}
        </span>
      ) : (
        // Layout fixo: a coluna tem 160px fixos e o <td> é nowrap, então o
        // nome precisa quebrar linha aqui para não vazar na coluna vizinha.
        <span className="block whitespace-normal wrap-break-word">{info.getValue()}</span>
      ),
  });
}
