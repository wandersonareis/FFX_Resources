import { describe, expect, it } from 'vitest';
import {
  findUnclosedFragment,
  isProtectedTagInner,
  missingProtectedTags,
  newUnclosedFragment,
  removeUnclosedFragment,
  restoreProtectedTags,
} from './protected-tags';

describe('isProtectedTagInner', () => {
  it('protege valores calculados pelo jogo', () => {
    expect(isProtectedTagInner('CMD:01:02')).toBe(true);
    expect(isProtectedTagInner('HEX:DEADBEEF')).toBe(true);
    expect(isProtectedTagInner('UNKCHR:5A')).toBe(true);
    expect(isProtectedTagInner('VAR')).toBe(true);
    expect(isProtectedTagInner('VAR:count')).toBe(true);
    expect(isProtectedTagInner('PUA:A0: ')).toBe(true);
  });

  it('não protege tags nomeadas nem de formatação', () => {
    expect(isProtectedTagInner('PC:01:YUNA')).toBe(false);
    expect(isProtectedTagInner('MCR:s06:l3F:"Yevon"')).toBe(false);
    expect(isProtectedTagInner('BUTTON:30:TRIANGLE')).toBe(false);
    expect(isProtectedTagInner('ICON:80')).toBe(false);
    expect(isProtectedTagInner('PAUSE')).toBe(false);
    expect(isProtectedTagInner('CLR:RED')).toBe(false);
  });
});

describe('missingProtectedTags', () => {
  it('devolve vazia quando nenhuma protegida foi removida', () => {
    const text = 'Press {BUTTON:31:X} to jump{CMD:01}';
    expect(missingProtectedTags(text, text)).toEqual([]);
    // Renomear texto livre não mexe nas tags.
    expect(missingProtectedTags(text, 'Aperte {BUTTON:31:X} para pular{CMD:01}')).toEqual([]);
  });

  it('lista as removidas, na ordem do original', () => {
    const original = 'a {VAR:n} b {CMD:FF} c {PUA:A0:X} d';
    const edited = 'a b c d';
    expect(missingProtectedTags(original, edited)).toEqual(['{VAR:n}', '{CMD:FF}', '{PUA:A0:X}']);
  });

  it('conta multiplicidade (duas iguais, uma sumiu → uma faltando)', () => {
    const original = '{CMD:01} {CMD:01} fim';
    expect(missingProtectedTags(original, '{CMD:01} fim')).toEqual(['{CMD:01}']);
    expect(missingProtectedTags(original, 'fim')).toEqual(['{CMD:01}', '{CMD:01}']);
  });

  it('ignora tags não protegidas removidas de propósito', () => {
    const original = 'Oi {PC:01:YUNA} {PAUSE} {MCR:s01:l05:"Hi"}';
    expect(missingProtectedTags(original, 'Oi')).toEqual([]);
  });
});

describe('restoreProtectedTags', () => {
  it('devolve o texto intacto quando não falta nada', () => {
    const original = 'Hello {VAR:x} world';
    expect(restoreProtectedTags(original, 'Olá {VAR:x} mundo')).toBe('Olá {VAR:x} mundo');
    expect(restoreProtectedTags(original, original)).toBe(original);
  });

  it('reinsere as protegidas e deixa a conferência zerada', () => {
    const original = 'Press {VAR:a} now{CMD:FF}';
    const edited = 'Aperte agora';
    const restored = restoreProtectedTags(original, edited);
    expect(missingProtectedTags(original, restored)).toEqual([]);
    expect(restored).toContain('{VAR:a}');
    expect(restored).toContain('{CMD:FF}');
    // O texto do tradutor é preservado.
    expect(restored).toContain('Aperte');
    expect(restored).toContain('agora');
  });

  it('não reinsere tags não protegidas', () => {
    const original = 'Oi {PC:01:YUNA}{PAUSE} tudo bem';
    const restored = restoreProtectedTags(original, 'Tudo ótimo');
    expect(restored).not.toContain('{PC:01:YUNA}');
    expect(restored).not.toContain('{PAUSE}');
    expect(missingProtectedTags(original, restored)).toEqual([]);
  });

  it('reinsere todas as ocorrências removidas', () => {
    const original = '{CMD:01} {CMD:01} fim';
    const restored = restoreProtectedTags(original, 'fim');
    expect(missingProtectedTags(original, restored)).toEqual([]);
    expect(restored.match(/\{CMD:01\}/g)).toHaveLength(2);
  });

  it('mantém protegida ainda presente e reinsere só a que sumiu', () => {
    const original = '{VAR:a} meio {VAR:b}';
    const restored = restoreProtectedTags(original, '{VAR:a} fim');
    expect(missingProtectedTags(original, restored)).toEqual([]);
    expect(restored).toContain('{VAR:a}');
    expect(restored).toContain('{VAR:b}');
  });
});

describe('fragmentos abertos', () => {
  it('findUnclosedFragment pega o { pendente no fim', () => {
    expect(findUnclosedFragment('abc')).toBeNull();
    expect(findUnclosedFragment('a{b}c')).toBeNull();
    expect(findUnclosedFragment('texto {PC')).toBe('{PC');
    expect(findUnclosedFragment('fecha {PC:01:YUNA} e abre {MCR')).toBe('{MCR');
  });

  it('newUnclosedFragment só acusa o que a edição introduziu', () => {
    expect(newUnclosedFragment('abc', 'abc {PC')).toBe('{PC');
    expect(newUnclosedFragment('{PC', 'x {PC')).toBeNull();
    expect(newUnclosedFragment('abc', 'abc')).toBeNull();
    // Fechou o que o original tinha aberto → não é caso novo.
    expect(newUnclosedFragment('{PC', '{PC:01:YUNA}')).toBeNull();
  });
});

describe('removeUnclosedFragment', () => {
  it('corta só o sufixo aberto', () => {
    expect(removeUnclosedFragment('Olá {PC')).toBe('Olá');
    expect(removeUnclosedFragment('{PC')).toBe('');
    expect(removeUnclosedFragment('abc')).toBe('abc');
    expect(removeUnclosedFragment('{PC:01:YUNA}')).toBe('{PC:01:YUNA}');
    expect(removeUnclosedFragment('a {PAUSE} b {MCR')).toBe('a {PAUSE} b');
  });

  it('some com o espaço digitado junto da tentativa abortada', () => {
    expect(removeUnclosedFragment('Olá {PC', 'Olá')).toBe('Olá');
  });

  it('preserva o espaço que já existia no original', () => {
    // original termina em espaço → só o trecho sai, o resto fica intacto.
    expect(removeUnclosedFragment('Olá  {PC', 'Olá ')).toBe('Olá  ');
  });

  it('deixa o texto limpo para o gate passar', () => {
    const original = 'Olá mundo';
    const edited = 'Olá mundo {';
    const fixed = removeUnclosedFragment(edited, original);
    expect(fixed).toBe(original);
    expect(newUnclosedFragment(original, fixed)).toBeNull();
  });
});
