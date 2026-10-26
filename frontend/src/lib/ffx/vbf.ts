import {
  ExportVbfSelection,
  ExtractVbfImagesSelection,
  ExtractVbfSelection,
  GetVbfImageEntry,
  GetVbfTextEntry,
  ImportVbfImage,
  ListVbfDir,
  ListVbfMacroChunks,
  ListVbfRoots,
  PreviewVbfExtraction,
  ReplicateVbfImage,
  SelectVbfExtractDir,
  SelectVbfImageExtractDir,
  SaveVbfImage,
  VbfImageExportPath,
} from '@/bindings/ffxresources/vbfservice';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import type { ExportFormat } from './export-selection';

/**
 * Navegador de container .vbf (SOMENTE LEITURA).
 *
 * O backend nunca lê o container inteiro e nunca escreve nele: a árvore sai
 * do índice (cabeçalho) e só o arquivo clicado é decodificado, em memória,
 * dentro de um escopo que some no fim da chamada. Aqui fica a camada de
 * acesso + um cache LRU para reabrir o que já foi decodificado sem pagar o
 * decode de novo (trocar de nó na árvore é a interação mais frequente).
 */

/** Raiz descoberta perto do executável do jogo. */
export type VbfRoot = dto.VbfRoot;

/** Filho de um diretório do .vbf — ou chunk virtual do macrodic. */
export type VbfNode = dto.VbfNode;

/**
 * Cache LRU por chave: mantém a ordem de acesso e derruba o mais antigo.
 * Só guarda o que já foi pedido — nada aqui é pré-extraído.
 */
class Lru<T> {
  private readonly map = new Map<string, T>();

  constructor(private readonly capacity: number) {}

  get(key: string): T | undefined {
    const value = this.map.get(key);
    if (value === undefined) return undefined;
    // Toca para mover para o fim (mais recente).
    this.map.delete(key);
    this.map.set(key, value);
    return value;
  }

  set(key: string, value: T): void {
    this.map.delete(key);
    this.map.set(key, value);
    while (this.map.size > this.capacity) {
      const oldest = this.map.keys().next();
      if (oldest.done) break;
      this.map.delete(oldest.value);
    }
  }

  clear(): void {
    this.map.clear();
  }

  delete(key: string): void {
    this.map.delete(key);
  }
}

const dirCache = new Lru<VbfNode[]>(64);
const entryCache = new Lru<dto.FileEntry>(24);
const imageCache = new Lru<dto.ImageEntry>(12);
let rootsPromise: Promise<VbfRoot[]> | null = null;

/**
 * Raízes .vbf (uma promessa compartilhada: todas as abas veem a mesma
 * lista e o backend só varre a pasta do executável uma vez).
 */
export function loadVbfRoots(): Promise<VbfRoot[]> {
  rootsPromise ??= ListVbfRoots().catch((error) => {
    rootsPromise = null;
    throw error;
  });
  return rootsPromise;
}

/** Filhos imediatos de um diretório do .vbf (diretórios primeiro). */
export function loadVbfDir(root: string, dir: string): Promise<VbfNode[]> {
  const key = `${root}|${dir.toLowerCase()}`;
  const cached = dirCache.get(key);
  if (cached) return Promise.resolve(cached);
  return ListVbfDir(root, dir).then((nodes) => {
    dirCache.set(key, nodes ?? []);
    return nodes ?? [];
  });
}

/**
 * Filhos virtuais de macrodic.dcp (chunk_00, chunk_01, …): o dicionário é
 * UM arquivo por localização e o app o trata como grupo.
 */
export function loadVbfMacroChunks(
  root: string,
  macroPath: string
): Promise<VbfNode[]> {
  const key = `macro|${root}|${macroPath.toLowerCase()}`;
  const cached = dirCache.get(key);
  if (cached) return Promise.resolve(cached);
  return ListVbfMacroChunks(root, macroPath).then((nodes) => {
    dirCache.set(key, nodes ?? []);
    return nodes ?? [];
  });
}

/** Decodifica um arquivo de TEXTO do .vbf e devolve a tabela. */
export function loadVbfEntry(
  root: string,
  path: string,
  id: string
): Promise<dto.FileEntry> {
  const key = `${root}|${path.toLowerCase()}|${id}`;
  const cached = entryCache.get(key);
  if (cached) return Promise.resolve(cached);
  return GetVbfTextEntry(root, path, id).then((entry) => {
    entryCache.set(key, entry);
    return entry;
  });
}

/** Decodifica uma textura (.dds.phyre) do .vbf (data URL + metadados). */
export function loadVbfImage(
  root: string,
  path: string
): Promise<dto.ImageEntry> {
  const key = `${root}|${path.toLowerCase()}`;
  const cached = imageCache.get(key);
  if (cached) return Promise.resolve(cached);
  // A imagem recém-aberta pode ter acrescentado novas duplicatas conhecidas
  // ao índice incremental do backend. Não manter previews antigos com listas
  // de cópias obsoletas.
  imageCache.clear();
  return GetVbfImageEntry(root, path).then((image) => {
    imageCache.set(key, image);
    return image;
  });
}

export function invalidateVbfImage(root: string, path: string): void {
  imageCache.delete(`${root}|${path.toLowerCase()}`);
}

export function importVbfImage(root: string, path: string, ddsPath: string): Promise<void> {
  return ImportVbfImage(root, path, ddsPath);
}

export function saveVbfImage(
  root: string,
  path: string,
  format: 'dds' | 'png',
  destPath: string
): Promise<void> {
  return SaveVbfImage(root, path, format, destPath);
}

export function replicateVbfImage(
  root: string,
  sourcePath: string,
  targetPaths: string[]
): Promise<dto.BatchResult> {
  return ReplicateVbfImage(root, sourcePath, targetPaths);
}

/**
 * Descarta tudo: o executável do jogo mudou (outra pasta de containers) ou
 * o usuário pediu reload da árvore — os caminhos antigos não valem mais.
 */
export function invalidateVbfCache(): void {
  dirCache.clear();
  entryCache.clear();
  imageCache.clear();
  rootsPromise = null;
}

// ---- extração/export da SELEÇÃO da árvore do .vbf --------------------------
//
// A seleção é uma lista de caminhos internos (arquivos e diretórios);
// diretório = toda a subárvore (expandida contra o índice no backend).

/** Resumo antes de extrair: arquivos, bytes e quantos já existem no destino. */
export function previewVbfExtraction(
  root: string,
  paths: string[],
  destRoot: string
): Promise<dto.VbfExtractPreview> {
  return PreviewVbfExtraction(root, paths, destRoot);
}

/** Extrai a seleção preservando a estrutura de caminhos em destRoot. */
export function extractVbfSelection(
  root: string,
  paths: string[],
  destRoot: string
): Promise<dto.BatchResult> {
  return ExtractVbfSelection(root, paths, destRoot);
}

/** Extrai DDS/PNG somente das imagens compatíveis nos caminhos selecionados. */
export function extractVbfImagesSelection(
  root: string,
  paths: string[],
  destRoot: string
): Promise<dto.BatchResult> {
  return ExtractVbfImagesSelection(root, paths, destRoot);
}

/** Seletor nativo de pasta do "Extrair para…" (default data/). "" = cancelou. */
export function selectVbfExtractDir(): Promise<string> {
  return SelectVbfExtractDir();
}

/** Seletor nativo do destino customizado para DDS/PNG. */
export function selectVbfImageExtractDir(): Promise<string> {
  return SelectVbfImageExtractDir();
}

/**
 * Exporta os kinds de TEXTO da seleção em JSON, .strings, CSV ou XLSX — mesmo lugar
 * (mods/edits) e convenção do export de data/.
 */
export function exportVbfSelection(
  root: string,
  format: ExportFormat,
  paths: string[],
  langs: string[] = []
): Promise<string[]> {
  return ExportVbfSelection(root, format, paths, langs);
}

/** Caminho padrão de exportação (.dds ou .png) em mods/images para textura do .vbf — export direto sem diálogo. */
export function vbfImageExportPath(
  root: string,
  path: string,
  format: 'dds' | 'png'
): Promise<string> {
  return VbfImageExportPath(root, path, format);
}
