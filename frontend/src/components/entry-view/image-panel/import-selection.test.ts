import { describe, expect, it } from 'vitest';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import {
  allSelectedState,
  buildImageImportSelection,
  entryMarkedCopies,
  entryWritesImage,
  nextAllSelected,
  plannedWrites,
} from './import-selection';

/** Entrada nova (difere do binário) com duas cópias ausentes marcadas. */
function entry(
  id: string,
  opts: Partial<{
    differs: boolean;
    selected: boolean;
    copies: Array<{ id: string; selected: boolean }>;
    error: string;
  }> = {}
): dto.ImageImportPreviewEntry {
  return {
    id,
    display_path: `${id}.dds.phyre`,
    differs: opts.differs ?? true,
    copy_count: opts.copies?.length ?? 0,
    copy_missing_count: opts.copies?.length ?? 0,
    copies: (opts.copies ?? []).map((c) => ({
      id: c.id,
      display_path: `${c.id}.dds.phyre`,
      in_mods: false,
      identical: false,
      selected: c.selected,
    })),
    validation_error: opts.error,
    selected: opts.selected ?? true,
  };
}

function preview(entries: dto.ImageImportPreviewEntry[]): dto.ImageImportPreview {
  return {
    root: 'C:/game/mods/translated/images',
    entries,
    max: entries.length,
    new_count: entries.filter((e) => e.differs).length,
    identical_count: entries.filter((e) => !e.differs).length,
    errors: entries.map((e) => e.validation_error).filter(Boolean) as string[],
  };
}

describe('regra da imagem igual ao binário com cópias marcadas', () => {
  it('imagem já importada (differs=false) DESmarcada com cópia marcada regrava a imagem', () => {
    const e = entry('a', {
      differs: false,
      selected: false,
      copies: [{ id: 'a1', selected: true }],
    });
    expect(entryWritesImage(e)).toBe(true);
    expect(entryMarkedCopies(e)).toEqual(['a1']);
  });

  it('imagem diferente DESmarcada com cópia marcada NÃO regrava a imagem', () => {
    const e = entry('a', {
      differs: true,
      selected: false,
      copies: [{ id: 'a1', selected: true }],
    });
    expect(entryWritesImage(e)).toBe(false);
    expect(entryMarkedCopies(e)).toEqual(['a1']);
  });

  it('imagem marcada entra mesmo sem cópias', () => {
    expect(entryWritesImage(entry('a', { selected: true }))).toBe(true);
  });

  it('entrada com erro de validação nunca entra na seleção', () => {
    const e = entry('bad', { error: 'formato inválido' });
    expect(entryWritesImage(e)).toBe(false);
    expect(entryMarkedCopies(e)).toEqual([]);
  });
});

describe('plannedWrites / buildImageImportSelection', () => {
  it('conta imagens + cópias marcadas', () => {
    const p = preview([
      entry('a', { copies: [{ id: 'a1', selected: true }] }),
      entry('b', { selected: false }),
    ]);
    // a (imagem) + a1 (cópia) = 2 gravações; b desmarcado = 0.
    expect(plannedWrites(p)).toBe(2);
  });

  it('monta a seleção só com o que vai gravar', () => {
    const p = preview([
      entry('a', { differs: false, selected: false, copies: [{ id: 'a1', selected: true }] }),
      entry('b', { selected: false }),
      entry('bad', { error: 'formato inválido' }),
    ]);
    expect(buildImageImportSelection(p)).toEqual({
      entries: [{ id: 'a', image: true, copies: ['a1'] }],
    });
  });

  it('seleção vazia quando nada está marcado', () => {
    const p = preview([entry('a', { selected: false }), entry('b', { selected: false })]);
    expect(buildImageImportSelection(p)).toEqual({ entries: [] });
    expect(plannedWrites(p)).toBe(0);
  });
});

describe('estado do Todas (3 estados)', () => {
  it('tudo desmarcado = false; tudo marcado = true; parcial = indeterminate', () => {
    const off = preview([entry('a', { selected: false })]);
    expect(allSelectedState(off)).toBe(false);
    expect(nextAllSelected(off)).toBe(true);

    const on = preview([entry('a', { selected: true })]);
    expect(allSelectedState(on)).toBe(true);
    expect(nextAllSelected(on)).toBe(false);

    const partial = preview([
      entry('a', { selected: true }),
      entry('b', { selected: false }),
    ]);
    expect(allSelectedState(partial)).toBe('indeterminate');
    expect(nextAllSelected(partial)).toBe(true);
  });

  it('considera as cópias: imagem marcada com cópia desmarcada é indeterminada', () => {
    const p = preview([
      entry('a', { selected: true, copies: [{ id: 'a1', selected: false }] }),
    ]);
    expect(allSelectedState(p)).toBe('indeterminate');
  });

  it('entradas com erro não contam como selecionáveis', () => {
    const p = preview([entry('bad', { error: 'formato inválido', selected: false })]);
    expect(allSelectedState(p)).toBe(false);
  });
});
