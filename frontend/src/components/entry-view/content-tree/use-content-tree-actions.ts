'use client';

import { useCallback, useMemo, useRef, type MouseEvent } from 'react';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import {
  exportArtifactBatch,
  extractImageSelection,
} from '@/lib/ffx/tree-data';
import {
  exportVbfSelection,
  extractVbfImagesSelection,
  invalidateVbfImage,
  selectVbfImageExtractDir,
} from '@/lib/ffx/vbf';
import { parseError } from '@/lib/ffx/error-handler';
import { progressWasShown } from '@/lib/ffx/progress-store';
import { exportSelection } from '@/lib/ffx/export-selection';
import { exportLangs } from '@/lib/ffx/export-langs';
import { vbfSelection } from '@/lib/ffx/vbf-selection';
import {
  entryIdsInNode,
  isKindRootNode,
  type SideNode,
} from '@/lib/ffx/content-tree/model';
import type { TreeCtxNode } from '@/lib/ffx/content-tree/menu-target';
import type { EntryRow } from '@/lib/ffx/tree-data';
import type { EntryView } from '../entry-view-store';
import type { VbfExtractRequest } from './vbf-extract-dialog';

/**
 * Operações da árvore: marcar (checkbox), exportar, extrair e apagar.
 *
 * Saiu do componente porque nada disso é UI — tudo escreve em store, chama o
 * backend ou monta toast. Lê o estado de que precisa por parâmetro, então o
 * ContentTree continua sendo o único dono das assinaturas.
 */
export function useContentTreeActions({
  view,
  ctxNode,
  selectedEntry,
  selectedImageIds,
  setCtxNode,
  openExtractRequest,
}: {
  view: EntryView;
  ctxNode: TreeCtxNode | null;
  selectedEntry: EntryRow | null;
  selectedImageIds: string[];
  setCtxNode: (node: TreeCtxNode | null) => void;
  openExtractRequest: (request: VbfExtractRequest) => void;
}) {
  const { version, actions } = view;
  const extractRequestCounter = useRef(0);

  // O menu do .vbf opera somente sobre os caminhos marcados no container
  // clicado — o nó sob o cursor apenas determina qual container recebe o
  // menu, não troca a seleção por esse nó.
  const vbfPathsForContext = useMemo(
    () =>
      ctxNode?.vbfRoot
        ? (ctxNode.selectedVbfPaths ?? vbfSelection.pathsOf(ctxNode.vbfRoot))
        : [],
    [ctxNode]
  );

  // Checkbox de data/: folha = 1 id; grupo = todos os filhos; raiz do kind
  // = o kind INTEIRO (`whole`), que deduplica a exportação para o artefato
  // bulk único.
  // Checkbox do .vbf: guarda o caminho (diretório = subárvore, expandida
  // pelo backend); inclui imagens e formatos fora do escopo de texto para
  // permitir a extração dos binários originais.
  const checkNode = useCallback(
    (node: SideNode, checked: boolean) => {
      if (node.vbf) {
        const root = node.vbfRoot ?? node.entry?.vbf?.root;
        const path = node.vbfPath ?? node.entry?.vbf?.path ?? '';
        if (root) vbfSelection.set(root, path, checked);
        return;
      }
      if (!node.kind) return;
      // A raiz do kind (kind:<kind>) cobre tudo que a árvore serve dele:
      // marcar ela é marcar o kind inteiro — daí o `whole`.
      const whole = isKindRootNode(node) && checked;
      exportSelection.setMany(
        version,
        node.kind,
        entryIdsInNode(node),
        checked,
        whole
      );
    },
    [version]
  );

  const onExtractSelectedImages = useCallback(async () => {
    if (selectedImageIds.length === 0) {
      toast.message('Nenhuma imagem marcada para extrair.');
      return;
    }
    try {
      const result = await extractImageSelection(selectedImageIds, version);
      if (result.failed.length > 0) {
        toast.warning(`Extraídas ${result.done.length} de ${result.total} imagens.`, {
          description: result.failed.slice(0, 3).join('; '),
        });
      } else {
        toast.success(`Extraídas ${result.done.length} imagens selecionadas.`);
      }
      await actions.reload();
    } catch (error) {
      toast.error(parseError(error));
    }
  }, [actions, selectedImageIds, version]);

  const onDeleteSelectedImages = useCallback(() => {
    if (selectedImageIds.length === 0) return;
    actions.openImageAction({
      type: 'delete-selection',
      id: selectedImageIds[0],
      ids: selectedImageIds,
      label: `${selectedImageIds.length} imagens selecionadas`,
      duplicates: null,
    });
  }, [actions, selectedImageIds]);

  // Exporta só o que está marcado na árvore de data/ no FORMATO SELECIONADO
  // no topo (JSON | Strings | CSV | XLSX) — leitura imperativa do store
  // compartilhado. Uma ação = modal de progresso com o resumo no final.
  //
  // Seleção marcada pela RAIZ do kind (`complete`) deduplica para o
  // artefato bulk único (prefixo tree_ no backend): um arquivo em vez de N.
  const onCtxExport = useCallback(async () => {
    const kind = ctxNode?.kind;
    if (!kind) return;
    const ids = exportSelection.idsOf(version, kind);
    if (ids.length === 0) {
      toast.message(`Nenhum arquivo marcado para exportar (${kind}).`);
      return;
    }
    const format = exportSelection.formatOf();
    const complete = exportSelection.completeOf(version, kind);
    try {
      const paths = await exportArtifactBatch(
        [{ kind, ids, complete }],
        version,
        exportLangs.langsOf(),
        format
      );
      if ((paths ?? []).length === 0) {
        toast.info('Nada para exportar.');
      }
      // Modal de progresso mostrou o resumo — sem toast de sucesso.
    } catch (error) {
      if (!progressWasShown()) toast.error(parseError(error));
    }
  }, [ctxNode, version]);

  const onVbfExtract = useCallback(
    (mode: 'data' | 'choose') => {
      if (!ctxNode?.vbfRoot || vbfPathsForContext.length === 0) return;
      openExtractRequest({
        requestId: ++extractRequestCounter.current,
        root: ctxNode.vbfRoot,
        paths: [...vbfPathsForContext],
        mode,
      });
      setCtxNode(null);
    },
    [ctxNode, openExtractRequest, setCtxNode, vbfPathsForContext]
  );

  const onVbfExtractImages = useCallback(
    (mode: 'default' | 'choose') => {
      if (!ctxNode?.vbfRoot || vbfPathsForContext.length === 0) return;
      const root = ctxNode.vbfRoot;
      const paths = [...vbfPathsForContext];
      setCtxNode(null);
      void (async () => {
        let destRoot = '';
        if (mode === 'choose') {
          destRoot = await selectVbfImageExtractDir();
          if (!destRoot) return;
        }
        try {
          const result = await extractVbfImagesSelection(root, paths, destRoot);
          if (result.total === 0) {
            toast.info('A seleção não contém imagens compatíveis para extrair.');
          } else if (result.failed.length > 0) {
            toast.warning(
              `Extraídas ${result.done.length} de ${result.total} imagens.`,
              { description: result.failed.slice(0, 3).join('; ') }
            );
          } else {
            toast.success(`Extraídas ${result.done.length} imagens do .vbf.`);
          }
          if (selectedEntry?.vbf && result.done?.includes(selectedEntry.vbf.path)) {
            invalidateVbfImage(selectedEntry.vbf.root, selectedEntry.vbf.path);
            await actions.selectEntry(selectedEntry);
          }
        } catch (error) {
          toast.error(parseError(error));
        }
      })();
    },
    [actions, ctxNode, selectedEntry, setCtxNode, vbfPathsForContext]
  );

  const onVbfExport = useCallback(() => {
    if (!ctxNode?.vbfRoot || vbfPathsForContext.length === 0) return;
    const root = ctxNode.vbfRoot;
    const paths = [...vbfPathsForContext];
    setCtxNode(null);
    void (async () => {
      const format = exportSelection.formatOf();
      try {
        const exported = await exportVbfSelection(root, format, paths, exportLangs.langsOf());
        if (exported.length === 0) {
          toast.info('A seleção não contém textos compatíveis para extrair.');
          return;
        }
        // O backend agrupa por tipo com um modal de progresso por ação —
        // o resumo (arquivos + destino) aparece lá, sem toast paralelo.
      } catch (error) {
        if (!progressWasShown()) toast.error(parseError(error));
      }
    })();
  }, [ctxNode, setCtxNode, vbfPathsForContext]);

  /**
   * Clique direito na árvore → estado do menu. O atributo `data-node-id`
   * decide o alvo; no .vbf a seleção congelada no clique tem prioridade
   * sobre a marcação atual, e sem marcação o menu opera só no arquivo
   * clicado (uma pasta precisa ser marcada para expandir sua subárvore).
   */
  const onNodeContextMenu = useCallback(
    (event: MouseEvent) => {
      const row = (event.target as HTMLElement).closest('[data-node-id]');
      if (!(row instanceof HTMLElement)) {
        setCtxNode(null);
        return;
      }
      const kind = row.getAttribute('data-node-kind') as
        | TreeCtxNode['kind']
        | null;
      const isVbf = row.getAttribute('data-node-vbf') === 'true';
      const id = row.getAttribute('data-node-id') ?? '';
      const label = row.getAttribute('data-node-label') ?? '';
      if (isVbf) {
        const vbfRoot = row.getAttribute('data-node-vbf-root') ?? '';
        const selectedVbfPaths = vbfSelection.pathsOf(vbfRoot);
        const clickedPath = row.getAttribute('data-node-vbf-path') ?? '';
        const isFile = row.getAttribute('data-node-vbf-file') === 'true';
        const paths =
          vbfSelection.countOf(vbfRoot) > 0
            ? selectedVbfPaths
            : isFile && clickedPath
              ? [clickedPath]
              : [];
        setCtxNode(
          vbfRoot && paths.length > 0
            ? { id, label, vbfRoot, vbfPath: clickedPath, selectedVbfPaths: paths }
            : null
        );
        return;
      }
      setCtxNode(kind ? { id, kind, label } : null);
    },
    [setCtxNode]
  );

  return {
    checkNode,
    onCtxExport,
    onExtractSelectedImages,
    onDeleteSelectedImages,
    onVbfExtract,
    onVbfExtractImages,
    onVbfExport,
    onNodeContextMenu,
  };
}
