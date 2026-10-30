'use client';

import { useSelector } from '@tanstack/react-store';
import { RefreshCw } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { menuTargetOf, type TreeCtxNode } from '@/lib/ffx/content-tree/menu-target';
import { buildContentRoots, highlightedNodeId, type SideNode } from '@/lib/ffx/content-tree/model';
import type { EntryKind } from '@/lib/ffx/display-names';
import { useExportSelection } from '@/lib/ffx/export-selection';
import { openSearchDialog } from '@/lib/ffx/search-store';
import { entryKindsFor, imageKindsFor } from '@/lib/ffx/tree-data';
import { useVbfSelection } from '@/lib/ffx/vbf-selection';
import { EntryActionsMenu } from '../entry-actions-menu';
import type { EntryView } from '../entry-view-store';
import { SearchDialog } from '../search/search-dialog';
import { SearchControl } from './search-control';
import { ContentTreeProvider, type ContentTreeValue } from './tree-context';
import { TreeList } from './tree-list';
import { useContentTreeActions } from './use-content-tree-actions';
import { useContentTreeHotkeys } from './use-content-tree-hotkeys';
import { VbfExtractDialog, type VbfExtractRequest } from './vbf-extract-dialog';

/**
 * Sidebar da aba: árvore de kinds/grupos/arquivos com expandir, seleção de
 * exportação (checkbox tri-state), menu de contexto (Exportar / Abrir até o
 * arquivo / Deletar) e navegação por teclado. A busca vive no MODAL
 * (SearchControl é só o gatilho visual + Ctrl+K) e um resultado clicado
 * revela o arquivo aqui (revealNodePath + scroll).
 *
 * O estado da árvore vive no store da view; aqui ficam só o que é local
 * (nó do menu e requisição de extração) e a projeção desses dados para a
 * árvore. Operações => use-content-tree-actions, teclado =>
 * use-content-tree-hotkeys, render => tree-list.
 */
export function ContentTree({ view }: { view: EntryView }) {
  const { version, store, actions } = view;
  const roots = useSelector(store, (s) => s.roots);
  const vbfRoots = useSelector(store, (s) => s.vbfRoots);
  const vbfStatus = useSelector(store, (s) => s.vbfStatus);
  const expanded = useSelector(store, (s) => s.expanded);
  const selectedEntry = useSelector(store, (s) => s.selectedEntry);
  const image = useSelector(store, (s) => s.image);
  const loading = useSelector(store, (s) => s.loading);
  const revealedNodeId = useSelector(store, (s) => s.revealedNodeId);
  // Projeção PURA das raízes: sem filtro de busca — a árvore é estável e o
  // reveal do modal só mexe no `expanded`.
  const contentRoots = useMemo(() => buildContentRoots(roots), [roots]);

  const [ctxNode, setCtxNode] = useState<TreeCtxNode | null>(null);
  const [extractRequest, setExtractRequest] = useState<VbfExtractRequest | null>(null);
  const closeExtractDialog = useCallback(() => setExtractRequest(null), []);
  // Container da árvore: consulta os nós visíveis para o foco por setas e é o
  // ALVO dos hotkeys — o listener só vê eventos que borbulham por aqui (foco
  // dentro da árvore), sem gate por `enabled` que ficaria obsoleto no foco.
  const treeRef = useRef<HTMLDivElement>(null);

  const selection = useExportSelection();
  const vbfSelectionSnapshot = useVbfSelection();
  // Seleção de exportação por kind (checkbox tri-state + menu de contexto).
  // Depende de `byKind`, não do snapshot inteiro: o store preserva a
  // identidade do mapa quando só o formato muda, então trocar JSON/Strings
  // não reconstrói os Set por kind nem a lista de imagens marcadas.
  const selectedByKind = useMemo(() => {
    const out = new Map<EntryKind, ReadonlySet<string>>();
    for (const kind of [...entryKindsFor(version), ...imageKindsFor(version)]) {
      out.set(kind, new Set(selection.byKind.get(`${version}|${kind}`) ?? []));
    }
    return out;
  }, [selection.byKind, version]);
  const selectedImageIds = useMemo(
    () => [...(selection.byKind.get(`${version}|images`) ?? [])],
    [selection.byKind, version],
  );
  // Alvo do menu: o nó clicado (folha leva o id; grupo/raiz, não).
  const ctxTarget = useMemo(() => menuTargetOf(ctxNode), [ctxNode]);
  const selectedId = useMemo(() => highlightedNodeId(selectedEntry, image), [selectedEntry, image]);

  const treeActions = useContentTreeActions({
    view,
    ctxNode,
    selectedEntry,
    selectedImageIds,
    setCtxNode,
    openExtractRequest: setExtractRequest,
  });

  useContentTreeHotkeys({
    view,
    treeRef,
    selectedImageIds,
    deleteSelectedImages: treeActions.onDeleteSelectedImages,
  });

  const onSelect = useCallback((node: SideNode) => void actions.selectNode(node), [actions]);
  // O valor do contexto é MEMOIZADO: sem isso todo item da árvore re-renderiza
  // a cada tecla de estado da view (o provider antigo era remontado a cada
  // render — known issue registrada no tree-context).
  const treeValue: ContentTreeValue = useMemo(
    () => ({
      expanded,
      selectedId,
      selectedByKind,
      vbfSelectionByRoot: vbfSelectionSnapshot.byRoot,
      onToggle: actions.toggleNode,
      onSelect,
      onCheck: treeActions.checkNode,
    }),
    [
      expanded,
      selectedId,
      selectedByKind,
      vbfSelectionSnapshot.byRoot,
      actions,
      onSelect,
      treeActions.checkNode,
    ],
  );

  // Reveal do modal: o caminho já entrou no `expanded` no mesmo patch, então
  // o nó está no DOM — só rolar até ele e limpar o pedido.
  useEffect(() => {
    if (!revealedNodeId) return;
    const attribute = `[data-node-id="${revealedNodeId.replace(/["\\]/g, '\\$&')}"]`;
    treeRef.current?.querySelector<HTMLElement>(attribute)?.scrollIntoView({ block: 'nearest' });
    actions.consumeRevealedNode();
  }, [revealedNodeId, actions]);

  return (
    // A largura vem do ResizablePanel (o `defaultSize`/layout salvo fica no
    // grupo); o `border-r` saiu daqui porque a própria divisória é o handle.
    <aside className="h-full w-full p-2 flex flex-col min-h-0">
      <div className="flex items-center justify-between font-semibold px-2">
        <span className="text-primary text-lg font-malva-medium">Conteúdo</span>
        <Button
          variant="ghost"
          size="icon"
          disabled={loading}
          onClick={() => void actions.coldReload()}
          aria-label="Recarregar"
        >
          <RefreshCw size={20} className={loading ? 'animate-spin' : undefined} />
        </Button>
      </div>
      <SearchControl onOpen={() => openSearchDialog(version)} />
      <EntryActionsMenu
        view={view}
        target={ctxTarget}
        onOpenChange={(open) => {
          if (!open) setCtxNode(null);
        }}
        onExport={() => void treeActions.onCtxExport()}
        selectedImageCount={selectedImageIds.length}
        onExtractImageSelection={() => void treeActions.onExtractSelectedImages()}
        onDeleteImageSelection={treeActions.onDeleteSelectedImages}
        onVbfExtract={treeActions.onVbfExtract}
        onVbfExtractImages={treeActions.onVbfExtractImages}
        onVbfExport={treeActions.onVbfExport}
      >
        {/* O filho direto do ContextMenuTrigger precisa ser ELEMENTO DOM: o
            `asChild` do Radix encaminha a ele o `onContextMenu` que grava a
            posição do clique e cancela o menu do navegador. ContentTreeProvider
            não é DOM e descarta essas props — sem este wrapper o menu abre em
            (0,0) e o menu nativo aparece por cima. */}
        <div className="flex min-h-0 flex-1 flex-col">
          <ContentTreeProvider value={treeValue}>
            <TreeList
              treeRef={treeRef}
              version={version}
              vbfRoots={vbfRoots}
              vbfStatus={vbfStatus}
              roots={contentRoots}
              onNodeContextMenu={treeActions.onNodeContextMenu}
            />
          </ContentTreeProvider>
        </div>
      </EntryActionsMenu>
      <VbfExtractDialog
        key={extractRequest?.requestId ?? 'closed'}
        request={extractRequest}
        onClose={closeExtractDialog}
        onDone={() => {
          setExtractRequest(null);
          void actions.coldReload();
        }}
      />
      <SearchDialog view={view} />
    </aside>
  );
}
