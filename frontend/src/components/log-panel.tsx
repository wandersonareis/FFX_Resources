'use client';

import { cn } from 'cn';
import { useEffect, useRef, useState } from 'react';
import type { Entry } from '@/bindings/ffxresources/backend/uilog/models';
import { uiLog, useUILog } from '@/lib/ffx/uilog-store';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';

/**
 * LogPanel é SÓ a lista rolável do log de operações (backend/uilog) — o
 * renderador único compartilhado pelos dois hosts (janela desktop /w/#logs e
 * drawer web). Monoespaçado estilo terminal: timestamp + [ação] + mensagem
 * e, nas linhas de arquivo, caminho com o resultado na MESMA linha
 * ("command.bin — gravado: imported/command.bin"). Cores do nível:
 * success verde, error vermelho, info azul, warn laranja.
 *
 * Os controles (contador + Limpar) NÃO moram aqui — ficam na barra do host
 * (titlebar do WindowChrome / header do Sheet), para a janela ter UMA
 * barra só. Este componente é o único assinante dos eventos UILog/UILogCleared
 * de cada janela e escreve no store de módulo (uilog-store), que alimenta
 * também os controles.
 */

/** Cores do terminal por nível (verde/vermelho/azul/laranja). */
const LEVEL_CLASS: Record<string, string> = {
  info: 'text-sky-400',
  success: 'text-green-500',
  warn: 'text-orange-500',
  error: 'text-red-500',
};

function levelClass(level: string): string {
  return LEVEL_CLASS[level] ?? 'text-foreground';
}

function fmtTime(ms: number): string {
  return new Date(ms).toLocaleTimeString('pt-BR', { hour12: false });
}

export function LogPanel({ className }: { className?: string }) {
  const { entries } = useUILog();
  // Seguir o fim: colado no rodo, o auto-scroll roda; usuário subiu para
  // ler = pausa até voltar ao fim (nada de roubar a rolagem).
  const [stick, setStick] = useState(true);
  const listRef = useRef<HTMLDivElement>(null);

  useWailsEvent('UILog', (data) => uiLog.append(data as Entry));
  useWailsEvent('UILogCleared', () => uiLog.cleared());

  // `entries` é o GATILHO do auto-scroll: entrada nova = rolagem para o
  // fim (enquanto `stick`). Sem ele o log parava de acompanhar as operações.
  // biome-ignore lint/correctness/useExhaustiveDependencies: entries é o gatilho do auto-scroll
  useEffect(() => {
    const el = listRef.current;
    if (el && stick) el.scrollTop = el.scrollHeight;
  }, [entries, stick]);

  const onScroll = () => {
    const el = listRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
    if (atBottom !== stick) setStick(atBottom);
  };

  return (
    <div
      ref={listRef}
      onScroll={onScroll}
      className={cn(
        'min-h-0 flex-1 overflow-auto px-3 py-2 font-mono text-xs leading-5',
        className,
      )}
    >
      {entries.length === 0 ? (
        <p className="text-muted-foreground">
          Nada registrado ainda — as operações do app (import, export, binários compilados) aparecem
          aqui em tempo real.
        </p>
      ) : (
        entries.map((e) => (
          <div
            key={`${e.time}|${e.level}|${e.action ?? ''}|${e.path ?? ''}|${e.status ?? ''}|${e.message}`}
            className="whitespace-pre-wrap break-all"
          >
            <span className="text-muted-foreground">{fmtTime(e.time)} </span>
            {e.action ? <span className="font-semibold">[{e.action}] </span> : null}
            {e.path ? (
              <>
                <span>{e.path}</span>
                <span className={cn('font-semibold', levelClass(e.level))}>
                  {' '}
                  — {e.status || e.level}
                  {e.message ? `: ${e.message}` : ''}
                </span>
              </>
            ) : (
              <span className={levelClass(e.level)}>{e.message}</span>
            )}
          </div>
        ))
      )}
    </div>
  );
}
