'use client';

import { useState } from 'react';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import type { EntryRow } from '@/lib/ffx/tree-data';
import {
  importImage,
  importImageGroup,
  importImages,
  previewImportImages,
  refreshImageDuplicates,
  saveImage,
  selectImageFile,
  imageExportPath,
} from '@/lib/ffx/tree-data';
import {
  extractVbfImagesSelection,
  importVbfImage,
  invalidateVbfImage,
  saveVbfImage,
  selectVbfImageExtractDir,
  vbfImageExportPath,
} from '@/lib/ffx/vbf';
import { parseError } from '@/lib/ffx/error-handler';
import type { EntryView } from '@/components/entry-view/entry-view-store';

export type ImageBusy =
  | 'extract'
  | 'extractTo'
  | 'export'
  | 'import'
  | 'importBatch'
  | 'refresh'
  | null;

/**
 * Fluxos imperativos do painel: salvar, importar (com passo de alcance
 * quando há cópias), reanalisar duplicatas e abrir ação de imagem.
 *
 * Todo o "chama binding → grava → toast → recarrega" vive aqui; o JSX fica
 * só com o que pintar. `busy` trava os botões durante a gravação e
 * `pendingImport` é o .dds já escolhido aguardando a escolha de alcance.
 */
export function useImageActions({
  view,
  entry,
  duplicates,
}: {
  view: EntryView;
  /** null = nenhuma textura selecionada (o painel não renderiza nada). */
  entry: EntryRow | null;
  duplicates: dto.ImageDuplicate[];
}) {
  const { version, actions } = view;
  const [busy, setBusy] = useState<ImageBusy>(null);
  /**
   * .dds já escolhido aguardando o usuário decidir o alcance do import
   * (null = sem diálogo aberto). Só aparece quando a imagem tem cópias.
   */
  const [pendingImport, setPendingImport] = useState<string | null>(null);

  /**
   * Import em lote de <TranslateLocation>/images: `pendingBatch` guarda o
   * resumo da varredura (null = diálogo fechado) e `pickedDirs` os
   * diretórios após ps3data que o usuário marcou — cada marcação refaz a
   * varredura para atualizar quantas texturas são novas.
   */
  const [pendingBatch, setPendingBatch] = useState<dto.ImageBatch | null>(null);
  const [pickedDirs, setPickedDirs] = useState<string[]>([]);

  /**
   * Textura aberta pelo navegador de .vbf: o container é somente leitura;
   * extrair usa o caminho interno e importar/salvar grava fora dele. Delete e
   * replicate de grupos continuam restritos à árvore data/.
   */
  const readOnly = Boolean(entry?.vbf);

  const refuseVbf = (): boolean => {
    if (!readOnly) return false;
    toast.message(
      'Exclusão e replicação estão disponíveis apenas para imagens da árvore de data/.',
      { duration: 4000 }
    );
    return true;
  };

  /**
   * Ações de imagem (extrair / replicar / deletar) abrem o diálogo único da
   * aba (ImageActionDialogs) com a textura aberta como alvo — ali ficam a
   * escolha de alcance, o aviso de irreversível e a contagem de cópias.
   */
  const openAction = (type: 'extract' | 'replicate' | 'delete') => {
    // O hook é chamado antes do painel decidir que não há textura; sem alvo
    // não há ação a abrir.
    if (!entry) return;
    if (readOnly) {
      if (type === 'extract' && entry.vbf) {
        setBusy('extract');
        void extractVbfImagesSelection(entry.vbf.root, [entry.vbf.path], '')
          .then((result) => {
            if (result.failed.length > 0) {
              toast.warning(`Extraídas ${result.done.length} de ${result.total} imagens.`, {
                description: result.failed.slice(0, 3).join('; '),
              });
            } else if (result.total > 0) {
              toast.success('Imagem extraída como .dds e .png.');
            }
            if (result.done.length > 0 && entry.vbf) {
              invalidateVbfImage(entry.vbf.root, entry.vbf.path);
              void actions.selectEntry(entry);
            }
          })
          .catch((error) => toast.error(parseError(error)))
          .finally(() => setBusy(null));
        return;
      }
      if (type === 'replicate' && entry.vbf) {
        actions.openImageAction({
          type: 'replicate',
          id: entry.id,
          label: entry.label,
          duplicates,
          vbf: entry.vbf,
        });
        return;
      }
      refuseVbf();
      return;
    }
    actions.openImageAction({
      type,
      id: entry.id,
      label: entry.label,
      duplicates,
    });
  };

  const onExport = async (format: 'dds' | 'png' | 'dds+png') => {
    if (!entry) return;
    setBusy('export');
    try {
      const formats = format === 'dds+png' ? (['dds', 'png'] as const) : [format];
      const dests: string[] = [];

      for (const fmt of formats) {
        let dest: string;
        if (entry.vbf) {
          dest = await vbfImageExportPath(entry.vbf.root, entry.vbf.path, fmt);
          await saveVbfImage(entry.vbf.root, entry.vbf.path, fmt, dest);
        } else {
          dest = await imageExportPath(entry.id, fmt, version);
          await saveImage(entry.id, fmt, dest, version);
        }
        dests.push(dest);
      }

      if (dests.length === 1) {
        toast.success(`Imagem exportada: ${dests[0]}`);
      } else {
        toast.success(`Imagem exportada como DDS e PNG: ${dests.join(', ')}`);
      }
    } catch (error) {
      toast.error(parseError(error));
    } finally {
      setBusy(null);
    }
  };

  const onExtractTo = async (format: 'dds' | 'png' | 'dds+png') => {
    if (!entry || !entry.vbf) return;
    setBusy('extractTo');
    try {
      const destRoot = await selectVbfImageExtractDir();
      if (!destRoot) return; // usuário cancelou
      const result = await extractVbfImagesSelection(entry.vbf.root, [entry.vbf.path], destRoot);
      if (result.failed.length > 0) {
        toast.warning(`Extraídas ${result.done.length} de ${result.total} imagens.`, {
          description: result.failed.slice(0, 3).join('; '),
        });
      } else if (result.total > 0) {
        const fmtLabel = format === 'dds+png' ? '.dds e .png' : `.${format}`;
        toast.success(`Imagem extraída como ${fmtLabel} em ${destRoot}`);
      }
    } catch (error) {
      toast.error(parseError(error));
    } finally {
      setBusy(null);
    }
  };

  /**
   * Importa o .dds escolhido. `all=true` propaga para as cópias idênticas
   * (grupo de payload original — o backend recusa alvo fora dele).
   */
  const runImport = async (path: string, all: boolean) => {
    if (!entry) return;
    setBusy('import');
    try {
      if (entry.vbf) {
        await importVbfImage(entry.vbf.root, entry.vbf.path, path);
        invalidateVbfImage(entry.vbf.root, entry.vbf.path);
        toast.success(`Textura importada em mods/: ${entry.label}`);
        await actions.selectEntry(entry);
        return;
      }
      if (!all) {
        await importImage(entry.id, path, version);
        toast.success(`Textura importada: ${path}`);
      } else {
        const targets = duplicates.map((d) => d.id);
        const result = await importImageGroup(entry.id, path, targets, version);
        if (result.failed.length > 0) {
          toast.warning(
            `Importadas ${result.updated.length} de ${result.total} texturas.`,
            { description: result.failed.slice(0, 3).join('; ') }
          );
        } else {
          toast.success(
            `Textura importada em ${result.updated.length} textura(s) idêntica(s).`
          );
        }
      }
      // Recarrega árvore + textura: os containers em mods/ mudaram.
      await actions.reload();
    } catch (error) {
      toast.error(parseError(error));
    } finally {
      // O diálogo fica aberto durante a gravação (botões travados) e fecha
      // no fim — sucesso ou erro.
      setPendingImport(null);
      setBusy(null);
    }
  };

  const onImport = async () => {
    if (!entry) return;
    try {
      const path = await selectImageFile(entry.id, version);
      if (!path) return;
      if (readOnly) {
        await runImport(path, false);
        return;
      }
      // Com cópias idênticas o usuário escolhe o alcance antes de gravar.
      if (duplicates.length > 0) {
        setPendingImport(path);
        return;
      }
      await runImport(path, false);
    } catch (error) {
      toast.error(parseError(error));
    }
  };

  /** Reconstrói o índice de duplicatas (edição externa no hex editor). */
  const onRefresh = async () => {
    if (refuseVbf()) return;
    setBusy('refresh');
    try {
      await refreshImageDuplicates(version);
      toast.success('Duplicatas reanalisadas.');
      // Árvore junto: o índice é quem decide quais cópias ficam ocultas.
      await actions.reload();
    } catch (error) {
      toast.error(parseError(error));
    } finally {
      setBusy(null);
    }
  };

  /**
   * Import em lote de mods/translated/images: abre varrendo sem seleção
   * (para descobrir os diretórios disponíveis), reanexa a cada marcação e
   * aplica só os containers diferentes dos de mods/.
   */
  const openImportImages = async () => {
    try {
      setPickedDirs([]);
      setPendingBatch(await previewImportImages(version, []));
    } catch (error) {
      toast.error(parseError(error));
    }
  };

  const toggleImportDir = async (dir: string) => {
    const next = pickedDirs.includes(dir)
      ? pickedDirs.filter((d) => d !== dir)
      : [...pickedDirs, dir];
    setPickedDirs(next);
    try {
      setPendingBatch(await previewImportImages(version, next));
    } catch (error) {
      toast.error(parseError(error));
    }
  };

  const confirmImportImages = async () => {
    setBusy('importBatch');
    const toastId = 'importImages';
    toast.loading('Importando imagens…', { id: toastId });
    try {
      const result = await importImages(version, pickedDirs);
      setPendingBatch(null);
      if (result.new_count === 0) {
        toast.info('Nada novo para importar.', { id: toastId });
      } else {
        const pulados =
          result.errors.length > 0
            ? ` (${result.errors.length} pulada(s))`
            : '';
        toast.success(
          `${result.new_count} de ${result.max} nova(s) importada(s).${pulados}`,
          { id: toastId }
        );
        await actions.reload();
      }
    } catch (error) {
      toast.error(parseError(error), { id: toastId });
    } finally {
      setBusy(null);
    }
  };

  return {
    busy,
    readOnly,
    pendingImport,
    setPendingImport,
    pendingBatch,
    pickedDirs,
    openImportImages,
    toggleImportDir,
    confirmImportImages,
    closeImportImages: () => setPendingBatch(null),
    openAction,
    onExport,
    onExtractTo,
    onImport,
    runImport,
    onRefresh,
  };
}
