'use client';

import { ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  IMAGE_EXPORT_FORMAT_LABELS,
  imageExportFormatSelection,
  useImageExportFormat,
  ImageExportFormat,
} from '@/lib/ffx/image-export-selection';

/**
 * Seletor de formato de exportação de imagens (barra de ferramentas, ao lado do
 * botão Exportar): define o formato (DDS | PNG | DDS + PNG) usado por TODAS
 * as exportações de imagem. A store `imageExportFormatSelection` é a única
 * fonte — o formato fica persistido no localStorage (`ffx.image-export-format`).
 */
export function ImageExportFormatMenu() {
  const { format } = useImageExportFormat();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="min-w-19 gap-1 text-sm"
          title="Formato de exportação das imagens"
        >
          {IMAGE_EXPORT_FORMAT_LABELS[format]}
          <ChevronDown size={14} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel className="text-muted-foreground">Formato</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={format}
          onValueChange={(value) => imageExportFormatSelection.setFormat(value as ImageExportFormat)}
        >
          {(Object.keys(IMAGE_EXPORT_FORMAT_LABELS) as ImageExportFormat[]).map((fmt) => (
            <DropdownMenuRadioItem key={fmt} value={fmt}>
              {IMAGE_EXPORT_FORMAT_LABELS[fmt]}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}