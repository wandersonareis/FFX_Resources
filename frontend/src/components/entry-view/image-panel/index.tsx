'use client';

import { useSelector } from '@tanstack/react-store';
import { useState } from 'react';
import { EntryActionsMenu } from '@/components/entry-view/entry-actions-menu';
import type { EntryView } from '@/components/entry-view/entry-view-store';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import type { EntryMenuTarget } from '@/lib/ffx/content-tree/menu-target';
import { resolveEntryLabel } from '@/lib/ffx/display-names';
import { imageInfoStore, toggleImageInfo } from '@/lib/ffx/image-info-store';
import { LAYOUT_DEFAULTS_PX } from '@/lib/ffx/layout-storage';
import type { EntryRow } from '@/lib/ffx/tree-data';
import { usePersistedLayout } from '@/lib/ffx/use-persisted-layout';
import { ImageInfoPanel } from './image-info-panel';
import { ImagePreview } from './image-preview';
import { ImageImportDialog } from './import-dialog';
import { ImportImagesDetailedDialog } from './import-images-detailed-dialog';
import { ImageToolbar } from './toolbar';
import { useImageActions } from './use-image-actions';
import { useImagePreview } from './use-image-preview';

/**
 * Painel da textura (kind=images): ocupa o lugar da tabela Original/
 * Traduzido e mostra a pré-visualização PNG que o backend já devolve
 * (data URL — nenhum decoder de imagem no JS).
 *
 * Ações: extrair cópias de trabalho (.dds + .png em mods/images),
 * salvar em qualquer lugar do disco, importar um .dds editado (repack só do
 * mesmo formato/dimensão nesta etapa) e, quando a imagem tem cópias
 * idênticas (otimização do DVD), propagar o import para elas.
 *
 * As duplicatas vêm do índice de duplicatas do backend (hash do PAYLOAD, não
 * do arquivo: o namespace embute o nome e tornaria inútil o hash do
 * arquivo). O índice é memoizado; "Reanalisar" o reconstrói para pegar
 * edição externa em hex editor.
 */
export function ImagePanel({ view }: { view: EntryView }) {
  const { store } = view;
  const entry = useSelector(store, (s) => s.selectedEntry);
  const image = useSelector(store, (s) => s.image);
  const imageAction = useSelector(store, (s) => s.imageAction);
  const loading = useSelector(store, (s) => s.loading);
  /**
   * Menu de contexto do painel (o mesmo da árvore). O alvo é SEMPRE a
   * textura aberta — todo o painel é o nó — então o target é fixo quando
   * aberto e null quando fechado.
   */
  const [menuTarget, setMenuTarget] = useState<EntryMenuTarget | null>(null);
  /**
   * Largura da coluna de informações persistida. O painel monta/desmonta ao
   * trocar de kind, então o initializer relê o localStorage a cada abertura —
   * idempotente e já no cliente (sem SSR).
   */
  const { defaultLayout, onLayoutChanged } = usePersistedLayout('imageInfo');
  /**
   * Coluna de informações OCULTA (sessão): sobrevive à troca imagem↔texto e
   * entre abas de versão, mas não ao fechamento do app (padrão = aberta).
   * Vive no store de módulo, fora daqui — o ImagePanel desmonta.
   */
  const infoHidden = useSelector(imageInfoStore, (s) => s.hidden);

  const {
    busy,
    readOnly,
    pendingImport,
    setPendingImport,
    pendingPreview,
    openImportImages,
    confirmImportImages,
    closeImportImages,
    openAction,
    onExport,
    onImport,
    onExtractTo,
    runImport,
    onRefresh,
    toggleAllEntries,
    toggleEntry,
    toggleCopy,
  } = useImageActions({
    view,
    entry,
    duplicates: image?.duplicates ?? [],
  });

  // Painel sem overlay por cima (diálogo de escopo, diálogo de ação ou menu
  // de contexto) — os atalhos só valem para a textura em exibição.
  const panelIdle =
    entry !== null &&
    pendingImport === null &&
    pendingPreview === null &&
    imageAction === null &&
    menuTarget === null &&
    busy === null;

  const preview = useImagePreview({ entryId: entry?.id ?? '', enabled: panelIdle });

  if (!entry) return null;
  if (loading && !image) return <p className="mt-8 opacity-70">Carregando…</p>;
  if (!image) {
    return <p className="mt-8 opacity-70">Não foi possível carregar a textura.</p>;
  }

  const duplicates = image.duplicates ?? [];
  const copyTotal = duplicates.length + 1;

  /** Navega para uma cópia (a árvore já tem o entry — é só selecionar). */
  const goToCopy = async (id: string, key: string, vbfPath?: string): Promise<void> => {
    const target: EntryRow = {
      kind: 'images',
      id,
      key,
      label: resolveEntryLabel('images', id),
      ...(entry.vbf && vbfPath ? { vbf: { root: entry.vbf.root, path: vbfPath } } : {}),
    };
    await view.actions.selectEntry(target);
  };

  // Alvo do menu de contexto: o painel inteiro é a textura aberta. Textura
  // vinda do .vbf não tem menu (as ações são sobre data/ + mods/).
  const panelTarget: EntryMenuTarget | null = readOnly
    ? null
    : {
        kind: 'images',
        id: entry.id,
        label: entry.label,
      };

  // Botão direito em QUALQUER ponto do painel abre o mesmo menu da árvore
  // (Exportar / Abrir até o arquivo / Replicar / Deletar).
  return (
    <EntryActionsMenu
      view={view}
      target={menuTarget}
      onOpenChange={(open) => setMenuTarget(open ? panelTarget : null)}
      initialId={entry.id}
      initialDuplicates={duplicates}
    >
      <div
        className="flex h-full flex-col gap-4 pt-4"
        onContextMenuCapture={() => setMenuTarget(readOnly ? null : panelTarget)}
      >
        <ImageToolbar
          busy={busy}
          readOnly={readOnly}
          flipped={preview.flipped}
          duplicateCount={duplicates.length}
          infoHidden={infoHidden}
          onExport={(format) => void onExport(format)}
          onExtractTo={(format) => void onExtractTo(format)}
          onImport={() => void onImport()}
          onImportTranslated={() => void openImportImages()}
          onReplicate={() => openAction('replicate')}
          onToggleFlip={preview.toggleFlip}
          onToggleInfo={toggleImageInfo}
          onRefresh={() => void onRefresh()}
        />

        {/* O grupo preenche a altura DISPONÍVEL da aba — não mede mais pelo
            conteúdo. O wrapper tem flex-1/min-h-0 (altura definida dentro do
            flex-col do painel), o height:100% inline do grupo resolve contra
            ele, e a página deixa de crescer/rolar por causa de textura alta:
            a rolagem mora DENTRO do box da prévia (imagem maior que o
            container) e do wrapper da coluna de info (metadados altos). */}
        <div className="min-h-0 flex-1">
          <ResizablePanelGroup
            orientation="horizontal"
            defaultLayout={defaultLayout}
            onLayoutChanged={onLayoutChanged}
          >
            <ResizablePanel id="preview" minSize={280}>
              <ImagePreview
                src={image.pngData}
                alt={entry.label}
                width={image.width}
                height={image.height}
                zoomLevel={preview.zoomLevel}
                flipped={preview.flipped}
                pixelated={preview.pixelated}
                onZoomIn={preview.zoomIn}
                onResetZoom={preview.resetZoom}
                onSetZoomLevel={preview.setZoomLevel}
                onTogglePixelated={preview.togglePixelated}
              />
            </ResizablePanel>
            {/*
                Coluna oculta (botão Info da barra): handle e painel saem do
                grupo juntos e a prévia fica com 100% da largura. Ocultar não
                polui a largura PERSISTIDA — o re-layout programático chega no
                onLayoutChanged com isUserInteraction=false e não grava; ao
                reexibir, o painel volta pela defaultLayout/defaultSize (a
                largura salva, ou 320px pelo padrão).
            */}
            {!infoHidden ? (
              <>
                <ResizableHandle withHandle />
                <ResizablePanel
                  id="info"
                  // relative: containing block do card absoluto da coluna
                  // (o visual full-height). A v4 aplica o className no
                  // wrapper interno do painel — exatamente quem precisa
                  // posicionar o card.
                  className="relative"
                  defaultSize={LAYOUT_DEFAULTS_PX.imageInfo}
                  minSize={180}
                  maxSize={480}
                >
                  <ImageInfoPanel
                    image={image}
                    duplicates={duplicates}
                    onGoToCopy={(id, key, vbfPath) => void goToCopy(id, key, vbfPath)}
                  />
                </ResizablePanel>
              </>
            ) : null}
          </ResizablePanelGroup>
        </div>

        <ImageImportDialog
          open={pendingImport !== null}
          entryId={entry.id}
          duplicates={duplicates}
          total={copyTotal}
          busy={busy}
          onCancel={() => setPendingImport(null)}
          onImportOne={() => pendingImport && void runImport(pendingImport, false)}
          onImportAll={() => pendingImport && void runImport(pendingImport, true)}
        />

        <ImportImagesDetailedDialog
          open={pendingPreview !== null}
          preview={pendingPreview}
          busy={busy === 'importBatch'}
          onOpenChange={(open) => {
            if (!open) closeImportImages();
          }}
          onConfirm={() => void confirmImportImages()}
          onToggleAll={() => void toggleAllEntries()}
          onToggleEntry={toggleEntry}
          onToggleCopy={toggleCopy}
        />
      </div>
    </EntryActionsMenu>
  );
}
