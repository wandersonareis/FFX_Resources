import { describe, expect, it } from 'vitest';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { editDraft, rowKey } from './edit-draft';

/** Entrada do dicionário (chunk_07) com uma linha casada e uma órfã. */
function macroEntry(): dto.FileEntry {
  return dto.FileEntry.createFrom({
    metadata: dto.Metadata.createFrom({
      key: 'ffx2/menu/macrodic.dcp',
      id: 'chunk_07',
      row_count: 2,
    }),
    rows: [
      dto.TextRow.createFrom({ index: 109, name: 'name', text: { us: 'Zanarkand Abes' } }),
      dto.TextRow.createFrom({
        index: 109,
        name: 'name_simplified',
        text: { us: 'anarkand Abes' },
        missingInOriginal: true,
      }),
    ] as unknown as dto.TextRow[],
  });
}

const len = (entry: dto.FileEntry) => entry.rows.filter((r) => r.name === 'name_simplified').length;

describe('editDraft.deleteRow (linha órfã)', () => {
  it('marca a órfã no rascunho, some do payload e mantém a linha do original', () => {
    editDraft.clearAll();
    editDraft.setBase('ffx2', 'macro', 'chunk_07', macroEntry());

    const row = macroEntry().rows[1];
    expect(editDraft.deleteRow('ffx2', 'macro', 'chunk_07', row, '')).toBe(true);

    const payload = editDraft.buildCollection('ffx2', 'macro');
    expect(payload.chunk_07, 'entrada entra no lote mesmo sem edição de texto').toBeTruthy();
    expect(len(payload.chunk_07), 'a órfã não vai para o save').toBe(0);
    expect(payload.chunk_07.rows.length).toBe(1);
    expect(editDraft.isRowDeleted('ffx2', 'macro', 'chunk_07', row, '')).toBe(true);
    editDraft.clearAll();
  });

  it('recusa excluir linha que existe no original (guarda)', () => {
    editDraft.clearAll();
    editDraft.setBase('ffx2', 'macro', 'chunk_07', macroEntry());
    const matched = macroEntry().rows[0];
    expect(editDraft.deleteRow('ffx2', 'macro', 'chunk_07', matched, '')).toBe(false);
    expect(editDraft.isRowDeleted('ffx2', 'macro', 'chunk_07', matched, '')).toBe(false);
    editDraft.clearAll();
  });

  it('orphanRows lista as órfãs com arquivo, bloco e texto', () => {
    editDraft.clearAll();
    editDraft.setBase('ffx2', 'macro', 'chunk_07', macroEntry());
    // Sem rascunho sujo, a prévia fica vazia.
    expect(editDraft.orphanRows('ffx2', 'macro')).toHaveLength(0);
    // Qualquer edição torna a entrada "suja" e a órfã entra na prévia.
    const matched = macroEntry().rows[0];
    editDraft.setCell('ffx2', 'macro', 'chunk_07', matched, 'us', 'Zanarkand Abes!');
    const rows = editDraft.orphanRows('ffx2', 'macro');
    expect(rows).toHaveLength(1);
    expect(rows[0].file).toContain('macrodic');
    expect(rows[0].block).toBe(7);
    expect(rows[0].index).toBe(109);
    expect(rows[0].text).toBe('anarkand Abes');
    editDraft.clearAll();
  });
});

// Mantém o rowKey exportado em uso (contrato compartilhado com o backend).
it('rowKey é índice:nome', () => {
  expect(rowKey({ index: 7, name: 'name' })).toBe('7:name');
});
