"use client";

import { useEffect, useState } from "react";
import { Window } from "@wailsio/runtime";
import { WindowChrome } from "@/components/window/window-chrome";
import { ConfigWindowContent } from "@/components/window/config-window";
import { LogWindowContent } from "@/components/window/log-window";
import { LogPanelControls } from "@/components/log-panel-controls";

/**
 * Host das janelas filhas (rota /w): o backend cria a janela com
 * URL "/w/#<nome>" e o hash escolhe o conteúdo montado aqui dentro.
 *
 * ssr:false (dynamic): a rota é prerenderizada estática pelo Next, mas o
 * hash só existe no cliente — ler `location.hash` no initializer evita
 * divergência de hidratação (mesmo padrão do page.tsx raiz).
 *
 * O WindowChrome mora AQUI (a titlebar é do host, uma só por janela); cada
 * Content é só o conteúdo. TitleExtra (opcional) entra na MESMA titlebar —
 * a janela de Logs usa para o contador + Limpar ficarem ao lado dos botões
 * de janela.
 *
 * O Sobre saiu daqui: é conteúdo 100% estático e virou janela de HTML
 * puro embutida no appwindows (WebviewWindowOptions.HTML) — pinta no
 * primeiro quadro, sem Next no caminho.
 */
const WINDOWS: Record<
  string,
  {
    title: string;
    Content: () => React.JSX.Element;
    TitleExtra?: () => React.JSX.Element;
  }
> = {
  config: { title: "Configurações", Content: ConfigWindowContent },
  logs: {
    title: "Logs",
    Content: LogWindowContent,
    TitleExtra: () => <LogPanelControls />,
  },
};

export default function WindowHost() {
  const [name, setName] = useState<string | null>(null);

  useEffect(() => {
    setName(window.location.hash.slice(1) || null);
  }, []);

  const entry = name ? WINDOWS[name] : undefined;

  if (!entry) {
    // Hash desconhecido ou ausente: fecha para não deixar janela órfã.
    if (name !== null) {
      void Window.Close().catch(() => undefined);
    }
    return null;
  }

  const { title, Content, TitleExtra } = entry;
  return (
    <WindowChrome title={title} titleExtra={TitleExtra ? <TitleExtra /> : undefined}>
      <Content />
    </WindowChrome>
  );
}
