import { createStore } from '@tanstack/store';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import type { EntryKind } from './display-names';
import { editDraft } from './edit-draft';
import type { GameVersionId } from './game-version';

/**
 * Exclusão MANUAL de uma linha órfã (só em mods, sem par no original).
 * Abre o aviso destrutivo; confirmar marca a linha no rascunho e o save a
 * deixa fora do binário. O save também poda as demais órfãs (com aviso
 * próprio) — este caminho serve para o usuário ESCOLHER e conferir o texto.
 */
export interface DeleteOrphanRequest {
  version: GameVersionId;
  kind: EntryKind;
  id: string;
  source: string;
  row: dto.TextRow;
}

export const deleteOrphanStore = createStore<{ request: DeleteOrphanRequest | null }>({
  request: null,
});

export function askDeleteOrphan(request: DeleteOrphanRequest): void {
  deleteOrphanStore.setState(() => ({ request }));
}

export function cancelDeleteOrphan(): void {
  deleteOrphanStore.setState(() => ({ request: null }));
}

export function confirmDeleteOrphan(): void {
  const request = deleteOrphanStore.state.request;
  if (request) {
    editDraft.deleteRow(request.version, request.kind, request.id, request.row, request.source);
  }
  cancelDeleteOrphan();
}
