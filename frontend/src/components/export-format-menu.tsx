'use client';

import { ChevronDown } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Button } from '@/components/ui/button';
import {
  EXPORT_FORMAT_LABELS,
  exportSelection,
  ExportFormat,
  useExportSelection,
} from '@/lib/ffx/export-selection';

/**
 * Seletor de formato de exportação (header, à esquerda do seletor de
 * idiomas): define o formato (JSON | Strings | CSV | XLSX | PO) usado por
 * TODAS as exportações do app. A store módulo `exportSelection` é a
 * única fonte — o formato fica persistido no localStorage
 * (`ffx.export-format`). Sem props, como no header original.
 */
export function ExportFormatMenu() {
  const exportFormat = useExportSelection().format;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="min-w-19 h-9 gap-1 px-2 text-sm"
        >
          {EXPORT_FORMAT_LABELS[exportFormat]}
          <ChevronDown size={14} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel className="text-muted-foreground">
          Formato
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={exportFormat}
          onValueChange={(value) =>
            exportSelection.setFormat(value as ExportFormat)
          }
        >
          {(Object.keys(EXPORT_FORMAT_LABELS) as ExportFormat[]).map(
            (format) => (
              <DropdownMenuRadioItem key={format} value={format}>
                {EXPORT_FORMAT_LABELS[format]}
              </DropdownMenuRadioItem>
            )
          )}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
