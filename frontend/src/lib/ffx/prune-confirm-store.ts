import { createStore } from '@tanstack/store';
import type * as services from '@/bindings/ffxresources/backend/services/models';

/**
 * Aviso destrutivo do save: linhas que só existem em mods (sem par no
 * original) vão ser EXCLUÍDAS do binário quando o save rodar. O modal
 * mostra a lista completa e o save só continua com a confirmação.
 *
 * O store guarda a lista aberta; a Promise resolvida por
 * resolvePruneConfirm é o que o saveAllDrafts aguarda (true = continuar).
 */
export interface PruneConfirmState {
  open: boolean;
  rows: services.PrunedRowView[];
}

export const pruneConfirmStore = createStore<PruneConfirmState>({
  open: false,
  rows: [],
});

let pending: ((ok: boolean) => void) | null = null;

/** Abre o aviso com a lista e devolve a decisão do usuário. */
export function askPruneConfirm(rows: services.PrunedRowView[]): Promise<boolean> {
  return new Promise((resolve) => {
    pending = resolve;
    pruneConfirmStore.setState(() => ({ open: true, rows }));
  });
}

/** Fecha o aviso e resolve o save pendente (true = continuar). */
export function resolvePruneConfirm(ok: boolean): void {
  pruneConfirmStore.setState((prev) => ({ ...prev, open: false, rows: prev.rows }));
  const resolve = pending;
  pending = null;
  resolve?.(ok);
}
