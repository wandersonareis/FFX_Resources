import { describe, expect, it } from 'vitest';
import { vbfSelection } from './vbf-selection';

const ROOT = 'vbf-selection-test-container';

describe('vbfSelection: regras recursivas da árvore', () => {
  it('marcar diretório e desmarcar um filho cria uma exceção', () => {
    const dir = 'ffx_data/gamedata/folder';
    const one = `${dir}/one.bin`;
    const two = `${dir}/two.bin`;

    vbfSelection.set(ROOT, dir, true);
    vbfSelection.set(ROOT, one, false);

    expect(vbfSelection.has(ROOT, one)).toBe(false);
    expect(vbfSelection.has(ROOT, two)).toBe(true);
    expect(vbfSelection.checkedOf(ROOT, dir, [{ path: one }, { path: two }])).toBe('indeterminate');
    expect(vbfSelection.pathsOf(ROOT)).toEqual([dir, `!${one}`]);

    vbfSelection.clear(ROOT);
  });

  it('uma inclusão específica volta a marcar sob uma exceção', () => {
    const dir = 'ffx_data/gamedata/folder';
    const file = `${dir}/one.bin`;

    vbfSelection.set(ROOT, 'ffx_data', true);
    vbfSelection.set(ROOT, dir, false);
    vbfSelection.set(ROOT, file, true);

    expect(vbfSelection.has(ROOT, dir)).toBe(false);
    expect(vbfSelection.has(ROOT, file)).toBe(true);
    expect(vbfSelection.countOf(ROOT)).toBe(2);

    vbfSelection.clear(ROOT);
  });

  it('desmarcar a raiz limpa a seleção inteira', () => {
    vbfSelection.set(ROOT, '', true);
    vbfSelection.set(ROOT, 'ffx_data/file.bin', false);
    expect(vbfSelection.has(ROOT, 'ffx_data/file.bin')).toBe(false);

    vbfSelection.set(ROOT, '', false);
    expect(vbfSelection.pathsOf(ROOT)).toEqual([]);
    expect(vbfSelection.countOf(ROOT)).toBe(0);
  });
});
