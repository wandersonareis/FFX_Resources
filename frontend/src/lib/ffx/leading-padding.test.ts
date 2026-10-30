import { describe, expect, it } from 'vitest';
import { splitLeadingPadding, summarizePadding } from './leading-padding';

describe('splitLeadingPadding', () => {
  it('sem padding: body igual ao original', () => {
    expect(splitLeadingPadding('Hello')).toEqual({ padding: '', body: 'Hello' });
  });

  it('um {TEXT_NEWLINE} inicial', () => {
    expect(splitLeadingPadding('{TEXT_NEWLINE}Hello')).toEqual({
      padding: '{TEXT_NEWLINE}',
      body: 'Hello',
    });
  });

  it('vários tokens misturados', () => {
    expect(splitLeadingPadding('{TEXT_NEWLINE} {TEXT_NEWLINE}\n  Hello')).toEqual({
      padding: '{TEXT_NEWLINE} {TEXT_NEWLINE}\n  ',
      body: 'Hello',
    });
  });

  it('case-insensitive', () => {
    expect(splitLeadingPadding('{text_newline}Hi').body).toBe('Hi');
  });

  it('{TIME:00} NÃO é padding', () => {
    const r = splitLeadingPadding('{TIME:00}{TEXT_NEWLINE}Hi');
    expect(r.padding).toBe('');
    expect(r.body).toBe('{TIME:00}{TEXT_NEWLINE}Hi');
  });

  it('texto vazio', () => {
    expect(splitLeadingPadding('')).toEqual({ padding: '', body: '' });
  });
});

describe('summarizePadding', () => {
  it('resume quebras e espaços', () => {
    expect(summarizePadding('{TEXT_NEWLINE}  \n')).toBe('{TEXT_NEWLINE} ×2 · ␣ ×2');
  });
  it('vazio', () => {
    expect(summarizePadding('')).toBe('');
  });
});
