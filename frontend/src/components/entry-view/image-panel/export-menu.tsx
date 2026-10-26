'use client';

import { ChevronDown, Download, FolderInput, FolderOpen, Save } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  imageExportFormatSelection,
  IMAGE_EXPORT_FORMAT_LABELS,
  ImageExportFormat,
  useImageExportFormat,
} from '@/lib/ffx/image-export-selection';

/**
 * "Exportar como" do painel: usa o formato selecionado no seletor de formato.
 *
 *  - `Exportar` (item do menu) → salva no formato selecionado (DDS, PNG ou DDS+PNG)
 *    direto em mods/images (sem diálogo).
 *  - `Exportar para...`        → abre seletor de pasta nativo e extrai no formato
 *    selecionado para destino customizado.
 *
 * O .dds é o padrão sem perda (volta no repack); o .png é só para
 * visualizar/distribuir. DDS+PNG gera os dois formatos.
 */
export function ImageExportMenu({
  disabled,
  readOnly,
  onExport,
  onExtractTo,
}: {
  disabled: boolean;
  /** true = textura veio do .vbf (extração não abre diálogo de alcance). */
  readOnly: boolean;
  /** Exporta no formato selecionado para mods/images (sem diálogo). */
  onExport: (format: ImageExportFormat) => void;
  /** Exporta no formato selecionado para pasta customizada (com diálogo). */
  onExtractTo: (format: ImageExportFormat) => void;
}) {
  const { format } = useImageExportFormat();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          disabled={disabled}
          title="Exportar — salva no formato selecionado em mods/images, ou escolha pasta customizada"
        >
          <Save size={16} />
          Exportar
          <ChevronDown size={12} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="min-w-56"
      >
        <DropdownMenuItem
          onClick={() => onExport(format)}
          title={`Exporta como ${IMAGE_EXPORT_FORMAT_LABELS[format]} para mods/images (sem diálogo)`}
        >
          <Save size={14} />
          Exportar
          <span className="ml-auto pl-3 text-xs opacity-60">mods/</span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => onExtractTo(format)}
          title="Escolhe a pasta de destino e exporta no formato selecionado, preservando o caminho interno"
        >
          <FolderOpen size={14} />
          Exportar para…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
