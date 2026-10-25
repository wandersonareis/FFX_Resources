import { describe, expect, it } from 'vitest';
import { exportSelection } from './export-selection';

describe('export selection snapshot', () => {
  it('trocar o formato preserva a identidade de byKind', () => {
    const before = exportSelection.getSnapshot();
    const other = before.format === 'json' ? 'strings' : 'json';

    exportSelection.setFormat(other);
    const after = exportSelection.getSnapshot();

    expect(after.format).toBe(other);
    expect(after.byKind).toBe(before.byKind);
    expect(after.revision).toBeGreaterThan(before.revision);

    // Volta ao estado anterior para não vazar para os demais testes.
    exportSelection.setFormat(before.format);
    expect(exportSelection.getSnapshot().format).toBe(before.format);
  });

  it('marcar entradas reconstrói byKind (identidade nova)', () => {
    const before = exportSelection.getSnapshot();
    exportSelection.setMany('us', 'events', ['k'], true);
    const after = exportSelection.getSnapshot();
    expect(after.byKind).not.toBe(before.byKind);
    expect(after.byKind.get('us|events')).toEqual(['k']);

    exportSelection.setMany('us', 'events', ['k'], false);
    expect(exportSelection.getSnapshot().byKind.has('us|events')).toBe(false);
  });
});
