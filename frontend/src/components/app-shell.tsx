'use client';

import { useCallback, useEffect, startTransition, useState } from 'react';
import { useHotkey, useHotkeys } from '@tanstack/react-hotkeys';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { SHORTCUTS, shortcutTip } from '@/lib/ffx/shortcuts';
import { openSearchDialog } from '@/lib/ffx/search-store';
import { openFileFind } from '@/lib/ffx/find-store';
import { QuitApp } from '@/bindings/ffxresources/app';
import { SetGameVersion } from '@/bindings/ffxresources/configservice';
import { GAME_VERSIONS, GameVersionId } from '@/lib/ffx/game-version';
import { tabStorage } from '@/lib/ffx/tab-storage';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';
import { saveAllDrafts } from '@/lib/ffx/save-all';
import { exportSelection, EXPORT_FORMAT_LABELS } from '@/lib/ffx/export-selection';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs';
import { StatusBar } from '@/components/status-bar/status-bar';
import { GameVersionTab } from '@/components/game-version-tab';
import { ConfigDialog } from '@/components/dialogs/config-dialog';
import { AppHeader } from '@/components/app-header';
import { ExportImportControls } from '@/components/export-import-controls';
import { ProgressNotifier } from '@/components/progress-notifier';
import { ProgressModal } from '@/components/progress/progress-modal';

export function AppShell() {
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [saving, setSaving] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);

  useEffect(() => {
    // Hydration-safe: localStorage só existe no cliente.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSelectedIndex(tabStorage.load(GAME_VERSIONS.length));
  }, []);

  const setVersion = useCallback(async (version: GameVersionId) => {
    await SetGameVersion(version);
  }, []);

  // ---- Atalhos globais (TanStack Hotkeys) ----
  // Trocar de aba monta o GameVersionTab pesado: o setSelectedIndex roda em
  // startTransition, então a aba antiga permanece visível/interativa até a nova
  // estar pronta. tabStorage.save/setVersion (efeitos colaterais) ficam fora.
  const switchTab = (index: number) => {
    if (index < 0 || index >= GAME_VERSIONS.length || index === selectedIndex)
      return;
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
    }))
  );
  // Ctrl+K: abrir o modal de busca da aba ATIVA (convenção de palette; o
  // store da busca é de módulo por versão, então o atalho só aponta o id).
  useHotkey(
    SHORTCUTS.search.keys,
    () => openSearchDialog(GAME_VERSIONS[selectedIndex].id),
    { meta: { name: SHORTCUTS.search.label, group: 'Global' } }
  );
  // Ctrl+F: abre a busca & substituição local na tabela da aba ATIVA (a barra
  // renderiza dentro da aba, sob a save bar).
  useHotkey(
    SHORTCUTS.findInFile.keys,
    () => openFileFind(GAME_VERSIONS[selectedIndex].id),
    { meta: { name: SHORTCUTS.findInFile.label, group: 'Global' } }
  );
  // Ctrl+, : abrir Configurações (convenção de settings).
  useHotkey(SHORTCUTS.config.keys, () => setConfigOpen(true), {
    meta: { name: SHORTCUTS.config.label, group: 'Global' },
  });
  // Ctrl+Alt+F: alterna formato JSON ↔ Strings direto (popover não abre).
  // Fora do catálogo: não é exibido em texto nenhum.
  useHotkey('Mod+Alt+F', () => {
    const next =
      exportSelection.formatOf() === 'json' ? 'strings' : 'json';
    exportSelection.setFormat(next);
    toast.info(`Formato de exportação: ${EXPORT_FORMAT_LABELS[next]}`);
  });
  // Ctrl+S: salvar rascunhos (mesma ação do botão Salvar; sem fechar o app).
  useHotkey(
    SHORTCUTS.save.keys,
    () => {
      void saveAllDrafts(() => saving, setSaving);
    },
    { meta: { name: SHORTCUTS.save.label, group: 'Global' } }
  );

  useWailsEvent('SaveRequested', () => {
    void saveAllDrafts(() => saving, setSaving).then(() => QuitApp());
  });

  const onTabChange = (index: string) => {
    const idx = Number.parseInt(index, 10);
    startTransition(() => setSelectedIndex(idx));
    tabStorage.save(idx);
    setVersion(GAME_VERSIONS[idx].id);
  };

  return (
    <div className="flex h-screen flex-col">
      <AppHeader onOpenConfig={() => setConfigOpen(true)} />

      <Tabs value={String(selectedIndex)} onValueChange={onTabChange} className="flex-1 min-h-0 flex flex-col">
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
                  {shortcutTip(
                    SHORTCUTS.versionTabs.label,
                    SHORTCUTS.versionTabs.keys[index]
                  )}
                </TooltipContent>
              </Tooltip>
            ))}
          </TabsList>
          <div className="flex items-center gap-1">
            <ExportImportControls
              version={GAME_VERSIONS[selectedIndex].id}
            />
          </div>
        </div>
        {GAME_VERSIONS.map((tab, index) => (
          <TabsContent key={tab.id} value={String(index)} className="flex-1 min-h-0 mt-0">
            <GameVersionTab version={tab.id} />
          </TabsContent>
        ))}
      </Tabs>

      <StatusBar />

      <ConfigDialog open={configOpen} onOpenChange={setConfigOpen} />
      <ProgressNotifier />
      <ProgressModal />
    </div>
  );
}
