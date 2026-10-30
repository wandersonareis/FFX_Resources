import type { ReactTable } from '@tanstack/react-table';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { F } from './table-types';

/**
 * Larguras do layout FIXO da tabela (table-fixed: o navegador lê os
 * tamanhos do primeiro row, ou seja, deste cabeçalho):
 *
 * - "#": 40px de conteúdo + os 8px de padding que a célula já tem de
 *   cada lado = 56px no total;
 * - "Nome"/"Tipo" (só objects/lockit): 160px;
 * - Original e Traduzido: dividem o resto meio a meio via calc().
 *
 * Sem o layout fixo o auto layout cresce com o texto de cada linha e as
 * duas colunas nunca ficam com a mesma largura.
 */
const ID_WIDTH = '56px';
const NAME_WIDTH = '160px';

export function EntryTableHeader({ table }: { table: ReactTable<F, dto.TextRow> }) {
  const hasName = table.getAllColumns().some((c) => c.id === 'name');
  const textWidth = hasName
    ? `calc((100% - ${ID_WIDTH} - ${NAME_WIDTH}) / 2)`
    : `calc((100% - ${ID_WIDTH}) / 2)`;

  const widthOf = (id: string): string | undefined => {
    if (id === 'index' || id === 'seq') return ID_WIDTH;
    if (id === 'name') return NAME_WIDTH;
    if (id === 'original' || id === 'translated') return textWidth;
    return undefined;
  };

  return (
    // sticky nas CÉLULAS (e não só no thead): com border-collapse (preflight
    // do Tailwind) o Chromium pinta o thead sticky atrás das linhas do corpo,
    // que passam por cima — th sticky pinta na ordem correta. top-0 é o topo
    // do rolante da aba (a barra de título/Salvar fica FORA dele, num
    // componente separado), então o cabeçalho gruda logo embaixo da barra.
    // z menor que a barra: em qualquer coincidência ela passa por baixo.
    <TableHeader className="sticky top-0 z-[5]">
      {table.getHeaderGroups().map((headerGroup) => (
        <TableRow key={headerGroup.id}>
          {headerGroup.headers.map((header) => (
            <TableHead
              key={header.id}
              className="sticky top-0 z-[5] bg-background"
              style={{ width: widthOf(header.column.id) }}
            >
              {header.isPlaceholder ? null : <table.FlexRender header={header} />}
            </TableHead>
          ))}
        </TableRow>
      ))}
    </TableHeader>
  );
}
