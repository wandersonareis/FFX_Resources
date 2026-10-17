"use client";

import { Save, Search } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { shortcutTip, SHORTCUTS } from "@/lib/ffx/shortcuts";
import type { EntryProgress } from "@/lib/ffx/entry-progress";

/**
 * Barra de título da tabela: tipo + entrada + badge de progresso, e o
 * botão Salvar (e o de buscar neste arquivo). Fica FORA do rolante (o
 * painel é flex-col: barra em cima, tabela rolando embaixo), então não
 * precisa de sticky — fixa por construção, independente do tamanho do texto.
 */
export function TableBar({
  kindLabel,
  entryLabel,
  progress,
  hasDirty,
  saving,
  onSave,
  onOpenFind,
}: {
  kindLabel: string;
  entryLabel?: string | null;
  progress?: EntryProgress | null;
  hasDirty: boolean;
  saving: boolean;
  onSave: () => void;
  /** Abre a busca & substituição local (ausente em kinds sem rows, p/ images). */
  onOpenFind?: () => void;
}) {
  return (
    <div className="flex shrink-0 items-center justify-between gap-4">
      <h3 className="flex gap-2 items-center text-primary text-lg font-malva-medium">
        {kindLabel}
        {/* O span volta ao Malva padrão (Regular, família base), só desanimado. */}
        {entryLabel ? (
          <span className="opacity-70"> · {entryLabel}</span>
        ) : null}
        {progress ? (
          <Badge
            variant="outline"
            className={
              progress.translated >= progress.total && progress.total > 0
                ? "text-success border-success/40"
                : "text-muted-foreground"
            }
          >
            {progress.translated}/{progress.total} linhas traduzidas (
            {progress.pct}%)
          </Badge>
        ) : null}
      </h3>
      <div className="flex items-center gap-1">
        {onOpenFind ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" aria-label="Buscar neste arquivo" onClick={onOpenFind}>
                <Search size={18} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {shortcutTip(SHORTCUTS.findInFile.label, SHORTCUTS.findInFile.keys)}
            </TooltipContent>
          </Tooltip>
        ) : null}
        <Tooltip>
          <TooltipTrigger asChild>
            {/* O botão desabilitado não dispara pointer events, então o
                trigger é o span envoltório (padrão Radix p/ disabled). */}
            <span className="inline-flex">
              <Button disabled={!hasDirty || saving} onClick={onSave}>
                <Save size={18} />
                {saving ? "Salvando…" : "Salvar"}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {shortcutTip(SHORTCUTS.save.label, SHORTCUTS.save.keys)}
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
