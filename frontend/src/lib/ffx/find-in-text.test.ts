import { describe, expect, it } from 'vitest';
import { findMatches, replaceMatches, visibleSegments } from './find-in-text';

// O motor da busca local casa SÓ o texto visível: tags {…} e quebras de
// linha ficam de fora da comparação, e a substituição nunca as toca. Estes
// testes fixam essa separação — é o que garante que substituir não quebra
// chip nem formatação.

const NO_OPTS = { matchCase: false, wholeWord: false };

describe('visibleSegments', () => {
  it('texto puro vira um único trecho visível', () => {
    expect(visibleSegments('Hello world')).toEqual([{ start: 0, end: 11 }]);
  });

  it('tags {…} e quebras interrompem o visível', () => {
    // "Oi {CLR:RED}mundo{TEXT_NEWLINE}!"
    const segments = visibleSegments('Oi {CLR:RED}mundo{TEXT_NEWLINE}!');
    expect(segments).toEqual([
      { start: 0, end: 3 }, // "Oi "
      { start: 12, end: 17 }, // "mundo"
      { start: 31, end: 32 }, // "!"
    ]);
  });

  it('token de chip (ex: BUTTON) também é tag (não visível)', () => {
    expect(visibleSegments('a{BUTTON:X}b')).toEqual([
      { start: 0, end: 1 },
      { start: 11, end: 12 },
    ]);
  });
});

describe('findMatches', () => {
  it('encontra ocorrências no texto visível', () => {
    const matches = findMatches('Hello world hello', 'hello', NO_OPTS);
    expect(matches).toEqual([
      { start: 0, end: 5 },
      { start: 12, end: 17 },
    ]);
  });

  it('ignora tags por padrão (a busca é por palavras, não tags)', () => {
    // "mundo" está dentro de {CLR:RED}? Não — aqui o texto é "mundo" após a tag.
    const matches = findMatches('Oi {CLR:RED}mundo{TEXT_NEWLINE}!', 'mundo', NO_OPTS);
    expect(matches).toEqual([{ start: 12, end: 17 }]);
    // Uma "busca" que casaria o NOME da tag não encontra nada.
    expect(findMatches('Oi {CLR:RED}mundo', 'CLR', NO_OPTS)).toEqual([]);
    expect(findMatches('Oi {TEXT_NEWLINE}mundo', 'TEXT_NEWLINE', NO_OPTS)).toEqual([]);
  });

  it('respeita matchCase', () => {
    expect(findMatches('Hello hello', 'hello', { matchCase: true, wholeWord: false })).toEqual([
      { start: 6, end: 11 },
    ]);
    expect(findMatches('Hello hello', 'hello', NO_OPTS)).toHaveLength(2);
  });

  it('respeita wholeWord (bordas não-palavra)', () => {
    const text = 'cat category cat.';
    // wholeWord: só os "cat" isolados (o de "category" e o colado em "." não).
    expect(findMatches(text, 'cat', { matchCase: false, wholeWord: true })).toEqual([
      { start: 0, end: 3 },
      { start: 13, end: 16 },
    ]);
  });

  it('não casa dentro de tags mesmo com wholeWord/case', () => {
    expect(findMatches('{TEXT_ITALIC}Oi', 'TEXT', NO_OPTS)).toEqual([]);
  });

  it('devolve vazio sem query ou sem texto', () => {
    expect(findMatches('', 'x', NO_OPTS)).toEqual([]);
    expect(findMatches('abc', '', NO_OPTS)).toEqual([]);
  });
});

describe('replaceMatches', () => {
  it('substitui o intervalo dado', () => {
    expect(replaceMatches('Hello world', [{ start: 0, end: 5 }], 'Hi')).toBe('Hi world');
  });

  it('substitui múltiplos intervalos (ordenados)', () => {
    const out = replaceMatches(
      'a b c',
      [
        { start: 0, end: 1 },
        { start: 4, end: 5 },
      ],
      'X',
    );
    expect(out).toBe('X b X');
  });

  it('nunca toca tags: substitui só o visível', () => {
    // "Oi {CLR:RED}mundo" — substitui "mundo" (visível), a tag fica intacta.
    const raw = 'Oi {CLR:RED}mundo';
    const matches = findMatches(raw, 'mundo', NO_OPTS);
    expect(replaceMatches(raw, matches, 'terra')).toBe('Oi {CLR:RED}terra');
  });

  it('com texto vazio de matches devolve o original', () => {
    expect(replaceMatches('abc', [], 'X')).toBe('abc');
  });
});
