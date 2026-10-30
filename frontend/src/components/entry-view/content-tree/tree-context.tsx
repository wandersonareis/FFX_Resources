'use client';

import { createContext, type ReactNode, useContext } from 'react';
import type { SideNode } from '@/lib/ffx/content-tree/model';
import type { EntryKind } from '@/lib/ffx/display-names';

/**
 * Estado e ações compartilhados por TODA a árvore. Existe só para a
 * recursão do TreeItem não arrastar 8 props nível a nível — o dono dos
 * dados continua sendo o store da view (index.tsx apenas o projeta aqui).
 *
 * O dono MEMOIZA o valor (useMemo com deps estáveis) e o TreeItem é memo:
 * com contexto estável, nó intocado re-renderiza só quando o próprio nó muda.
 */
export interface ContentTreeValue {
  expanded: ReadonlySet<string>;
  selectedId: string | null;
  selectedByKind: ReadonlyMap<EntryKind, ReadonlySet<string>>;
  vbfSelectionByRoot: ReadonlyMap<string, readonly string[]>;
  onToggle: (node: SideNode) => void;
  onSelect: (node: SideNode) => void;
  onCheck: (node: SideNode, checked: boolean) => void;
}

const ContentTreeContext = createContext<ContentTreeValue | null>(null);

export function ContentTreeProvider({
  value,
  children,
}: {
  value: ContentTreeValue;
  children: ReactNode;
}) {
  return <ContentTreeContext.Provider value={value}>{children}</ContentTreeContext.Provider>;
}

export function useContentTree(): ContentTreeValue {
  const value = useContext(ContentTreeContext);
  if (!value) {
    throw new Error('useContentTree precisa estar dentro de ContentTreeProvider');
  }
  return value;
}
