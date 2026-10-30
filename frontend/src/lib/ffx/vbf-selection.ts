import { useSelector } from '@tanstack/react-store';
import { createStore } from '@tanstack/store';

/**
 * Seleção de EXTRAÇÃO/EXPORT da árvore do .vbf (checkboxes).
 *
 * Guarda CAMINHOS internos do container — arquivos OU diretórios. A árvore
 * do .vbf é preguiçosa (só o diretório aberto tem filhos no frontend), então
 * marcar um diretório NÃO enumera filhos: o caminho entra como está e o
 * backend expande o prefixo contra o índice na hora de agir. Regras negativas
 * (`!caminho`) são exceções: permitem desmarcar um arquivo dentro de um
 * diretório marcado sem materializar toda a árvore.
 *
 * Mesmo padrão do export-selection (singleton reativo + snapshot imutável).
 * Escopo por container: o mesmo caminho existe em dois .vbf diferentes.
 */

interface VbfSelectionSnapshot {
  revision: number;
  /** Caminhos marcados por container (chave = caminho absoluto do .vbf). */
  byRoot: ReadonlyMap<string, readonly string[]>;
}

const vbfStore = createStore<VbfSelectionSnapshot>({
  revision: 0,
  byRoot: new Map(),
});

function normalizePath(path: string): string {
  return path
    .replaceAll('\\', '/')
    .replace(/^\/+|\/+$/g, '')
    .toLowerCase();
}

function pathCovers(path: string, rule: string): boolean {
  return rule === '' || path === rule || path.startsWith(`${rule}/`);
}

export interface VbfSelectionTreeNode {
  path: string;
  children?: readonly VbfSelectionTreeNode[];
}

function rulesInclude(paths: readonly string[], path: string): boolean {
  const normalized = normalizePath(path);
  let winner: { path: string; included: boolean } | undefined;
  for (const rule of paths) {
    const excluded = rule.startsWith('!');
    const rulePath = normalizePath(excluded ? rule.slice(1) : rule);
    if (!pathCovers(normalized, rulePath)) continue;
    if (!winner || rulePath.length > winner.path.length) {
      winner = { path: rulePath, included: !excluded };
    }
  }
  return winner?.included ?? false;
}

function hasOverridesBelow(paths: readonly string[], path: string): boolean {
  const normalized = normalizePath(path);
  const baseline = rulesInclude(paths, normalized);
  return paths.some((rule) => {
    const rulePath = normalizePath(rule.startsWith('!') ? rule.slice(1) : rule);
    const below = normalized === '' ? rulePath !== '' : rulePath.startsWith(`${normalized}/`);
    return below && rule.startsWith('!') !== !baseline;
  });
}

/** Checkbox de um nó VBF; diretórios calculam o tri-state nos filhos já carregados. */
export function vbfNodeCheckState(
  paths: readonly string[],
  path: string,
  children: readonly VbfSelectionTreeNode[] = [],
): boolean | 'indeterminate' {
  if (children.length === 0) {
    return hasOverridesBelow(paths, path) ? 'indeterminate' : rulesInclude(paths, path);
  }
  const states = children.map((child) =>
    vbfNodeCheckState(paths, child.path, child.children ?? []),
  );
  if (states.every((state) => state === true)) return true;
  if (states.some((state) => state !== false) || hasOverridesBelow(paths, path)) {
    return 'indeterminate';
  }
  return false;
}

class VbfSelectionStore {
  private readonly selected = new Map<string, Set<string>>();

  private notify(): void {
    const copied = new Map<string, readonly string[]>();
    for (const [root, paths] of this.selected) copied.set(root, [...paths]);
    vbfStore.setState((prev) => ({
      revision: prev.revision + 1,
      byRoot: copied,
    }));
  }

  private includes(root: string, path: string): boolean {
    return rulesInclude(this.pathsOf(root), path);
  }

  /**
   * Marca/desmarca um caminho (arquivo, diretório ou raiz "").
   *
   * O caminho mais específico vence. Isso deixa diretórios marcáveis sem
   * enumerar seus filhos e permite exceções individuais sob diretórios
   * marcados.
   */
  set(root: string, path: string, checked: boolean): void {
    const key = normalizePath(path);
    const set = this.selected.get(root) ?? new Set<string>();
    if (checked) {
      // Um include explícito no caminho substitui exceções/regras inferiores.
      for (const rule of set) {
        const rulePath = normalizePath(rule.startsWith('!') ? rule.slice(1) : rule);
        if (rulePath === key || key === '' || rulePath.startsWith(`${key}/`)) {
          set.delete(rule);
        }
      }
      set.add(key);
    } else {
      if (key === '') {
        this.selected.delete(root);
        this.notify();
        return;
      }
      const hasDescendantSelection = [...set].some((rule) => {
        if (rule.startsWith('!')) return false;
        const rulePath = normalizePath(rule);
        return (
          rulePath !== key &&
          (key === '' || rulePath.startsWith(`${key}/`)) &&
          this.includes(root, rulePath)
        );
      });
      const inheritedSelection =
        this.includes(root, key) &&
        [...set].some((rule) => {
          if (rule.startsWith('!')) return false;
          const rulePath = normalizePath(rule);
          return rulePath !== key && pathCovers(key, rulePath);
        });
      for (const rule of set) {
        const rulePath = normalizePath(rule.startsWith('!') ? rule.slice(1) : rule);
        if (rulePath === key || key === '' || rulePath.startsWith(`${key}/`)) {
          set.delete(rule);
        }
      }
      if (inheritedSelection || hasDescendantSelection) set.add(`!${key}`);
      if (![...set].some((rule) => !rule.startsWith('!'))) {
        this.selected.delete(root);
        this.notify();
        return;
      }
    }
    if (set.size > 0) this.selected.set(root, set);
    if (set.size === 0) this.selected.delete(root);
    this.notify();
  }

  /** Caminhos marcados do container (ordem de inserção). */
  pathsOf(root: string): string[] {
    return [...(this.selected.get(root) ?? [])];
  }

  countOf(root: string): number {
    return [...(this.selected.get(root) ?? [])].filter((rule) => !rule.startsWith('!')).length;
  }

  /** Caminho exatamente marcado (não "dentro de" um diretório marcado). */
  has(root: string, path: string): boolean {
    return this.includes(root, path);
  }

  /**
   * Estado do checkbox de um nó: folha/arquivo é booleano; diretório é
   * tri-state calculado sobre os filhos CARREGADOS (a árvore é preguiçosa —
   * o tri-state é uma dica visual, não a verdade completa do container).
   */
  checkedOf(
    root: string,
    path: string,
    children: readonly VbfSelectionTreeNode[],
  ): boolean | 'indeterminate' {
    return vbfNodeCheckState(this.pathsOf(root), path, children);
  }

  clear(root: string): void {
    if (this.selected.delete(root)) this.notify();
  }
}

export const vbfSelection = new VbfSelectionStore();

/** Snapshot reativo (rerender ao marcar/desmarcar). */
export function useVbfSelection(): VbfSelectionSnapshot {
  return useSelector(vbfStore);
}
