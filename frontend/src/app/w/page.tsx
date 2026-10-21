"use client";

import { useEffect, useState } from "react";
import { Window } from "@wailsio/runtime";
import { WindowChrome } from "@/components/window/window-chrome";
import { ConfigWindowContent } from "@/components/window/config-window";

/**
 * Host das janelas filhas (rota /w): o backend cria a janela com
 * URL "/w#<nome>" e o hash escolhe o conteúdo montado aqui dentro.
 *
 * ssr:false (dynamic): a rota é prerenderizada estática pelo Next, mas o
 * hash só existe no cliente — ler `location.hash` no initializer evita
 * divergência de hidratação (mesmo padrão do page.tsx raiz).
 *
 * O Sobre saiu daqui: é conteúdo 100% estático e virou janela de HTML
 * puro embutida no appwindows (WebviewWindowOptions.HTML) — pinta no
 * primeiro quadro, sem Next no caminho.
 */
const WINDOWS: Record<string, { title: string; Content: () => React.JSX.Element }> = {
  config: { title: "Configurações", Content: ConfigWindowContent },
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

  const { title, Content } = entry;
  return (
    <WindowChrome title={title}>
      <Content />
    </WindowChrome>
  );
}
