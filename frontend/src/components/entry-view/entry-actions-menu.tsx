'use client';

import { Copy, Download, FolderOpen, HardDriveDownload, Trash2 } from 'lucide-react';
import { type ReactNode, useEffect, useState } from 'react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu';
import type { EntryMenuTarget } from '@/lib/ffx/content-tree/menu-target';
import { parseError } from '@/lib/ffx/error-handler';
import { EXPORT_FORMAT_LABELS, exportSelection } from '@/lib/ffx/export-selection';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { imageDuplicates, revealEntry } from '@/lib/ffx/tree-data';
import type { EntryView } from './entry-view-store';

/**
 * Menu de contexto compartilhado da árvore e do painel de imagem — MESMO
 * menu nos dois lugares.
 *
 * Exportar (imagem: o diálogo de extração; texto: o que o chamador marcou),
 * Abrir até o arquivo e Deletar vêm em todo kind; Replicar só aparece para
 * imagem e Deletar só HABILITA para imagem (apagar binário de texto não tem
 * escopo). O alvo é o NÓ CLICADO, não a entry selecionada.
 *
 * Controlado pelo chamador: `target === null` = fechado. As cópias do alvo
 * vêm do painel quando ele as tem (`initialDuplicates`), senão são buscadas
 * aqui num binding leve (índice memoizado, sem decode de imagem) — é o que
 * habilita Replicar e põe a contagem no rótulo.
 */
export function EntryActionsMenu({
  view,
  target,
  onOpenChange,
  initialId,
  initialDuplicates,
  onExport,
  onVbfExtract,
  onVbfExtractImages,
  onVbfExport,
  selectedImageCount = 0,
  onExtractImageSelection,
  onDeleteImageSelection,
  children,
}: {
  view: EntryView;
  target: EntryMenuTarget | null;
  onOpenChange: (open: boolean) => void;
  /** Cópias já conhecidas pelo chamador (id a que pertencem). */
  initialId?: string | null;
  initialDuplicates?: dto.ImageDuplicate[] | null;
  /** Exportar de TEXTO — a lógica de seleção/formato fica no chamador. */
  onExport?: () => void;
  /** Extrai os caminhos marcados do container em data/ ou pasta escolhida. */
  onVbfExtract?: (mode: 'data' | 'choose') => void;
  /** Extrai DDS/PNG das imagens VBF selecionadas. */
  onVbfExtractImages?: (mode: 'default' | 'choose') => void;
  /** Exporta em JSON/.strings os caminhos marcados no container. */
  onVbfExport?: () => void;
  /** Operações em lote na seleção de imagens da árvore data/. */
  selectedImageCount?: number;
  onExtractImageSelection?: () => void;
  onDeleteImageSelection?: () => void;
  children: ReactNode;
}) {
  const { version, actions } = view;
  const [fetched, setFetched] = useState<{
    id: string;
    list: dto.ImageDuplicate[];
  } | null>(null);

  const targetId = target?.id ?? null;
  const isVbf = Boolean(target?.vbfRoot);
  const isImage = target?.kind === 'images';

  const duplicates =
    initialId != null && initialId === targetId && initialDuplicates
      ? initialDuplicates
      : fetched && fetched.id === targetId
        ? fetched.list
        : null;

  useEffect(() => {
    if (!targetId || !isImage) return;
    if (initialId != null && initialId === targetId && initialDuplicates) return;
    if (fetched?.id === targetId) return;
    let dead = false;
    imageDuplicates(targetId, version)
      .then((res) => {
        if (!dead) setFetched({ id: targetId, list: res.duplicates ?? [] });
      })
      .catch((error) => {
        if (dead) return;
        // Sem índice o menu não some: segue com "sem cópias" (Replicar
        // desabilitado) e o erro aparece.
        setFetched({ id: targetId, list: [] });
        toast.error(parseError(error));
      });
    return () => {
      dead = true;
    };
  }, [targetId, isImage, initialId, initialDuplicates, fetched, version]);

  const canFile = targetId !== null;
  const canExtract = isImage && canFile;
  const loading = canExtract && duplicates === null;
  const copies = duplicates?.length ?? 0;
  const canReplicate = canExtract && !loading && copies > 0;
  const canDelete = isImage && canFile;

  const openAction = (type: 'extract' | 'replicate' | 'delete') => {
    if (!targetId || !target) return;
    actions.openImageAction({
      type,
      id: targetId,
      label: target.label,
      duplicates,
    });
  };

  const onReveal = () => {
    if (!targetId || !target?.kind) return;
    void revealEntry(target.kind, targetId, version)
      .then(() => toast.success(`Exibido no Explorer: ${target.label}`))
      .catch((error) => toast.error(parseError(error)));
  };

  const replicateLabel = loading
    ? 'Carregando cópias…'
    : copies > 0
      ? `Replicar para ${copies} cópia${copies > 1 ? 's' : ''}`
      : 'Replicar';

  return (
    <ContextMenu open={target !== null} onOpenChange={onOpenChange}>
      {/* `asChild` encaminha ao FILHO as props do trigger — entre elas o
          `onContextMenu` que grava a posição do clique e chama
          preventDefault(). O filho precisa, portanto, ser (ou renderizar) um
          ELEMENTO DOM que repasse props: um componente que só ignora props
          desconhecidas quebra o posicionamento e libera o menu do navegador. */}
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>
        {target?.label ? <ContextMenuLabel>{target.label}</ContextMenuLabel> : null}
        {isVbf ? (
          <>
            <ContextMenuItem onSelect={() => onVbfExtract?.('data')}>
              <HardDriveDownload size={16} />
              Extrair binário em data/
            </ContextMenuItem>
            <ContextMenuItem onSelect={() => onVbfExtract?.('choose')}>
              <FolderOpen size={16} />
              Extrair para…
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem onSelect={() => onVbfExtractImages?.('default')}>
              <Download size={16} />
              Extrair imagens (.dds + .png)
            </ContextMenuItem>
            <ContextMenuItem onSelect={() => onVbfExtractImages?.('choose')}>
              <FolderOpen size={16} />
              Extrair imagens para…
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem onSelect={onVbfExport}>
              <Download size={16} />
              Extrair texto ({EXPORT_FORMAT_LABELS[exportSelection.formatOf()]})
            </ContextMenuItem>
          </>
        ) : null}
        {!isVbf ? (
          <>
            <ContextMenuItem
              disabled={isImage && !canExtract}
              onSelect={() => (isImage ? openAction('extract') : onExport?.())}
            >
              <Download size={16} />
              Exportar
            </ContextMenuItem>
            <ContextMenuItem disabled={!canFile} onSelect={onReveal}>
              <FolderOpen size={16} />
              Abrir até o arquivo
            </ContextMenuItem>
            {isImage ? (
              <>
                <ContextMenuItem disabled={!canReplicate} onSelect={() => openAction('replicate')}>
                  <Copy size={16} />
                  {replicateLabel}
                </ContextMenuItem>
                <ContextMenuSeparator />
              </>
            ) : null}
            {isImage && selectedImageCount > 0 ? (
              <>
                <ContextMenuItem onSelect={onExtractImageSelection}>
                  <Download size={16} />
                  Extrair {selectedImageCount}{' '}
                  {selectedImageCount === 1 ? 'imagem selecionada' : 'imagens selecionadas'}
                </ContextMenuItem>
                <ContextMenuItem variant="destructive" onSelect={onDeleteImageSelection}>
                  <Trash2 size={16} />
                  Deletar {selectedImageCount}{' '}
                  {selectedImageCount === 1 ? 'imagem selecionada' : 'imagens selecionadas'}
                </ContextMenuItem>
              </>
            ) : null}
            <ContextMenuItem
              variant="destructive"
              disabled={!canDelete}
              onSelect={() => openAction('delete')}
            >
              <Trash2 size={16} />
              Deletar
            </ContextMenuItem>
          </>
        ) : null}
      </ContextMenuContent>
    </ContextMenu>
  );
}
