import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Entry } from '@/bindings/ffxresources/backend/uilog/models';
import { uiLog } from './uilog-store';

/**
 * Testes do buffer de exibição de logs no HOST WEB (drawer lateral): o
 * binding LogService é mockado — no browser puro (server mode) o runtime
 * Wails não existe e o hydrate tem de falhar em silêncio, deixando a lista
 * vazia em vez de quebrar o render.
 *
 * A store é singleton de módulo (mesma forma da produção): os testes de
 * hydrate rodam PRIMEIRO (o flag `hydrated` é por instância e, depois de
 * uma semente bem-sucedida, hydrate vira no-op) e os demais esvaziam o
 * buffer no beforeEach via uiLog.cleared().
 */

/** Espelho do MAX_LINES do store — mesmo teto do bufferCap do backend/uilog. */
const MAX_LINES = 10_000;

const { History, ClearLog } = vi.hoisted(() => ({
  History: vi.fn(async () => []),
  ClearLog: vi.fn(async () => undefined),
}));

vi.mock('../../../bindings/ffxresources/logservice', () => ({ History, ClearLog }));

function entry(partial: Partial<Entry> = {}): Entry {
  return { time: Date.now(), level: 'info', message: 'linha', ...partial } as Entry;
}

describe('hydrate (semeia do backend)', () => {
  it('falha de binding deixa a lista vazia sem lançar, e a corrida chama o binding uma vez só', async () => {
    History.mockRejectedValueOnce(new Error('sem runtime Wails'));

    // Duas chamadas em corrida (dois hosts montando ao mesmo tempo): o guard
    // `hydrating` tem de emitir UMA chamada ao backend.
    await Promise.all([uiLog.hydrate(), uiLog.hydrate()]);

    expect(History).toHaveBeenCalledTimes(1);
    expect(uiLog.snapshot().entries).toHaveLength(0);
  });

  it('semeia o histórico do backend e corta no teto (as mais antigas caem)', async () => {
    const seed = Array.from({ length: MAX_LINES + 5 }, (_, i) =>
      entry({ time: i + 1, message: `linha ${i}` }),
    );
    History.mockResolvedValueOnce(seed);

    // O hydrate anterior falhou: o flag ficou limpo e a nova tentativa roda.
    await uiLog.hydrate();

    const snap = uiLog.snapshot();
    expect(snap.entries).toHaveLength(MAX_LINES);
    expect(snap.entries[0].message).toBe('linha 5');
    expect(snap.entries[MAX_LINES - 1].message).toBe(`linha ${MAX_LINES + 4}`);
    expect(snap.revision).toBeGreaterThan(0);
  });

  it('é idempotente: o remount da janela não refaz a chamada ao backend', async () => {
    const calls = History.mock.calls.length;

    await uiLog.hydrate();

    expect(History.mock.calls.length).toBe(calls);
  });
});

describe('linhas ao vivo (evento UILog)', () => {
  beforeEach(() => {
    uiLog.cleared();
  });

  it('append publica no store e respeita o mesmo teto do backend', () => {
    for (let i = 0; i < MAX_LINES + 3; i++) {
      uiLog.append(entry({ message: `viva ${i}` }));
    }

    const snap = uiLog.snapshot();
    expect(snap.entries).toHaveLength(MAX_LINES);
    expect(snap.entries[0].message).toBe('viva 3');
    expect(snap.entries[MAX_LINES - 1].message).toBe(`viva ${MAX_LINES + 2}`);
  });

  it('cada linha notifica os assinantes (revision sobe de um em um)', () => {
    const before = uiLog.snapshot().revision;

    uiLog.append(entry());
    uiLog.append(entry());

    expect(uiLog.snapshot().revision).toBe(before + 2);
  });

  it('snapshot é cópia: linhas novas não reescrevem snapshots antigos', () => {
    const before = uiLog.snapshot();

    uiLog.append(entry());

    expect(before.entries).toHaveLength(0);
    expect(uiLog.snapshot().entries).toHaveLength(1);
  });

  it('cleared esvazia o buffer (eco do evento UILogCleared)', () => {
    uiLog.append(entry());
    uiLog.append(entry());

    uiLog.cleared();

    expect(uiLog.snapshot().entries).toHaveLength(0);
  });
});

describe('botão Limpar', () => {
  beforeEach(() => {
    uiLog.cleared();
    ClearLog.mockClear();
  });

  it('clear delega ao backend (o eco UILogCleared é quem esvazia a lista)', () => {
    uiLog.append(entry());

    uiLog.clear();

    expect(ClearLog).toHaveBeenCalledTimes(1);
    // Simula o eco do backend (o host real recebe o evento UILogCleared).
    uiLog.cleared();
    expect(uiLog.snapshot().entries).toHaveLength(0);
  });

  it('falha do backend é silenciosa: o clear nunca lança nem derruba o host', async () => {
    ClearLog.mockRejectedValueOnce(new Error('app fechando'));

    expect(() => uiLog.clear()).not.toThrow();
    await new Promise((resolve) => setTimeout(resolve, 0)); // assenta a promise flutuante

    expect(ClearLog).toHaveBeenCalledTimes(1);
  });
});
