import { afterEach, beforeEach, describe, expect, it, jest, mock } from 'bun:test';
import { isDesktop } from './env';
import {
  logWindowStore,
  setLogWindowOpen,
  toggleLogWindow,
} from './log-window-store';

/**
 * Testes do checkbox "Logs" no HOST WEB (server mode / browser puro): com o
 * env real em modo web (sem window), o toggle NÃO pode chamar WindowService
 * — o host é o drawer (LogDrawer), que lê só o store. Para o caminho
 * desktop, o MESMO env real é ligado definindo um window de mentira
 * (chrome.webview.postMessage, o sinal do WebView2): o teste exercita a
 * detecção de verdade, sem mockar ./env.
 *
 * Os bindings de janela são mockados (no browser puro eles falham) e o
 * toast de erro é mockado para os assertion de feedback.
 */

const OpenWindow = jest.fn(async (_name: string) => undefined);
const CloseWindow = jest.fn(async (_name: string) => undefined);
mock.module('@/bindings/ffxresources/windowservice', () => ({ OpenWindow, CloseWindow }));

const toastError = jest.fn();
mock.module('./toast-logged', () => ({
  loggedToast: Object.assign(jest.fn(), {
    error: toastError,
    info: jest.fn(),
    success: jest.fn(),
    warning: jest.fn(),
    loading: jest.fn(),
    message: jest.fn(),
  }),
}));

/** Devolve o ambiente ao modo web (sem window) entre os testes. */
const hadWindow = 'window' in globalThis;
const realWindow = globalThis.window;

/** Liga o env real para o modo desktop (sinal do WebView2). */
function asDesktop(): void {
  (globalThis as { window?: unknown }).window = {
    chrome: { webview: { postMessage: () => undefined } },
  };
}

beforeEach(() => {
  setLogWindowOpen(false);
  OpenWindow.mockClear();
  CloseWindow.mockClear();
  toastError.mockClear();
});

afterEach(() => {
  if (hadWindow) {
    globalThis.window = realWindow;
  } else {
    delete (globalThis as { window?: unknown }).window;
  }
});

describe('env real (o anfitrião decide o host)', () => {
  it('no browser puro isDesktop é false — o host é o drawer', () => {
    expect(isDesktop()).toBe(false);
  });

  it('com chrome.webview.postMessage isDesktop é true — o host é a janela', () => {
    asDesktop();
    expect(isDesktop()).toBe(true);
  });
});

describe('web (drawer): o toggle só mexe no store', () => {
  it('marcar abre o drawer sem tocar no WindowService', () => {
    toggleLogWindow(true);

    expect(logWindowStore.state.open).toBe(true);
    expect(OpenWindow).not.toHaveBeenCalled();
    expect(toastError).not.toHaveBeenCalled();
  });

  it('desmarcar fecha o drawer sem tocar no WindowService', () => {
    setLogWindowOpen(true);

    toggleLogWindow(false);

    expect(logWindowStore.state.open).toBe(false);
    expect(CloseWindow).not.toHaveBeenCalled();
  });
});

describe('espelho de estado (checkbox nunca mente)', () => {
  it('setLogWindowOpen alterna o estado lido pelo drawer', () => {
    setLogWindowOpen(true);
    expect(logWindowStore.state.open).toBe(true);
    setLogWindowOpen(false);
    expect(logWindowStore.state.open).toBe(false);
  });

  it('setLogWindowOpen repetido não notifica os assinantes de novo', () => {
    let notified = 0;
    const sub = logWindowStore.subscribe(() => {
      notified += 1;
    });

    setLogWindowOpen(true);
    setLogWindowOpen(true);

    expect(notified).toBe(1);
    sub.unsubscribe();
  });
});

describe('desktop (janela lateral)', () => {
  it('marcar abre a janela filha "logs"', () => {
    asDesktop();

    toggleLogWindow(true);

    expect(OpenWindow).toHaveBeenCalledWith('logs');
    expect(logWindowStore.state.open).toBe(true);
  });

  it('desmarcar fecha a janela (o caminho de desfazer do X)', () => {
    asDesktop();
    setLogWindowOpen(true);

    toggleLogWindow(false);

    expect(CloseWindow).toHaveBeenCalledWith('logs');
    expect(logWindowStore.state.open).toBe(false);
  });

  it('falha ao abrir desmarca de volta e mostra o erro', async () => {
    asDesktop();
    OpenWindow.mockRejectedValueOnce(new Error('janela indisponível'));

    toggleLogWindow(true);

    // O checkbox não fica marcado sem exibição atrás (o catch roda no
    // microtask seguinte — a promise rejeitada já está na fila).
    expect(logWindowStore.state.open).toBe(true);

    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(logWindowStore.state.open).toBe(false);
    expect(toastError).toHaveBeenCalledWith('janela indisponível', {
      duration: 4000,
    });
  });

  it('falha ao fechar é silenciosa (o X da janela já pode tê-la fechado)', async () => {
    asDesktop();
    CloseWindow.mockRejectedValueOnce(new Error('janela já fechada'));

    toggleLogWindow(false);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(logWindowStore.state.open).toBe(false);
    expect(toastError).not.toHaveBeenCalled();
  });
});
