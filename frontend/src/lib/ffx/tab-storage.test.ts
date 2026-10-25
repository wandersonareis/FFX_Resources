import { describe, expect, it } from 'vitest';
import { normalizeTabIndex } from './tab-storage';

describe('normalizeTabIndex', () => {
  it('mantém um índice dentro da faixa de abas', () => {
    expect(normalizeTabIndex(2, 4)).toBe(2);
    expect(normalizeTabIndex(0, 1)).toBe(0);
  });

  it('cai para a primeira aba quando o índice está fora da faixa', () => {
    expect(normalizeTabIndex(4, 4)).toBe(0);
    expect(normalizeTabIndex(-1, 4)).toBe(0);
  });

  it('cai para a primeira aba quando o valor persistido não é número', () => {
    expect(normalizeTabIndex(Number.NaN, 4)).toBe(0);
    expect(normalizeTabIndex(1.5, 4)).toBe(0);
  });
});
