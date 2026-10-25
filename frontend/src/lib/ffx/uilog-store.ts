import { useEffect } from 'react';
import { useSelector } from '@tanstack/react-store';
import { createStore } from '@tanstack/store';
import { ClearLog, History } from '../../../bindings/ffxresources/logservice';
import type { Entry } from '../../../bindings/ffxresources/backend/uilog/models';

/**
 * Buffer de exibição dos logs de operação (backend/uilog) — store de MÓDULO
 * por webview: a janela desktop e o drawer web têm cada um a própria
 * instância, semeada por LogService.History() no mount e mantida viva pelos
 * eventos UILog/UILogCleared.
 *
 * Quem escreve: o LogPanel (único assinante dos eventos em cada janela).
 * Quem lê: o LogPanel (a lista) E os controles da barra do host
 * (LogPanelControls — contador + Limpar na titlebar do WindowChrome no
 * desktop, no header do Sheet na web). Uma barra só, sem toolbar extra.
 */

/** Espelho do bufferCap do backend — mesmo teto no cliente. */
const MAX_LINES = 10000;

interface UILogSnapshot {
  revision: number;
  entries: readonly Entry[];
}

const logStore = createStore<UILogSnapshot>({ revision: 0, entries: [] });

class UILogStore {
  private entries: Entry[] = [];
  private hydrated = false;
  private hydrating = false;

  private notify(): void {
    logStore.setState((prev) => ({
      revision: prev.revision + 1,
      entries: [...this.entries],
    }));
  }

  /**
   * Semeia com o histórico da sessão (idempotente — remount da janela);
   * falha de binding (dev puro no browser) deixa a lista vazia.
   */
  async hydrate(): Promise<void> {
    if (this.hydrated || this.hydrating) return;
    this.hydrating = true;
    try {
      const all = await History();
      this.entries = ((all ?? []) as Entry[]).slice(-MAX_LINES);
      this.hydrated = true;
      this.notify();
    } catch {
      // sem runtime Wails — segue vazio
    } finally {
      this.hydrating = false;
    }
  }

  /** Linha nova (payload cru do evento UILog — mesma forma de Entry). */
  append(entry: Entry): void {
    this.entries.push(entry);
    if (this.entries.length > MAX_LINES) {
      this.entries = this.entries.slice(-MAX_LINES);
    }
    this.notify();
  }

  /** Eco do UILogCleared: o backend zerou o histórico da sessão. */
  cleared(): void {
    this.entries = [];
    this.notify();
  }

  /** Botão "Limpar": o backend zera e o eco UILogCleared esvazia a lista. */
  clear(): void {
    void ClearLog().catch(() => undefined);
  }

  /** Snapshot atual da store (leitura imperativa fora do React). */
  snapshot(): UILogSnapshot {
    return logStore.state;
  }
}

export const uiLog = new UILogStore();

/** Snapshot reativo (rerender a cada linha/limpeza); hidrata no mount. */
export function useUILog(): UILogSnapshot {
  const snapshot = useSelector(logStore, (s) => s);

  useEffect(() => {
    void uiLog.hydrate();
  }, []);

  return snapshot;
}
