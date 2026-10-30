'use client';

import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { Badge } from '@/components/ui/badge';
import { copyState, resolveEntryLabel } from '@/lib/ffx/display-names';

/**
 * Lista da textura de referência + cópias, com o estado de cada uma.
 *
 * Compartilhada: serve aos diálogos de ação de imagem (extrair / replicar /
 * deletar) e ao diálogo de importação em lote do painel — a marcação é a
 * mesma nos quatro lugares.
 */
export function DuplicateList({ id, list }: { id: string; list: dto.ImageDuplicate[] }) {
  return (
    <ul className="max-h-56 space-y-1 overflow-auto rounded border bg-muted/40 p-2 text-xs">
      <li className="flex items-center gap-1.5">
        <span className="min-w-0 flex-1 truncate">{resolveEntryLabel('images', id)}</span>
        <Badge variant="outline" className="text-[10px]">
          esta
        </Badge>
      </li>
      {list.map((d) => (
        <li key={d.id} className="flex items-center gap-1.5">
          <span className="min-w-0 flex-1 truncate">{resolveEntryLabel('images', d.id)}</span>
          <Badge variant={d.identical ? 'outline' : 'secondary'} className="text-[10px]">
            {copyState(d)}
          </Badge>
        </li>
      ))}
    </ul>
  );
}
