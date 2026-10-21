'use client';

import { useEffect, useReducer, useState } from 'react';
import { ArrowDownToLine, CheckCircle2, Loader2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { parseError } from '@/lib/ffx/error-handler';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';
import { CheckNow, CurrentVersion, Restart } from '@/bindings/ffxresources/updateservice';
import type { UpProgress, UpRelease } from '@/lib/ffx/updater-types';
import { initialBannerState, updateBannerReducer } from '@/lib/ffx/update-banner-state';

/** Rótulos do estado 'working' por sub-etapa (mesmos textos da janela nativa). */
const WORKING_LABEL = {
  check: 'Verificando atualizações…',
  download: 'Baixando atualização…',
  verify: 'Verificando a integridade…',
  install: 'Preparando a nova versão…',
} as const;

/** Converte bytes em texto curto (7,3 MB) — só a banner usa. */
function fmtBytes(n?: number): string {
  if (!n || n <= 0) return '';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

/**
 * Banner de atualização (canto inferior direito, acima da StatusBar).
 *
 * Estado: reducer puro em lib/ffx/update-banner-state.ts alimentado pelos
 * eventos `wails:updater:*` (bus do app — docs/updater.md §6). Ações:
 * bindings do UpdateService (abrir a janela nativa / reiniciar). Oculta por
 * padrão: sem evento não há UI nenhuma (nada de piscar no startup; em modo
 * servidor o updater é stub e nenhum evento chega).
 *
 * Existe junto da janela nativa porque é a única coisa que sobrevive quando
 * o usuário fecha a janela de update no meio do download.
 */
export function UpdateBanner() {
  const [state, dispatch] = useReducer(updateBannerReducer, initialBannerState);
  const [currentVersion, setCurrentVersion] = useState('');

  useEffect(() => {
    void CurrentVersion()
      .then(setCurrentVersion)
      .catch(() => setCurrentVersion(''));
  }, []);

  useWailsEvent('wails:updater:update-available', (data) => {
    const rel = data as UpRelease | undefined;
    if (rel && typeof rel.version === 'string') dispatch({ type: 'available', release: rel });
  });
  useWailsEvent('wails:updater:check-started', () => dispatch({ type: 'checking' }));
  useWailsEvent('wails:updater:download-started', () => dispatch({ type: 'download-started' }));
  useWailsEvent('wails:updater:download-progress', (data) =>
    dispatch({ type: 'download-progress', progress: data as UpProgress })
  );
  useWailsEvent('wails:updater:verifying', () => dispatch({ type: 'verifying' }));
  useWailsEvent('wails:updater:installing', () => dispatch({ type: 'installing' }));
  useWailsEvent('wails:updater:update-ready', (data) => {
    const rel = data as UpRelease | undefined;
    dispatch({ type: 'ready', release: rel });
  });
  useWailsEvent('wails:updater:no-update', () => dispatch({ type: 'no-update' }));
  useWailsEvent('wails:updater:error', (data) => {
    const info = data as { stage?: string; message?: string } | undefined;
    toast.error(`Atualização falhou: ${info?.message ?? 'erro inesperado'}`);
    dispatch({ type: 'error' });
  });

  const handleCheck = () => {
    void CheckNow().catch((err) => toast.error(parseError(err)));
  };
  const handleRestart = () => {
    void Restart().catch((err) => toast.error(parseError(err)));
  };
  const dismiss = () => dispatch({ type: 'dismiss' });

  if (state.phase === 'hidden') return null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="fixed bottom-12 right-4 z-50 flex w-80 items-start gap-2 rounded-xl border bg-card p-3 text-card-foreground shadow-lg"
    >
      <button
        type="button"
        aria-label="Dispensar aviso de atualização"
        onClick={dismiss}
        className="absolute right-2 top-2 rounded p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
      >
        <X className="h-4 w-4" />
      </button>

      {state.phase === 'available' && (
        <>
          <ArrowDownToLine className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
          <div className="min-w-0 flex-1 pr-5">
            <p className="text-sm font-semibold">Atualização disponível</p>
            <p className="text-xs text-muted-foreground">
              {currentVersion ? `v${currentVersion} → ` : ''}v{state.release?.version}
              {state.release?.artifact?.size
                ? ` · ${fmtBytes(state.release.artifact.size)}`
                : ''}
            </p>
            <div className="mt-2 flex gap-2">
              <Button size="sm" onClick={handleCheck}>
                Ver detalhes
              </Button>
              <Button size="sm" variant="ghost" onClick={dismiss}>
                Depois
              </Button>
            </div>
          </div>
        </>
      )}

      {state.phase === 'working' && (
        <>
          <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin text-primary" />
          <div className="min-w-0 flex-1 pr-5">
            <p className="text-sm font-semibold">
              {state.step ? WORKING_LABEL[state.step] : 'Atualizando…'}
            </p>
            {state.step === 'download' && state.progress && (
              <>
                <p className="text-xs text-muted-foreground">
                  {state.progress.total > 0
                    ? `${Math.round((state.progress.written / state.progress.total) * 100)}% · ${fmtBytes(state.progress.written)} de ${fmtBytes(state.progress.total)}`
                    : fmtBytes(state.progress.written)}
                </p>
                <progress
                  className="mt-1.5 h-1 w-full"
                  max={state.progress.total > 0 ? 100 : undefined}
                  value={
                    state.progress.total > 0
                      ? Math.round((state.progress.written / state.progress.total) * 100)
                      : undefined
                  }
                />
              </>
            )}
          </div>
        </>
      )}

      {state.phase === 'ready' && (
        <>
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-green-500" />
          <div className="min-w-0 flex-1 pr-5">
            <p className="text-sm font-semibold">Atualização pronta</p>
            <p className="text-xs text-muted-foreground">
              v{state.release?.version ?? ''} — reinicie para aplicar
            </p>
            <div className="mt-2">
              <Button size="sm" onClick={handleRestart}>
                Reiniciar agora
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
