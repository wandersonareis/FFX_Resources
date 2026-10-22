'use client';

import { isDesktop } from '@/lib/ffx/env';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { LogPanel } from '@/components/log-panel';
import { LogPanelControls } from '@/components/log-panel-controls';
import { setLogWindowOpen, useLogWindowOpen } from '@/lib/ffx/log-window-store';

/**
 * Host WEB da exibição de logs (server mode / browser): Sheet lateral
 * direito NÃO-MODAL — sem overlay e sem foco preso, o app segue usável com
 * o painel aberto (companheiro de trabalho, não diálogo); fecha pelo X, Esc
 * ou checkbox, nunca por clique fora. No desktop este componente não
 * renderiza: lá o host é a JANELA /w/#logs (WindowService.OpenWindow).
 *
 * UMA barra: o header do Sheet carrega o contador + Limpar (reservando a
 * direita para o X embutido) — o LogPanel é só a lista rolável.
 */
export function LogDrawer() {
  const open = useLogWindowOpen();
  if (isDesktop()) return null;
  return (
    <Sheet open={open} modal={false} onOpenChange={setLogWindowOpen}>
      <SheetContent
        side="right"
        overlay={false}
        className="w-[420px] max-w-[420px] gap-0 p-0"
      >
        <SheetHeader className="border-b border-border/60 px-4 py-3">
          <div className="flex items-center gap-2">
            <SheetTitle className="text-xs font-medium text-muted-foreground">
              Logs
            </SheetTitle>
            {/* mr-9 desvia do X absoluto do SheetContent (top-2 right-2). */}
            <LogPanelControls className="ml-auto mr-9" />
          </div>
          <SheetDescription className="sr-only">
            Operações do aplicativo em tempo real
          </SheetDescription>
        </SheetHeader>
        <LogPanel className="min-h-0 flex-1" />
      </SheetContent>
    </Sheet>
  );
}
