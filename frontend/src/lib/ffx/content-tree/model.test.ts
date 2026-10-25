import { describe, expect, it } from 'vitest';
import {
  ancestorPathOf,
  buildContentRoots,
  buildNodeIndex,
  entryCheckState,
  entryIdsInNode,
  type SideNode,
} from './model';

const imageTree: SideNode = {
  id: 'category',
  label: 'category',
  kind: 'images',
  children: [
    {
      id: 'directory',
      label: 'directory',
      kind: 'images',
      children: [
        {
          id: 'leaf:a',
          label: 'a',
          kind: 'images',
          entry: { kind: 'images', id: 'a', key: 'a', label: 'a' },
        },
        {
          id: 'leaf:b',
          label: 'b',
          kind: 'images',
          entry: { kind: 'images', id: 'b', key: 'b', label: 'b' },
        },
      ],
    },
  ],
};

describe('data tree checkbox selection', () => {
  it('collects every image leaf under nested groups', () => {
    expect(entryIdsInNode(imageTree)).toEqual(['a', 'b']);
  });

  it('calculates tri-state from every descendant leaf', () => {
    expect(entryCheckState(imageTree, new Set())).toBe(false);
    expect(entryCheckState(imageTree, new Set(['a']))).toBe('indeterminate');
    expect(entryCheckState(imageTree, new Set(['a', 'b']))).toBe(true);
  });
});

describe('buildNodeIndex', () => {
  it('indexes nested nodes so the keyboard can resolve [data-node-id]', () => {
    const index = buildNodeIndex([imageTree], []);
    expect(index.get('category')).toBe(imageTree);
    expect(index.get('directory')?.id).toBe('directory');
    expect(index.get('leaf:b')?.entry?.id).toBe('b');
    expect(index.get('missing')).toBeUndefined();
  });

  it('merges data/ roots with .vbf roots', () => {
    const vbfRoot: SideNode = { id: 'vbf-root', label: 'mod.vbf', vbf: true, vbfRoot: '/mod.vbf' };
    const index = buildNodeIndex([imageTree], [vbfRoot]);
    expect(index.size).toBe(5);
    expect(index.get('vbf-root')?.vbfRoot).toBe('/mod.vbf');
  });
});

describe('buildContentRoots', () => {
  it('groups text kinds under Texto and leaves Imagens as its own root', () => {
    const events: SideNode = { id: 'kind:events', label: 'Eventos', kind: 'events' };
    const objects: SideNode = { id: 'kind:objects', label: 'Sistema', kind: 'objects' };
    const images: SideNode = { id: 'kind:images', label: 'Imagens', kind: 'images' };

    const roots = buildContentRoots([events, objects, images]);

    expect(roots).toEqual([
      { id: 'kind:text', label: 'Texto', children: [events, objects] },
      images,
    ]);
    expect(roots[0].kind).toBeUndefined();
    expect(roots[1].kind).toBe('images');

    const index = buildNodeIndex(roots, []);
    expect(index.get('kind:text')?.children).toEqual([events, objects]);
    expect(index.get('kind:images')).toBe(images);
  });

  it('does not create an empty Texto root if there are no text kinds', () => {
    const images: SideNode = { id: 'kind:images', label: 'Imagens', kind: 'images' };
    expect(buildContentRoots([images])).toEqual([images]);
  });
});

describe('ancestorPathOf', () => {
  const events: SideNode = {
    id: 'kind:events',
    label: 'Eventos (1)',
    kind: 'events',
    children: [
      {
        id: 'group:events:azit',
        label: 'Home (az)',
        kind: 'events',
        children: [
          {
            id: 'leaf:events:azit0000',
            label: 'azit0000',
            kind: 'events',
            entry: { kind: 'events', id: 'azit0000', key: 'k', label: 'azit0000' },
          },
        ],
      },
    ],
  };

  it('walks from the kind root down to the leaf, inclusive — for the reveal', () => {
    expect(ancestorPathOf([events], 'leaf:events:azit0000')).toEqual([
      'kind:events',
      'group:events:azit',
      'leaf:events:azit0000',
    ]);
  });

  it('handles depth-1 leaves and reports missing nodes as an empty path', () => {
    expect(ancestorPathOf([imageTree], 'leaf:b')).toEqual(['category', 'directory', 'leaf:b']);
    expect(ancestorPathOf([events], 'leaf:events:missing')).toEqual([]);
    expect(ancestorPathOf([], 'leaf:events:azit0000')).toEqual([]);
  });
});
