'use client';

import { useSelector } from '@tanstack/react-store';
import { cn } from 'cn';
import { AlertTriangle, Info, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { activeFileStore, clearActiveFile } from '@/lib/ffx/active-file-store';
import { KIND_LABELS } from '@/lib/ffx/display-names';
import { hotkeyLabel, SHORTCUTS, shortcutTip } from '@/lib/ffx/shortcuts';
import {
  dismissStatusBarWarning,
  pushStatusBarWarning,
  type StatusBarWarning,
  useStatusBarWarnings,
} from '@/lib/ffx/statusbar-store';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';

/**
 * Intervalo da rotação de avisos do backend (ms) — mais de um aviso ativo
 * alterna a cada 3 segundos.
 */
const WARNING_ROTATION_MS = 3000;

/**
 * Ordem exibida na faixa: cima, baixo, direita, esquerda. A tecla vem do
 * catálogo (SHORTCUTS.navigation) — a MESMA fonte do registro das
 * hotkeys na árvore e na tabela.
 */
const NAV_HINTS = [
  SHORTCUTS.navigation.up,
  SHORTCUTS.navigation.down,
  SHORTCUTS.navigation.right,
  SHORTCUTS.navigation.left,
] as const;

/**
 * Tecla de navegação como DICA visual (não como botão): ↑/↓ movem
 * entre nós na árvore e entre linhas na tabela, → expande/segue e ←
 * volta — o sentido depende do FOCO, então um clique aqui seria
 * ambíguo.
 */
function ArrowHint({ hotkey, label }: { hotkey: string; label: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded border bg-muted px-1 text-[11px] leading-none font-medium text-muted-foreground">
          {hotkeyLabel(hotkey)}
        </kbd>
      </TooltipTrigger>
      <TooltipContent>{shortcutTip(label, hotkey)}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Barra de status do rodapé — três faixas, da esquerda para a direita:
 *
 *  - ESQUERDA, na mesma margem da sidebar: as setas de navegação, só como
 *    dica das teclas.
 *  - MEIO: progresso de tradução do arquivo aberto (fixo na abertura).
 *    Clicável → alert com os detalhes ao lado da barra.
 *  - DIREITA: avisos do backend (desalinhamentos). Mais de um → alterna
 *    temporizado (3s). Clicável → alert com os detalhes; fechar o alert
 *    DESCARTA o aviso (some da barra e não volta por reemissão na sessão).
 */
export function StatusBar() {
  // Avisos do backend: dedupe/dismiss fica no store da sessão.
  useWailsEvent('StatusWarning', (data) => {
    pushStatusBarWarning(data as unknown as StatusBarWarning);
  });

  const activeFile = useSelector(activeFileStore, (s) => s);
  const warnings = useStatusBarWarnings();

  const [warnIndex, setWarnIndex] = useState(0);
  const [openPanel, setOpenPanel] = useState<'progress' | 'warning' | null>(null);

  // Rotação: só quando há mais de um aviso. O índice é normalizado contra
  // a lista corrente (dismiss encurta a lista sem quebrar a rotação).
  useEffect(() => {
    if (warnings.length <= 1) return;
    const timer = setInterval(
      () => setWarnIndex((i) => (i + 1) % warnings.length),
      WARNING_ROTATION_MS,
    );
    return () => clearInterval(timer);
  }, [warnings.length]);

  // Reinicia a rotação quando a lista muda (novo aviso: o visível muda).
  // `warnings` é o GATILHO: sem ele a rotação não reiniciava quando a lista
  // de avisos mudava.
  // biome-ignore lint/correctness/useExhaustiveDependencies: warnings é o gatilho da rotação
  useEffect(() => {
    setWarnIndex(0);
  }, [warnings]);

  // Fecho o painel quando o aviso exibido saiu da lista (dismiss externo).
  useEffect(() => {
    if (openPanel === 'warning' && warnings.length === 0) {
      setOpenPanel(null);
    }
  }, [openPanel, warnings.length]);

  // Troca de aba: a aba antiga desmonta → arquivo ativo some (o novo patch
  // quando a entrada carregar).
  useEffect(
    () => () => {
      if (activeFile.version === null) return;
      clearActiveFile();
    },
    [activeFile.version],
  );

  const warning: StatusBarWarning | null =
    warnings.length > 0 ? warnings[warnIndex % warnings.length] : (warnings[0] ?? null);

  const progressPanelOpen = openPanel === 'progress';
  const warningPanelOpen = openPanel === 'warning';

  return (
    <footer className="relative z-20 border-t bg-chrome">
      <div className="flex h-8 items-center gap-2 text-xs">
        {/* Três colunas iguais: as setas começam na MESMA margem da
            sidebar (pl-3), o progresso cai no centro exato da janela e o
            aviso encosta à direita. */}
        <fieldset
          className="m-0 flex min-w-0 flex-1 items-center gap-1 border-0 p-0 pl-3"
          aria-label={SHORTCUTS.navigation.label}
        >
          {NAV_HINTS.map((hint) => (
            <ArrowHint key={hint.key} hotkey={hint.key} label={hint.label} />
          ))}
        </fieldset>

        {/* Progresso (meio): fixo na abertura do arquivo. */}
        <div className="flex min-w-0 flex-1 items-center justify-center">
          {activeFile.progress && activeFile.kind ? (
            <Alert
              variant={progressPanelOpen ? 'info' : 'default'}
              role="button"
              tabIndex={0}
              aria-expanded={progressPanelOpen}
              onClick={() => setOpenPanel((p) => (p === 'progress' ? null : 'progress'))}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  setOpenPanel((p) => (p === 'progress' ? null : 'progress'));
                }
              }}
              className="h-6 w-fit max-w-full cursor-pointer items-center border-0 bg-transparent px-2 py-0 grid-cols-[auto_auto] shadow-none"
            >
              <Info className="shrink-0 opacity-70" />
              <span className="truncate">
                {activeFile.entryLabel}:{' '}
                <span className="font-medium">
                  {activeFile.progress.translated}/{activeFile.progress.total}
                </span>{' '}
                linhas traduzidas ({activeFile.progress.pct}%)
              </span>
            </Alert>
          ) : null}
        </div>

        {/* Aviso do backend (direita): rotação temporizada. */}
        <div className="flex min-w-0 flex-1 items-center justify-end pr-3">
          {warning ? (
            <Alert
              variant={warning.severity === 'error' ? 'destructive' : 'warning'}
              role="button"
              tabIndex={0}
              aria-expanded={warningPanelOpen}
              onClick={() => setOpenPanel((p) => (p === 'warning' ? null : 'warning'))}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  setOpenPanel((p) => (p === 'warning' ? null : 'warning'));
                }
              }}
              className={cn(
                'h-6 w-fit max-w-full cursor-pointer items-center border-0 bg-transparent px-2 py-0 grid-cols-[auto_auto] shadow-none',
                warningPanelOpen && 'font-medium',
              )}
            >
              <AlertTriangle className="shrink-0" />
              <span className="truncate" title={warning.message}>
                {warning.message}
              </span>
            </Alert>
          ) : null}
        </div>
      </div>

      {/* Alert detalhado — próximo à barra, ancorado à esquerda. */}
      {progressPanelOpen && activeFile.progress ? (
        <Alert
          variant="info"
          className="absolute bottom-9 left-3 z-30 w-fit max-w-[80ch] shadow-md"
        >
          <Info />
          <AlertTitle>
            {KIND_LABELS[activeFile.kind ?? 'events']}
            {activeFile.entryLabel ? ` · ${activeFile.entryLabel}` : ''}
          </AlertTitle>
          <AlertDescription>
            <p>
              {activeFile.progress.translated} de {activeFile.progress.total} linhas comparáveis
              traduzidas ({activeFile.progress.pct}%).
            </p>
            <p className="text-xs opacity-70">
              Contagem fixa na abertura do arquivo; linhas sem original de data/ ficam fora da
              contagem. As repetições linkadas (dedup) aparecem na tabela, mas o texto vive na def —
              não contam aqui.
            </p>
          </AlertDescription>
        </Alert>
      ) : null}

      {/* Alert detalhado — próximo à barra, ancorado à direita. Fechar
          DESCARTA o aviso. */}
      {warningPanelOpen && warning ? (
        <Alert
          variant={warning.severity === 'error' ? 'destructive' : 'warning'}
          className="absolute bottom-9 right-3 z-30 w-fit max-w-[90ch] shadow-md"
        >
          <AlertTriangle />
          <AlertTitle className="pr-6">{warning.message}</AlertTitle>
          <button
            type="button"
            aria-label="Fechar e descartar aviso"
            className="absolute right-2 top-2 rounded p-0.5 opacity-70 transition-opacity hover:opacity-100"
            onClick={(e) => {
              e.stopPropagation();
              dismissStatusBarWarning(warning.id);
              setOpenPanel(null);
            }}
          >
            <X size={14} />
          </button>
          <AlertDescription>
            <p className="text-xs opacity-80">
              {warning.kind in KIND_LABELS
                ? KIND_LABELS[warning.kind as keyof typeof KIND_LABELS]
                : warning.kind}
              {' · '}
              {warning.entryId} ({warning.version})
            </p>
            {warning.details ? (
              <div className="mt-1 flex flex-col gap-0.5 text-xs">
                <span>
                  {Number(warning.details.mods_rows ?? 0)} linhas no binário atual ·{' '}
                  {Number(warning.details.data_rows ?? 0)} no original de data/
                </span>
                {Array.isArray(warning.details.rows_somente_mods) &&
                  warning.details.rows_somente_mods.length > 0 && (
                    <span>
                      Só na tradução:{' '}
                      {formatDiagRows(warning.details.rows_somente_mods as DiagRow[])}
                    </span>
                  )}
                {Array.isArray(warning.details.rows_somente_data) &&
                  warning.details.rows_somente_data.length > 0 && (
                    <span>
                      Só no original:{' '}
                      {formatDiagRows(warning.details.rows_somente_data as DiagRow[])}
                    </span>
                  )}
              </div>
            ) : null}
            <p className="text-xs opacity-60">Fechar este alerta descarta o aviso da sessão.</p>
          </AlertDescription>
        </Alert>
      ) : null}
    </footer>
  );
}

interface DiagRow {
  index: number;
  name?: string;
  snippet?: string;
}

/** Lista compacta das rows divergentes: "index/snippet", no máximo 6. */
function formatDiagRows(rows: DiagRow[]): string {
  return (rows ?? [])
    .slice(0, 6)
    .map((r) => `${r.index}${r.name ? `:${r.name}` : ''} “${r.snippet ?? ''}”`)
    .join(', ');
}
