import {
  ExportArtifactBatch,
  ExportEntry,
  GetTextEntry,
  ImportEntry,
  ImportFile,
  ImportTranslated,
  ListTextEntries,
  PreviewImport,
  PreviewImportTranslated,
  RevealEntryFile,
  SearchTextEntries,
  SelectImportFile,
  WarmTextSearch,
} from '@/bindings/ffxresources/textservice';
import {
  DeleteImages,
  DeleteImageSelection,
  ExtractImageGroup,
  ExtractImageSelection,
  GetImageEntry,
  ImageDuplicates,
  ImageSelectionCopies,
  ImageExists,
  ImportImage,
  ImportImageGroup,
  ImportImages,
  PreviewImportImages,
  RefreshImageDuplicates,
  ReplicateImage,
  SaveImage,
  SelectImageFile,
  SelectImageSavePath,
} from '@/bindings/ffxresources/imageservice';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import * as services from '@/bindings/ffxresources/backend/services/models';
import { resolveEntryLabel, EntryKind } from './display-names';
import type { GameVersionId } from './game-version';
import type { ExportFormat } from './export-selection';

export interface EntryRow {
  kind: EntryKind;
  id: string;
  key: string;
  label: string;
  /**
   * Presente quando a entrada veio do navegador de .vbf (não da árvore
   * data/): `root` é o container absoluto e `path` o caminho interno do
   * arquivo. É o que muda a carga para os bindings VBF e mantém a entrada
   * SOMENTE LEITURA (nada aqui escreve no .vbf).
   */
  vbf?: { root: string; path: string };
}

/**
 * Extrai o código de idioma do caminho do arquivo dentro do container
 * (padrão `new_<code>pc`: "new_jppc" → "jp", "new_sppc" → "sp").
 * Null quando o caminho não tem raiz de localização.
 */
export function locFromVbfPath(path: string): string | null {
  const m = /\/new_([a-z]{2})pc\//.exec(path.replace(/\\/g, '/'));
  return m ? m[1] : null;
}

// Kinds servidos por versão na sidebar. As expansões não têm dicionário
// próprio: eternalcalm (sob o ffx) mostra só events (scene*.bin); lastmiss
// (sob o ffx2) mostra events + objects (kernel lm_*). O lockit (kit de
// localização do menu/launcher) existe em FFX e FFX-2. Os painéis de ajuda
// (.sps2 em help/) são FFX-only: a árvore do ffx2 não tem a pasta help/.
export function entryKindsFor(version: GameVersionId): EntryKind[] {
  // battletext (battle/btl) e cloud (cloudsave) existem em FFX e FFX-2;
  // tutorial (menu/tutorial.msb) é só FFX-2. As expansões dividem a árvore
  // do jogo-pai mas não têm essas folhas próprias.
  if (version === 'eternalcalm') return ['events'];
  if (version === 'lastmiss') return ['events', 'objects'];
  if (version === 'ffx') return ['events', 'objects', 'macro', 'lockit', 'help', 'battletext', 'cloud', 'menumain'];
  return ['events', 'objects', 'macro', 'lockit', 'battletext', 'cloud', 'tutorial'];
}

/**
 * Kinds de IMAGEM (.dds.phyre). Só ffx e ffx2: as expansões (eternalcalm
 * sob o ffx, lastmiss sob o ffx2) dividem a árvore do jogo-pai mas não
 * têm texturas próprias.
 *
 * Fica separado de entryKindsFor de propósito — esse é o conjunto de kinds
 * de TEXTO (escopo do Exportar/JSON, save-all e preload), e imagem não
 * participa de nada disso.
 */
export function imageKindsFor(version: GameVersionId): EntryKind[] {
  if (version === 'lastmiss' || version === 'eternalcalm') return [];
  return ['images'];
}

/** Todos os kinds da árvore da versão (texto + imagem), em ordem canônica. */
export function allKindsFor(version: GameVersionId): EntryKind[] {
  return [...entryKindsFor(version), ...imageKindsFor(version)];
}

/**
 * Fluxo novo (DTO): o backend envia o sumário canônico (id + key) e o DTO
 * estruturado sob demanda. Os artefatos de export/import são JSON + .strings
 * em mods/edits; não há mais árvore de diretórios nem .txt.
 */
export async function loadKindEntries(
  kind: EntryKind,
  version: GameVersionId
): Promise<EntryRow[]> {
  const summaries: services.EntrySummary[] = await ListTextEntries(
    kind,
    version as Parameters<typeof ListTextEntries>[1]
  );
  return (summaries ?? [])
    .map((s) => ({
      kind,
      id: s.id,
      key: s.key,
      label: resolveEntryLabel(kind, s.id),
    }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

/** Busca no texto decodificado de data/ e mods/ (idioma us), sem trazer DTOs. */
export function searchTextEntries(
  version: GameVersionId,
  query: string
): Promise<services.TextSearchResponse> {
  return SearchTextEntries(
    version as Parameters<typeof SearchTextEntries>[0],
    query
  );
}

/**
 * Pré-aquece o índice de busca da versão em background (chamado depois que a
 * árvore carrega): a primeira consulta do modal não paga a construção.
 */
export function warmTextSearch(version: GameVersionId): void {
  WarmTextSearch(version as Parameters<typeof WarmTextSearch>[0]);
}

export async function loadEntry(
  kind: EntryKind,
  id: string,
  version: GameVersionId
): Promise<dto.FileEntry> {
  return GetTextEntry(
    kind,
    id,
    version as Parameters<typeof GetTextEntry>[2]
  );
}

// ---- Imagens (.dds.phyre) ----
// kind=images não é texto: não tem rows, não participa de export/import de
// texto. Cada chamada abaixo espelha um binding do App.

/** Carrega a textura e devolve a pré-visualização (data URL) + metadados. */
export async function loadImage(
  id: string,
  version: GameVersionId
): Promise<dto.ImageEntry> {
  return GetImageEntry(
    'images',
    id,
    version as Parameters<typeof GetImageEntry>[2]
  );
}

/** Reempacota um .dds sobre o container pristine e grava em mods/. */
export function importImage(
  id: string,
  ddsPath: string,
  version: GameVersionId
): Promise<void> {
  return ImportImage(
    'images',
    id,
    ddsPath,
    version as Parameters<typeof ImportImage>[3]
  );
}

/**
 * A textura ainda existe (data/ OU mods/)? Usada na revalidação da seleção
 * após reload: id apagado pelo delete NÃO deve ser re-carregado — o erro
 * "não encontrada em data/ nem em mods/" só faria confusão. Falha de
 * binding = não existe (não serve para exibição).
 */
export async function imageExists(
  id: string,
  version: GameVersionId
): Promise<boolean> {
  try {
    return await ImageExists('images', id, version);
  } catch {
    return false;
  }
}

/**
 * Reempacota o MESMO .dds na textura e nas cópias idênticas dela (payload
 * original igual — as réplicas da otimização do DVD). O backend valida o
 * lote antes de gravar: alvo fora do grupo é recusado inteiro.
 */
export function importImageGroup(
  id: string,
  ddsPath: string,
  targets: string[],
  version: GameVersionId
): Promise<dto.ImageImportResult> {
  return ImportImageGroup(
    'images',
    id,
    ddsPath,
    targets,
    version as Parameters<typeof ImportImageGroup>[4]
  );
}

/**
 * Varre <TranslateLocation>/images e resume o lote SEM gravar: devolve os
 * diretórios após ps3data (events, map…) e quantos candidatos são NOVOS
 * (diferentes do container já em mods/).
 */
export function previewImportImages(
  version: GameVersionId,
  subdirs: string[]
): Promise<dto.ImageBatch> {
  return PreviewImportImages(
    version as Parameters<typeof PreviewImportImages>[0],
    subdirs
  );
}

/**
 * Grava sobre mods/ apenas os .dds.phyre diferentes dos já presentes —
 * idênticos são pulados.
 */
export function importImages(
  version: GameVersionId,
  subdirs: string[]
): Promise<dto.ImageBatch> {
  return ImportImages(
    version as Parameters<typeof ImportImages>[0],
    subdirs
  );
}

/**
 * Descarta o índice de duplicatas em cache (rebuild na próxima abertura da
 * textura) — existe para edição EXTERNA em hex editor, que o cache não vê.
 */
export function refreshImageDuplicates(version: GameVersionId): Promise<void> {
  return RefreshImageDuplicates(
    'images',
    version as Parameters<typeof RefreshImageDuplicates>[1]
  );
}

/**
 * Cópias da textura SEM carregar a imagem (só o índice memoizado): é o que
 * o menu de contexto e os diálogos de extrair/replicar/deletar pedem para
 * agir sobre o NÓ CLICADO, que pode não ser a entry selecionada.
 */
export function imageDuplicates(
  id: string,
  version: GameVersionId
): Promise<dto.ImageDuplicates> {
  return ImageDuplicates(
    'images',
    id,
    version as Parameters<typeof ImageDuplicates>[2]
  );
}

/** União de cópias adicionais dos ids selecionados, sem carregar imagens. */
export function imageSelectionCopies(
  ids: string[],
  version: GameVersionId
): Promise<string[]> {
  return ImageSelectionCopies(
    'images',
    ids,
    version as Parameters<typeof ImageSelectionCopies>[2]
  );
}

/** Extrai .dds + .png desta textura e das cópias escolhidas (lote). */
export function extractImageGroup(
  id: string,
  targets: string[],
  version: GameVersionId
): Promise<dto.BatchResult> {
  return ExtractImageGroup(
    'images',
    id,
    targets,
    version as Parameters<typeof ExtractImageGroup>[3]
  );
}

/** Extrai somente as imagens marcadas, sem incluir cópias não selecionadas. */
export function extractImageSelection(
  ids: string[],
  version: GameVersionId
): Promise<dto.BatchResult> {
  return ExtractImageSelection(
    'images',
    ids,
    version as Parameters<typeof ExtractImageSelection>[2]
  );
}

/**
 * Reempacota a IMAGEM ABERTA em mods/ das cópias escolhidas (o "dupe"): a
 * fonte é o próprio conteúdo servido ao painel, sem diálogo de arquivo.
 */
export function replicateImage(
  id: string,
  targets: string[],
  version: GameVersionId
): Promise<dto.BatchResult> {
  return ReplicateImage(
    'images',
    id,
    targets,
    version as Parameters<typeof ReplicateImage>[3]
  );
}

/**
 * Apaga os containers escolhidos no escopo pedido e SEMPRE os artefatos
 * derivados (.dds/.png extraídos). Escopo vem explícito do diálogo.
 */
export function deleteImages(
  id: string,
  targets: string[],
  scope: 'data' | 'mods' | 'both',
  version: GameVersionId
): Promise<dto.BatchResult> {
  return DeleteImages(
    'images',
    id,
    targets,
    scope,
    version as Parameters<typeof DeleteImages>[4]
  );
}

/** Delete em lote das imagens marcadas; a opção de cópias vale para cada id. */
export function deleteImageSelection(
  ids: string[],
  withCopies: boolean,
  scope: 'data' | 'mods' | 'both',
  version: GameVersionId
): Promise<dto.BatchResult> {
  return DeleteImageSelection(
    'images',
    ids,
    withCopies,
    scope,
    version as Parameters<typeof DeleteImageSelection>[4]
  );
}

/**
 * "Abrir até o arquivo": abre o explorador já com o arquivo da entrada
 * selecionado, resolvido mods-first (o arquivo que está valendo).
 */
export function revealEntry(
  kind: EntryKind,
  id: string,
  version: GameVersionId
): Promise<void> {
  return RevealEntryFile(
    kind,
    id,
    version as Parameters<typeof RevealEntryFile>[2]
  );
}

/** Salva .dds ou .png no caminho escolhido pelo usuário. */
export function saveImage(
  id: string,
  format: 'dds' | 'png',
  destPath: string,
  version: GameVersionId
): Promise<void> {
  return SaveImage(
    'images',
    id,
    format,
    destPath,
    version as Parameters<typeof SaveImage>[4]
  );
}

/** Seletor nativo para escolher um .dds a importar (abre em mods/images/<raiz>/<dir do id>). */
export function selectImageFile(id: string, version: GameVersionId): Promise<string> {
  return SelectImageFile(
    id,
    version as Parameters<typeof SelectImageFile>[1]
  );
}

/** Diálogo "Salvar como" para .dds/.png, já no diretório de trabalho da textura. */
export function selectImageSavePath(
  format: 'dds' | 'png',
  suggestedName: string,
  id: string,
  version: GameVersionId
): Promise<string> {
  return SelectImageSavePath(
    format,
    suggestedName,
    id,
    version as Parameters<typeof SelectImageSavePath>[3]
  );
}

/** Exporta a entrada em JSON + .strings (mods/edits). */
export async function exportEntry(
  kind: EntryKind,
  id: string,
  version: GameVersionId,
  langs: string[] = []
): Promise<string[]> {
  return ExportEntry(
    kind,
    id,
    version as Parameters<typeof ExportEntry>[2],
    langs
  );
}

/** Lê o artefato JSON padrão em mods/edits e aplica no binário. */
export async function importEntry(
  kind: EntryKind,
  id: string,
  version: GameVersionId
): Promise<string[]> {
  return ImportEntry(
    kind,
    id,
    version as Parameters<typeof ImportEntry>[2]
  );
}

/** Exporta o lote como UMA operação agrupada: um item {kind, ids} por ação
 * (ids vazio = tudo do kind), no formato pedido (json | strings | csv |
 * xlsx). O modal de progresso (steps por kind, barra própria por ação) é
 * dirigido pelos eventos do backend — o caller não mexe em toast.
 *
 * `complete: true` marca a seleção TOTAL do kind (checkbox da raiz na
 * árvore): o backend devolve o artefato bulk ÚNICO com prefixo `tree_`,
 * em vez de um arquivo por entrada. */
export async function exportArtifactBatch(
  items: { kind: EntryKind; ids: string[]; complete?: boolean }[],
  version: GameVersionId,
  langs: string[] = [],
  format: ExportFormat
): Promise<string[]> {
  return ExportArtifactBatch(
    items.map((it) => ({
      kind: it.kind,
      ids: it.ids,
      complete: it.complete ?? false,
    })),
    version as Parameters<typeof ExportArtifactBatch>[1],
    langs,
    format
  );
}

/**
 * Resumo do destino do export para o toast: os artefatos saem em mods/edits
 * (todos os kinds), então mostra a contagem + diretório (ou o caminho único).
 */
export function exportDestination(paths: string[]): string {
  const list = paths ?? [];
  if (list.length === 0) return '';
  if (list.length === 1) return list[0];
  const dir = list[0].replace(/[\\/][^\\/]*$/, '');
  return `${list.length} arquivos em ${dir}`;
}

/** Seletor nativo de arquivo (.json/.strings/.csv/.xlsx). "" = cancelado. */
export function selectImportFile(): Promise<string> {
  return SelectImportFile();
}

/** Valida o arquivo e devolve o resumo do modal (sem aplicar nada). */
export async function previewImport(
  path: string,
  version: GameVersionId
): Promise<dto.ImportSummary> {
  return PreviewImport(path, version as Parameters<typeof PreviewImport>[1]);
}

/** Aplica o arquivo validado; devolve quantos textos 'us' mudaram. */
export async function importFile(
  path: string,
  version: GameVersionId
): Promise<number> {
  return ImportFile(path, version as Parameters<typeof ImportFile>[1]);
}

/**
 * Varre <TranslateLocation>/text e resume o lote SEM aplicar — alimenta o
 * diálogo de confirmação do Importar em lote.
 */
export async function previewImportTranslated(
  version: GameVersionId,
  kinds: EntryKind[]
): Promise<dto.ImportBatch> {
  return PreviewImportTranslated(
    version as Parameters<typeof PreviewImportTranslated>[0],
    kinds
  );
}

/**
 * Aplica o lote de <TranslateLocation>/text, arquivo a arquivo; devolve o
 * resumo agregado (arquivos, entradas, textos alterados e falhas).
 */
export async function importTranslated(
  version: GameVersionId,
  kinds: EntryKind[]
): Promise<dto.ImportBatch> {
  return ImportTranslated(
    version as Parameters<typeof ImportTranslated>[0],
    kinds
  );
}