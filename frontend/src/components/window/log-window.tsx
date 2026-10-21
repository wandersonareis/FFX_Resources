"use client";

import { useTheme } from "next-themes";
import { LogPanel } from "@/components/log-panel";
import { useWailsEvent } from "@/lib/ffx/use-wails-event";

/**
 * Conteúdo da janela filha de Logs (/w#logs): SÓ o conteúdo — o WindowChrome
 * é do HOST (app/w/page.tsx), que também recebe os controles (LogPanelControls)
 * pela entrada do registry como titleExtra da MESMA barra. Envolver num
 * segundo WindowChrome aqui duplicaria a titlebar (o bug das "duas barras").
 *
 * Tema ao vivo: cada janela é uma webview com o PRÓPRIO next-themes (lê o
 * localStorage da mesma origem na carga — abre no tema certo), e o evento
 * ffx:theme (o mesmo que a janela do updater escuta) sincroniza as trocas
 * feitas na principal enquanto esta está aberta.
 */
export function LogWindowContent() {
  const { setTheme } = useTheme();
  useWailsEvent("ffx:theme", (theme) => {
    if (theme === "dark" || theme === "light") setTheme(theme);
  });

  return <LogPanel className="h-full" />;
}
