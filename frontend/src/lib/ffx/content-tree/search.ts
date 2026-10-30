import type * as services from '@/bindings/ffxresources/backend/services/models';
import type { EntryKind } from '../display-names';
import type { SideNode } from './model';

/**
 * Lógica PURA da lista de resultados do modal de busca. Nada aqui toca
 * store, DOM ou backend: o modal monta os itens aqui e só renderiza.
 */

/** Teto de match de NOME por consulta (a árvore inteira pode ser enorme). */
export const SEARCH_NAME_MATCH_LIMIT = 20;

/**
 * Forma de MATCHING sem acentos ("difícil" -> "dificil", "coração" ->
 * "coracao"): NFD + remoção das marcas combinantes, embutido no runtime.
 * Mesma semântica do FoldSearchText do backend — a busca de nomes casa nos
 * dois sentidos, igual à de conteúdo.
 */
export function foldDiacritics(value: string): string {
  return value.toLowerCase().normalize('NFD').replace(/\p{M}/gu, '');
}

/**
 * Item da lista do modal: um ARQUIVO (cabeçalho, clique leva ao arquivo com
 * a primeira row focada) ou uma ROW logo abaixo dele, indentada (clique leva
 * exatamente àquela linha na tabela).
 */
export interface SearchListItem {
  /** Chave React estável (identidade do item na navegação por setas). */
  key: string;
  /** Id da folha na árvore (leaf:<kind>:<id>) — é o alvo do reveal. */
  leafId: string;
  kind: EntryKind;
  /** Rótulo provisório (id); o modal resolve pelo índice da árvore. */
  label: string;
  /** true = cabeçalho de arquivo; false = row com match. */
  fileHeader: boolean;
  /** Rows do arquivo (só no cabeçalho; 0 = match só por nome). */
  rowCount: number;
  /** Locator da row (presente só nos itens de row). */
  match?: services.TextSearchMatch;
}

/**
 * Busca por NOME na árvore já carregada (label/id/key), sem backend — é o
 * "Buscar nome" do antigo filtro inline, agora como seção do modal.
 */
export function searchTreeNames(
  roots: readonly SideNode[],
  query: string,
  limit: number = SEARCH_NAME_MATCH_LIMIT,
): SideNode[] {
  // Matching sem acentos nos dois sentidos ("coracao" acha "Coração").
  const normalized = foldDiacritics(query.trim());
  if (!normalized) return [];

  const hits: SideNode[] = [];
  const walk = (nodes: readonly SideNode[]) => {
    for (const node of nodes) {
      if (hits.length >= limit) return;
      if (node.entry) {
        const { label, id, key } = node.entry;
        if ([label, id, key].some((value) => foldDiacritics(value).includes(normalized))) {
          hits.push(node);
        }
        continue;
      }
      if (node.children) walk(node.children);
    }
  };
  walk(roots);
  return hits;
}

/**
 * Monta a lista do modal a partir dos resultados de conteúdo (backend) e dos
 * matches de nome (árvore): para cada arquivo, o cabeçalho e as rows abaixo.
 * Matches de nome já cobertos por conteúdo não duplicam o cabeçalho.
 */
export function buildSearchItems(
  results: readonly services.TextSearchResult[],
  nameMatches: readonly SideNode[],
): SearchListItem[] {
  const items: SearchListItem[] = [];
  const covered = new Set<string>();

  for (const result of results) {
    const kind = result.kind as EntryKind;
    const leafId = `leaf:${kind}:${result.id}`;
    covered.add(leafId);
    items.push({
      key: `${leafId}|file`,
      leafId,
      kind,
      label: result.id,
      fileHeader: true,
      rowCount: result.rows.length,
    });
    for (const [position, match] of result.rows.entries()) {
      items.push({
        key: `${leafId}|row|${match.index}|${match.name ?? ''}|${position}`,
        leafId,
        kind,
        label: result.id,
        fileHeader: false,
        rowCount: 0,
        match,
      });
    }
  }

  for (const node of nameMatches) {
    if (covered.has(node.id) || !node.entry) continue;
    items.push({
      key: `${node.id}|name`,
      leafId: node.id,
      kind: node.entry.kind,
      label: node.label,
      fileHeader: true,
      rowCount: 0,
    });
  }
  return items;
}
