import * as fs from 'node:fs';
import * as path from 'node:path';

/**
 * Detecção de fixtures POR EXISTÊNCIA — os testes se adaptam ao que existe
 * na máquina (a regra do projeto):
 *
 *  - data/  (GameFilesLocation, default <exec>/data): existe → testes de
 *    exibição de conteúdo (Original obrigatório);
 *  - mods/   (<gamefiles>/mods): existe → coluna Traduzido viva e testes de
 *    mods; não existe → só o teste de escrita/salvar em binário (que cria
 *    mods/ na hora);
 *  - .vbf    (ao lado do executável do jogo): o mínimo requerido — o jogo
 *    inteiro mora no container. Configurado → navegação do container;
 *    ausente → o app deve mostrar o erro pedindo para apontá-lo.
 *
 * Tudo sai do config.json do app (build/bin/config/config.json) — a mesma
 * fonte de verdade que o binário spira-server.exe lê no GetExecDir().
 */

/** Diretório do executável do app (GetExecDir): config/, data/, logs/. */
export const BIN_DIR = path.resolve(__dirname, '..', '..', 'build', 'bin');
export const CONFIG_PATH = path.join(BIN_DIR, 'config', 'config.json');

interface RawConfig {
  Locations?: { GameFilesLocation?: string; TranslateLocation?: string };
  GameExeLocation?: string;
  GameVersion?: string;
  EnableMods?: boolean;
}

function readConfig(): RawConfig | null {
  try {
    return JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8')) as RawConfig;
  } catch {
    return null;
  }
}

const cfg = readConfig();

/** Raiz data/ do jogo para o app (GameFilesLocation). */
export const gameFiles =
  cfg?.Locations?.GameFilesLocation?.trim() || path.join(BIN_DIR, 'data');

/** Raiz de textos traduzidos (reimport em lote). */
export const translateDir =
  cfg?.Locations?.TranslateLocation?.trim() ||
  path.join(gameFiles, 'mods', 'translated');

/** Executável do jogo configurado ('' = sem raízes .vbf na sidebar). */
export const gameExe = cfg?.GameExeLocation?.trim() ?? '';

/** Overlay mods/ — destino do Salvar e fonte da coluna Traduzido. */
export const modsDir = path.join(gameFiles, 'mods');

/** Árvore de originais da aba FFX (ffx_data) — exibição de conteúdo. */
export const hasData = fs.existsSync(path.join(gameFiles, 'ffx_data'));

/** Overlay mods/ presente no disco. */
export const hasMods = fs.existsSync(modsDir);

/** Texturas (.dds.phyre) extraídas em data/ — painel de imagem de data/. */
export const hasImages = hasData && findFirstPhyre(path.join(gameFiles, 'ffx_data')) !== null;

function findFirstPhyre(root: string): string | null {
  const walk = (dir: string): string | null => {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return null;
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        const hit = walk(full);
        if (hit) return hit;
      } else if (/\.dds\.phyre$/i.test(entry.name)) {
        return full;
      }
    }
    return null;
  };
  return walk(root);
}

/**
 * Containers de CONTEÚDO (*.vbf que a sidebar exibe — mesmo filtro do
 * isContentVbf do backend) nas pastas varridas: a do exe e a data/ ao lado.
 */
export const vbfFiles = discoverVbf(gameExe);
export const hasVbf = vbfFiles.length > 0;

function discoverVbf(exe: string): string[] {
  if (!exe) return [];
  const dir = path.dirname(exe);
  const found: string[] = [];
  for (const search of [dir, path.join(dir, 'data')]) {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(search, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (entry.isFile() && /^ffx2?_data\.vbf$/i.test(entry.name)) {
        found.push(path.join(search, entry.name));
      }
    }
  }
  return found;
}

/** Resumo legível para o relatório (beforeAll dos specs). */
export function describeFixtures(): string {
  return [
    `gameFiles=${gameFiles || '(default)'}`,
    `data=${hasData}`,
    `mods=${hasMods}`,
    `vbf=${hasVbf ? vbfFiles.map((p) => path.basename(p)).join(', ') : 'ausente'}`,
  ].join(' ');
}
