import type { ReactTable } from '@tanstack/react-table';
import type { RefObject } from 'react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { TableBody, TableCell, TableRow } from '@/components/ui/table';
import { rowKey } from '@/lib/ffx/edit-draft';
import type { F } from './table-types';

export function EntryTableBody({
  table,
  rowRefs,
  focusedRowId,
  onFocusRow,
}: {
  table: ReactTable<F, dto.TextRow>;
  rowRefs: RefObject<Map<string, HTMLTableRowElement>>;
  focusedRowId: string | null;
  onFocusRow: (key: string) => void;
}) {
  return (
    <TableBody>
      {table.getRowModel().rows.map((tRow) => {
        const r = tRow.original;
        const key = rowKey(r);
        return (
          <TableRow
            key={tRow.id}
            ref={(el) => {
              if (el) rowRefs.current.set(key, el);
              else rowRefs.current.delete(key);
            }}
            tabIndex={0}
            data-row-key={key}
            onFocus={() => onFocusRow(key)}
            className={
              focusedRowId === key
                ? 'bg-primary/10 outline outline-primary/60'
                : 'outline-none focus-visible:bg-muted/40'
            }
          >
            {tRow.getAllCells().map((cell) => (
              <TableCell key={cell.id}>
                <table.FlexRender cell={cell} />
              </TableCell>
            ))}
          </TableRow>
        );
      })}
    </TableBody>
  );
}
