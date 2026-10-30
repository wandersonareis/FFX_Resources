'use client';

import { useHotkeys } from '@tanstack/react-hotkeys';
import { useRef, useState } from 'react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { entryNodeId } from '@/lib/ffx/content-tree/model';
import { rowKey } from '@/lib/ffx/edit-draft';
import { SHORTCUTS } from '@/lib/ffx/shortcuts';
import type { EntryRow } from '@/lib/ffx/tree-data';
import type { EntryView } from '../entry-view-store';

/**
 * Teclado e foco da tabela — devolve as refs e o estado de foco que o corpo
 * (ref callback / onFocus), o wrapper (alvo dos hotkeys) e o efeito da 1ª
 * linha pedido pela árvore compartilham com o componente pai.
 */
export function useEntryTableHotkeys({
  rows,
  refLinks,
  selectedEntry,
  actions,
}: {
  rows: dto.TextRow[];
  refLinks: Record<string, dto.RefLink>;
  selectedEntry: EntryRow | null;
  actions: EntryView['actions'];
}) {
  const [focusedRowId, setFocusedRowId] = useState<string | null>(null);
  const rowRefs = useRef(new Map<string, HTMLTableRowElement>());
  /**
   * Wrapper externo da tabela — o `<table>` da shadcn não reencaminha ref. É
   * o ALVO dos hotkeys: o listener só vê eventos que borbulham por aqui (foco
   * dentro da tabela), e o ref nulo (sem linhas servíveis) desliga o alvo.
   */
  const tableRef = useRef<HTMLDivElement>(null);

  // ---- Teclado: tabela — ↑/↓ movem o foco entre linhas; Enter abre o
  // modal do editor da linha focada; ← devolve o foco para a árvore (fecha o
  // ciclo de navegação árvore ↔ tabela).

  /**
   * Linha da tabela sob o evento — qualquer foco dentro dela (a própria `<tr>`
   * ou um botão de célula). `null` = foco fora da tabela.
   */
  const rowElOf = (event: KeyboardEvent): HTMLElement | null => {
    const target = event.target;
    if (!(target instanceof HTMLElement)) return null;
    return target.closest<HTMLElement>('[data-row-key]');
  };

  /**
   * A própria `<tr>` sob o foco — `null` quando o alvo é um botão da célula
   * (Reverter / editar como divergência). Só o Enter usa este guard: sem ele
   * o `preventDefault` mataria o click nativo do botão.
   */
  const rowFocused = (event: KeyboardEvent): HTMLElement | null => {
    const row = rowElOf(event);
    return row && row === event.target ? row : null;
  };

  /** Linhas na MESMA ordem do DOM (== ordem de `rows`, que as renderiza). */
  const rowEls = () =>
    Array.from(tableRef.current?.querySelectorAll<HTMLElement>('[data-row-key]') ?? []);

  const focusRow = (rowEl: HTMLElement | undefined) => {
    if (!rowEl) return;
    setFocusedRowId(rowEl.dataset.rowKey ?? null);
    rowEl.focus();
  };

  const moveRow = (event: KeyboardEvent, delta: number) => {
    const rowEl = rowElOf(event);
    if (!rowEl) return;
    // Consome a tecla para não rolar a página. Setas não têm uso nativo em
    // `<tr>`/`<button>`, então isso é seguro mesmo com foco na célula.
    event.preventDefault();
    const list = rowEls();
    focusRow(list[list.indexOf(rowEl) + delta]);
  };

  const openFocusedRow = (event: KeyboardEvent) => {
    // Guard estrito: só a linha consome o Enter. Nos botões internos ele
    // segue nativo (aciona o botão) em vez de abrir o editor por cima.
    const rowEl = rowFocused(event);
    if (!rowEl) return;
    event.preventDefault();
    const key = rowEl.dataset.rowKey ?? '';
    const row = rows.find((r) => rowKey(r) === key);
    if (!row) return;
    setFocusedRowId(key);
    // Row de ref dedupada: o editor abre NA DEF (através do link) — nunca em
    // cima do "$hash".
    if (refLinks[key]) actions.openLinked(row);
    else actions.openDialog(row);
  };

  const backToTree = (event: KeyboardEvent) => {
    if (!rowElOf(event)) return;
    event.preventDefault();
    // Volta o foco para o nó selecionado na árvore. Se o nó estiver
    // desmontado (grupo colapsado), cai no primeiro botão visível da árvore.
    // O id de uma folha do .vbf leva caminho absoluto de Windows (barra
    // invertida), então a comparação é pelo atributo — nunca por seletor CSS.
    const tree = document.getElementById('entry-tree');
    const nodeId = selectedEntry ? entryNodeId(selectedEntry) : '';
    const buttons = Array.from(tree?.querySelectorAll<HTMLElement>('[data-node-button]') ?? []);
    const node =
      buttons.find(
        (button) => button.closest('[data-node-id]')?.getAttribute('data-node-id') === nodeId,
      ) ?? buttons[0];
    node?.focus();
  };

  // Callbacks e options são sincronizados a cada render pelo hook, então não
  // há closure velha de `rows`/`actions` (dispensa useCallback).
  useHotkeys(
    [
      {
        hotkey: SHORTCUTS.navigation.down.key,
        callback: (event) => moveRow(event, 1),
        options: { meta: { name: 'Próxima linha', group: 'Tabela' } },
      },
      {
        hotkey: SHORTCUTS.navigation.up.key,
        callback: (event) => moveRow(event, -1),
        options: { meta: { name: 'Linha anterior', group: 'Tabela' } },
      },
      {
        hotkey: 'Enter',
        callback: openFocusedRow,
        options: { meta: { name: 'Abrir o editor da linha', group: 'Tabela' } },
      },
      {
        hotkey: SHORTCUTS.navigation.left.key,
        callback: backToTree,
        options: { meta: { name: 'Voltar para a árvore', group: 'Tabela' } },
      },
    ],
    // `preventDefault`/`stopPropagation` desligados: o cancelamento é manual
    // e só quando o alvo É a linha, e o evento precisa continuar subindo
    // (Delete em document e handlers sintéticos dos ancestres).
    { target: tableRef, preventDefault: false, stopPropagation: false, ignoreInputs: true },
  );

  return { tableRef, rowRefs, focusedRowId, setFocusedRowId };
}
