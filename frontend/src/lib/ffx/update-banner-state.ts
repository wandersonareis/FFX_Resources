import type { UpProgress, UpRelease } from '@/lib/ffx/updater-types';

/**
 * Estado da banner de atualização (frontend/src/components/update-banner.tsx)
 * como reducer puro: os eventos `wails:updater:*` viram ações tipadas e a UI
 * só renderiza BannerState.
 *
 * Fica num módulo separado (sem JSX) para poder ser testado depois sem
 * runner de testes no projeto hoje.
 */

export type BannerPhase = 'hidden' | 'available' | 'working' | 'ready';

export type BannerStep = 'check' | 'download' | 'verify' | 'install';

export interface BannerState {
  phase: BannerPhase;
  /** Versão disponível/pronta (payload do update-available). */
  release?: UpRelease;
  /** Sub-estado quando phase é 'working'. */
  step?: BannerStep;
  /** Progresso do download (phase 'working' e step 'download'). */
  progress?: UpProgress;
}

export const initialBannerState: BannerState = { phase: 'hidden' };

export type BannerAction =
  | { type: 'available'; release: UpRelease }
  | { type: 'checking' }
  | { type: 'download-started' }
  | { type: 'download-progress'; progress: UpProgress }
  | { type: 'verifying' }
  | { type: 'installing' }
  | { type: 'ready'; release?: UpRelease }
  | { type: 'error' }
  | { type: 'no-update' }
  | { type: 'dismiss' };

/**
 * O bus do Wails despacha cada Emit numa goroutine, então os eventos chegam
 * FORA de ordem (o próprio template nativo já viu "installing → verifying →
 * update-ready"). Ranqueamento monotônico — o mesmo critério do
 * window.html — para um evento atrasado não rebaixar a UI.
 */
const RANK: Record<BannerPhase, number> = {
  hidden: 0,
  available: 1,
  working: 2,
  ready: 3,
};

export function updateBannerReducer(
  state: BannerState,
  action: BannerAction
): BannerState {
  switch (action.type) {
    // Só sobe: um update-available atrasado não volta de working/ready.
    case 'available':
      if (RANK[state.phase] >= RANK.available) return state;
      return { phase: 'available', release: action.release };

    // Só aparece se a banner já está viva: o check silencioso do startup
    // não puxa a UI do nada (a janela nativa cuida disso quando interessa).
    case 'checking':
      if (state.phase === 'hidden') return state;
      return { ...state, phase: 'working', step: 'check', progress: undefined };

    case 'download-started':
      if (RANK[state.phase] >= RANK.ready) return state;
      return { ...state, phase: 'working', step: 'download', progress: undefined };

    case 'download-progress':
      if (state.phase !== 'working' || state.step !== 'download') return state;
      return { ...state, progress: action.progress };

    case 'verifying':
    case 'installing': {
      if (RANK[state.phase] >= RANK.ready) return state;
      const step: BannerStep = action.type === 'verifying' ? 'verify' : 'install';
      return { ...state, phase: 'working', step, progress: undefined };
    }

    case 'ready':
      if (RANK[state.phase] >= RANK.ready) return state;
      return { phase: 'ready', release: action.release ?? state.release };

    // error/no-update fecham a banner (o erro vira toast no componente) e o
    // dismiss é o usuário mandando embora.
    case 'error':
    case 'no-update':
    case 'dismiss':
      return initialBannerState;

    default:
      return state;
  }
}
