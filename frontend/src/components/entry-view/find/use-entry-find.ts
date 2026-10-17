'use client';

import { useCallback, useDeferredValue, useMemo } from 'react';
import { useSelector } from '@tanstack/react-store';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { useEditDraft, rowKey } from '@/lib/ffx/edit-draft';
import { SOURCE_LANG } from '@/lib/ffx/save-all';
import { locFromVbfPath } from '@/lib/ffx/tree-data';
import {
  findMatches,
  replaceMatches,
  type FindRange,
} from '@/lib/ffx/find-in-text';
import {
  displayedTranslatedText,
  translatedEditTarget,
  type TranslatedEditTarget,
  type TranslatedTextDeps,
} from '@/lib/ffx/translated-text';
import type { GameTextMark } from '@/lib/ffx/game-text-parser';
import {
  closeFileFind,
  findStoreFor,
  openFileFind,
  setFindActiveIndex,
  setFindQuery,
  setFindReplaceText,
  toggleFindMatchCase,
  toggleFindReplace,
  toggleFindWholeWord,
} from '@/lib/ffx/find-store';
import type { EntryView } from '../entry-view-store';

/** Um match da busca local: a linha da tabela + o intervalo no texto dela. */
export interface EntryFindMatch {
  rowKey: string;
  range: FindRange;
}

/** O que a barra de busca e a tabela consomem do find da aba. */
export interface EntryFind {
  open: boolean;
  query: string;
  replaceText: string;
  showReplace: boolean;
  matchCase: boolean;
  wholeWord: boolean;
  /** Matches na ordem das rows (e, dentro da row, do texto). */
  matches: EntryFindMatch[];
  /** Destaque por linha (o ativo leva current=find-current). */
  matchesByRow: Map<string, GameTextMark[]>;
  /** Match ativo (null = nenhum). */
  activeMatch: EntryFindMatch | null;
  activeIndex: number;
  hasQuery: boolean;
  // ações de estado
  openFind: () => void;
  closeFind: () => void;
  setQuery: (query: string) => void;
  setReplaceText: (text: string) => void;
  toggleReplace: () => void;
  toggleMatchCase: () => void;
  toggleWholeWord: () => void;
  // navegação
  next: () => void;
  prev: () => void;
  // substituição
  replaceCurrent: () => void;
  replaceAll: () => Promise<void>;
}

/**
 * Busca & substituição local no arquivo ABERTO — deriva os matches das rows
 * visíveis (texto da coluna Traduzido, incluindo defs linkadas) a partir do
 * estado da barra no find-store. É o espelho do que a tabela pinta: o
 * highlighting e a substituição usam a MESMA régua de exibição/roteamento
 * do editor (divergente > link > rascunho > texto).
 */
export function useEntryFind(view: EntryView): EntryFind {
  const { version, store, actions } = view;
  const rows = useSelector(store, (s) => s.rows);
  const selectedEntry = useSelector(store, (s) => s.selectedEntry);
  const refLinks = useSelector(store, (s) => s.refLinks);
  const loading = useSelector(store, (s) => s.loading);
  const find = useSelector(findStoreFor(version), (s) => s);
  // Rascunho em edição muda o texto exibido → recompõe os matches.
  const { snapshot } = useEditDraft();

  // Idioma do binário exibido (mesmo cálculo das colunas).
  const textLoc = selectedEntry?.vbf?.path
    ? (locFromVbfPath(selectedEntry.vbf.path) ?? SOURCE_LANG)
    : SOURCE_LANG;

  const deps: TranslatedTextDeps = useMemo(
    () => ({ version, entry: selectedEntry, refLinks, textLoc }),
    [version, selectedEntry, refLinks, textLoc]
  );

  // Query ADIADO: o cálculo de matches (que varre todas as rows e re-renderiza
  // a tabela com <mark>) roda numa renderização de baixa prioridade, então
  // digitando rápido o input nunca espera a tabela. O React reintegra o valor
  // quando a digitação trava. (startTransition não serviria: find.query é um
  // store externo e o useSelector re-renderiza de forma urgente.)
  const deferredQuery = useDeferredValue(find.query);
  const hasQuery = deferredQuery.trim() !== '';

  const matches = useMemo<EntryFindMatch[]>(() => {
    if (!find.open || !hasQuery || !selectedEntry || loading) return [];
    const options = { matchCase: find.matchCase, wholeWord: find.wholeWord };
    const out: EntryFindMatch[] = [];
    for (const row of rows) {
      const text = displayedTranslatedText(row, deps);
      if (!text) continue;
      const key = rowKey(row);
      for (const range of findMatches(text, deferredQuery, options)) {
        out.push({ rowKey: key, range });
      }
    }
    return out;
  }, [
    find.open,
    hasQuery,
    deferredQuery,
    find.matchCase,
    find.wholeWord,
    selectedEntry,
    loading,
    rows,
    deps,
    snapshot.revision,
  ]);

  // Índice ativo limitado ao tamanho real (a lista muda a cada substituição).
  const activeIndex =
    matches.length === 0
      ? 0
      : Math.min(find.activeIndex, matches.length - 1);

  const activeMatch = matches.length > 0 ? (matches[activeIndex] ?? null) : null;

  const matchesByRow = useMemo(() => {
    const map = new Map<string, GameTextMark[]>();
    if (matches.length === 0) return map;
    matches.forEach((match, index) => {
      const list = map.get(match.rowKey) ?? [];
      list.push({
        start: match.range.start,
        end: match.range.end,
        current: index === activeIndex,
      });
      map.set(match.rowKey, list);
    });
    return map;
  }, [matches, activeIndex]);

  const next = useCallback(() => {
    if (matches.length === 0) return;
    setFindActiveIndex(version, (activeIndex + 1) % matches.length);
  }, [matches.length, activeIndex, version]);

  const prev = useCallback(() => {
    if (matches.length === 0) return;
    setFindActiveIndex(version, (activeIndex - 1 + matches.length) % matches.length);
  }, [matches.length, activeIndex, version]);

  /** Substitui o match ATIVO (ou o último ativo, se a lista encolheu). */
  const replaceCurrent = useCallback(() => {
    const match = matches[activeIndex];
    if (!match || !selectedEntry) return;
    const row = rows.find((r) => rowKey(r) === match.rowKey);
    if (!row) return;
    const target = translatedEditTarget(row, deps);
    if (!target) return;
    // O texto casado é o EXIBIDO da row (link resolve a def; self resolve a
    // própria row) — é nele que o intervalo foi encontrado.
    const currentText = displayedTranslatedText(row, deps);
    const newText = replaceMatches(currentText, [match.range], find.replaceText);
    if (newText === currentText) return;
    void actions.ensureCellEditBase(target).then((ok) => {
      if (!ok) return;
      actions.applyCellEdit(target, target.row, target.lang, newText);
      // A lista recompõe sozinha; o índice aponta para o próximo match
      // (comportamento do VSCode: segue para a próxima ocorrência).
    });
  }, [matches, activeIndex, selectedEntry, rows, deps, find.replaceText, actions]);

  /** Substitui TODOS os matches, agrupando por alvo (defs linkadas = 1×). */
  const replaceAll = useCallback(async () => {
    if (matches.length === 0 || !selectedEntry) return;
    interface Group {
      target: TranslatedEditTarget;
      row: dto.TextRow;
      lang: string;
      text: string;
      ranges: FindRange[];
    }
    const groups = new Map<string, Group>();
    for (const match of matches) {
      const row = rows.find((r) => rowKey(r) === match.rowKey);
      if (!row) continue;
      const target = translatedEditTarget(row, deps);
      if (!target) continue;
      // Cópias linkadas da mesma DEF (e a própria DEF) caibam num ÚNICO
      // alvo: a def é editada uma vez só, com todos os seus matches.
      const key = `${target.kind}|${target.id}|${target.source}|${target.row.index}:${target.row.name ?? ''}`;
      const group = groups.get(key);
      if (group) {
        // O link exibe o MESMO texto da def → ranges idênticos; deduplica.
        if (
          !group.ranges.some(
            (r) => r.start === match.range.start && r.end === match.range.end
          )
        ) {
          group.ranges.push(match.range);
        }
      } else {
        groups.set(key, {
          target,
          row: target.row,
          lang: target.lang,
          text: displayedTranslatedText(row, deps),
          ranges: [match.range],
        });
      }
    }
    if (groups.size === 0) return;

    // Prepara as bases (defs em outra entrada) ANTES de gravar qualquer
    // coisa — se alguma falhar, aborta sem aplicar nada.
    for (const group of groups.values()) {
      if (!(await actions.ensureCellEditBase(group.target))) return;
    }

    // Snapshot do texto ANTES (para o desfazer) + aplica de uma vez.
    const snapshot = [...groups.values()].map((group) => ({
      target: group.target,
      row: group.row,
      lang: group.lang,
      before: group.text,
    }));
    let replaced = 0;
    for (const group of groups.values()) {
      const sorted = group.ranges.slice().sort((a, b) => a.start - b.start);
      const newText = replaceMatches(group.text, sorted, find.replaceText);
      if (newText === group.text) continue;
      actions.applyCellEdit(group.target, group.row, group.lang, newText);
      replaced += sorted.length;
    }
    setFindActiveIndex(version, 0);
    if (replaced === 0) return;
    toast.success(
      `Substituídas ${replaced} ocorrência(s) em ${groups.size} linha(s).`,
      {
        duration: 8000,
        action: {
          label: 'Desfazer',
          onClick: () => {
            for (const item of snapshot) {
              actions.applyCellEdit(item.target, item.row, item.lang, item.before);
            }
          },
        },
      }
    );
  }, [matches, selectedEntry, rows, deps, find.replaceText, actions, version]);

  return {
    open: find.open,
    query: find.query,
    replaceText: find.replaceText,
    showReplace: find.showReplace,
    matchCase: find.matchCase,
    wholeWord: find.wholeWord,
    matches,
    matchesByRow,
    activeMatch,
    activeIndex,
    hasQuery,
    openFind: () => openFileFind(version),
    closeFind: () => closeFileFind(version),
    setQuery: (query) => setFindQuery(version, query),
    setReplaceText: (text) => setFindReplaceText(version, text),
    toggleReplace: () => toggleFindReplace(version),
    toggleMatchCase: () => toggleFindMatchCase(version),
    toggleWholeWord: () => toggleFindWholeWord(version),
    next,
    prev,
    replaceCurrent,
    replaceAll,
  };
}
