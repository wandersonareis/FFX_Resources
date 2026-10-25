import { describe, expect, it } from 'vitest';
import {
  LAYOUT_MAX_AGE_MS,
  parseLayoutConfig,
} from './layout-storage';

const NOW = 1_700_000_000_000;
const DAY = 24 * 60 * 60 * 1000;

function raw(payload: unknown): string {
  return JSON.stringify(payload);
}

describe('parseLayoutConfig', () => {
  it('devolve null quando nunca houve nada salvo', () => {
    expect(parseLayoutConfig(null, NOW)).toBeNull();
  });

  it('devolve null para JSON malformado', () => {
    expect(parseLayoutConfig('{não é json', NOW)).toBeNull();
  });

  it('devolve null quando o root não é objeto', () => {
    expect(parseLayoutConfig('42', NOW)).toBeNull();
    expect(parseLayoutConfig('[1,2,3]', NOW)).toBeNull();
    expect(parseLayoutConfig('null', NOW)).toBeNull();
  });

  it('devolve null quando savedAt não é número finito', () => {
    expect(parseLayoutConfig(raw({ sidebar: { tree: 30 }, savedAt: 'ontem' }), NOW)).toBeNull();
    expect(parseLayoutConfig(raw({ sidebar: { tree: 30 }, savedAt: Number.NaN }), NOW)).toBeNull();
    expect(parseLayoutConfig(raw({ sidebar: { tree: 30 } }), NOW)).toBeNull();
  });

  it('aceita o layout salvo pouco antes de agora', () => {
    const config = parseLayoutConfig(
      raw({ sidebar: { tree: 27, main: 73 }, savedAt: NOW - 5 * DAY }),
      NOW
    );
    expect(config).not.toBeNull();
    expect(config?.sidebar).toEqual({ tree: 27, main: 73 });
    expect(config?.savedAt).toBe(NOW - 5 * DAY);
  });

  it('mantém os dois grupos independentes', () => {
    const config = parseLayoutConfig(
      raw({
        sidebar: { tree: 25, main: 75 },
        imageInfo: { preview: 70, info: 30 },
        savedAt: NOW,
      }),
      NOW
    );
    expect(config?.sidebar).toEqual({ tree: 25, main: 75 });
    expect(config?.imageInfo).toEqual({ preview: 70, info: 30 });
  });

  it('expira com mais de 30 dias e devolve null para descartar a chave', () => {
    expect(
      parseLayoutConfig(
        raw({ sidebar: { tree: 30, main: 70 }, savedAt: NOW - LAYOUT_MAX_AGE_MS - 1 }),
        NOW
      )
    ).toBeNull();
  });

  it('ainda vale exatamente no limite de 30 dias', () => {
    const config = parseLayoutConfig(
      raw({ sidebar: { tree: 30, main: 70 }, savedAt: NOW - LAYOUT_MAX_AGE_MS }),
      NOW
    );
    expect(config).not.toBeNull();
  });

  it('descarta savedAt no futuro (relógio andou para trás)', () => {
    expect(
      parseLayoutConfig(raw({ sidebar: { tree: 30 }, savedAt: NOW + LAYOUT_MAX_AGE_MS + 1 }), NOW)
    ).toBeNull();
  });

  it('ignora um grupo malformado sem derrubar o outro', () => {
    const config = parseLayoutConfig(
      raw({
        // fora da faixa 0..100 (percentual) => grupo inválido
        sidebar: { tree: 150 },
        imageInfo: { preview: 60, info: 40 },
        savedAt: NOW,
      }),
      NOW
    );
    expect(config?.sidebar).toBeNull();
    expect(config?.imageInfo).toEqual({ preview: 60, info: 40 });
  });

  it('rejeita percentual não finito ou <= 0', () => {
    expect(
      parseLayoutConfig(raw({ sidebar: { tree: 0, main: 100 }, savedAt: NOW }), NOW)?.sidebar
    ).toBeNull();
    expect(
      parseLayoutConfig(raw({ sidebar: { tree: -5, main: 105 }, savedAt: NOW }), NOW)?.sidebar
    ).toBeNull();
    expect(
      parseLayoutConfig(raw({ sidebar: { tree: Number.POSITIVE_INFINITY }, savedAt: NOW }), NOW)
        ?.sidebar
    ).toBeNull();
  });

  it('rejeita grupo vazio ou não-objeto', () => {
    expect(parseLayoutConfig(raw({ sidebar: {}, savedAt: NOW }), NOW)?.sidebar).toBeNull();
    expect(parseLayoutConfig(raw({ sidebar: [], savedAt: NOW }), NOW)?.sidebar).toBeNull();
    expect(parseLayoutConfig(raw({ sidebar: 'tree', savedAt: NOW }), NOW)?.sidebar).toBeNull();
    expect(parseLayoutConfig(raw({ sidebar: null, savedAt: NOW }), NOW)?.sidebar).toBeNull();
  });
});
