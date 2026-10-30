import { createStore } from '@tanstack/store';
import type { EntryKind } from './display-names';
import type { EntryProgress } from './entry-progress';
import type { GameVersionId } from './game-version';

/**
 * Arquivo ATIVO (aberto na tabela) — canal global para a barra de status:
 * a aba montada (uma por vez) publica o progresso por row na abertura do
 * arquivo e a barra de status assina. O progresso é FIXO na abertura.
 */
export interface ActiveFileState {
  version: GameVersionId | null;
  kind: EntryKind | null;
  entryId: string | null;
  entryLabel: string | null;
  progress: EntryProgress | null;
}

const initialActiveFile: ActiveFileState = {
  version: null,
  kind: null,
  entryId: null,
  entryLabel: null,
  progress: null,
};

export const activeFileStore = createStore<ActiveFileState>(initialActiveFile);

export function setActiveFile(partial: Partial<ActiveFileState>): void {
  activeFileStore.setState((prev) => ({ ...prev, ...partial }));
}

export function clearActiveFile(): void {
  activeFileStore.setState(() => initialActiveFile);
}
