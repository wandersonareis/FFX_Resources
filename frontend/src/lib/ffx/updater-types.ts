/**
 * Payloads dos eventos `wails:updater:*` — espelho em TS de
 * `pkg/updater/types.go` (Wails v3 beta.33). Campos marcados como opcionais
 * vêm `omitempty` no JSON do Go.
 *
 * Contrato completo (lista de eventos, exemplos, armadilhas) em
 * docs/updater.md §6.
 */

/** Artifact descreve o arquivo a baixar para a plataforma em execução. */
export interface UpArtifact {
  filename?: string;
  filetype?: string;
  size?: number;
  platform?: string;
  arch?: string;
}

/** Release é o que o provider devolve quando há algo mais novo. */
export interface UpRelease {
  version: string;
  channel?: string;
  name?: string;
  /** Corpo da release no GitHub — é o Markdown renderizado na janela. */
  notes?: string;
  publishedAt?: string;
  artifact?: UpArtifact;
}

/** Progress é o payload de wails:updater:download-progress (~10/s). */
export interface UpProgress {
  written: number;
  total: number;
  /** bytes/s suavizados nos últimos ~1s. */
  rate?: number;
  provider?: string;
}

/** Stage é o estágio que produziu o erro (payload de updater:error). */
export type UpStage = 'check' | 'download' | 'verify' | 'install';

/** ErrorInfo é o payload de wails:updater:error. */
export interface UpError {
  stage?: UpStage;
  message?: string;
  provider?: string;
}
