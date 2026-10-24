import { createStore, useSelector } from '@tanstack/react-store';
import {
  GetConfigSnapshot,
  SetEnableMods,
  SetGameExeLocation,
  SetGameLocation,
  SetTranslateLocation,
} from '@/bindings/ffxresources/configservice';
import { loggedToast as toast } from './toast-logged';
import { invalidateVbfCache } from './vbf';

/**
 * Store reativo do config do app (TanStack Store, mesmo padrão do
 * statusbar-store): o backend persiste no config.json e re-emite o
 * snapshot completo no evento "AppConfigChanged" — este store espelha a
 * VERDADE do backend (caminho normalizado pelo Abs, executável recusado
 * em silêncio, translated derivado do gamefiles) e distribui para
 * qualquer consumidor: o diálogo de configuração hoje; árvore/status
 * amanhã, sem prop drilling e sem re-fetch ao reabrir o modal.
 */

export interface AppConfig {
  gameFilesLocation: string;
  translateLocation: string;
  gameExeLocation: string;
  enableMods: boolean;
  gameVersion: string;
}

/** Payload cru do evento/consulta — chaves snake_case do DTO Go. */
interface AppConfigWire {
  game_files_location?: string;
  translate_location?: string;
  game_exe_location?: string;
  enable_mods?: boolean;
  game_version?: string;
}

type EnableModsListener = () => void;

/**
 * Último enableMods CONFIRMADO pelo backend (eco AppConfigChanged ou carga
 * inicial) — NÃO pelo set otimista do commit. O eco compara contra ELE:
 * o checkbox commita otimista (o store muda na hora, antes do eco), então
 * comparar com o estado atual esconderia a mudança real quando o eco
 * chega (prevMods já seria o valor novo e o coldReload nunca dispararia).
 * null = ainda sem confirmação (a primeira carga inicializa).
 */
let lastConfirmedMods: boolean | null = null;

/**
 * Assinantes de "a preferência de mods mudou de verdade no backend" —
 * as abas (GameVersionTab) recarregam o conteúdo (coldReload) quando o
 * toggle liga/desliga a tradução. Só dispara no ECO (AppConfigChanged),
 * nunca no set otimista nem na carga inicial: o eco chega depois de o
 * backend ter invalidado os caches de view, então o reload pega dados
 * frescos, sem corrida.
 */
const enableModsListeners = new Set<EnableModsListener>();

/** Assina mudanças reais de enableMods (eco do backend). Devolve unsubscribe. */
export function onEnableModsChanged(listener: EnableModsListener): () => void {
  enableModsListeners.add(listener);
  return () => enableModsListeners.delete(listener);
}

export const configStore = createStore<AppConfig>({
  gameFilesLocation: '',
  translateLocation: '',
  gameExeLocation: '',
  enableMods: true, // mods-first é o comportamento padrão do backend
  gameVersion: '',
});

/**
 * Aplica um snapshot cru (evento AppConfigChanged ou GetConfigSnapshot)
 * ao store. Payload inválido é ignorado: o store nunca fica pior do que
 * a última verdade conhecida.
 *
 * `fromEvent` distingue o ECO do backend (setters persistiram e, no caso
 * de enableMods, invalidaram os caches de view) da carga inicial e do set
 * otimista: só o eco dispara os listeners de mudança de enableMods — é a
 * única fase em que "o backend mudou de verdade" e recarregar pega dados
 * com a preferência nova.
 */
export function applyConfigSnapshot(raw: unknown, fromEvent = false): void {
  if (!raw || typeof raw !== 'object') return;
  const s = raw as AppConfigWire;
  const prevExe = configStore.state.gameExeLocation;
  const nextExe = s.game_exe_location ?? '';
  const nextMods = Boolean(s.enable_mods);
  configStore.setState(() => ({
    gameFilesLocation: s.game_files_location ?? '',
    translateLocation: s.translate_location ?? '',
    gameExeLocation: nextExe,
    enableMods: nextMods,
    gameVersion: s.game_version ?? '',
  }));
  // Troca de executável muda as raízes .vbf. Este store é POR JANELA (cada
  // WebviewWindow tem sua instância), então quem recebe o eco invalida o
  // PRÓPRIO cache — é assim que a janela principal descobre a troca feita na
  // janela de config (e vice-versa), sem acoplamento entre janelas.
  if (nextExe !== prevExe) invalidateVbfCache();
  // Preferência de mods mudou NO BACKEND: o diff é contra o último valor
  // CONFIRMADO (o otimista do checkbox já reescreveu o estado do store, e
  // sem esse contraste o eco não veria a mudança). O backend invalidou os
  // caches de view dentro de SetEnableMods antes deste eco — recarregar
  // agora pega o conteúdo com a preferência nova.
  const modsChanged = nextMods !== (lastConfirmedMods ?? nextMods);
  lastConfirmedMods = nextMods;
  if (fromEvent && modsChanged) {
    for (const listener of enableModsListeners) listener();
  }
}

/** Carga inicial (mount do AppShell); atualizações vêm pelo evento. */
export async function loadConfigSnapshot(): Promise<void> {
  try {
    applyConfigSnapshot(await GetConfigSnapshot());
  } catch (error) {
    notify(error);
  }
}

function notify(error: unknown): void {
  const message =
    error instanceof Error ? error.message : String(error ?? 'Erro');
  // loggedToast.error já copia para o log do backend (WriteLog) — não
  // emitir "Notify" aqui: o evento circularia de volta ao ProgressNotifier.
  toast.error(message, { duration: 4000 });
}

/**
 * Commit otimista + eco: o store assume o valor novo NA HORA (a UI
 * responde instantaneamente) e o backend confirma em seguida pelo
 * evento AppConfigChanged, que sobrescreve com a verdade (Abs,
 * derivação do translated, exe recusado). Erro → rollback consultando o
 * que realmente ficou persistido.
 */
async function commit(
  optimistic: Partial<AppConfig>,
  call: () => Promise<unknown>
): Promise<void> {
  configStore.setState((prev) => ({ ...prev, ...optimistic }));
  try {
    await call();
  } catch (error) {
    notify(error);
    await loadConfigSnapshot();
  }
}

export function commitGameLocation(path: string): Promise<void> {
  return commit({ gameFilesLocation: path }, () => SetGameLocation(path));
}

export function commitTranslateLocation(path: string): Promise<void> {
  return commit(
    { translateLocation: path },
    () => SetTranslateLocation(path)
  );
}

export function commitGameExeLocation(path: string): Promise<void> {
  // O cache local de diretórios/entradas do .vbf aponta para containers
  // que podem nem existir mais (o backend fecha os handles abertos).
  invalidateVbfCache();
  return commit({ gameExeLocation: path }, () => SetGameExeLocation(path));
}

export function commitEnableMods(enabled: boolean): Promise<void> {
  return commit({ enableMods: enabled }, () => SetEnableMods(enabled));
}

/** Hook de leitura reativa (componentes). */
export function useAppConfig(): AppConfig {
  return useSelector(configStore, (s) => s);
}
