"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Window } from "@wailsio/runtime";
import { Copy, Minus, Square, X } from "lucide-react";
import { cn } from "cn";

/**
 * Chrome da janela filha (frameless): titlebar com região de arrasto
 * (`--wails-draggable: drag`) + botões min/max/fechar do sistema via runtime,
 * bordas de redimensionamento (`--wails-resize: all`) — os controles continuam
 * HTML, mas a janela se comporta como desktop: arrastável, focável, sobre o
 * app. Esc fecha, fechando o mesmo papel do Esc do modal (Radix).
 */
export function WindowChrome({
  title,
  children,
  className,
}: {
  title: string;
  children: ReactNode;
  className?: string;
}) {
  const [maximised, setMaximised] = useState(false);

  useEffect(() => {
    void Window.IsMaximised()
      .then(setMaximised)
      .catch(() => undefined);
  }, []);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        void Window.Close().catch(() => undefined);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const toggleMaximise = async (): Promise<void> => {
    try {
      if (await Window.IsMaximised()) {
        await Window.Restore();
        setMaximised(false);
      } else {
        await Window.Maximise();
        setMaximised(true);
      }
    } catch {
      // runtime indisponível (build web): janela filha não existe lá.
    }
  };

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-popover text-popover-foreground [--wails-resize:all]">
      <div className="flex h-9 shrink-0 items-center justify-between border-b border-border/60 pr-1 pl-4 select-none [--wails-draggable:drag]">
        <span className="truncate text-xs font-medium text-muted-foreground">
          {title}
        </span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground [--wails-draggable:no-drag]"
            onClick={() => void Window.Minimise().catch(() => undefined)}
            aria-label="Minimizar"
            title="Minimizar"
          >
            <Minus size={14} />
          </button>
          <button
            type="button"
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground [--wails-draggable:no-drag]"
            onClick={() => void toggleMaximise()}
            aria-label={maximised ? "Restaurar" : "Maximizar"}
            title={maximised ? "Restaurar" : "Maximizar"}
          >
            {maximised ? <Copy size={14} /> : <Square size={14} />}
          </button>
          <button
            type="button"
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive hover:text-destructive-foreground [--wails-draggable:no-drag]"
            onClick={() => void Window.Close().catch(() => undefined)}
            aria-label="Fechar"
            title="Fechar"
          >
            <X size={14} />
          </button>
        </div>
      </div>
      <div className={cn("min-h-0 flex-1 overflow-auto", className)}>
        {children}
      </div>
    </div>
  );
}
