import { useSelector } from '@tanstack/react-store';
import { createStore } from '@tanstack/store';
import { useEffect } from 'react';
import { ListLanguages } from '@/bindings/ffxresources/textservice';
import { SOURCE_LANG } from './save-all';

/**
 * Idioma default da exportação: espelho frontend de
 * `common.DefaultLocalization` (backend/common/constants.go) — o mesmo
 * conceito do SOURCE_LANG de edição. Sai SEMPRE nos artefatos (sem
 * checkbox na UI, sempre no topo do picker). Se a constante do backend
 * um dia trocar, este é o único ponto a alinhar: nada aqui escreve 'us'
 * literalmente — o default é a referência da qual os extras são
 * derivados (`available.filter(code !== DEFAULT_LANG)`), então um troca
 * de default automaticamente promove o novo código ao topo e baixa o
 * antigo para a pilha com checkbox.
 */
export const DEFAULT_LANG = SOURCE_LANG;

const LANGS_STORAGE_KEY = 'ffx.export-langs';

/** Chave/valor de idioma (mesmo shape do binding ListLanguages). */
export interface Language {
  code: string;
  name: string;
}

interface LangsSnapshot {
  revision: number;
  /** Idiomas do backend (ordem do backend); vazio antes da hidratação. */
  available: readonly Language[];
  /** Códigos dos extras marcados para exportação. */
  checked: ReadonlySet<string>;
  /** false antes da hidratação (ListLanguages + storage). */
  loaded: boolean;
}

/**
 * Store reativo da seleção de idiomas (mesmo padrão do export-selection):
 * snapshot imutável {revision, available, checked, loaded}, dados de
 * trabalho imperativos no singleton.
 */
const langStore = createStore<LangsSnapshot>({
  revision: 0,
  available: [],
  checked: new Set<string>(),
  loaded: false,
});

/**
 * Seleção de idiomas para TODAS as exportações do frontend (Exportar do
 * topo, exportar da árvore, VBF). O array passado ao backend é
 * `[default, ...extras marcados]`; `[]` (pré-hidratação/falha de binding)
 * mantém o contrato do backend: vazilo = TODOS os idiomas.
 */
class ExportLangsStore {
  private available: Language[] = [];
  private readonly checked = new Set<string>();
  private loaded = false;
  private hydrating = false;

  getSnapshot(): LangsSnapshot {
    return langStore.state;
  }

  private notify(): void {
    langStore.setState((prev) => ({
      revision: prev.revision + 1,
      available: [...this.available],
      checked: new Set(this.checked),
      loaded: this.loaded,
    }));
  }

  /**
   * Idempotente (remount da aba): busca os idiomas via binding e aplica o
   * storage. Se o binding falhar (dev no browser, runtime Wails ausente),
   * `loaded` fica false e `langsOf()` devolve `[]` = todos — o
   * comportamento de antes do seletor.
   */
  async hydrate(): Promise<void> {
    if (this.hydrating || this.loaded) return;
    this.hydrating = true;
    try {
      const langs = await ListLanguages();
      this.applyAvailable(langs ?? []);
    } catch {
      // sem runtime Wails — segue com o fallback "todos os idiomas"
    } finally {
      this.hydrating = false;
    }
  }

  private applyAvailable(langs: Language[]): void {
    this.available = langs;
    const extras = new Set(langs.map((l) => l.code).filter((code) => code !== DEFAULT_LANG));
    // Sem storage (ou inválido) = TODOS marcados (comportamento atual);
    // com storage = só os códigos ainda válidos — o código que virou
    // default e códigos desconhecidos caem fora em silêncio.
    let stored: string[] | null = null;
    try {
      const raw = localStorage.getItem(LANGS_STORAGE_KEY);
      const parsed: unknown = raw === null ? null : JSON.parse(raw);
      if (Array.isArray(parsed)) {
        stored = parsed.filter((c): c is string => typeof c === 'string');
      }
    } catch {
      stored = null;
    }
    this.checked.clear();
    for (const code of stored === null ? extras : stored) {
      if (extras.has(code)) this.checked.add(code);
    }
    this.loaded = true;
    this.notify();
  }

  /** Marca/desmarca um extra e persiste a lista completa no storage. */
  toggle(code: string): void {
    if (code === DEFAULT_LANG) return; // default não tem marcação
    if (!this.available.some((l) => l.code === code)) return;
    if (this.checked.has(code)) {
      this.checked.delete(code);
    } else {
      this.checked.add(code);
    }
    try {
      localStorage.setItem(LANGS_STORAGE_KEY, JSON.stringify([...this.checked].sort()));
    } catch {
      // localStorage indisponível — segue só em memória
    }
    this.notify();
  }

  /**
   * Array de idiomas para o backend: o default PRIMEIRO (sempre, por
   * construção — independente do storage) e depois os extras marcados na
   * ordem do backend. Pré-hidratação/falha = `[]` = todos os idiomas.
   */
  langsOf(): string[] {
    if (!this.loaded) return [];
    return [
      DEFAULT_LANG,
      ...this.available
        .map((l) => l.code)
        .filter((code) => code !== DEFAULT_LANG && this.checked.has(code)),
    ];
  }
}

export const exportLangs = new ExportLangsStore();

/** Snapshot reativo (rerender ao marcar/desmarcar). Hydration-safe. */
export function useExportLangs(): LangsSnapshot {
  const snapshot = useSelector(langStore);

  // Mesmo padrão do useExportSelection: localStorage/binding só no
  // cliente; hydrate cedo-retorna quando já hidratado (remount da aba).
  useEffect(() => {
    void exportLangs.hydrate();
  }, []);

  return snapshot;
}
