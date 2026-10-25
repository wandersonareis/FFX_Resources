import { describe, expect, it } from 'vitest';
import { buttonSpriteClasses } from './game-text-tags';

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
