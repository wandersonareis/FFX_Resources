import { describe, expect, it } from 'vitest';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { linkedRowText } from './hash-ref';

// A anotação de link vem do backend (FileEntry.refs, chave = dto.RowKey =
// "index:name") e é o ÚNICO caminho para a célula de ref não pintar o
// "$hash". Estes casos fixam: quem é ref, de onde sai o texto e qual def
// é consultada — o DOM nunca vê o ponteiro de collapse.
const ref = (over: Partial<dto.RefLink> = {}): dto.RefLink => ({
  text: 'Texto da def',
  original: 'Original da def',
  sourceIndex: 12,
  ...over,
});

describe('linkedRowText', () => {
  it('sem anotação a row não é ref — devolve undefined', () => {
    expect(linkedRowText('235', undefined, () => 'não chamado')).toBeUndefined();
    expect(linkedRowText('235', null, () => 'não chamado')).toBeUndefined();
  });

  it('def na MESMA entrada usa o id aberto e a chave index:name', () => {
    const seen: Array<[string, string]> = [];
    const text = linkedRowText('235', ref({ sourceId: '' }), (id, key) => {
      seen.push([id, key]);
      return 'tradução no rascunho';
    });
    expect(text).toBe('tradução no rascunho');
    expect(seen).toEqual([['235', '12:']]);
  });

  it('def em OUTRA entrada consulta o rascunho dela, pelo id anotado', () => {
    const seen: Array<[string, string]> = [];
    const text = linkedRowText('236', ref({ sourceId: '235' }), (id, key) => {
      seen.push([id, key]);
      return undefined;
    });
    expect(seen).toEqual([['235', '12:']]);
    // sem rascunho: cai no texto da def tal como veio na entrega
    expect(text).toBe('Texto da def');
  });

  it('a chave leva o Name da row (mesma fórmula do dto.RowKey)', () => {
    const seen: Array<[string, string]> = [];
    linkedRowText('235', ref({ sourceIndex: 7, sourceName: 'npc_01' }), (id, key) => {
      seen.push([id, key]);
      return undefined;
    });
    expect(seen).toEqual([['235', '7:npc_01']]);
  });

  it('anotação sem texto entregue devolve "" — nunca "$hash"', () => {
    expect(linkedRowText('235', ref({ text: '', sourceId: '235' }), () => undefined)).toBe('');
  });
});
