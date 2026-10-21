'use client';

import { Checkbox } from '@/components/ui/checkbox';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';
import {
  setLogWindowOpen,
  toggleLogWindow,
  useLogWindowOpen,
} from '@/lib/ffx/log-window-store';

/**
 * Checkbox "Logs" do header (à direita do seletor de idiomas de export):
 * desmarcado por padrão. Marcar abre a exibição de logs — JANELA lateral
 * direita no desktop, DRAWER lateral direito no web —; desmarcar fecha.
 *
 * Sincronia nos dois sentidos: o fechamento da janela pelo próprio X/Esc
 * (desktop) chega aqui pelo evento LogWindowClosed e desmarca o checkbox —
 * o estado nunca mente sobre o que está aberto.
 */
export function LogWindowToggle() {
  const open = useLogWindowOpen();
  useWailsEvent('LogWindowClosed', () => setLogWindowOpen(false));

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <label
          className="flex h-9 cursor-pointer items-center gap-1.5 rounded-md px-2 text-sm text-foreground select-none transition-colors hover:bg-muted"
          aria-label="Exibir janela de logs"
        >
          <Checkbox
            checked={open}
            onCheckedChange={(checked) => toggleLogWindow(checked === true)}
          />
          <span>Logs</span>
        </label>
      </TooltipTrigger>
      <TooltipContent>
        Logs de operações
      </TooltipContent>
    </Tooltip>
  );
}
