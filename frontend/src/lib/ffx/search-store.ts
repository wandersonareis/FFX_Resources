import { createStore, type Store } from '@tanstack/store';
import type * as services from '@/bindings/ffxresources/backend/services/models';
import { sendErrorNotification } from './error-handler';
import type { GameVersionId } from './game-version';
import { searchTextEntries } from './tree-data';

/**
 * Modal de busca por ABA/VERSÃO — um store de módulo por versão, fora do
 * createEntryView de propósito: trocar de aba desmonta a view (e seu store),
 * e a última busca precisa sobreviver para reabrir o modal exatamente como
 * ficou. O app-shell abre pelo id da aba ativa.
 */

/** Debounce do input do modal: só consulta o backend quando a digitação para. */
export const SEARCH_DEBOUNCE_MS = 600;

/** Espelha o TextSearchMinQueryRunes do backend: conteúdo exige 2 runes. */
export const SEARCH_MIN_QUERY_RUNES = 2;

export type SearchStatus = 'idle' | 'pending' | 'ready';

export interface SearchDialogState {
  open: boolean;
  /** Último termo executado (o rascunho vive no input do modal). */
  query: string;
  results: services.TextSearchResult[];
  status: SearchStatus;
  /** Totais da base inteira (o corte de arquivos só limita o payload). */
  totalRows: number;
  totalFiles: number;
  truncated: boolean;
}

const INITIAL: SearchDialogState = {
  open: false,
  query: '',
  results: [],
  status: 'idle',
  totalRows: 0,
  totalFiles: 0,
  truncated: false,
};

const stores = new Map<GameVersionId, Store<SearchDialogState>>();
/** Guarda de sequência por versão: resposta fora de ordem é descartada. */
const sequences = new Map<GameVersionId, number>();

export function searchStoreFor(version: GameVersionId): Store<SearchDialogState> {
  let store = stores.get(version);
  if (!store) {
    store = createStore({ ...INITIAL });
    stores.set(version, store);
  }
  return store;
}

export function openSearchDialog(version: GameVersionId): void {
  searchStoreFor(version).setState((prev) => ({ ...prev, open: true }));
}

export function closeSearchDialog(version: GameVersionId): void {
  searchStoreFor(version).setState((prev) => ({ ...prev, open: false }));
}

/**
 * Executa a busca de CONTEÚDO no backend. Termo abaixo do mínimo limpa os
 * resultados sem tocar no backend; respostas velhas (query anterior ou
 * invalidação no meio do voo) são descartadas pela guarda de sequência.
 * É ESTA função que escreve `query` no store (query = último termo
 * EXECUTADO) — o rascunho do input nunca toca no estado da busca, senão o
 * guard do debounce do dialog compara draft === query e nunca agenda.
 */
export async function runTextSearch(version: GameVersionId, query: string): Promise<void> {
  const store = searchStoreFor(version);
  const trimmed = query.trim();
  const sequence = (sequences.get(version) ?? 0) + 1;
  sequences.set(version, sequence);

  if ([...trimmed].length < SEARCH_MIN_QUERY_RUNES) {
    store.setState((prev) => ({
      ...prev,
      query,
      results: [],
      status: 'idle',
      totalRows: 0,
      totalFiles: 0,
      truncated: false,
    }));
    return;
  }

  store.setState((prev) => ({ ...prev, query, status: 'pending' }));
  try {
    const response = await searchTextEntries(version, trimmed);
    if (sequence !== sequences.get(version)) return;
    store.setState((prev) => ({
      ...prev,
      status: 'ready',
      results: response?.results ?? [],
      totalRows: response?.totalRows ?? 0,
      totalFiles: response?.totalFiles ?? 0,
      truncated: response?.truncated ?? false,
    }));
  } catch (error) {
    if (sequence !== sequences.get(version)) return;
    store.setState((prev) => ({
      ...prev,
      status: 'ready',
      results: [],
      totalRows: 0,
      totalFiles: 0,
      truncated: false,
    }));
    sendErrorNotification(error);
  }
}

/**
 * Resultados descartados porque a base mudou (reload frio, import, save):
 * o termo fica, os resultados voltam a idle — o próximo Enter/busca traz a
 * foto nova. Também cancela respostas em voo da consulta anterior.
 */
export function invalidateTextSearchResults(version: GameVersionId): void {
  sequences.set(version, (sequences.get(version) ?? 0) + 1);
  searchStoreFor(version).setState((prev) => ({
    ...prev,
    results: [],
    status: 'idle',
    totalRows: 0,
    totalFiles: 0,
    truncated: false,
  }));
}
