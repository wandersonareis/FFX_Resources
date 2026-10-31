'use client';

import { useHotkey, useHotkeys } from '@tanstack/react-hotkeys';
import { useSelector } from '@tanstack/react-store';
import { startTransition, useCallback, useEffect, useState } from 'react';
import { QuitApp } from '@/bindings/ffxresources/app';
import { SetGameVersion } from '@/bindings/ffxresources/configservice';
import { AppHeader } from '@/components/app-header';
import { ConfigDialog } from '@/components/dialogs/config-dialog';
import { DeleteOrphanDialog } from '@/components/dialogs/delete-orphan-dialog';
import { PruneOrphansDialog } from '@/components/dialogs/prune-orphans-dialog';
import { ExportImportControls } from '@/components/export-import-controls';
import { GameVersionTab } from '@/components/game-version-tab';
import { LogDrawer } from '@/components/log-drawer';
import { ProgressModal } from '@/components/progress/progress-modal';
import { ProgressNotifier } from '@/components/progress-notifier';
import { StatusBar } from '@/components/status-bar/status-bar';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  configDialogStore,
  openConfigDialog,
  setConfigDialogOpen,
} from '@/lib/ffx/config-dialog-store';
import { applyConfigSnapshot, commitEnableMods, loadConfigSnapshot } from '@/lib/ffx/config-store';
import { editDraft } from '@/lib/ffx/edit-draft';
import { EXPORT_FORMAT_LABELS, exportSelection } from '@/lib/ffx/export-selection';
import { openFileFind } from '@/lib/ffx/find-store';
import { GAME_VERSIONS, type GameVersionId } from '@/lib/ffx/game-version';
import { saveAllDrafts } from '@/lib/ffx/save-all';
import { openSearchDialog } from '@/lib/ffx/search-store';
import { SHORTCUTS, shortcutTip } from '@/lib/ffx/shortcuts';
import { tabStorage } from '@/lib/ffx/tab-storage';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';

export function AppShell() {
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [saving, setSaving] = useState(false);
  // Diálogo de Configurações: estado no store de módulo — a árvore de
  // containers também o abre (aviso "aponte o executável").
  const configOpen = useSelector(configDialogStore, (s) => s.open);

  useEffect(() => {
    // Hydration-safe: localStorage só existe no cliente.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSelectedIndex(tabStorage.load(GAME_VERSIONS.length));
    // Carga inicial do config no store — as mudanças vêm pelo eco
    // AppConfigChanged abaixo (o dialog de config lê o store, sem
    // fetch ao abrir).
    void loadConfigSnapshot();
  }, []);

  const setVersion = useCallback(async (version: GameVersionId) => {
    await SetGameVersion(version);
  }, []);

  // ---- Atalhos globais (TanStack Hotkeys) ----
  // Trocar de aba monta o GameVersionTab pesado: o setSelectedIndex roda em
  // startTransition, então a aba antiga permanece visível/interativa até a nova
  // estar pronta. tabStorage.save/setVersion (efeitos colaterais) ficam fora.
  const switchTab = (index: number) => {
    if (index < 0 || index >= GAME_VERSIONS.length || index === selectedIndex) return;
    startTransition(() => setSelectedIndex(index));
    tabStorage.save(index);
    setVersion(GAME_VERSIONS[index].id);
  };

  // A tecla vem do catálogo lib/ffx/shortcuts.ts, que é a mesma fonte do
  // tooltip de cada controle (nenhuma tecla é escrita em dois lugares).
  // Ctrl+1..4: trocar aba de versão — uma tecla por GAME_VERSIONS (a lista
  // deriva do catálogo: acrescentar versão é acrescentar uma tecla ali).
  useHotkeys(
    SHORTCUTS.versionTabs.keys.map((hotkey, index) => ({
      hotkey,
      callback: () => switchTab(index),
      options: {
        meta: {
          name: `${SHORTCUTS.versionTabs.label}: ${GAME_VERSIONS[index].label}`,
          group: 'Abas',
        },
      },
    })),
  );
  // Ctrl+K: abrir o modal de busca da aba ATIVA (convenção de palette; o
  // store da busca é de módulo por versão, então o atalho só aponta o id).
  useHotkey(SHORTCUTS.search.keys, () => openSearchDialog(GAME_VERSIONS[selectedIndex].id), {
    meta: { name: SHORTCUTS.search.label, group: 'Global' },
  });
  // Ctrl+F: abre a busca & substituição local na tabela da aba ATIVA (a barra
  // renderiza dentro da aba, sob a save bar).
  useHotkey(SHORTCUTS.findInFile.keys, () => openFileFind(GAME_VERSIONS[selectedIndex].id), {
    meta: { name: SHORTCUTS.findInFile.label, group: 'Global' },
  });
  // Ctrl+, : abrir Configurações (convenção de settings) — janela no
  // desktop, modal no browser (openConfigDialog decide).
  useHotkey(SHORTCUTS.config.keys, () => openConfigDialog(), {
    meta: { name: SHORTCUTS.config.label, group: 'Global' },
  });
  // Ctrl+Alt+F: alterna formato JSON ↔ Strings direto (popover não abre).
  // Fora do catálogo: não é exibido em texto nenhum.
  useHotkey('Mod+Alt+F', () => {
    const next = exportSelection.formatOf() === 'json' ? 'strings' : 'json';
    exportSelection.setFormat(next);
    toast.info(`Formato de exportação: ${EXPORT_FORMAT_LABELS[next]}`);
  });
  // Ctrl+S: salvar rascunhos (mesma ação do botão Salvar; sem fechar o app).
  useHotkey(
    SHORTCUTS.save.keys,
    () => {
      void saveAllDrafts(() => saving, setSaving);
    },
    { meta: { name: SHORTCUTS.save.label, group: 'Global' } },
  );

  useWailsEvent('SaveRequested', () => {
    // Cancelar o aviso de poda ABORTA o save — não pode fechar o app.
    void saveAllDrafts(() => saving, setSaving).then((saved) => {
      if (saved) void QuitApp();
    });
  });

  // Eco do config: cada setter do backend persiste e re-emite o snapshot
  // completo — o store espelha a verdade (caminho Abs, exe recusado em
  // silêncio, translated derivado) para todos os consumidores. fromEvent:
  // é o eco pós-mudança (não a carga inicial) que dispara o coldReload de
  // enableMods nas abas.
  useWailsEvent('AppConfigChanged', (data) => applyConfigSnapshot(data, true));

  // Decisão da guarda do toggle de mods (ConfigService.ResolveEnableMods
  // Change, publicado pelo modal de config OU pela janela filha): os
  // rascunhos de tradução moram NESTA janela (o editDraft é por janela),
  // então salvar/descartar só pode acontecer aqui. "save" que não salva
  // (gate de tags protegidas, erro de escrita) ABORTA o toggle — o
  // rascunho continua sujo e o checkbox segue no estado antigo. Aplicar
  // (commitEnableMods) emite o eco que invalida os caches do backend e
  // dispara o coldReload das abas: todo o conteúdo troca para a
  // preferência nova.
  useWailsEvent('EnableModsChange', (data) => {
    const payload = (data ?? {}) as { enabled?: unknown; decision?: unknown };
    if (typeof payload.enabled !== 'boolean') return;
    if (payload.decision !== 'save' && payload.decision !== 'discard') return;
    const enabled = payload.enabled;
    const decision = payload.decision;
    void (async () => {
      if (decision === 'save') {
        await saveAllDrafts(() => saving, setSaving);
        // Falhou/bloqueado: o rascunho segue sujo — o toggle não aplica.
        if (editDraft.getSnapshot().hasDirty) return;
      } else {
        // Descartar limpa os rascunhos; a flag do backend (unsavedEdits)
        // desce pelo efeito de hasDirty do GameVersionTab.
        editDraft.clearAll();
      }
      await commitEnableMods(enabled);
    })();
  });

  const onTabChange = (index: string) => {
    const idx = Number.parseInt(index, 10);
    startTransition(() => setSelectedIndex(idx));
    tabStorage.save(idx);
    setVersion(GAME_VERSIONS[idx].id);
  };

  return (
    <div className="flex h-screen flex-col">
      <AppHeader onOpenConfig={openConfigDialog} />

      <Tabs
        value={String(selectedIndex)}
        onValueChange={onTabChange}
        className="flex-1 min-h-0 flex flex-col"
      >
        <div className="mx-4 mt-2 flex items-center justify-between gap-1">
          <TabsList variant="gradient" className="w-fit">
            {GAME_VERSIONS.map((tab, index) => (
              <Tooltip key={tab.id}>
                <TooltipTrigger asChild>
                  <TabsTrigger
                    variant="gradient"
                    value={String(index)}
                    className="h-8 px-2.5 py-0 rounded-lg"
                  >
                    {tab.label}
                  </TabsTrigger>
                </TooltipTrigger>
                <TooltipContent>
                  {shortcutTip(SHORTCUTS.versionTabs.label, SHORTCUTS.versionTabs.keys[index])}
                </TooltipContent>
              </Tooltip>
            ))}
          </TabsList>
          <div className="flex items-center gap-1">
            <ExportImportControls version={GAME_VERSIONS[selectedIndex].id} />
          </div>
        </div>
        {GAME_VERSIONS.map((tab, index) => (
          <TabsContent key={tab.id} value={String(index)} className="flex-1 min-h-0 mt-0">
            <GameVersionTab version={tab.id} />
          </TabsContent>
        ))}
      </Tabs>

      <StatusBar />

      <ConfigDialog open={configOpen} onOpenChange={setConfigDialogOpen} />
      <PruneOrphansDialog />
      <DeleteOrphanDialog />
      <LogDrawer />
      <ProgressNotifier />
      <ProgressModal />
    </div>
  );
}
