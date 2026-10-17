"use client";

import { useEffect, useMemo, useState } from "react";
import { useSelector } from "@tanstack/react-store";
import { SetUnsavedEdits } from "@/bindings/ffxresources/app";
import { KIND_LABELS } from "@/lib/ffx/display-names";
import type { GameVersionId } from "@/lib/ffx/game-version";
import { useEditDraft } from "@/lib/ffx/edit-draft";
import { useWailsEvent } from "@/lib/ffx/use-wails-event";
import { saveAllDrafts } from "@/lib/ffx/save-all";
import { setActiveFile } from "@/lib/ffx/active-file-store";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";
import { LAYOUT_DEFAULTS_PX } from "@/lib/ffx/layout-storage";
import { usePersistedLayout } from "@/lib/ffx/use-persisted-layout";
import { invalidateTextSearchResults } from "@/lib/ffx/search-store";
import { warmTextSearch } from "@/lib/ffx/tree-data";
import {
  createEntryView,
  type EntryView,
} from "@/components/entry-view/entry-view-store";
import { ContentTree } from "@/components/entry-view/content-tree";
import { EntryTable } from "@/components/entry-view/entry-table";
import { TableBar } from "@/components/entry-view/entry-table/table-bar";
import { useEntryFind } from "@/components/entry-view/find/use-entry-find";
import { FindBar } from "@/components/entry-view/find/find-bar";
import { ImageActionDialogs } from "@/components/dialogs/image-action-dialogs";
import { ImagePanel } from "@/components/entry-view/image-panel";
import { TranslationDialog } from "@/components/dialogs/translation-dialog";
import { PreloadVersions } from "@/bindings/ffxresources/app";
import { GAME_VERSIONS } from "@/lib/ffx/game-version";

/**
 * Aba de uma versão do jogo: só orquestra. O estado compartilhado entre
 * árvore, tabela e diálogo vive no store da view (createEntryView) — um por
 * aba — e cada filho se assina com useSelector.
 */
export function GameVersionTab({ version }: { version: GameVersionId }) {
  const view: EntryView = useMemo(() => createEntryView(version), [version]);
  const { store: drafts, snapshot } = useEditDraft();
  const activeKind = useSelector(view.store, (s) => s.activeKind);
  const selectedEntry = useSelector(view.store, (s) => s.selectedEntry);
  const loading = useSelector(view.store, (s) => s.loading);
  const progress = useSelector(view.store, (s) => s.progress);
  const [saving, setSaving] = useState(false);
  // Busca & substituição local no arquivo aberto (Ctrl+F). O estado fica no
  // find-store por versão; os matches derivam das rows aqui montadas.
  const find = useEntryFind(view);
  // A barra só faz sentido com um arquivo de TEXTO aberto (images não tem rows).
  const showFind = find.open && !!selectedEntry && activeKind !== "images";
  // Largura da sidebar persistida (percentual do grupo por id de painel).
  const { defaultLayout, onLayoutChanged } = usePersistedLayout("sidebar");

  const hasDirty = snapshot.hasDirty;

  useEffect(() => {
    void SetUnsavedEdits(hasDirty);
  }, [hasDirty]);

  // Publica o arquivo ativo (progresso por row) para a barra de status.
  // O progresso é fixo na abertura; troca de arquivo/aba atualiza aqui.
  useEffect(() => {
    setActiveFile({
      version,
      kind: activeKind,
      entryId: selectedEntry?.id ?? null,
      entryLabel: selectedEntry?.label ?? null,
      progress,
    });
  }, [version, activeKind, selectedEntry, progress]);

  useEffect(() => {
    // Sincroniza com o backend ao montar/trocar de versão. Ao concluir a
    // carga da aba VISUALIZADA, pré-carrega as demais em background —
    // trocar de aba depois cai no caminho rápido (cache do backend).
    void view.actions.reload().then(() => {
      // Índice de busca da aba aquecido em background: o primeiro Ctrl+K
      // não paga a construção síncrona do índice.
      warmTextSearch(version);
      void PreloadVersions(
        GAME_VERSIONS.filter((t) => t.id !== version).map((t) => t.id),
      );
    });
  }, [view, version]);

  useEffect(
    () =>
      drafts.onSaved(() => {
        // Salvou = os textos mudaram: resultados do modal ficaram velhos.
        invalidateTextSearchResults(version);
        void view.actions.reload();
      }),
    [drafts, view, version],
  );

  // Após importar, o backend emite ImportDone → recarrega árvore e tabela.
  useWailsEvent("ImportDone", () => {
    invalidateTextSearchResults(version);
    void view.actions.reload();
  });

  const onSaveAll = () => {
    void saveAllDrafts(() => saving, setSaving);
  };

  return (
    <div className="flex h-full min-h-0 border-t">
      <ResizablePanelGroup
        orientation="horizontal"
        className="min-w-0"
        defaultLayout={defaultLayout}
        onLayoutChanged={onLayoutChanged}
      >
        <ResizablePanel
          id="tree"
          defaultSize={LAYOUT_DEFAULTS_PX.sidebar}
          minSize={200}
          maxSize={600}
        >
          <ContentTree view={view} />
        </ResizablePanel>
        <ResizableHandle withHandle />

        <ResizablePanel id="main" minSize={300}>
          <main className="h-full min-w-0 flex flex-col p-2 px-4">
            <TableBar
              kindLabel={KIND_LABELS[activeKind]}
              entryLabel={selectedEntry?.label ?? null}
              progress={progress}
              hasDirty={hasDirty}
              saving={saving}
              onSave={() => void onSaveAll()}
              onOpenFind={activeKind !== "images" ? find.openFind : undefined}
            />
            {/* Busca & substituição local: fora do rolante (shrink-0), abaixo
                da save bar — digita já destaca e salta ao 1º match da tabela. */}
            {showFind ? <FindBar find={find} /> : null}
            {/* Único rolante da aba: a barra está fora do overflow, então só
                as linhas rolam (o cabeçalho gruda em top-0 desse wrapper). */}
            <div className="min-h-0 flex-1 overflow-y-auto">
            {selectedEntry ? (
              // images não tem rows: ocupa o lugar da tabela a pré-visualização
              // + ações (extrair/salvar/importar) da textura.
              activeKind === "images" ? (
                <ImagePanel view={view} />
              ) : (
                <EntryTable
                  view={view}
                  matchesByRow={find.matchesByRow}
                  activeRowKey={find.activeMatch?.rowKey ?? null}
                />
              )
            ) : loading ? (
              <p className="mt-4 opacity-70">Carregando…</p>
            ) : (
              <p className="mt-4 opacity-70">
                Selecione um arquivo no sidebar.
              </p>
            )}
            </div>
          </main>
        </ResizablePanel>
      </ResizablePanelGroup>

      <TranslationDialog view={view} />
      {/* Ações de imagem (extrair/replicar/deletar): um diálogo só, aberto
          pelo menu da árvore ou pelos botões do painel. */}
      <ImageActionDialogs view={view} />
    </div>
  );
}
