import { createStore, type Store } from '@tanstack/store';
import type { GameVersionId } from './game-version';

/**
 * Busca & substituição local no arquivo aberto — um store de MÓDULO por
 * versão (mesmo padrão do search-store), fora do createEntryView de
 * propósito: o Ctrl+F global (app-shell) abre a busca da aba ATIVA pelo id,
 * e o estado sobrevive à troca de arquivo dentro da aba.
 *
 * Guarda SÓ o estado da barra (query/opções/índice). Os matches são
 * derivados das rows abertas em `use-entry-find` — este store não conhece
 * rows nem rascunho, só a intenção do usuário.
 */
export interface FileFindState {
  open: boolean;
  /** Termo digitado na busca. */
  query: string;
  /** Texto digitado na substituição. */
  replaceText: string;
  /** Linha de substituir expandida (colapse). */
  showReplace: boolean;
  /** Diferenciar maiúsculas/minúsculas (botão Aa). */
  matchCase: boolean;
  /** Casa apenas palavras inteiras. */
  wholeWord: boolean;
  /** Índice do match ativo (para o destaque find-current e a navegação). */
  activeIndex: number;
}

const INITIAL: FileFindState = {
  open: false,
  query: '',
  replaceText: '',
  showReplace: false,
  matchCase: false,
  wholeWord: false,
  activeIndex: 0,
};

const stores = new Map<GameVersionId, Store<FileFindState>>();

export function findStoreFor(version: GameVersionId): Store<FileFindState> {
  let store = stores.get(version);
  if (!store) {
    store = createStore({ ...INITIAL });
    stores.set(version, store);
  }
  return store;
}

export function openFileFind(version: GameVersionId): void {
  findStoreFor(version).setState((prev) => ({ ...prev, open: true }));
}

export function closeFileFind(version: GameVersionId): void {
  findStoreFor(version).setState((prev) => ({ ...prev, open: false }));
}

export function setFindQuery(version: GameVersionId, query: string): void {
  // Trocou o termo = recomeça do primeiro match.
  findStoreFor(version).setState((prev) => ({ ...prev, query, activeIndex: 0 }));
}

export function setFindReplaceText(version: GameVersionId, text: string): void {
  findStoreFor(version).setState((prev) => ({ ...prev, replaceText: text }));
}

export function toggleFindReplace(version: GameVersionId): void {
  findStoreFor(version).setState((prev) => ({ ...prev, showReplace: !prev.showReplace }));
}

export function toggleFindMatchCase(version: GameVersionId): void {
  // Opção muda = o conjunto de matches muda; volta ao primeiro.
  findStoreFor(version).setState((prev) => ({ ...prev, matchCase: !prev.matchCase, activeIndex: 0 }));
}

export function toggleFindWholeWord(version: GameVersionId): void {
  findStoreFor(version).setState((prev) => ({ ...prev, wholeWord: !prev.wholeWord, activeIndex: 0 }));
}

export function setFindActiveIndex(version: GameVersionId, index: number): void {
  findStoreFor(version).setState((prev) => ({ ...prev, activeIndex: Math.max(0, index) }));
}
