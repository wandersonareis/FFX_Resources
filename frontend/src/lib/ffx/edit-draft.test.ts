import { describe, expect, it } from 'vitest';
import * as dto from '../../../bindings/ffxresources/backend/dto/models';
import type { EntryKind } from './display-names';
import { editDraft } from './edit-draft';

// Editar por uma REF significa gravar no arquivo da DEF — que pode ser um
// arquivo que o usuário nunca abriu. O rascunho só aceita edição com a base
// registrada (o payload de apply é montado dela), então a base é o
// pré-requisito que o fluxo do link precisa garantir antes de abrir o
// editor: sem ela, setCell descarta em silêncio.
const VERSION = 'ffx';
const KIND: EntryKind = 'events';
const ID = 'base-requirement';

const defRow = dto.TextRow.createFrom({
  index: 3,
  name: '',
  hash: { us: 'abcdefabcdefabcd' },
  text: { us: 'Original' },
  original: { us: 'Original' },
});

const defEntry = (): dto.FileEntry =>
  dto.FileEntry.createFrom({
    metadata: { key: ID, row_count: 1 },
    rows: [defRow],
  });

describe('editDraft: base é pré-requisito da edição', () => {
  it('setCell sem base é descartado — e hasBase denuncia a ausência', () => {
    expect(editDraft.hasBase(VERSION, KIND, ID, '')).toBe(false);
    editDraft.setCell(VERSION, KIND, ID, defRow, 'us', 'Tradução', '');
    expect(editDraft.hasBase(VERSION, KIND, ID, '')).toBe(false);
    expect(editDraft.editTextOf(VERSION, KIND, ID, '3:', 'us', '')).toBeUndefined();
  });

  it('com base, a edição da def fica visível para os links e para o salvar', () => {
    editDraft.setBase(VERSION, KIND, ID, defEntry(), '');
    expect(editDraft.hasBase(VERSION, KIND, ID, '')).toBe(true);

    editDraft.setCell(VERSION, KIND, ID, defRow, 'us', 'Tradução da def', '');
    // É o que a célula linkada da ref lê para repintar ao vivo.
    expect(editDraft.editTextOf(VERSION, KIND, ID, '3:', 'us', '')).toBe('Tradução da def');
    expect(editDraft.isDirty(VERSION, KIND, ID, '')).toBe(true);

    editDraft.clear(VERSION, KIND, ID, '');
    expect(editDraft.hasBase(VERSION, KIND, ID, '')).toBe(false);
  });

  it('preserva o modo divergente no lote de apply, sem vazar original de display', () => {
    editDraft.setBase(VERSION, KIND, ID, defEntry(), '');
    editDraft.setCell(VERSION, KIND, ID, defRow, 'us', 'Divergência local', '', true);

    // Reabrir o diálogo preenche a row visual com o texto do rascunho; salvar
    // sem alterar esse texto não pode confundi-lo com a base do binário.
    const dialogRow = dto.TextRow.createFrom({
      ...defRow,
      text: { us: 'Divergência local' },
    });
    editDraft.setCell(VERSION, KIND, ID, dialogRow, 'us', 'Divergência local', '');

    expect(editDraft.isDivergent(VERSION, KIND, ID, defRow, '')).toBe(true);
    const payload = editDraft.buildSourceCollection(VERSION, KIND, '')[ID];
    expect(payload.rows[0].text.us).toBe('Divergência local');
    expect(payload.rows[0].divergent).toBe(true);
    expect(payload.rows[0].original).toBeUndefined();

    editDraft.clear(VERSION, KIND, ID, '');
    expect(editDraft.isDivergent(VERSION, KIND, ID, defRow, '')).toBe(false);
  });

  it('fontes têm namespaces separados (data/ e .vbf não colidem)', () => {
    editDraft.setBase(VERSION, KIND, ID, defEntry(), '');
    expect(editDraft.hasBase(VERSION, KIND, ID, 'C:\\game\\data.vbf')).toBe(false);
    editDraft.clear(VERSION, KIND, ID, '');
  });
});
