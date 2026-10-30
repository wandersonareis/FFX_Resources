import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import type { EntryKind } from '../display-names';
import type { EntryRow } from '../tree-data';

/**
 * Modelo PURO da árvore da sidebar (raiz de kind, grupo de eventos ou folha).
 *
 * Fora do componente de propósito: nada aqui toca store, DOM ou notificação —
 * só transforma nós. Quem monta os nós continua no store da view.
 */

/** Nó da árvore da sidebar (raiz de kind, grupo de eventos ou folha). */
export interface SideNode {
  id: string;
  label: string;
  kind?: EntryKind;
  entry?: EntryRow;
  children?: SideNode[];
  /** Raiz de kind em carga: spinner no lugar do chevron, filhos a caminho. */
  loading?: boolean;

  // ---- Navegador do container .vbf (fonte somente leitura) ----
  /** true = nó que pertence ao container .vbf. */
  vbf?: boolean;
  /** Caminho absoluto do container — só nas raízes. */
  vbfRoot?: string;
  /** Caminho interno dentro do container ("" = raiz do .vbf). */
  vbfPath?: string;
  /** É macrodic.dcp: os filhos saem de loadVbfMacroChunks (chunk_XX). */
  macro?: boolean;
  /**
   * Pode ser expandido mesmo sem filhos carregados (diretório do .vbf ainda
   * não buscado). Vira false quando a listagem devolve vazio.
   */
  expandable?: boolean;
  /** Arquivo que o app não serve (áudio, vídeo, .exe…): clique avisa. */
  unsupported?: boolean;
}

/** IDs das folhas de entrada contidas no nó; grupos de imagens são aninhados. */
export function entryIdsInNode(node: SideNode): string[] {
  if (node.entry) return [node.entry.id];
  return (node.children ?? []).flatMap(entryIdsInNode);
}

/** Estado tri-state do checkbox de um nó data/ sobre todas as folhas abaixo. */
export function entryCheckState(
  node: SideNode,
  selected: ReadonlySet<string>,
): boolean | 'indeterminate' {
  const ids = entryIdsInNode(node);
  const count = ids.filter((id) => selected.has(id)).length;
  return count === 0 ? false : count === ids.length ? true : 'indeterminate';
}

/**
 * Id estável de uma folha da árvore. Folhas de .vbf são identificadas pelo
 * CAMINHO no container (o mesmo id de data/ pode existir nos dois lados e
 * em dois containers diferentes).
 */
export function entryNodeId(entry: EntryRow): string {
  return entry.vbf
    ? `vbfleaf:${entry.vbf.root}|${entry.vbf.path}`
    : `leaf:${entry.kind}:${entry.id}`;
}

export const EMPTY_IDS: ReadonlySet<string> = new Set<string>();

/** ID visual do grupo de kinds textuais de data/. */
export const TEXT_ROOT_NODE_ID = 'kind:text';

/**
 * true para a raiz de UM kind de data/ (`kind:events`, `kind:battletext`…).
 * `Texto` (kind:text) também casa no prefixo, mas NÃO tem `kind` — o
 * agrupamento é só visual e nunca é enviado ao backend como EntryKind.
 * Use com `node.kind` para decidir o que é selecionável/exportável.
 */
export function isKindRootNode(node: SideNode): boolean {
  return node.id.startsWith('kind:');
}

/**
 * Agrupa as raizes reais para exibicao. `Texto` e apenas um no visual sem
 * `kind`, portanto nao e enviado ao backend como EntryKind. Imagens e todos
 * os kinds textuais preservam seus nos/kinds originais.
 */
export function buildContentRoots(kindRoots: readonly SideNode[]): SideNode[] {
  const textRoots: SideNode[] = [];
  const imageRoots: SideNode[] = [];
  for (const root of kindRoots) {
    if (root.kind === 'images') imageRoots.push(root);
    else textRoots.push(root);
  }

  return [
    ...(textRoots.length > 0
      ? [{ id: TEXT_ROOT_NODE_ID, label: 'Texto', children: textRoots }]
      : []),
    ...imageRoots,
  ];
}

/**
 * Índice id → nó de TODOS os nós visíveis (containers .vbf e data/). O
 * callback dos hotkeys recebe só o evento, então o nó é resolvido pelo DOM
 * ([data-node-id]) e devolvido por este mapa — as teclas nunca carregam
 * referências de nó na closure.
 */
export function buildNodeIndex(
  roots: readonly SideNode[],
  vbfRoots: readonly SideNode[],
): Map<string, SideNode> {
  const map = new Map<string, SideNode>();
  const walk = (node: SideNode) => {
    map.set(node.id, node);
    node.children?.forEach(walk);
  };
  // Ordem dos containers primeiro: só afeta a ordem de iteração do Map, que a
  // árvore não usa (consulta é sempre por id).
  vbfRoots.forEach(walk);
  roots.forEach(walk);
  return map;
}

/**
 * Caminho da RAIZ até a folha (inclusive), de cima para baixo — é o que o
 * reveal da busca expande no `expanded` sem tocar no resto do que o usuário
 * abriu. [] = a folha não existe nas raízes dadas (árvore ainda em carga).
 */
export function ancestorPathOf(roots: readonly SideNode[], leafId: string): string[] {
  const walk = (nodes: readonly SideNode[]): string[] | null => {
    for (const node of nodes) {
      if (node.id === leafId) return [node.id];
      const hit = node.children ? walk(node.children) : null;
      if (hit) return [node.id, ...hit];
    }
    return null;
  };
  return walk(roots) ?? [];
}

/**
 * Nó realçado para a seleção. Em imagens a árvore só guarda o
 * REPRESENTANTE do grupo de cópias — quando a seleção é uma cópia oculta
 * (a lista "Repetidas" navega até ela), o realce salta para o representante
 * em vez de sumir da árvore.
 */
export function highlightedNodeId(
  selectedEntry: EntryRow | null,
  image: dto.ImageEntry | null,
): string | null {
  if (!selectedEntry) return null;
  // Folha de .vbf é identificada pelo caminho no container: o mesmo id de
  // data/ pode existir dos dois lados (e em dois containers).
  if (selectedEntry.vbf) return entryNodeId(selectedEntry);
  const base = `leaf:${selectedEntry.kind}:${selectedEntry.id}`;
  if (selectedEntry.kind !== 'images' || !image) return base;
  if (image.metadata.id !== selectedEntry.id) return base;
  const group = [selectedEntry.id, ...(image.duplicates ?? []).map((d) => d.id)].sort();
  return `leaf:images:${group[0]}`;
}
