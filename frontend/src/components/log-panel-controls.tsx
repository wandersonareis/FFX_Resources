'use client';

import { Trash2 } from 'lucide-react';
import { cn } from 'cn';
import { Button } from '@/components/ui/button';
import { uiLog, useUILog } from '@/lib/ffx/uilog-store';

/**
 * Controles do log para a BARRA do host (titlebar do WindowChrome na janela
 * desktop; header do Sheet no drawer web): contador de linhas + Limpar. É o
 * mesmo par de controles nos dois hosts — uma barra só por exibição, sem
 * toolbar extra embaixo.
 */
export function LogPanelControls({ className }: { className?: string }) {
  const { entries } = useUILog();
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <span className="text-xs text-muted-foreground select-none">
        {entries.length} linha(s)
      </span>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 gap-1 px-2 text-xs"
        onClick={() => uiLog.clear()}
        title="Limpar o histórico da janela — o arquivo de log continua íntegro"
      >
        <Trash2 size={14} />
        Limpar
      </Button>
    </div>
  );
}
