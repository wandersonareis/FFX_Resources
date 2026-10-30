import { type EntryKind, resolveEntryLabel } from '../display-names';

/**
 * Modelo PURO do menu de contexto da árvore: o que o DOM entrega quando o
 * usuário clique com o botão direito e o que o menu entende.
 */

/**
 * Nó capturado pelo clique direito na árvore. Os atributos `data-node-*` do
 * DOM viram campos; `selectedVbfPaths` congela a seleção de extração do
 * container NAQUELE clique (o menu não lê o store de novo depois de abrir).
 */
export interface TreeCtxNode {
  id: string;
  kind?: EntryKind;
  label: string;
  vbfRoot?: string;
  vbfPath?: string;
  selectedVbfPaths?: string[];
}

/** O nó sob o cursor: folha tem id (arquivo); grupo/raiz, não. */
export interface EntryMenuTarget {
  kind?: EntryKind;
  id?: string | null;
  label: string;
  /** Presente para nó do navegador .vbf. */
  vbfRoot?: string;
  vbfPath?: string;
}

/** Nó da árvore → alvo do menu (folha carrega o id, grupo não). */
export function menuTargetOf(node: TreeCtxNode | null): EntryMenuTarget | null {
  if (!node) return null;
  if (node.vbfRoot) {
    return {
      label: node.label,
      vbfRoot: node.vbfRoot,
      vbfPath: node.vbfPath ?? '',
    };
  }
  if (!node.kind) return null;
  const prefix = `leaf:${node.kind}:`;
  const isLeaf = node.id.startsWith(prefix);
  const id = isLeaf ? node.id.slice(prefix.length) : null;
  return {
    kind: node.kind,
    id,
    label: id ? resolveEntryLabel(node.kind, id) : node.label,
  };
}
