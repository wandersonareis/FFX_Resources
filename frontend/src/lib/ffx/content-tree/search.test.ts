import { describe, expect, it } from 'vitest';
import * as services from '@/bindings/ffxresources/backend/services/models';
import type { SideNode } from './model';
import { buildSearchItems, foldDiacritics, searchTreeNames } from './search';

const roots: SideNode[] = [
  {
    id: 'kind:events',
    label: 'Eventos (2)',
    kind: 'events',
    children: [
      {
        id: 'group:events:azit',
        label: 'Home (az) - 2',
        kind: 'events',
        children: [
          {
            id: 'leaf:events:azit0000',
            label: 'azit0000',
            kind: 'events',
            entry: { kind: 'events', id: 'azit0000', key: 'ffx/event/azit0000.bin', label: 'azit0000' },
          },
          {
            id: 'leaf:events:azmm0000',
            label: 'azmm0000',
            kind: 'events',
            entry: { kind: 'events', id: 'azmm0000', key: 'ffx/event/azmm0000.bin', label: 'azmm0000' },
          },
        ],
      },
    ],
  },
  {
    id: 'kind:objects',
    label: 'Sistema (1)',
    kind: 'objects',
    children: [
      {
        id: 'leaf:objects:command',
        label: 'Comandos',
        kind: 'objects',
        entry: { kind: 'objects', id: 'command', key: 'ffx/battle/kernel/command.bin', label: 'Comandos' },
      },
    ],
  },
];

describe('searchTreeNames', () => {
  it('matches the visible label, id, and full filename in any case', () => {
    const hits = searchTreeNames(roots, 'COMMAND.BIN');
    expect(hits.map((node) => node.entry?.id)).toEqual(['command']);
  });

  it('keeps the deepest leaf nodes and never returns groups', () => {
    const hits = searchTreeNames(roots, 'az');
    expect(hits.map((node) => node.id)).toEqual([
      'leaf:events:azit0000',
      'leaf:events:azmm0000',
    ]);
  });

  it('caps the number of name matches and returns nothing for a blank query', () => {
    expect(searchTreeNames(roots, 'az', 1)).toHaveLength(1);
    expect(searchTreeNames(roots, '   ')).toEqual([]);
  });

  it('matches ignoring accents in both directions', () => {
    const accented: SideNode[] = [
      {
        id: 'kind:objects',
        label: 'Sistema',
        kind: 'objects',
        children: [
          {
            id: 'leaf:objects:coracao',
            label: 'Coração',
            kind: 'objects',
            entry: {
              kind: 'objects',
              id: 'coracao',
              key: 'ffx/battle/difícil.bin',
              label: 'Coração',
            },
          },
        ],
      },
    ];
    // Sem acento acha com acento e vice-versa (label e key).
    expect(
      searchTreeNames(accented, 'coracao').map((node) => node.entry?.id)
    ).toEqual(['coracao']);
    expect(searchTreeNames(accented, 'CORACAO')).toHaveLength(1);
    expect(
      searchTreeNames(accented, 'dificil').map((node) => node.entry?.id)
    ).toEqual(['coracao']);
    expect(searchTreeNames(accented, 'difícil')).toHaveLength(1);
  });
});

describe('foldDiacritics', () => {
  it('strips diacritics keeping the base letter, lowercased', () => {
    expect(foldDiacritics('DifÍcil CORAÇÃO')).toBe('dificil coracao');
  });
});

describe('buildSearchItems', () => {
  const results: services.TextSearchResult[] = [
    {
      kind: 'events',
      id: 'azmm0000',
      rows: [
        { index: 30, name: 'description', data: true, mods: false, snippetHit: 'tearful' },
        { index: 48, name: '', data: false, mods: true, snippetHit: 'sendoff' },
      ],
    },
  ];

  it('lays out the file header followed by its tabbed rows', () => {
    const items = buildSearchItems(results, []);

    expect(items).toHaveLength(3);
    expect(items[0]).toMatchObject({
      leafId: 'leaf:events:azmm0000',
      fileHeader: true,
      rowCount: 2,
    });
    expect(items[1]).toMatchObject({ fileHeader: false, match: { index: 30 } });
    expect(items[2]).toMatchObject({ fileHeader: false, match: { index: 48 } });
    // Cabeçalho e rows apontam para a MESMA folha (o reveal é o mesmo).
    expect(new Set(items.map((item) => item.leafId)).size).toBe(1);
  });

  it('appends name matches that content results do not already cover', () => {
    const nameOnly: SideNode[] = [
      {
        id: 'leaf:objects:command',
        label: 'Comandos',
        kind: 'objects',
        entry: { kind: 'objects', id: 'command', key: 'k', label: 'Comandos' },
      },
      // Já coberto pelo resultado de conteúdo: não duplica o cabeçalho.
      {
        id: 'leaf:events:azmm0000',
        label: 'azmm0000',
        kind: 'events',
        entry: { kind: 'events', id: 'azmm0000', key: 'k', label: 'azmm0000' },
      },
    ];

    const items = buildSearchItems(results, nameOnly);

    expect(items.filter((item) => item.fileHeader).map((item) => item.leafId)).toEqual([
      'leaf:events:azmm0000',
      'leaf:objects:command',
    ]);
    const nameItem = items.at(-1)!;
    expect(nameItem).toMatchObject({ key: 'leaf:objects:command|name', rowCount: 0 });
    expect(nameItem.match).toBeUndefined();
  });

  it('produces stable keys for repeated queries', () => {
    const first = buildSearchItems(results, []);
    const second = buildSearchItems(results, []);
    expect(first.map((item) => item.key)).toEqual(second.map((item) => item.key));
  });
});
