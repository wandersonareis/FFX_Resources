import { useEffect } from 'react';
import { createStore } from '@tanstack/store';
import { useSelector } from '@tanstack/react-store';
import type { EntryKind } from './display-names';
import type { GameVersionId } from './game-version';

export type ExportFormat = 'json' | 'strings' | 'csv' | 'xlsx';

export const EXPORT_FORMAT_LABELS: Record<ExportFormat, string> = {
  json: 'JSON',
  strings: 'Strings',
  csv: 'CSV',
  xlsx: 'XLSX (Excel)',
};

const FORMAT_STORAGE_KEY = 'ffx.export-format';

function loadStoredFormat(): ExportFormat {
  try {
    const stored = localStorage.getItem(FORMAT_STORAGE_KEY);
    if (
      stored === 'json' ||
      stored === 'strings' ||
      stored === 'csv' ||
      stored === 'xlsx'
    ) {
      return stored;
    }
    return 'json';
  } catch {
    return 'json';
  }
}

interface SelectionSnapshot {
  revision: number;
  /** ids selecionados por `${version}|${kind}` (imutável entre notificações). */
  byKind: ReadonlyMap<string, readonly string[]>;
  /**
   * Kinds cuja seleção cobre o kind INTEIRO (o usuário marcou a raiz do
   * kind na árvore). Quando true, a exportação daquele kind vira o
   * artefato bulk único com prefixo `tree_`, em vez de um arquivo por
   * entrada. Chave: `${version}|${kind}`.
   */
  completeByKind: ReadonlyMap<string, boolean>;
  /** Formato ativo para todas as exportações feitas pelo frontend. */
  format: ExportFormat;
}

const stateKey = (version: GameVersionId, kind: EntryKind): string =>
  `${version}|${kind}`;

/**
 * Store reativo da seleção (TanStack Store): o estado é o snapshot imutável
 * {revision, byKind, format} consumido pelo hook. Os dados de trabalho ficam
 * imperativos no singleton abaixo.
 */
const exportStore = createStore<SelectionSnapshot>({
  revision: 0,
  byKind: new Map(),
  completeByKind: new Map(),
  // 'json' é o default de hidratação; o formato persistido no localStorage é
  // carregado no cliente dentro do useExportSelection (mesmo padrão do
  // tabStorage no app-shell).
  format: 'json',
});

/**
 * Seleção de entradas para exportação (singleton reativo, mesmo padrão do
 * edit-draft). Marcar na sidebar alimenta um destino só: o menu de contexto
 * da árvore (Exportar). O botão Exportar do topo não lê a seleção — ele
 * exporta por ESCOPO DE TIPO (os checkboxes do popover).
 *
 * `complete` registra que o kind inteiro foi marcado (raiz do kind): a
 * exportação daquele kind vira o artefato bulk único (`tree_…`).
 */
class ExportSelectionStore {
  private format: ExportFormat = 'json';
  private readonly selected = new Map<string, Set<string>>();
  /** Chaves `${version}|${kind}` marcadas por completo (raiz do kind). */
  private readonly complete = new Set<string>();

  /** Snapshot imutável atual (leitura imperativa). */
  getSnapshot(): SelectionSnapshot {
    return exportStore.state;
  }

  private notify(): void {
    const copied = new Map<string, readonly string[]>();
    for (const [key, ids] of this.selected) copied.set(key, [...ids]);
    const complete = new Map<string, boolean>();
    for (const key of this.complete) complete.set(key, true);
    exportStore.setState((prev) => ({
      revision: prev.revision + 1,
      byKind: copied,
      completeByKind: complete,
      format: this.format,
    }));
  }

  /**
   * Define o formato de exportação usado por TODAS as exportações do
   * frontend (Exportar do topo e menu de contexto). Persistido em
   * localStorage, como o tabStorage faz com a aba ativa.
   */
  setFormat(format: ExportFormat): void {
    if (this.format === format) return;
    this.format = format;
    try {
      localStorage.setItem(FORMAT_STORAGE_KEY, format);
    } catch {
      // localStorage indisponível — segue só em memória
    }
    // Formato não mexe em quem está marcado: `byKind`/`completeByKind`
    // mantêm a identidade (só `revision`/`format` mudam), para o
    // consumidor memoizado por `byKind` não refazer os mapas de seleção a
    // cada troca JSON/Strings.
    exportStore.setState((prev) => ({
      revision: prev.revision + 1,
      byKind: prev.byKind,
      completeByKind: prev.completeByKind,
      format: this.format,
    }));
  }

  formatOf(): ExportFormat {
    return this.format;
  }

  /**
   * Marca/desmarca várias entradas de uma vez (grupo = filhos; clique em
   * indeterminado chega como checked=true via radix). Uma única notificação.
   *
   * `whole` marca a operação como "o kind inteiro" (a raiz do kind):
   * liga o flag de dedupe do artefato bulk. Desmarcar QUALQUER coisa —
   * a raiz, um grupo ou uma folha — apaga o flag: a seleção virou parcial
   * e volta a ser um arquivo por entrada.
   */
  setMany(
    version: GameVersionId,
    kind: EntryKind,
    ids: string[],
    checked: boolean,
    whole = false
  ): void {
    if (ids.length === 0) return;
    const key = stateKey(version, kind);
    let set = this.selected.get(key);
    if (!set) {
      if (!checked) return;
      set = new Set();
      this.selected.set(key, set);
    }
    if (checked) {
      for (const id of ids) set.add(id);
      if (whole) this.complete.add(key);
    } else {
      for (const id of ids) set.delete(id);
      // Toda remoção derruba o "kind inteiro" — mesmo desmarcar a raiz.
      this.complete.delete(key);
    }
    if (set.size === 0) this.selected.delete(key);
    this.notify();
  }

  has(version: GameVersionId, kind: EntryKind, id: string): boolean {
    return this.selected.get(stateKey(version, kind))?.has(id) ?? false;
  }

  /** ids marcados do kind. Vazio = nenhum → exportação considera "tudo". */
  idsOf(version: GameVersionId, kind: EntryKind): string[] {
    return [...(this.selected.get(stateKey(version, kind)) ?? [])];
  }

  countOf(version: GameVersionId, kind: EntryKind): number {
    return this.selected.get(stateKey(version, kind))?.size ?? 0;
  }

  /**
   * true quando a seleção do kind cobre o kind INTEIRO (marcado pela raiz
   * na árvore). É o que faz a exportação deduplicar para o artefato bulk
   * único com prefixo `tree_`.
   */
  completeOf(version: GameVersionId, kind: EntryKind): boolean {
    return this.complete.has(stateKey(version, kind));
  }
}

export const exportSelection = new ExportSelectionStore();

/** Snapshot reativo da seleção (rerender ao marcar/desmarcar). */
export function useExportSelection(): SelectionSnapshot {
  const snapshot = useSelector(exportStore);

  // Hydration-safe: localStorage só existe no cliente. setFormat cedo-retorna
  // quando o formato já é o mesmo, então chamadas repetidas (remount da
  // aba) são no-op.
  useEffect(() => {
    exportSelection.setFormat(loadStoredFormat());
  }, []);

  return snapshot;
}
