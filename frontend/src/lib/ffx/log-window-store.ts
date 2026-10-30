import { useSelector } from '@tanstack/react-store';
import { createStore } from '@tanstack/store';
import { CloseWindow, OpenWindow } from '../../../bindings/ffxresources/windowservice';
import { isDesktop } from './env';
import { loggedToast } from './toast-logged';

/**
 * Janela/painel de LOGS — estado de MÓDULO (mesmo padrão do
 * config-dialog-store): o checkbox do header é o único controle, e qualquer
 * ponto do app pode consultar/alternar sem prop drilling.
 *
 * SÓ SESSÃO, sem localStorage: toda inicialização começa desmarcado (a
 * janela abre por ação direta do usuário, nunca sozinha).
 *
 * Desktop → JANELA filha lateral (WindowService.OpenWindow("logs"), à
 * direita da principal); web/server mode → DRAWER Sheet não-modal na
 * lateral direita (LogDrawer, renderizado pelo AppShell) — o mesmo
 * desenho do config: isDesktop() decide o host, o estado é um só.
 */
export const logWindowStore = createStore<{ open: boolean }>({
  open: false,
});

/** Espelha o estado sem tocar nas janelas (eco de eventos do backend). */
export function setLogWindowOpen(open: boolean): void {
  logWindowStore.setState((prev) => (prev.open === open ? prev : { ...prev, open }));
}

/**
 * Alterna a exibição de logs: marca/desmarca o checkbox E abre/fecha o
 * host correspondente (janela no desktop; o drawer web só lê o store).
 * Falha ao abrir (app fechando) desmarca de volta — checkbox nunca fica
 * marcado sem exibição atrás.
 */
export function toggleLogWindow(open: boolean): void {
  setLogWindowOpen(open);
  if (!isDesktop()) return;
  if (open) {
    void OpenWindow('logs').catch((error: unknown) => {
      setLogWindowOpen(false);
      const message = error instanceof Error ? error.message : String(error ?? 'Erro');
      loggedToast.error(message, { duration: 4000 });
    });
    return;
  }
  // Fechar falha só se a janela nem existir (X já a fechou) — silencioso.
  void CloseWindow('logs').catch(() => undefined);
}

/** Snapshot reativo do estado (rerender ao marcar/desmarcar). */
export function useLogWindowOpen(): boolean {
  return useSelector(logWindowStore, (s) => s.open);
}
