'use client';

import { useSelector } from '@tanstack/react-store';
import type { EntryView } from '@/components/entry-view/entry-view-store';
import { DeleteImageDialog } from './image-delete-dialog';
import { DeleteImageSelectionDialog } from './image-delete-selection-dialog';
import { ExtractImageDialog } from './image-extract-dialog';
import { ReplicateImageDialog } from './image-replicate-dialog';

/**
 * Diálogos das ações de imagem (extrair / replicar / deletar).
 *
 * Um só ponto de renderização, na aba (game-version-tab): menu da árvore e
 * botões do painel apenas ABREM a ação no store — assim a ação serve ao nó
 * clicado, mesmo que não seja a entry selecionada, e não existe diálogo
 * duplicado por componente.
 */
export function ImageActionDialogs({ view }: { view: EntryView }) {
  const action = useSelector(view.store, (s) => s.imageAction);
  if (!action) return null;
  switch (action.type) {
    case 'extract':
      return <ExtractImageDialog view={view} action={action} />;
    case 'replicate':
      return <ReplicateImageDialog view={view} action={action} />;
    case 'delete-selection':
      return (
        <DeleteImageSelectionDialog
          key={`${view.version}:${action.ids?.join('\0') ?? ''}`}
          view={view}
          action={action}
        />
      );
    default:
      return <DeleteImageDialog view={view} action={action} />;
  }
}
