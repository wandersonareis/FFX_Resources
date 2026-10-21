import { createStore } from '@tanstack/store';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import * as services from '@/bindings/ffxresources/backend/services/models';
import {
  KIND_LABELS,
  IMAGE_TREE_PREFIX,
  resolveEntryLabel,
  type EntryKind,
} from '@/lib/ffx/display-names';
import {
  formatEventGroupLabel,
  resolveEventGroup,
  shortenedOf,
} from '@/lib/ffx/event-group-names';
import type { GameVersionId } from '@/lib/ffx/game-version';
import {
  type EntryRow,
  allKindsFor,
  imageExists,
  loadImage,
  loadEntry,
  loadKindEntries,
  locFromVbfPath,
} from '@/lib/ffx/tree-data';
import { editDraft, rowKey } from '@/lib/ffx/edit-draft';
import { entryProgress, type EntryProgress } from '@/lib/ffx/entry-progress';
import {
  sendErrorNotification,
  sendErrorNotificationWithMessage,
} from '@/lib/ffx/error-handler';
import { isBlankSourceRow } from '@/lib/ffx/blank-us';
import { SOURCE_LANG } from '@/lib/ffx/save-all';
import {
  invalidateVbfCache,
  loadVbfDir,
  loadVbfEntry,
  loadVbfImage,
  loadVbfMacroChunks,
  loadVbfRoots,
  type VbfNode,
} from '@/lib/ffx/vbf';
import {
  TEXT_ROOT_NODE_ID,
  ancestorPathOf,
  buildNodeIndex,
  type SideNode,
} from '@/lib/ffx/content-tree/model';
import { invalidateTextSearchResults } from '@/lib/ffx/search-store';

/** Estado de carga de um item principal da árvore. */
export type KindLoadStatus = 'loading' | 'ready' | 'error';

/**
 * Kinds de TEXTO com fluxo de save/edição na sessão do .vbf (espelho do
 * backend): todos — o estado decodificado fica na sessão do container e a
 * gravação é sempre em mods/ (o .vbf é só fonte, nunca é escrito).
 */
export const EDITABLE_VBF_KINDS: ReadonlySet<string> = new Set([
  'events',
  'battletext',
  'cloud',
  'tutorial',
  'menumain',
  'help',
  'macro',
  'objects',
  'lockit',
]);

/**
 * Alvo da edição: a entrada onde o rascunho registra a tradução. A célula
 * linkada abre o editor na def — mesmo que a def viva em OUTRA entrada
 * (o link anota o arquivo e o rowKey dela). source = "" (data/) ou o
 * caminho do .vbf.
 */
export interface TranslationTarget {
  kind: EntryKind;
  id: string;
  source: string;
}

/**
 * Ação de imagem em curso — extrair / replicar / deletar sobre um id
 * CLICADO na árvore ou no painel (que pode não ser a entry selecionada).
 *
 * Vive no store porque o diálogo é renderizado uma vez só, na aba: menu e
 * painel só abrem. `duplicates` null = "buscar no backend" (o menu não sabe
 * as cópias do nó sob o cursor); `label` é o nome exibido no título.
 */
export type ImageActionState = {
  type: 'extract' | 'replicate' | 'delete' | 'delete-selection';
  id: string;
  label: string;
  duplicates: dto.ImageDuplicate[] | null;
  /** Presente apenas em delete-selection: arquivos explicitamente marcados. */
  ids?: string[];
  /** A origem veio de um container .vbf somente leitura. */
  vbf?: { root: string; path: string };
};

export interface EntryViewState {
  roots: SideNode[];
  /**
   * Raízes dos containers .vbf descobertos perto do executável do jogo
   * (somente leitura). Vindas do backend, não dos kinds de data/ — por isso
   * um campo próprio: os kinds são montados e unidos em `roots`, e o
   * carregamento deles é independente desta lista.
   */
  vbfRoots: SideNode[];
  /**
   * Carga das raízes .vbf: 'loading' = a descoberta ainda não respondeu (a
   * lista vazia NÃO significa nada — o aviso "aponte o executável" só vale
   * depois de 'ready').
   */
  vbfStatus: 'loading' | 'ready';
  expanded: Set<string>;
  activeKind: EntryKind;
  selectedEntry: EntryRow | null;
  rows: dto.TextRow[];
  /**
   * Anotações de link das rows de ref dedupada da entrada aberta
   * (rowKey → texto atual da def + origem). Vem do backend na entrega
   * dedupada (FileEntry.refs) e alimenta o texto linkado na tabela
   * (cor própria, tooltip "repetição").
   */
  refLinks: Record<string, dto.RefLink>;
  /**
   * Textura aberta (kind=images). null = nenhum / kind de texto.
   * Guardada aqui junto da seleção para o painel de imagem não precisar de
   * estado próprio (e para sobreviver à troca de componente).
   */
  image: dto.ImageEntry | null;
  loading: boolean;
  /**
   * Progresso de tradução da entrada aberta (por row, todos os formatos):
   * FIXA na abertura do arquivo — não acompanha rascunho em edição.
   * null = sem arquivo aberto.
   */
  progress: EntryProgress | null;
  /** Estado de carga por item principal (árvore parcial). */
  kindStatus: Record<EntryKind, KindLoadStatus>;
  /** Geração do reload — patches de geração passada são descartados. */
  generation: number;
  /** Linha aberta no modal do editor. */
  translationRow: dto.TextRow | null;
  /**
   * Alvo da edição do modal: null = a entrada aberta (selectedEntry);
   * preenchido quando o editor abre na DEF de outra entrada através do
   * link (a tradução é registrada no rascunho do arquivo da def).
   */
  translationTarget: TranslationTarget | null;
  /** Row-ref da entrada aberta — destino alternativo quando o usuário escolhe divergir. */
  translationCopyRow: dto.TextRow | null;
  /** Valor inicial do checkbox "salvar nesta cópia" ao abrir pelo ícone. */
  translationDivergent: boolean;
  dialogOpen: boolean;
  /**
   * Diálogo de ação de imagem aberto (extrair/replicar/deletar) ou null.
   * Um só por aba — menu e painel apenas abrem.
   */
  imageAction: ImageActionState | null;
  /**
   * Pedido de foco na 1ª linha da tabela (ArrowRight na folha da árvore).
   * Vive no store (igual ao antigo pendingTableFocusRef) para sobreviver ao
   * unmount/remount da tabela e ser consumido quando as rows chegarem.
   */
  pendingTableFocus: boolean;
  /** Row alvo vindo da busca; mantém também qual coluna encontrou o texto. */
  pendingTableRowFocus: services.TextSearchMatch | null;
  /**
   * Folha a REVELAR na árvore (resultado do modal de busca): o caminho
   * inteiro entra no `expanded` no mesmo patch e este id manda o ContentTree
   * rolar até o nó (consumido pelo efeito de scroll).
   */
  revealedNodeId: string | null;
  /**
   * Reveal adiado: o arquivo pedido no modal ainda não está na árvore (kind
   * em carga). Conclui sozinho quando os kinds terminam de carregar.
   */
  pendingReveal: { leafId: string; match: services.TextSearchMatch | null } | null;
}

export interface EntryActions {
  /** Recarrega a árvore e revalida a entrada selecionada. */
  reload(): Promise<void>;
  /** Reload frio (botão): limpa a árvore para placeholders e recarrega tudo. */
  coldReload(): Promise<void>;
  /** Abre um arquivo na tabela (carrega rows e registra a base do rascunho). */
  selectEntry(entry: EntryRow, searchMatch?: services.TextSearchMatch): Promise<void>;
  /** Clique/Enter num nó da árvore: folha abre; grupo/raiz expande (como
   * no .vbf) e raiz de kind também troca o kind ativo. */
  selectNode(node: SideNode): Promise<void>;
  /** Alterna expandir/colapsar grupo ou raiz. */
  toggleNode(node: SideNode): void;
  /** ArrowRight na folha: pede foco na 1ª linha após o arquivo carregar. */
  requestTableFocus(): void;
  /** Tabela focou a 1ª linha: limpa o pedido pendente. */
  consumeTableFocus(): void;
  /** A tabela consumiu o locator de row vindo da busca. */
  consumeTableRowFocus(): void;
  /**
   * Resultado do modal de busca: expande o caminho do arquivo na árvore
   * (reveal + scroll) e abre a entrada com a row do match focada na tabela.
   */
  openSearchResult(leafId: string, match?: services.TextSearchMatch): Promise<void>;
  /** A árvore rolou até o revealedNodeId: limpa o pedido. */
  consumeRevealedNode(): void;
  /**
   * Abre o modal do editor para a linha clicada/teclada na tabela. target
   * preenchido = edita a DEF de outra entrada através do link (o rascunho
   * registra no arquivo da def).
   */
  openDialog(row: dto.TextRow, target?: TranslationTarget): void;
  /** Célula linkada: abre o editor na row da def apontada pelo link. */
  openLinked(row: dto.TextRow, divergent?: boolean): void;
  /** Rascunha o original desta própria row, self-target, para re-convergir. */
  revertToOriginal(row: dto.TextRow): void;
  /**
   * Busca local (substituição): garante a base do rascunho do ALVO — defs
   * em outra entrada precisam ser carregadas antes de gravar (mesmo caminho
   * do editor aberto pela ref). false = não deu para preparar (não grave).
   */
  ensureCellEditBase(target: TranslationTarget): Promise<boolean>;
  /**
   * Busca local (substituição): grava `text` na célula endereçada por
   * (target, row, lang) via rascunho e repinta a tabela — o MESMO caminho do
   * commit do editor, sem abrir diálogo. O salvar existente persiste depois.
   */
  applyCellEdit(
    target: TranslationTarget,
    row: dto.TextRow,
    lang: string,
    text: string
  ): void;
  /** Navegação ←/→ do modal sem fechar (value aplicado como rascunho). */
  navigateRow(direction: 'prev' | 'next', value?: string): void;
  /** Fechamento do modal (value != undefined aplica a edição). */
  commitRow(value?: string, divergent?: boolean): void;
  /** Abre o diálogo de ação de imagem (extrair/replicar/deletar). */
  openImageAction(action: ImageActionState): void;
  /** Fecha o diálogo de ação de imagem (cancelar/ao concluir). */
  closeImageAction(): void;
}

export interface EntryView {
  readonly version: GameVersionId;
  readonly store: ReturnType<typeof createEntryStore>;
  readonly actions: EntryActions;
}

/**
 * Agrupa os arquivos de events por fragmento do eventID. Fragmentos com nome
 * no dicionário viram nós próprios; sem nome, fundem-se no primeiro fragmento
 * nomeado do mesmo shortened (azmm entra em azit = Home). Órfão sem
 * irmão nomeado exibe o próprio fragmento, igual ao nome da pasta no disco.
 */
function eventGroups(version: GameVersionId, entries: EntryRow[]): SideNode[] {
  const byFragment = new Map<string, EntryRow[]>();
  for (const entry of entries) {
    const fragment = shortenedOf(entry.id);
    const list = byFragment.get(fragment) ?? [];
    list.push(entry);
    byFragment.set(fragment, list);
  }
  const fragments = [...byFragment.keys()];

  // Fragmentos do mesmo shortened podem fundir-se em um alvo comum.
  const fragmentsByTarget = new Map<string, string[]>();
  const targetNames = new Map<string, string>();
  for (const fragment of fragments) {
    const { target, name } = resolveEventGroup(version, fragment, fragments);
    if (name) targetNames.set(target, name);
    const list = fragmentsByTarget.get(target) ?? [];
    list.push(fragment);
    fragmentsByTarget.set(target, list);
  }

  const nodes: SideNode[] = [];
  for (const [target, frags] of [...fragmentsByTarget.entries()].sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const files = frags.flatMap((fragment) => byFragment.get(fragment) ?? []);
    const name = targetNames.get(target);
    nodes.push({
      id: `group:events:${target}`,
      label: formatEventGroupLabel(name ?? '', target, files.length),
      kind: 'events' as EntryKind,
      children: files.map((entry) => ({
        id: `leaf:events:${entry.id}`,
        label: entry.label,
        kind: entry.kind,
        entry,
      })),
    });
  }
  return nodes;
}

function createEntryStore(version: GameVersionId) {
  const kinds = allKindsFor(version);
  return createStore<EntryViewState>({
    roots: kinds.map((kind) => loadingRootNode(kind)),
    vbfRoots: [],
    vbfStatus: 'loading' as const,
    // Mantem os kinds de texto visiveis dentro do grupo de apresentacao.
    expanded: new Set([TEXT_ROOT_NODE_ID]),
    activeKind: 'events',
    selectedEntry: null,
    rows: [],
    refLinks: {},
    image: null,
    loading: true,
    progress: null,
    kindStatus: Object.fromEntries(kinds.map((k) => [k, 'loading' as const])) as Record<
      EntryKind,
      KindLoadStatus
    >,
    generation: 0,
    translationRow: null,
    translationTarget: null,
    translationCopyRow: null,
    translationDivergent: false,
    dialogOpen: false,
    imageAction: null,
    pendingTableFocus: false,
    pendingTableRowFocus: null,
    revealedNodeId: null,
    pendingReveal: null,
  });
}

/** Raiz de kind em carga: label sem contagem + spinner no lugar dos filhos. */
function loadingRootNode(kind: EntryKind): SideNode {
  return { id: `kind:${kind}`, label: KIND_LABELS[kind], kind, loading: true };
}

function loadingStatuses(kinds: EntryKind[]): Record<EntryKind, KindLoadStatus> {
  return Object.fromEntries(
    kinds.map((k) => [k, 'loading' as const])
  ) as Record<EntryKind, KindLoadStatus>;
}

/**
 * Id de um nó do navegador de .vbf. Caminho interno identifica arquivo e
 * diretório dentro do container; `id` extra cobre os chunks virtuais do
 * macrodic (todos com o mesmo caminho do dicionário).
 */
function vbfNodeId(root: string, node: Pick<VbfNode, 'path' | 'id'>): string {
  return `vbf:${root}|${node.path}|${node.id ?? ''}`;
}

/**
 * Converte um nó do índice do .vbf em SideNode.
 *
 * - diretório → grupo expansível (filhos vão pro backend na expansão);
 * - macrodic.dcp → grupo com filhos chunk_XX (o app trata dicionário assim);
 * - arquivo com kind servível (evento, lockit, textura, …) → folha com
 *   `entry.vbf`, que é o que troca a carga para os bindings VBF;
 * - o resto (áudio, vídeo, .exe…) → folha sem entry: o clique avisa que o
 *   formato está fora do escopo do app, sem tentar decodificar.
 *
 * Nada aqui escreve no container: é só o espelho do índice.
 */
function vbfSideNode(root: string, node: VbfNode): SideNode {
  const id = vbfNodeId(root, node);
  if (node.isDir) {
    return {
      id,
      label: node.name,
      vbf: true,
      vbfRoot: root,
      vbfPath: node.path,
      expandable: true,
    };
  }
  if (node.macro) {
    return {
      id,
      label: node.name,
      vbf: true,
      vbfRoot: root,
      vbfPath: node.path,
      macro: true,
      expandable: true,
    };
  }
  if (!node.kind || !node.id) {
    return {
      id,
      label: node.name,
      vbf: true,
      vbfRoot: root,
      vbfPath: node.path,
      unsupported: true,
    };
  }
  const kind = node.kind as EntryKind;
  const label = resolveEntryLabel(kind, node.id);
  return {
    id,
    label,
    kind,
    vbf: true,
    entry: {
      kind,
      id: node.id,
      key: node.id,
      label,
      vbf: { root, path: node.path },
    },
  };
}

/** Estado "sem arquivo aberto" — limpa tabela, progresso e imagem juntos. */
const NO_SELECTION: Partial<EntryViewState> = {
  selectedEntry: null,
  rows: [],
  refLinks: {},
  progress: null,
  image: null,
  pendingTableRowFocus: null,
};

/**
 * Árvore de imagens: categoria (1º segmento após gamedata/ps3data) →
 * diretório da textura (o trecho antes de /d3d11, que é só a variante de
 * renderizador) → folha com o nome do arquivo. Os ids são caminhos longos e
 * repetitivos; agrupar assim deixa a árvore navegável sem perder o caminho.
 */
function imageTreeNodes(entries: EntryRow[]): SideNode[] {
  const byCategory = new Map<string, Map<string, EntryRow[]>>();
  for (const entry of entries) {
    const rel = entry.id.startsWith(IMAGE_TREE_PREFIX)
      ? entry.id.slice(IMAGE_TREE_PREFIX.length)
      : entry.id;
    const firstSlash = rel.indexOf('/');
    const category = firstSlash < 0 ? rel : rel.slice(0, firstSlash);
    const rest = firstSlash < 0 ? '' : rel.slice(firstSlash + 1);
    const lastSlash = rest.lastIndexOf('/');
    const dir =
      lastSlash < 0
        ? ''
        : rest.slice(0, lastSlash).replace(/(^|\/)d3d11$/, '');
    const byDir = byCategory.get(category) ?? new Map<string, EntryRow[]>();
    const list = byDir.get(dir) ?? [];
    list.push(entry);
    byDir.set(dir, list);
    byCategory.set(category, byDir);
  }

  const nodes: SideNode[] = [];
  for (const [category, byDir] of [...byCategory.entries()].sort(([a], [b]) =>
    a.localeCompare(b)
  )) {
    const count = [...byDir.values()].reduce((n, list) => n + list.length, 0);
    nodes.push({
      id: `group:images:${category}`,
      label: `${category} (${count})`,
      kind: 'images',
      children: [...byDir.entries()]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([dir, list]) => ({
          id: `group:images:${category}/${dir}`,
          label: dir ? `${dir} (${list.length})` : `raiz (${list.length})`,
          kind: 'images' as EntryKind,
          children: list.map((entry) => ({
            id: `leaf:images:${entry.id}`,
            label: entry.label,
            kind: entry.kind,
            entry,
          })),
        })),
    });
  }
  return nodes;
}

/**
 * Store por aba de versão (TanStack Store): concentra o estado compartilhado
 * entre árvore, tabela e diálogo de tradução, eliminando prop drilling entre
 * os componentes de entry-view. Uma instância por GameVersionTab — trocar de
 * aba não preserva seleção (como no original com useState).
 */
export function createEntryView(version: GameVersionId): EntryView {
  const store = createEntryStore(version);

  const patch = (partial: Partial<EntryViewState>): void => {
    store.setState((prev) => ({ ...prev, ...partial }));
  };

  /**
   * Atualiza UM nó da árvore em qualquer profundidade (raízes de kind e
   * raízes VBF são caminhos separados, por isso os dois no mesmo passe).
   */
  const patchNode = (id: string, partial: Partial<SideNode>): void => {
    const walk = (list: SideNode[]): SideNode[] =>
      list.map((node) =>
        node.id === id
          ? { ...node, ...partial }
          : node.children
            ? { ...node, children: walk(node.children) }
            : node
      );
    store.setState((prev) => ({
      ...prev,
      roots: walk(prev.roots),
      vbfRoots: walk(prev.vbfRoots),
    }));
  };

  /**
   * Expansão preguiçosa de um nó do .vbf: o container nunca é indexado de
   * uma vez — só o diretório clicado é consultado (e o macrodic quebra em
   * chunk_XX). Guardado por id para um duplo clique não disparar duas buscas.
   */
  const loadingVbf = new Set<string>();
  const loadVbfChildren = async (node: SideNode): Promise<void> => {
    const root = node.vbfRoot ?? node.entry?.vbf?.root;
    if (!root || loadingVbf.has(node.id)) return;
    loadingVbf.add(node.id);
    patchNode(node.id, { loading: true });
    try {
      const path = node.vbfPath ?? '';
      const list = node.macro
        ? await loadVbfMacroChunks(root, path)
        : await loadVbfDir(root, path);
      const children = list.map((child) => vbfSideNode(root, child));
      patchNode(node.id, {
        children,
        loading: false,
        expandable: children.length > 0,
      });
      if (children.length > 0) {
        store.setState((prev) => ({
          ...prev,
          expanded: new Set(prev.expanded).add(node.id),
        }));
      }
    } catch (error) {
      patchNode(node.id, { loading: false, expandable: false });
      sendErrorNotification(error);
    } finally {
      loadingVbf.delete(node.id);
    }
  };

  const selectEntry = async (
    entry: EntryRow,
    searchMatch?: services.TextSearchMatch,
  ): Promise<void> => {
    patch({
      loading: true,
      pendingTableFocus: false,
      pendingTableRowFocus: searchMatch ?? null,
    });
    try {
      patch({ activeKind: entry.kind, selectedEntry: entry });
      if (entry.kind === 'images') {
        // Textura: sem rows/progresso — só a imagem (data URL) + metadados.
        const image = entry.vbf
          ? await loadVbfImage(entry.vbf.root, entry.vbf.path)
          : await loadImage(entry.id, version);
        patch({ image, rows: [], progress: null });
        return;
      }
      patch({ image: null });
      const full = entry.vbf
        ? await loadVbfEntry(entry.vbf.root, entry.vbf.path, entry.id)
        : await loadEntry(entry.kind, entry.id, version);
      // Rascunho por fonte: data/ e o .vbf têm namespaces separados (o
      // mesmo id pode existir nos dois). A tabela aberta pelo .vbf alimenta
      // o rascunho do container: o salvar roteia ao binding de .vbf.
      editDraft.setBase(
        version,
        entry.kind,
        entry.id,
        full,
        entry.vbf?.root ?? ''
      );
      // Progresso por row: FIXO na abertura (não acompanha rascunho).
      // Entrada vbf de idioma não-us: contagem por 'us' não se aplica — o
      // texto deste binário já está na coluna exibida.
      const vbfLocProgress = entry.vbf ? locFromVbfPath(entry.vbf.path) : null;
      patch({
        progress:
          vbfLocProgress && vbfLocProgress !== SOURCE_LANG
            ? null
            : entryProgress(full),
      });
      // Anotações de link das refs dedupadas (texto da def + origem) —
      // alimenta o texto linkado na tabela (cor própria, tooltip).
      // O v3 gera `refs` como `{ [_ in string]?: RefLink }` (valor opcional),
      // enquanto o v2 gerava `Record<string, RefLink>`. O mapa vem de um
      // `map[string]RefLink` do Go, cujo valor nunca é nil — o cast devolve
      // exatamente a tipagem que os consumidores já assumem.
      patch({ refLinks: (full.refs ?? {}) as Record<string, dto.RefLink> });
      // Filtro de linhas: para .vbf de idioma não-us, olha o idioma do
      // binário clicado (o .vbf é só visualização — nada a traduzir).
      const vbfLoc = entry.vbf ? locFromVbfPath(entry.vbf.path) : null;
      const searchRowKey = searchMatch
        ? `${searchMatch.index}:${searchMatch.name ?? ''}`
        : null;
      const rows = (full.rows ?? []).filter((r) =>
        (vbfLoc && vbfLoc !== SOURCE_LANG
          ? !isBlankSourceRow(r.text?.[vbfLoc])
          : !isBlankSourceRow(r.text?.[SOURCE_LANG])) ||
        (searchRowKey !== null && rowKey(r) === searchRowKey)
      );
      patch({ rows });
    } catch (error) {
      sendErrorNotification(error);
      patch({ ...NO_SELECTION });
    } finally {
      patch({ loading: false });
    }
  };

  const buildKindNode = (kind: EntryKind, entries: EntryRow[]): SideNode => ({
    id: `kind:${kind}`,
    label: `${KIND_LABELS[kind]} (${entries.length})`,
    kind,
    children:
      kind === 'events'
        ? eventGroups(version, entries)
        : kind === 'images'
          ? imageTreeNodes(entries)
          : entries.map((entry) => ({
              id: `leaf:${kind}:${entry.id}`,
              label: entry.label,
              kind: entry.kind,
              entry,
            })),
  });

  /** Contador de geração: patches de geração passada são descartados. */
  const generationCounter = { current: 0 };

  /**
   * Carga da árvore em PARTES para uma geração: todos os kinds disparam em
   * paralelo e cada um entra na árvore assim que chega (upsert, na ordem
   * canônica dos kinds — nós não trocam de posição). A árvore NUNCA colapsa
   * o que já está aberto (expansão em union).
   */
  const loadAllKinds = async (gen: number): Promise<void> => {
    const kinds = allKindsFor(version);
    await Promise.allSettled(
      kinds.map(async (kind) => {
        try {
          const entries = await loadKindEntries(kind, version);
          if (gen !== generationCounter.current) return;
          store.setState((prev) => {
            const byKind = new Map(prev.roots.map((r) => [r.kind!, r] as const));
            // Kind sem nada (ex.: images numa versão sem texturas) não vira
            // raiz vazia "Imagens (0)" — a árvore só mostra o que existe.
            if (entries.length === 0) byKind.delete(kind);
            else byKind.set(kind, buildKindNode(kind, entries));
            const roots = kinds
              .map((k) => byKind.get(k))
              .filter((n): n is SideNode => Boolean(n));
            // A árvore nasce FECHADA, inclusive a de data/: nada é expandido
            // automaticamente. O expanded que já está aqui guarda só o que
            // o USUÁRIO abriu, e sobrevive ao reload (união) — quem quer ver
            // os filhos expande o nó.
            return {
              ...prev,
              roots,
              kindStatus: { ...prev.kindStatus, [kind]: 'ready' as const },
            };
          });
          // Revalida a seleção quando o KIND dela completa (não no fim de
          // tudo): entrada removida por import limpa a tabela. Seleção que
          // veio do .vbf fica de fora — a árvore de data/ não tem como
          // revalidá-la (o arquivo pode nem estar extraído ali); se o
          // container sumir, a própria carga da entrada avisa.
          const current = store.state.selectedEntry;
          if (current && !current.vbf && current.kind === kind) {
            const found = entries.find((e) => e.id === current.id);
            if (found) {
              await selectEntry(found);
            } else if (
              kind === 'images' &&
              entries.length > 0 &&
              (await imageExists(current.id, version))
            ) {
              // Cópia oculta do grupo: a árvore só guarda o representante,
              // mas o id continua servível (a lista "Repetidas" navega até
              // ele) — quem decide se ainda existe é a carga, não a lista.
              await selectEntry(current);
            } else {
              // Não existe mais (ex.: imagem + cópias apagadas com escopo
              // both): limpa em SILÊNCIO — o Resolve erroaria "não
              // encontrada em data/ nem em mods/" para o id que o delete
              // acabou de remover com sucesso.
              patch({ ...NO_SELECTION });
            }
          }
        } catch (error) {
          if (gen !== generationCounter.current) return;
          store.setState((prev) => ({
            ...prev,
            kindStatus: { ...prev.kindStatus, [kind]: 'error' as const },
          }));
          sendErrorNotification(error);
        }
      })
    );
  };

  /**
   * Raízes dos containers .vbf descobertos perto do executável do jogo. O
   * backend varre a pasta uma vez (promessa compartilhada em vbf.ts) e cada
   * aba só espelha a lista na própria árvore. Sem executível configurado ele
   * devolve lista vazia — a árvore de data/ continua normal.
   */
  const loadVbfRootNodes = async (gen: number): Promise<void> => {
    try {
      const all = await loadVbfRoots();
      if (gen !== generationCounter.current) return;
      // UM container por versão: FFX_Data.vbf na aba FFX, FFX2_Data.vbf na
      // FFX-2 e NENHUM na Last Mission (que reusa a árvore do FFX-2 sem
      // container próprio). Nomes sem a extensão — a seção acima da árvore
      // já diz que é .vbf.
      const list = all.filter((root) => root.version === version);
      patch({
        vbfStatus: 'ready',
        vbfRoots: list.map((root) => ({
          // Mesmo formato de vbfNodeId com path/id vazios = a RAIZ do
          // container (é o que a expansão usa como diretório inicial).
          id: vbfNodeId(root.path, { path: '', id: '' }),
          label: root.name.replace(/\.vbf$/i, ''),
          vbf: true,
          vbfRoot: root.path,
          vbfPath: '',
          expandable: true,
        })),
      });
    } catch (error) {
      if (gen !== generationCounter.current) return;
      // A lista é final mesmo no erro (vazia): o aviso da árvore pode ser
      // exibido — a falha em si já foi notificada ao usuário.
      patch({ vbfStatus: 'ready', vbfRoots: [] });
      sendErrorNotification(error);
    }
  };

  /** Carga da aba: marca todos os principais como loading e carrega. */
  const reload = async (): Promise<void> => {
    const gen = ++generationCounter.current;
    const kinds = allKindsFor(version);
    patch({ loading: true, kindStatus: loadingStatuses(kinds) });
    void loadVbfRootNodes(gen);
    await loadAllKinds(gen);
    if (gen === generationCounter.current) {
      patch({ loading: false });
      resolvePendingReveal();
    }
  };

  /**
   * Reload frio (botão da sidebar): a aba perde os arquivos — raízes voltam
   * aos placeholders de carga e a carga recomeça do zero, com o mesmo
   * efeito da carga inicial (kinds em paralelo, expandidos ao completar).
   */
  const coldReload = async (): Promise<void> => {
    const gen = ++generationCounter.current;
    const kinds = allKindsFor(version);
    // Recomeça do zero também o lado do .vbf: cache de diretórios/entradas
    // descartado e as raízes re-descobertas (executável do jogo pode ter
    // mudado no diálogo de configuração).
    invalidateVbfCache();
    // Resultados do modal de busca apontam para a base ANTIGA: descarta (o
    // termo fica) — o modal stale re-executa a consulta sozinho ao reabrir.
    invalidateTextSearchResults(version);
    patch({
      roots: kinds.map((kind) => loadingRootNode(kind)),
      vbfRoots: [],
      vbfStatus: 'loading' as const,
      // Mantem os kinds de texto visiveis apos reload frio.
      expanded: new Set([TEXT_ROOT_NODE_ID]),
      selectedEntry: null,
      rows: [],
      image: null,
      kindStatus: loadingStatuses(kinds),
      generation: gen,
      loading: true,
      revealedNodeId: null,
      pendingReveal: null,
    });
    void loadVbfRootNodes(gen);
    await loadAllKinds(gen);
    if (gen === generationCounter.current) {
      patch({ loading: false });
      resolvePendingReveal();
    }
  };

  /**
   * Alterna expandir/colapsar. Um nó do .vbf ainda sem filhos busca no
   * backend ANTES de abrir (expansão preguiçosa, com spinner no chevron) —
   * o container nunca é indexado de uma vez.
   */
  const toggleNode = (node: SideNode): void => {
    if (node.vbf && node.expandable && !node.children) {
      void loadVbfChildren(node);
      return;
    }
    store.setState((prev) => {
      const next = new Set(prev.expanded);
      if (next.has(node.id)) next.delete(node.id);
      else next.add(node.id);
      return { ...prev, expanded: next };
    });
  };

  /**
   * Reveal da busca: expande o caminho da folha em UNION (o resto do que o
   * usuário abriu permanece) e marca o nó para o ContentTree rolar até ele.
   * `kind:text` entra junto porque é o wrapper VISUAL acima das raízes de
   * kind — sem ele a folha existe mas fica escondida no grupo.
   */
  const revealNodePath = (leafId: string): boolean => {
    const path = ancestorPathOf(store.state.roots, leafId);
    if (path.length === 0) return false;
    store.setState((prev) => {
      const expanded = new Set(prev.expanded).add(TEXT_ROOT_NODE_ID);
      for (const id of path) expanded.add(id);
      return { ...prev, expanded, revealedNodeId: leafId };
    });
    return true;
  };

  /**
   * Resultado do modal de busca: revela o arquivo na árvore e abre a entrada
   * com a row do match (o pendingTableRowFocus cuida do foco/scroll na
   * tabela). Folha fora da árvore (kind ainda em carga) fica pendente e
   * conclui sozinha quando os kinds terminam.
   */
  const openSearchResult = async (
    leafId: string,
    match?: services.TextSearchMatch,
  ): Promise<void> => {
    const node = buildNodeIndex(store.state.roots, store.state.vbfRoots).get(leafId);
    if (!node?.entry) {
      patch({ pendingReveal: { leafId, match: match ?? null } });
      return;
    }
    revealNodePath(leafId);
    await selectEntry(node.entry, match);
  };

  /** Conclui um reveal adiado — a árvore terminou de carregar. */
  const resolvePendingReveal = (): void => {
    const pending = store.state.pendingReveal;
    if (!pending) return;
    if (
      !buildNodeIndex(store.state.roots, store.state.vbfRoots).get(pending.leafId)
    ) {
      return;
    }
    patch({ pendingReveal: null });
    void openSearchResult(pending.leafId, pending.match ?? undefined);
  };

  /**
   * Caminho interno (no container) de um arquivo do .vbf pelo id — a folha
   * só existe na árvore depois que o diretório foi aberto (carga
   * preguiçosa). A def de uma ref está na sessão JUSTAMENTE porque já foi
   * aberta alguma vez, então a folha costuma estar lá.
   */
  const vbfPathOf = (root: string, id: string): string | undefined => {
    const walk = (nodes: SideNode[]): string | undefined => {
      for (const node of nodes) {
        const vbf = node.entry?.vbf;
        if (vbf && vbf.root === root && node.entry?.id === id) return vbf.path;
        const hit = node.children ? walk(node.children) : undefined;
        if (hit !== undefined) return hit;
      }
      return undefined;
    };
    return walk(store.state.vbfRoots);
  };

  /**
   * Garante a BASE do rascunho do alvo da edição — é o que torna possível
   * editar PELA REF: a def pode viver em um arquivo que o usuário nunca
   * abriu, e sem base o editDraft.setCell descarta a edição em silêncio (o
   * payload de apply é montado a partir dela). Devolve false quando a def
   * não pôde ser carregada: nesse caso o editor NÃO abre — gravar sem base
   * fabricaria um payload incompleto do arquivo da def.
   */
  const ensureDraftBase = async (t: TranslationTarget): Promise<boolean> => {
    if (editDraft.hasBase(version, t.kind, t.id, t.source)) return true;
    try {
      let full: dto.FileEntry;
      if (t.source) {
        const path = vbfPathOf(t.source, t.id);
        if (path === undefined) {
          sendErrorNotificationWithMessage(
            `O arquivo da def (${t.id}) não está na árvore deste .vbf — abra-o na sidebar para editá-lo pela ref.`
          );
          return false;
        }
        full = await loadVbfEntry(t.source, path, t.id);
      } else {
        full = await loadEntry(t.kind, t.id, version);
      }
      editDraft.setBase(version, t.kind, t.id, full, t.source);
      return true;
    } catch (error) {
      sendErrorNotification(error);
      return false;
    }
  };

  const actions: EntryActions = {
    reload,
    coldReload,
    selectEntry,
    selectNode: async (node) => {
      if (node.vbf && node.unsupported) {
        // Fora do escopo do app (áudio, vídeo, .exe…): avisa SEM tentar
        // decodificar — só o caminho do arquivo aparece, nunca conteúdo.
        sendErrorNotificationWithMessage(
          `Formato fora do escopo do app: ${node.label}`
        );
        return;
      }
      if (node.vbf && !node.entry) {
        // Raiz/diretório do .vbf: o clique alterna a expansão.
        toggleNode(node);
        return;
      }
      if (node.entry && node.kind) {
        await selectEntry(node.entry);
      } else if (node.kind) {
        // Grupo/raiz de kind de data/: igual ao .vbf, o clique no nome
        // também expande/colapsa (além de ativar o kind da aba).
        toggleNode(node);
        patch({ activeKind: node.kind });
      } else if (node.children?.length || node.expandable) {
        // Grupo visual sem kind de backend apenas alterna a expansao.
        toggleNode(node);
      }
    },
    toggleNode,
    requestTableFocus: () => patch({ pendingTableFocus: true }),
    consumeTableFocus: () => patch({ pendingTableFocus: false }),
    consumeTableRowFocus: () => patch({ pendingTableRowFocus: null }),
    openSearchResult,
    consumeRevealedNode: () => patch({ revealedNodeId: null }),
    openDialog: (row, target) => {
      // Todo kind de TEXTO do .vbf é editável (o salvar roteia ao binding
      // de .vbf com o estado da sessão, gravando em mods/). Só kind fora
      // desta lista cai aqui — hoje nenhum texto; imagens têm fluxo próprio.
      const entry = store.state.selectedEntry;
      if (entry?.vbf && !EDITABLE_VBF_KINDS.has(entry.kind)) {
        sendErrorNotificationWithMessage(
          `Edição de ${KIND_LABELS[entry.kind]} a partir do .vbf ainda não é suportada — abra pela árvore de data/.`
        );
        return;
      }
      const current = entry
        ? editDraft.editOf(
            version,
            entry.kind,
            entry.id,
            row,
            SOURCE_LANG,
            entry.vbf?.root ?? ''
          )
        : undefined;
      const dialogRow =
        current === undefined
          ? row
          : dto.TextRow.createFrom({
              ...row,
              text: { ...(row.text ?? {}), [SOURCE_LANG]: current },
            });
      patch({
        translationRow: dialogRow,
        translationTarget: target ?? null,
        translationCopyRow: null,
        translationDivergent: false,
        dialogOpen: true,
      });
    },
    openLinked: async (row, divergent = false) => {
      const entry = store.state.selectedEntry;
      if (!entry) return;
      const link = store.state.refLinks[rowKey(row)];
      if (!link) {
        actions.openDialog(row);
        return;
      }
      // Def na MESMA entrada (ou sem origem anotada): usa a row real.
      let def: dto.TextRow | undefined;
      if (!link.sourceId || link.sourceId === entry.id) {
        def = store.state.rows.find(
          (r) =>
            r.index === link.sourceIndex &&
            (r.name ?? '') === (link.sourceName ?? '')
        );
      }
      // Def em OUTRA entrada (ex.: ref de 236 apontando o 235): row
      // sintetizada + alvo override. Sempre prepara a base da def porque o
      // checkbox ainda pode escolher o fluxo padrão (editar a def).
      const rKey = `${link.sourceIndex}:${link.sourceName ?? ''}`;
      const target: TranslationTarget = {
        kind: entry.kind,
        id: link.sourceId ?? entry.id,
        source: entry.vbf?.root ?? '',
      };
      if (target.id !== entry.id && !(await ensureDraftBase(target))) return;
      const live = editDraft.editTextOf(
        version,
        target.kind,
        target.id,
        rKey,
        SOURCE_LANG,
        target.source
      );
      const defRow =
        def ??
        dto.TextRow.createFrom({
          index: link.sourceIndex,
          name: link.sourceName,
          hash: { [SOURCE_LANG]: row.hash?.[SOURCE_LANG] ?? '' },
          text: { [SOURCE_LANG]: live ?? link.text ?? '' },
          // Pristine da def em TODOS os idiomas (RefLink.Originals): sem o
          // mapa inteiro, a def em OUTRO arquivo perdia os idiomas extras no
          // editor (só 'us' aparecia). Fallback para o 'us' único do tooltip.
          original:
            link.originals ??
            (link.original !== undefined
              ? { [SOURCE_LANG]: link.original }
              : undefined),
        });
      const defText =
        live ??
        editDraft.editOf(version, entry.kind, target.id, defRow, SOURCE_LANG, target.source) ??
        link.text ??
        '';
      const dialogRow = dto.TextRow.createFrom({
        ...defRow,
        text: { ...(defRow.text ?? {}), [SOURCE_LANG]: defText },
      });
      patch({
        translationRow: dialogRow,
        translationTarget: target.id === entry.id ? null : target,
        translationCopyRow: row,
        translationDivergent: divergent,
        dialogOpen: true,
      });
    },
    navigateRow: (direction, value) => {
      const {
        translationRow: row,
        translationTarget: target,
        translationCopyRow,
      } = store.state;
      if (translationCopyRow) return;
      const entry = store.state.selectedEntry;
      if (value !== undefined && row && (target || entry)) {
        const t = target ?? {
          kind: entry!.kind,
          id: entry!.id,
          source: entry!.vbf?.root ?? '',
        };
        editDraft.setCell(version, t.kind, t.id, row, SOURCE_LANG, value, t.source);
        patch({ rows: [...store.state.rows] });
      }
      if (target) return; // alvo override: navegação desabilitada (row única)
      const idx = store.state.rows.findIndex(
        (r) => rowKey(r) === (row ? rowKey(row) : null)
      );
      const next =
        store.state.rows[idx + (direction === 'next' ? 1 : -1)];
      if (next) patch({ translationRow: next });
    },
    commitRow: (value, divergentChoice) => {
      const {
        translationRow: row,
        translationTarget: target,
        translationCopyRow,
      } = store.state;
      const entry = store.state.selectedEntry;
      if (value !== undefined && row && (target || entry)) {
        if (divergentChoice && translationCopyRow && entry) {
          editDraft.setCell(
            version,
            entry.kind,
            entry.id,
            translationCopyRow,
            SOURCE_LANG,
            value,
            entry.vbf?.root ?? '',
            true
          );
        } else {
          const t = target ?? {
            kind: entry!.kind,
            id: entry!.id,
            source: entry!.vbf?.root ?? '',
          };
          editDraft.setCell(version, t.kind, t.id, row, SOURCE_LANG, value, t.source);
        }
        patch({ rows: [...store.state.rows] });
      }
      patch({
        dialogOpen: false,
        translationRow: null,
        translationTarget: null,
        translationCopyRow: null,
        translationDivergent: false,
      });
    },
    revertToOriginal: (row) => {
      const entry = store.state.selectedEntry;
      const original = row.original?.[SOURCE_LANG];
      if (!entry || original === undefined || original === '') return;
      editDraft.setCell(
        version,
        entry.kind,
        entry.id,
        row,
        SOURCE_LANG,
        original,
        entry.vbf?.root ?? '',
        true
      );
      patch({ rows: [...store.state.rows] });
    },
    ensureCellEditBase: async (target) => {
      const entry = store.state.selectedEntry;
      // Alvo da própria entrada aberta: a base já foi registrada na abertura.
      if (!entry || (target.id === entry.id && target.source === (entry.vbf?.root ?? ''))) {
        return !!entry;
      }
      // Def em outra entrada (ou fonte diferente): prepara o rascunho dela.
      return ensureDraftBase(target);
    },
    applyCellEdit: (target, row, lang, text) => {
      editDraft.setCell(
        version,
        target.kind,
        target.id,
        row,
        lang,
        text,
        target.source
      );
      patch({ rows: [...store.state.rows] });
    },
    openImageAction: (action) => {
      patch({ imageAction: action });
    },
    closeImageAction: () => patch({ imageAction: null }),
  };

  return { version, store, actions };
}
