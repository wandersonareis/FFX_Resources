import * as fs from 'node:fs';
import * as path from 'node:path';

/**
 * Backup/restauração de árvores do fixture (mods/, config) e varredura por
 * mudanças — os testes de escrita PRECISAM gravar no fixture real (é isso
 * que validam), mas o deixam exatamente como encontraram:
 *
 *  - backup antes, restauração depois (cópia byte a byte);
 *  - quem NÃO existia volta a não existir (o teste de escrita com mods/
 *    ausente cria mods/ na hora e a limpeza remove o que criou);
 *  - a varredura por mtime prova que o Salvar gravou em mods/ sem depender
 *    de decodificar os binários do jogo.
 *
 * A restauração é POR ARQUIVO com retentativas: o servidor do teste mantém
 * handles abertos no fixture (e o Windows não deleta/substitui arquivo
 * aberto), então um "remove a pasta toda e copia de volta" pode falhar no
 * MEIO e deixar o fixture pela metade — o pior resultado possível. Aqui
 * cada arquivo é restaurado individualmente; o que não conseguir (handle
 * preso) entra na lista de falhas e o erro chega COMPLETO no fim, sem
 * estado intermediário silencioso.
 */

const TMP_ROOT = path.resolve(__dirname, '..', '.tmp');

/** Retentativa para operações de arquivo que esbarram em handle aberto. */
export async function withRetry<T>(fn: () => T, attempts = 8, delayMs = 400): Promise<T> {
  let lastError: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return fn();
    } catch (error) {
      lastError = error;
      if (i < attempts - 1) {
        // Sleep síncrono simples: as retentativas vivem no afterAll do spec.
        Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, delayMs);
      }
    }
  }
  throw lastError;
}

export interface TreeBackup {
  /** O alvo existia antes do teste? */
  existed: boolean;
  /** Caminho original (restaurado no fim). */
  target: string;
  /** Cópia guardada (vazia quando existed=false). */
  dir: string;
}

/** Copia a árvore para .tmp/<name> (idempotente: limpa cópias antigas). */
export function backupTree(target: string, name: string): TreeBackup {
  const existed = fs.existsSync(target);
  const dir = path.join(TMP_ROOT, name);
  fs.rmSync(dir, { recursive: true, force: true });
  if (existed) {
    fs.mkdirSync(path.dirname(dir), { recursive: true });
    fs.cpSync(target, dir, { recursive: true, force: true });
  }
  return { existed, target, dir };
}

function listFiles(root: string): Map<string, number> {
  const out = new Map<string, number>();
  if (!fs.existsSync(root)) return out;
  const walk = (dir: string, prefix: string): void => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full, rel);
      else if (entry.isFile()) out.set(rel, fs.statSync(full).mtimeMs);
    }
  };
  walk(root, '');
  return out;
}

/** Mapa relPath → mtimeMs de tudo sob root (vazio se root não existe). */
export function snapshotMtimes(root: string): Map<string, number> {
  return listFiles(root);
}

/** Arquivos criados ou regravados desde o snapshot (mtime mudou). */
export function changedSince(root: string, before: Map<string, number>): string[] {
  const now = listFiles(root);
  const changed: string[] = [];
  for (const [rel, mtime] of now) {
    const prev = before.get(rel);
    if (prev === undefined || mtime > prev) changed.push(rel);
  }
  return changed;
}

/**
 * Restaura o alvo contra o backup, arquivo a arquivo:
 *  1. devolve o conteúdo de cada arquivo do backup (sobrescreve);
 *  2. apaga os arquivos que NÃO existiam no backup (sujidade do teste);
 *  3. poda os diretórios vazios (uma árvore que não existia volta a sumir).
 *
 * Falhas individuais (handle preso no Windows) são retentadas e, se
 * persistirem, reportadas JUNTAS no fim — nunca interrompem no meio.
 */
export async function restoreTree(backup: TreeBackup): Promise<void> {
  const failed: string[] = [];
  if (!backup.existed) {
    // Árvore que não existia: cada arquivo atual é sujeira do teste.
    for (const rel of listFiles(backup.target).keys()) {
      try {
        await withRetry(() => fs.rmSync(path.join(backup.target, rel), { force: true }));
      } catch {
        failed.push(rel);
      }
    }
  } else {
    // 1. devolve cada arquivo do backup.
    for (const rel of listFiles(backup.dir).keys()) {
      try {
        const to = path.join(backup.target, rel);
        fs.mkdirSync(path.dirname(to), { recursive: true });
        await withRetry(() => fs.copyFileSync(path.join(backup.dir, rel), to));
      } catch {
        failed.push(rel);
      }
    }
    // 2. apaga o que o teste criou e não estava no backup.
    const original = listFiles(backup.dir);
    for (const rel of listFiles(backup.target).keys()) {
      if (!original.has(rel)) {
        try {
          await withRetry(() => fs.rmSync(path.join(backup.target, rel), { force: true }));
        } catch {
          failed.push(rel);
        }
      }
    }
  }
  // 3. poda diretórios vazios (o próprio alvo some se voltou a não existir).
  pruneEmptyDirs(backup.target);
  if (failed.length > 0) {
    throw new Error(
      `restoreTree: ${failed.length} arquivo(s) não restaurado(s) em ${backup.target} ` +
        `(handle aberto?):\n  ${failed.slice(0, 20).join('\n  ')}`
    );
  }
}

/** Remove subdiretórios vazios sob root (e o próprio root se ficar vazio). */
function pruneEmptyDirs(root: string): void {
  if (!fs.existsSync(root)) return;
  const walk = (dir: string): boolean => {
    let allGone = true;
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (walk(full)) fs.rmdirSync(full);
        else allGone = false;
      } else {
        allGone = false;
      }
    }
    return allGone;
  };
  if (walk(root)) fs.rmdirSync(root);
}

/** Arquivo único de apoio (config.json) — backup/restauração diretos. */
export interface FileBackup {
  existed: boolean;
  target: string;
  copy: string;
}

export function backupFile(target: string, name: string): FileBackup {
  const existed = fs.existsSync(target);
  fs.mkdirSync(TMP_ROOT, { recursive: true });
  const copy = path.join(TMP_ROOT, `${name}.bak`);
  if (existed) fs.copyFileSync(target, copy);
  return { existed, target, copy };
}

export function restoreFile(backup: FileBackup): void {
  if (!backup.existed) return;
  // Só devolve se divergiu (o app persistiu algo diferente no meio) —
  // evitar reescritas desnecessárias enquanto o processo está vivo.
  const current = fs.existsSync(backup.target)
    ? fs.readFileSync(backup.target)
    : null;
  const original = fs.readFileSync(backup.copy);
  if (!current || !current.equals(original)) {
    fs.copyFileSync(backup.copy, backup.target);
  }
}
