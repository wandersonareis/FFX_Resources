import { describe, expect, it } from 'vitest';
import {
  buttonSpriteClasses,
  getColorTagName,
  getResolvedColorFromTag,
  parseCssColor,
} from './game-text-tags';

// buttonSpriteClasses recebe o inner pelo parser ("BUTTON:…", sem chaves) e
// o attrs.value direto pelo renderHTML ("{BUTTON:…}") — as DUAS formas têm
// que resolver os mesmos glifos, senão o editor mostra chip textual onde o
// parser emitiu ícone (o efeito de valores com chaves).
describe('buttonSpriteClasses', () => {
  it('aceita a tag completa com chaves e o inner sem chaves', () => {
    expect(buttonSpriteClasses('{BUTTON:31:X}')).toEqual(['gb-a']);
    expect(buttonSpriteClasses('BUTTON:31:X')).toEqual(['gb-a']);
  });

  it('combos emitem as setas na ordem do nome do código', () => {
    expect(buttonSpriteClasses('{BUTTON:43:Direcional Up+Right}')).toEqual([
      'gb-arrow-up',
      'gb-arrow-right',
    ]);
    expect(buttonSpriteClasses('{BUTTON:4B:Direcional Up+Left+Right}')).toEqual([
      'gb-arrow-up',
      'gb-arrow-left',
      'gb-arrow-right',
    ]);
  });

  it('hex em caixa baixa resolve igual (texto digitado à mão)', () => {
    expect(buttonSpriteClasses('{button:4f:direcional all}')).toEqual([
      'gb-cursor',
    ]);
  });

  it('códigos sem glifo devolvem lista vazia (chip textual)', () => {
    expect(buttonSpriteClasses('{BUTTON:2D:Dummy}')).toEqual([]);
    expect(buttonSpriteClasses('{PAUSE}')).toEqual([]);
    expect(buttonSpriteClasses('{BUTTON:ZZ:X}')).toEqual([]);
  });
});

describe('getColorTagName', () => {
  it('nome e hex da paleta casam com o nome canonico', () => {
    expect(getColorTagName('BLUE')).toBe('BLUE');
    expect(getColorTagName('blue')).toBe('BLUE');
    expect(getColorTagName('#2563EB')).toBe('BLUE');
    expect(getColorTagName('#fff')).toBe('WHITE');
  });

  it('rgb() de HTML externo vira a cor da paleta (nunca RGB(...))', () => {
    expect(getColorTagName('rgb(37, 99, 235)')).toBe('BLUE');
    expect(getColorTagName('rgba(37, 99, 235, 0.4)')).toBe('BLUE');
    expect(getColorTagName('rgb(37 99 235 / 40%)')).toBe('BLUE');
  });

  it('hsl() e nomes CSS tambem sao normalizados', () => {
    expect(getColorTagName('hsl(221, 83%, 53%)')).toBe('BLUE');
    expect(getColorTagName('red')).toBe('RED');
  });

  it('cor fora da paleta cai na mais proxima (o jogo so tem 8)', () => {
    expect(getColorTagName('#1d4ed8')).toBe('BLUE');
    expect(getColorTagName('gold')).toBe('YELLOW');
  });

  it('valor que nao e cor nao vira tag', () => {
    expect(getColorTagName('inherit')).toBeNull();
    expect(getColorTagName('var(--x)')).toBeNull();
    expect(getColorTagName('')).toBeNull();
    expect(getColorTagName(null)).toBeNull();
  });
});

describe('getResolvedColorFromTag', () => {
  it('nomes e bytes canonicos do decoder resolvem', () => {
    expect(getResolvedColorFromTag('BLUE')).toEqual({
      hex: '#2563eb',
      isReset: false,
    });
    expect(getResolvedColorFromTag('88')).toEqual({
      hex: '#2563eb',
      isReset: false,
    });
    expect(getResolvedColorFromTag('41')).toEqual({ hex: null, isReset: true });
    expect(getResolvedColorFromTag('WHITE')).toEqual({
      hex: null,
      isReset: true,
    });
  });

  it('rascunho antigo corrompido ({CLR:RGB(...)}) volta a ser cor', () => {
    expect(getResolvedColorFromTag('RGB(37, 99, 235)')).toEqual({
      hex: '#2563eb',
      isReset: false,
    });
    expect(getResolvedColorFromTag('#DC2626')).toEqual({
      hex: '#DC2626',
      isReset: false,
    });
  });

  it('byte desconhecido continua sem resolucao (vira chip, preserva o byte)', () => {
    expect(getResolvedColorFromTag('58')).toBeNull();
    expect(getResolvedColorFromTag('00')).toBeNull();
  });
});

describe('parseCssColor', () => {
  it('le hex curto/longo, rgb, rgba, hsl e nome', () => {
    expect(parseCssColor('#0f0')).toEqual({ r: 0, g: 255, b: 0 });
    expect(parseCssColor('#00ff00')).toEqual({ r: 0, g: 255, b: 0 });
    expect(parseCssColor('rgb(0, 255, 0)')).toEqual({ r: 0, g: 255, b: 0 });
    expect(parseCssColor('rgba(0,255,0,0.2)')).toEqual({ r: 0, g: 255, b: 0 });
    expect(parseCssColor('hsl(120, 100%, 50%)')).toEqual({ r: 0, g: 255, b: 0 });
  });

  it('texto qualquer nao vira cor', () => {
    expect(parseCssColor('initial')).toBeNull();
    expect(parseCssColor('rgb(1,2)')).toBeNull();
    expect(parseCssColor('#12')).toBeNull();
  });
});
