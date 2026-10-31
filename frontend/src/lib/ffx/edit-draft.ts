import { useSelector } from '@tanstack/react-store';
import { createStore } from '@tanstack/store';
import type * as services from '@/bindings/ffxresources/backend/services/models';
import * as dto from '../../../bindings/ffxresources/backend/dto/models';
import type { EntryKind } from './display-names';
import type { GameVersionId } from './game-version';
import {
  missingProtectedTags,
  newUnclosedFragment,
  restoreProtectedTags as restoreTags,
} from './protected-tags';

/** Localização padrão do editor (mesma de save-all/SOURCE_LANG). */
const DRAFT_SOURCE_LANG = 'us';

/** Bloco (chunk) de uma entrada do dicionário; null nos formatos sem bloco. */
function chunkOfEntry(entry: dto.FileEntry): number | null {
  const match = /chunk_(\d+)/.exec(entry.metadata.id ?? '');
  if (!match) return null;
  const n = Number(match[1]);
  return Number.isFinite(n) ? n : null;
}

/** Chave de linha no rascunho: mesma que o backend usa (Index + Name). */
export function rowKey(row: Pick<dto.TextRow, 'index' | 'name'>): string {
  return `${row.index}:${row.name ?? ''}`;
}

interface DraftState {
  version: GameVersionId;
  kind: EntryKind;
  id: string;
  /** Entrada original carregada (base para reconstruir o lote no salvar). */
  entry: dto.FileEntry;
  /** Edições por rowKey → idioma → texto (só o que difere do original). */
  edits: Map<string, Map<string, string>>;
  /** Rows que devem aplicar só nesta cópia, sem replicar ao grupo. */
  divergentRows: Set<string>;
  /**
   * Rows ÓRFÃS (só em mods, sem par no original) que o usuário mandou
   * excluir. O save as deixa fora do binário; a tabela as esconde.
   */
  deleted: Set<string>;
  /**
   * Origem do rascunho: "" = data/; caminho do .vbf = tabela aberta no
   * navegador de container. O mesmo id pode existir nas duas fontes —
   * a chave leva a origem para não colidir, e o destino do salvar muda:
   * data/ → ApplyTextCollection; .vbf → ApplyVbfTextCollection.
   */
  source: string;
}

const draftKey = (version: GameVersionId, kind: EntryKind, id: string, source: string): string =>
  `${version}|${kind}|${id}|${source}`;

interface DraftSnapshot {
  hasDirty: boolean;
  revision: number;
}

/** Relatório do gate de tags no save-all (ver restoreProtectedTags). */
export interface GateReport {
  restored: number;
  unresolved: number;
  unclosed: number;
}

type ReloadListener = () => void;

/**
 * Store reativo do resumo de rascunhos (TanStack Store): {hasDirty, revision}
 * substitui o snapshot imutável que o useSyncExternalStore consumia. Os dados
 * (estados/dirtyFiles) ficam imperativos no singleton abaixo — só a notificação
 * é reativa.
 */
const editStore = createStore<DraftSnapshot>({
  hasDirty: false,
  revision: 0,
});

/**
 * Rascunhos de edição em memória (singleton reativo). Trocar de arquivo não
 * perde nada: o rascunho fica por (version|kind|id) até o usuário salvar ou
 * descartar. O "traduzido" é o texto editado que substitui o slot 'us' no
 * binário. As notificações disparam o TanStack Store compartilhado.
 */
class EditDraftStore {
  private readonly states = new Map<string, DraftState>();
  private readonly dirtyFiles = new Set<string>();
  private readonly reloadListeners = new Set<ReloadListener>();

  /** Snapshot imutável atual (leitura imperativa, p/ saveAllDrafts). */
  getSnapshot(): DraftSnapshot {
    return editStore.state;
  }

  /** Callback chamado após salvar tudo (reselect da aba ativa). */
  onSaved(listener: ReloadListener): () => void {
    this.reloadListeners.add(listener);
    return () => this.reloadListeners.delete(listener);
  }

  private notify(): void {
    editStore.setState((prev) => ({
      hasDirty: this.dirtyFiles.size > 0,
      revision: prev.revision + 1,
    }));
  }

  /** Registra a base (entrada original) ao abrir o arquivo na tabela. */
  setBase(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    entry: dto.FileEntry,
    source = '',
  ): void {
    const key = draftKey(version, kind, id, source);
    const existing = this.states.get(key);
    if (existing) {
      existing.entry = entry;
      return;
    }
    this.states.set(key, {
      version,
      kind,
      id,
      entry,
      edits: new Map(),
      divergentRows: new Set(),
      deleted: new Set(),
      source,
    });
    this.touch(key, false);
  }

  /**
   * A base da entrada já está registrada? É a guarda de quem edita ATRAVÉS
   * do link: sem base, setCell descarta a edição em silêncio.
   */
  hasBase(version: GameVersionId, kind: EntryKind, id: string, source = ''): boolean {
    return this.states.has(draftKey(version, kind, id, source));
  }

  /** Descarta rascunho sem edição (base fora de uso). */
  dropBase(version: GameVersionId, kind: EntryKind, id: string, source = ''): void {
    const key = draftKey(version, kind, id, source);
    const state = this.states.get(key);
    if (state && state.edits.size === 0 && state.deleted.size === 0) {
      this.states.delete(key);
    }
  }

  /** Registra/edita o texto de uma célula. Texto igual ao original limpa a edição. */
  setCell(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    row: dto.TextRow,
    lang: string,
    text: string,
    source = '',
    divergent?: boolean,
  ): void {
    const key = draftKey(version, kind, id, source);
    const state = this.states.get(key);
    if (!state) return;

    const rKey = rowKey(row);
    const baseRow = state.entry.rows.find((candidate) => rowKey(candidate) === rKey);
    const original = baseRow?.text?.[lang] ?? row.text?.[lang] ?? '';
    let byLang = state.edits.get(rKey);
    if (text === original) {
      if (byLang) {
        byLang.delete(lang);
        if (byLang.size === 0) {
          state.edits.delete(rKey);
        }
      }
      state.divergentRows.delete(rKey);
    } else {
      if (!byLang) {
        byLang = new Map();
        state.edits.set(rKey, byLang);
      }
      byLang.set(lang, text);
      if (divergent === true) state.divergentRows.add(rKey);
      else if (divergent === false) state.divergentRows.delete(rKey);
    }
    this.touch(key, state.edits.size > 0);
  }

  /** Valor editado da célula (undefined = sem edição). */
  editOf(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    row: dto.TextRow,
    lang: string,
    source = '',
  ): string | undefined {
    return this.editTextOf(version, kind, id, rowKey(row), lang, source);
  }

  isDivergent(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    row: dto.TextRow,
    source = '',
  ): boolean {
    const state = this.states.get(draftKey(version, kind, id, source));
    const rKey = rowKey(row);
    return !!state?.divergentRows.has(rKey) && (state.edits.get(rKey)?.size ?? 0) > 0;
  }

  /**
   * Valor editado da célula pelo rowKey — sem precisar do row. É o que a
   * célula linkada usa para mostrar a edição da def de OUTRA entrada ao
   * vivo (o link anota o rowKey da def).
   */
  editTextOf(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    rKey: string,
    lang: string,
    source = '',
  ): string | undefined {
    return this.states
      .get(draftKey(version, kind, id, source))
      ?.edits.get(rKey)
      ?.get(lang);
  }

  isDirty(version: GameVersionId, kind: EntryKind, id: string, source = ''): boolean {
    const state = this.states.get(draftKey(version, kind, id, source));
    return !!state && state.edits.size > 0;
  }

  /** Limpa o rascunho (após salvar a entrada). */
  clear(version: GameVersionId, kind: EntryKind, id: string, source = ''): void {
    const key = draftKey(version, kind, id, source);
    this.states.delete(key);
    this.touch(key, false);
  }

  /** Limpa todos os rascunhos (após salvar tudo). */
  clearAll(): void {
    this.states.clear();
    this.touchAll();
  }

  /**
   * Gate do save-all: restaura as tags protegidas que sumiram de todas as
   * células editadas (reinserção alinhada) e relata o que ainda impede o
   * salvamento:
   *
   *  - restored: células corrigidas automaticamente;
   *  - unresolved: células que continuam sem tag protegida (restore falhou);
   *  - unclosed: células com `{…` aberto que o original não tinha.
   *
   * `unresolved`/`unclosed` maiores que zero abortam o save — o texto não
   * vai ao binário com valor perdido ou tag pela metade.
   */
  restoreProtectedTags(): GateReport {
    let restored = 0;
    let unresolved = 0;
    let unclosed = 0;
    let touched = false;

    for (const state of this.states.values()) {
      if (state.edits.size === 0) continue;
      const rows = new Map<string, dto.TextRow>();
      for (const row of state.entry.rows) rows.set(rowKey(row), row);

      for (const [rKey, byLang] of state.edits) {
        const row = rows.get(rKey);
        if (!row) continue;
        for (const [lang, text] of [...byLang]) {
          const original = row.text?.[lang] ?? '';
          let current = text;

          if (missingProtectedTags(original, current).length > 0) {
            const restoredText = restoreTags(original, current);
            if (restoredText !== current) {
              current = restoredText;
              restored += 1;
              touched = true;
              if (current === original) byLang.delete(lang);
              else byLang.set(lang, current);
            }
            if (missingProtectedTags(original, current).length > 0) {
              unresolved += 1;
            }
          }
          if (newUnclosedFragment(original, current) !== null) unclosed += 1;
        }
        if (byLang.size === 0) {
          state.edits.delete(rKey);
          state.divergentRows.delete(rKey);
        }
      }
      if (state.edits.size === 0 && state.deleted.size === 0) touched = true;
    }

    if (touched) {
      // Só o que resta no rascunho continua sujo (uma célula que virou o
      // original deixa de exigir escrita no arquivo).
      this.dirtyFiles.clear();
      for (const [key, state] of this.states) {
        if (state.edits.size > 0 || state.deleted.size > 0) this.dirtyFiles.add(key);
      }
      this.notify();
    }
    return { restored, unresolved, unclosed };
  }

  /**
   * Monta a Collection do kind/version com as entradas editadas
   * (base + edições mescladas), pronta para ApplyTextCollection.
   * Só rascunhos de data/ (source = ""): os de .vbf vão pelo
   * buildSourceCollection, com destino próprio.
   */
  buildCollection(version: GameVersionId, kind: EntryKind): Record<string, dto.FileEntry> {
    return this.buildSourceCollection(version, kind, '');
  }

  /** Collection do kind/version para UMA fonte ('' = data/; .vbf = caminho). */
  buildSourceCollection(
    version: GameVersionId,
    kind: EntryKind,
    source: string,
  ): Record<string, dto.FileEntry> {
    const out: Record<string, dto.FileEntry> = {};
    for (const state of this.states.values()) {
      if (state.version !== version) continue;
      if (splitDraftKind(state) !== kind) continue;
      if (state.source !== source) continue;
      if (state.edits.size === 0 && state.deleted.size === 0) continue;
      out[splitDraftId(state)] = this.merge(state);
    }
    return out;
  }

  /**
   * Linhas que só existem em mods (sem par no original) nas entradas sujas
   * deste kind/versão. É a prévia do que o save vai EXCLUIR quando o kind
   * não reconstrói o arquivo inteiro (macro tem prévia própria no backend).
   * Mesmo formato do log/modal (PrunedRowView).
   */
  orphanRows(version: GameVersionId, kind: EntryKind): services.PrunedRowView[] {
    const out: services.PrunedRowView[] = [];
    for (const state of this.states.values()) {
      if (state.version !== version || splitDraftKind(state) !== kind) continue;
      if (state.edits.size === 0 && state.deleted.size === 0) continue;
      for (const row of state.entry.rows) {
        if (row.missingInOriginal !== true) continue;
        out.push({
          file: state.entry.metadata.key,
          block: chunkOfEntry(state.entry),
          index: row.index,
          name: row.name ?? '',
          text: row.text?.[DRAFT_SOURCE_LANG] ?? '',
          version: state.version,
          kind: splitDraftKind(state),
        });
      }
    }
    return out;
  }

  /**
   * Marca uma linha ÓRFÃ como excluída no rascunho (o save a deixa fora do
   * binário). Guarda: linha que existe no original é intocável — o binário
   * não aceita menos linhas que o original (tradução parcial preenche com as
   * originais).
   */
  deleteRow(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    row: dto.TextRow,
    source: string,
  ): boolean {
    if (row.missingInOriginal !== true) return false;
    const key = draftKey(version, kind, id, source);
    const state = this.states.get(key);
    if (!state) return false;
    const rowId = rowKey(row);
    if (state.deleted.has(rowId)) return false;
    state.deleted.add(rowId);
    state.edits.delete(rowId);
    state.divergentRows.delete(rowId);
    this.touch(key, true);
    return true;
  }

  /** Linha marcada para exclusão no rascunho (a tabela a esconde). */
  isRowDeleted(
    version: GameVersionId,
    kind: EntryKind,
    id: string,
    row: dto.TextRow,
    source: string,
  ): boolean {
    const state = this.states.get(draftKey(version, kind, id, source));
    return state?.deleted.has(rowKey(row)) ?? false;
  }

  /** Lotes por kind para os arquivos sujos de DATA/ (por versão). */
  dirtyBatches(): Map<GameVersionId, Map<EntryKind, string[]>> {
    const out = new Map<GameVersionId, Map<EntryKind, string[]>>();
    for (const state of this.states.values()) {
      if (state.edits.size === 0 && state.deleted.size === 0) continue;
      if (state.source !== '') continue;
      const kind = splitDraftKind(state);
      const byKind = out.get(state.version) ?? new Map<EntryKind, string[]>();
      const list = byKind.get(kind) ?? [];
      list.push(splitDraftId(state));
      byKind.set(kind, list);
      out.set(state.version, byKind);
    }
    return out;
  }

  /**
   * Lotes de .vbf sujos: versão → container (caminho) → kind → ids. O
   * destino do salvar muda: mesmo motor, escopo da sessão do container.
   */
  dirtyVbfBatches(): Map<GameVersionId, Map<string, Map<EntryKind, string[]>>> {
    const out = new Map<GameVersionId, Map<string, Map<EntryKind, string[]>>>();
    for (const state of this.states.values()) {
      if (state.edits.size === 0 && state.deleted.size === 0) continue;
      if (state.source === '') continue;
      const byRoot = out.get(state.version) ?? new Map<string, Map<EntryKind, string[]>>();
      const byKind = byRoot.get(state.source) ?? new Map<EntryKind, string[]>();
      const kind = splitDraftKind(state);
      const list = byKind.get(kind) ?? [];
      list.push(splitDraftId(state));
      byKind.set(kind, list);
      byRoot.set(state.source, byKind);
      out.set(state.version, byRoot);
    }
    return out;
  }

  private merge(state: DraftState): dto.FileEntry {
    // Linhas marcadas para exclusão (órfãs escolhidas pelo usuário) não vão
    // para o payload — o rebuild as deixa fora do binário.
    const rows = state.entry.rows
      .filter((row) => !state.deleted.has(rowKey(row)))
      .map((row) => {
        const edits = state.edits.get(rowKey(row));
        let text = row.text;
        if (edits && edits.size > 0) {
          text = { ...(row.text ?? {}) };
          for (const [lang, value] of edits) {
            text[lang] = value;
          }
        }
        // Payload de apply: só o que o binário precisa (index/name/hash/text).
        // `original` é exclusivo de EXIBIÇÃO (vem de data/) e não pode viajar
        // no payload — o backend o ignora, mas o contrato é não enviá-lo.
        return dto.TextRow.createFrom({
          index: row.index,
          name: row.name,
          hash: row.hash,
          text,
          divergent:
            (edits?.size ?? 0) > 0 && state.divergentRows.has(rowKey(row)) ? true : undefined,
        });
      });
    return dto.FileEntry.createFrom({ metadata: state.entry.metadata, rows });
  }

  private touch(key: string, dirty: boolean): void {
    if (dirty) {
      this.dirtyFiles.add(key);
    } else {
      this.dirtyFiles.delete(key);
    }
    this.notify();
  }

  private touchAll(): void {
    this.dirtyFiles.clear();
    this.notify();
  }

  private fireSaved(): void {
    for (const listener of this.reloadListeners) listener();
  }

  /** Chamado pelo saveAll após persistir tudo. */
  markSaved(): void {
    this.clearAll();
    this.fireSaved();
  }
}

export const editDraft = new EditDraftStore();

function splitDraftKind(state: DraftState): EntryKind {
  return state.kind;
}

function splitDraftId(state: DraftState): string {
  return state.id;
}

export function useEditDraft(): { store: EditDraftStore; snapshot: DraftSnapshot } {
  const snapshot = useSelector(editStore);
  return { store: editDraft, snapshot };
}
