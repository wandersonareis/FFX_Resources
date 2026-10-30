'use client';

import { Settings } from 'lucide-react';
import { ExportFormatMenu } from '@/components/export-format-menu';
import { ExportLangsMenu } from '@/components/export-langs-menu';
import { LogWindowToggle } from '@/components/log-window-toggle';
import { ThemeToggle } from '@/components/theme-toggle';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { SHORTCUTS, shortcutTip } from '@/lib/ffx/shortcuts';

/**
 * Barra superior (título + tema + configurações + formato e idiomas de
 * exportação). Só precisa do callback de abrir Configurações: formato e
 * idiomas vêm das stores módulo (`exportSelection` / `exportLangs`),
 * então nada disso atravessa props.
 */
export function AppHeader({ onOpenConfig }: { onOpenConfig: () => void }) {
  return (
    <header className="sticky top-0 z-10 flex items-center gap-0.5 bg-chrome px-4 py-2 border-b">
      <span className="text-xl font-malva-medium text-gradient">
        FINAL FANTASY X and X-2 HD Remaster Resources Editor
      </span>
      <span className="flex-1" />
      <ThemeToggle />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" onClick={onOpenConfig} aria-label="Configurações">
            <Settings size={22} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {shortcutTip(SHORTCUTS.config.label, SHORTCUTS.config.keys)}
        </TooltipContent>
      </Tooltip>
      <ExportFormatMenu />
      <ExportLangsMenu />
      <LogWindowToggle />
    </header>
  );
}
