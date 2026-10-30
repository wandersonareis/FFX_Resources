'use client';

import { Search } from 'lucide-react';
import { hotkeyLabel, SHORTCUTS } from '@/lib/ffx/shortcuts';

/**
 * Gatilho VISUAL da busca na sidebar: parece o antigo campo (mesmo lugar,
 * mesmo placeholder), mas não tem lógica de busca alguma — clicar abre o
 * modal (Ctrl+K), que é o único lugar onde a consulta acontece.
 */
export function SearchControl({ onOpen }: { onOpen: () => void }) {
  return (
    <div className="px-2 pb-2">
      <button
        type="button"
        onClick={onOpen}
        aria-label={`Buscar nome ou texto (${hotkeyLabel(SHORTCUTS.search.keys)})`}
        className="flex h-8 w-full min-w-0 items-center gap-2 rounded-lg border border-input bg-transparent px-2.5 py-1 text-left text-base text-muted-foreground transition-colors outline-none hover:bg-accent/50 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
      >
        <Search size={16} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate">Buscar nome ou texto (us)…</span>
        <kbd className="pointer-events-none shrink-0 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground">
          {hotkeyLabel(SHORTCUTS.search.keys)}
        </kbd>
      </button>
    </div>
  );
}
