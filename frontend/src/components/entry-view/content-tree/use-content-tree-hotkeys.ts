'use client';

import { useHotkey, useHotkeys } from '@tanstack/react-hotkeys';
import { useSelector } from '@tanstack/react-store';
import { type RefObject, useMemo } from 'react';
import { buildContentRoots, buildNodeIndex } from '@/lib/ffx/content-tree/model';
import { SHORTCUTS } from '@/lib/ffx/shortcuts';
import type { EntryView } from '../entry-view-store';

/**
 * Teclado da sidebar — ↑/↓ movem o foco entre nós visíveis; Enter alterna
 * expandir/fechar (grupo/raiz) ou abre o arquivo na tabela (folha); → folha
 * leva o foco para a tabela (↑/↓ direto); Delete apaga a seleção de imagens.
 *
 * Assina direto o store da view: os callbacks recebem SÓ o evento, então o
 * nó é resolvido pelo DOM ([data-node-id]) e não há closure velha.
 */
export function useContentTreeHotkeys({
  view,
  treeRef,
  selectedImageIds,
  deleteSelectedImages,
}: {
  view: EntryView;
  treeRef: RefObject<HTMLDivElement | null>;
  selectedImageIds: string[];
  deleteSelectedImages: () => void;
}) {
  const { store, actions } = view;
  const roots = useSelector(store, (s) => s.roots);
  const contentRoots = useMemo(() => buildContentRoots(roots), [roots]);
  const vbfRoots = useSelector(store, (s) => s.vbfRoots);
  const expanded = useSelector(store, (s) => s.expanded);
  const selectedEntry = useSelector(store, (s) => s.selectedEntry);
  const image = useSelector(store, (s) => s.image);
  const imageAction = useSelector(store, (s) => s.imageAction);

  // Índice id → nó: devolvido pelo [data-node-id] do evento.
  const nodeById = useMemo(() => buildNodeIndex(contentRoots, vbfRoots), [contentRoots, vbfRoots]);

  /**
   * Botão de nó sob o foco. `null` = chevron/checkbox/viewport: esses casos
   * ficam com o comportamento nativo do botão (Enter alterna de verdade).
   */
  const focusedNodeButton = (event: KeyboardEvent) =>
    event.target instanceof Element
      ? event.target.closest<HTMLElement>('[data-node-button]')
      : null;

  const moveFocus = (event: KeyboardEvent, delta: number) => {
    const button = focusedNodeButton(event);
    const buttons = Array.from(
      treeRef.current?.querySelectorAll<HTMLElement>('[data-node-button]') ?? [],
    );
    const idx = button ? buttons.indexOf(button) : -1;
    if (idx < 0) return;
    event.preventDefault();
    buttons[idx + delta]?.focus();
  };

  const activate = (event: KeyboardEvent, mode: 'enter' | 'right') => {
    const button = focusedNodeButton(event);
    if (!button) return;
    const id = button.closest('[data-node-id]')?.getAttribute('data-node-id');
    const node = id ? nodeById.get(id) : undefined;
    if (!node) return;
    // Folha abre; arquivo fora do escopo avisa; grupo/raiz alterna.
    // `preventDefault: false` no registro: o cancelamento é manual e SÓ aqui,
    // para matar o click nativo do <button> sem matar Enter no checkbox/chevron.
    event.preventDefault();
    if (mode === 'right') {
      if (node.entry) {
        // Folha: abre o arquivo e já leva o foco para a tabela (↑/↓ direto).
        actions.requestTableFocus();
        void actions.selectNode(node);
      } else if (node.unsupported) {
        void actions.selectNode(node);
      } else if (!expanded.has(node.id)) {
        // Grupo/raiz: expande se colapsado (convenção de treeview).
        actions.toggleNode(node);
      }
      return;
    }
    if (node.entry || node.unsupported) void actions.selectNode(node);
    else actions.toggleNode(node);
  };

  // Callbacks e options são sincronizados a cada render pelo hook, então não
  // há closure velha de `actions`/`expanded` (dispensa useCallback).
  useHotkeys(
    [
      {
        hotkey: SHORTCUTS.navigation.down.key,
        callback: (event) => moveFocus(event, 1),
        options: { meta: { name: 'Próximo nó', group: 'Árvore' } },
      },
      {
        hotkey: SHORTCUTS.navigation.up.key,
        callback: (event) => moveFocus(event, -1),
        options: { meta: { name: 'Nó anterior', group: 'Árvore' } },
      },
      {
        hotkey: 'Enter',
        callback: (event) => activate(event, 'enter'),
        options: { meta: { name: 'Abrir/alternar nó', group: 'Árvore' } },
      },
      {
        hotkey: SHORTCUTS.navigation.right.key,
        callback: (event) => activate(event, 'right'),
        options: { meta: { name: 'Expandir ou ir para a tabela', group: 'Árvore' } },
      },
    ],
    // `preventDefault`/`stopPropagation` desligados: senão Enter morre no
    // checkbox/chevron e o evento deixa de subir (cortando o Delete em
    // document e os handlers sintéticos dos ancestrais).
    { target: treeRef, preventDefault: false, stopPropagation: false, ignoreInputs: true },
  );

  useHotkey(
    'Delete',
    () => {
      if (imageAction) return;
      if (selectedImageIds.length > 0) {
        deleteSelectedImages();
        return;
      }
      if (selectedEntry?.kind !== 'images' || selectedEntry.vbf) return;
      actions.openImageAction({
        type: 'delete',
        id: selectedEntry.id,
        label: selectedEntry.label,
        duplicates: image?.duplicates ?? [],
      });
    },
    {
      enabled:
        !imageAction &&
        (selectedImageIds.length > 0 || (selectedEntry?.kind === 'images' && !selectedEntry.vbf)),
      ignoreInputs: true,
    },
  );
}
