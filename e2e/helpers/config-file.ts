import * as fs from 'node:fs';
import { CONFIG_PATH } from './fixtures';

/**
 * Leitura do config.json do app — a mesma fonte de verdade que o
 * spira-server.exe lê no GetExecDir(). O app grava o arquivo de forma
 * NÃO atômica (rename/copy direto), então quem lê "na hora" pode pegar o
 * arquivo no meio da escrita: toda leitura vem com retry curto.
 */

interface RawAppConfig {
  EnableMods?: boolean;
}

/** config.json completo, com retry para escrita não atômica em curso. */
export function readAppConfig<T = RawAppConfig>(): T {
  for (let i = 0; i < 40; i++) {
    try {
      const text = fs.readFileSync(CONFIG_PATH, 'utf8').trim();
      if (text.length > 0) return JSON.parse(text) as T;
    } catch {
      // arquivo no meio da escrita — tenta de novo
    }
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 50);
  }
  throw new Error(`config.json ilegível: ${CONFIG_PATH}`);
}

/**
 * EnableMods persistido. Ausente no arquivo = default `true` (mesma
 * regra do backend em ffx_app_config.go: sem a chave mantém o default).
 */
export function readEnableMods(): boolean {
  return readAppConfig().EnableMods ?? true;
}
