"use client";

import { useEffect } from "react";
import { ConfigDialogContent } from "@/components/dialogs/config-dialog";
import { applyConfigSnapshot, loadConfigSnapshot } from "@/lib/ffx/config-store";
import { useWailsEvent } from "@/lib/ffx/use-wails-event";

/**
 * Conteúdo da janela filha de Configurações (/w/#config).
 *
 * Cada janela é um React root PRÓPRIO: o store desta janela é outra instância
 * da store de módulo, então ela carrega o snapshot por binding no mount e se
 * mantém fresca pelo eco AppConfigChanged — mesmo caminho do AppShell. Os
 * commits passam pelos bindings do ConfigService e o backend faz broadcast
 * para todas as janelas (inclusive a principal), que atualizam os próprios
 * stores. Fechar é o X/Esc do WindowChrome no topo.
 */
export function ConfigWindowContent() {
  useEffect(() => {
    void loadConfigSnapshot();
  }, []);

  useWailsEvent("AppConfigChanged", (data) => applyConfigSnapshot(data, true));

  return (
    <div className="grid gap-4 p-4 text-sm">
      <ConfigDialogContent />
    </div>
  );
}
