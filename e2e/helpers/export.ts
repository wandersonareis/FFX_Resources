import { expect, type Locator, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { dialog, nodeRow } from './ui';
import { modsDir } from './fixtures';

/**
 * Drivers de export — tudo por papel/aria/rótulo (a UI é pt-BR). Nada de
 * lógica de negócio aqui: os specs só orquestram.
 *
 * Os três caminhos de export testados:
 *   - popover Exportar → "todos os tipos": kinds da versão, inteiros;
 *   - popover Exportar → "Exportar seleção": só os tipos marcados no
 *     ESCOPO (a seleção da árvore não entra aqui);
 *   - menu de contexto da árvore → Exportar: o que está MARCADO na árvore
 *     (parcial = um arquivo por entrada; kind inteiro = artefato bulk
 *     único com prefixo tree_).
 */

/** Destino de TODO export de texto: <game>/mods/edits. */
export const editsDir = path.join(modsDir, 'edits');

/** Rótulos exatos dos tipos no grupo Escopo (KIND_LABELS do frontend). */
export const SCOPE_LABELS = {
  events: 'Eventos',
  objects: 'Sistema',
  macro: 'Dicionário',
  lockit: 'Loc Kit',
  help: 'Painel de ajuda',
  battletext: 'Textos de batalha',
  cloud: 'Cloud',
  tutorial: 'Tutorial',
  menumain: 'Texto menu principal',
} as const;

export type ScopeKind = keyof typeof SCOPE_LABELS;

/** Prefixo do artefato bulk gerado pela seleção completa de um kind. */
export const TREE_PREFIX = 'tree_';

/** Artefatos em edits/ (basenames), ordenados. */
export function listArtifacts(): string[] {
  try {
    return fs.readdirSync(editsDir).sort();
  } catch {
    return [];
  }
}

/** Mapa name → mtimeMs de tudo em edits/ (vazio se a pasta não existe). */
export function snapshotArtifacts(): Map<string, number> {
  const out = new Map<string, number>();
  for (const name of listArtifacts()) {
    out.set(name, fs.statSync(path.join(editsDir, name)).mtimeMs);
  }
  return out;
}

/**
 * Artefatos CRIADOS ou REGRAVADOS desde o snapshot.
 *
 * Por mtime, não por conjunto de nomes: reexportar o mesmo kind regrava o
 * MESMO arquivo (nome canônico) — um diff por nome não veria nada e o
 * teste passaria por acaso.
 */
export function changedArtifacts(before: Map<string, number>): string[] {
  const out: string[] = [];
  for (const [name, mtime] of snapshotArtifacts()) {
    const prev = before.get(name);
    if (prev === undefined || mtime > prev) out.push(name);
  }
  return out.sort();
}

/** Só os artefatos tree_* entre os criados/regravados desde o snapshot. */
export function changedTreeArtifacts(before: Map<string, number>): string[] {
  return changedArtifacts(before).filter((name) => name.startsWith(TREE_PREFIX));
}

/**
 * true quando o artefato NÃO foi criado nem regravado pelo teste. Se ele
 * nem existia antes, precisa continuar não existindo.
 */
export function artifactUntouched(
  name: string,
  before: Map<string, number>
): boolean {
  const now = snapshotArtifacts().get(name);
  const prev = before.get(name);
  if (now === undefined) return prev === undefined;
  return prev !== undefined && now === prev;
}

/** Lê um artefato de edits/ como texto. */
export function readArtifact(name: string): string {
  return fs.readFileSync(path.join(editsDir, name), 'utf8');
}

/**
 * IDs de entrada de um artefato JSON (as chaves de topo da Collection).
 * Serve para comparar universos sem decodificar o binário do jogo.
 */
export function artifactEntryIds(name: string): string[] {
  const parsed = JSON.parse(readArtifact(name)) as Record<string, unknown>;
  return Object.keys(parsed).sort();
}

/** Abre o dropdown Exportar do header (primeiro botão "Exportar"). */
export async function openExportMenu(page: Page): Promise<Locator> {
  await page.getByRole('button', { name: /^Exportar/ }).first().click();
  const menu = page.getByRole('menu');
  await expect(menu).toBeVisible();
  return menu;
}

/** Item "Exportar / todos os tipos". */
export async function exportAllTypes(page: Page): Promise<void> {
  const menu = await openExportMenu(page);
  const item = menu.getByRole('menuitem').filter({ hasText: 'todos os tipos' });
  await expect(item).toBeVisible();
  await item.click();
}

/**
 * Item "Exportar seleção" do popover — exporta os tipos marcados no
 * ESCOPO, cada um por inteiro. A seleção da árvore NÃO participa.
 */
export async function exportScopeSelection(page: Page): Promise<void> {
  const menu = await openExportMenu(page);
  const item = menu.getByRole('menuitem').filter({ hasText: 'Exportar seleção' });
  await expect(item).toBeVisible();
  await item.click();
}

/**
 * Deixa marcados no grupo Escopo só os kinds dados (desmarca o resto) e
 * FECHA o menu no fim. Fechar é obrigatório: com o dropdown aberto o Radix
 * marca o resto da página como aria-hidden e o botão do header deixa de
 * existir para getByRole (o próximo menu não abriria).
 *
 * O estado dos checkboxes sobrevive ao fechar — vive no React state do
 * componente, não no menu.
 */
export async function setScope(page: Page, keep: ScopeKind[]): Promise<void> {
  const menu = await openExportMenu(page);
  const keepLabels = new Set<string>(keep.map((k) => SCOPE_LABELS[k]));
  // Alinha cada box ao alvo: clica só quando o estado atual diverge.
  const boxes = menu.getByRole('menuitemcheckbox');
  const count = await boxes.count();
  for (let i = 0; i < count; i++) {
    const box = boxes.nth(i);
    const label = ((await box.textContent()) ?? '').trim();
    const wanted = keepLabels.has(label);
    const isChecked = (await box.getAttribute('data-state')) === 'checked';
    if (wanted !== isChecked) await box.click();
  }
  await page.keyboard.press('Escape');
  await expect(menu).toBeHidden();
}

/** Marca o formato no seletor do header (JSON | Strings | CSV | XLSX). */
export async function setExportFormat(page: Page, label: string): Promise<void> {
  const button = page.getByRole('button', {
    name: /^(JSON|Strings|CSV|XLSX \(Excel\))$/,
  });
  await button.click();
  await page.getByRole('menuitemradio', { name: label }).click();
  await expect(button).toHaveText(label);
}

/** Expande um nó da árvore (chevron) — idempotente. */
export async function expandTreeNode(page: Page, nodeId: string): Promise<void> {
  const row = nodeRow(page, nodeId);
  await expect(row).toBeVisible({ timeout: 60_000 });
  const toggle = row.locator('button[aria-label^="Alternar"]').first();
  await toggle.click({ timeout: 60_000 });
}

/** Checkbox de seleção dentro da row de um nó. */
export function nodeCheckbox(page: Page, nodeId: string): Locator {
  return nodeRow(page, nodeId).getByRole('checkbox');
}

/** Estado tri-state do checkbox de um nó (checked/indeterminate/unchecked). */
export async function checkboxState(
  page: Page,
  nodeId: string
): Promise<'checked' | 'indeterminate' | 'unchecked'> {
  const state = await nodeCheckbox(page, nodeId).getAttribute('data-state');
  return (state ?? 'unchecked') as 'checked' | 'indeterminate' | 'unchecked';
}

/** Marca/desmarca o checkbox da RAIZ de um kind (kind inteiro). */
export async function checkKindRoot(
  page: Page,
  kind: string,
  checked = true
): Promise<void> {
  const box = nodeCheckbox(page, `kind:${kind}`);
  await expect(box).toBeVisible({ timeout: 60_000 });
  const current = (await box.getAttribute('data-state')) === 'checked';
  if (current !== checked) await box.click();
}

/** Primeira folha de um kind de TABELA (filhos diretos da raiz). */
export function firstLeafOfKind(page: Page, kind: string): Locator {
  return page.locator(`[data-node-id^="leaf:${kind}:"]`).first();
}

/** ID da primeira folha de um kind (leaf:<kind>:<id> → <id>). */
export async function firstLeafId(page: Page, kind: string): Promise<string> {
  const leaf = firstLeafOfKind(page, kind);
  await expect(leaf).toBeVisible({ timeout: 60_000 });
  const nodeId = (await leaf.getAttribute('data-node-id')) ?? '';
  return nodeId.slice(`leaf:${kind}:`.length);
}

/** Número de folhas visíveis de um kind (precisa da raiz expandida). */
export async function countLeavesOfKind(page: Page, kind: string): Promise<number> {
  return page.locator(`[data-node-id^="leaf:${kind}:"]`).count();
}

/**
 * IDs de TODAS as folhas visíveis de um kind (o universo que a árvore
 * serve). Precisa da raiz expandida.
 */
export async function allLeafIdsOfKind(page: Page, kind: string): Promise<string[]> {
  const ids = await page
    .locator(`[data-node-id^="leaf:${kind}:"]`)
    .evaluateAll((nodes) =>
      nodes.map((n) => n.getAttribute('data-node-id') ?? '')
    );
  const prefix = `leaf:${kind}:`;
  return ids.map((id) => id.slice(prefix.length)).sort();
}

/**
 * Menu de contexto da árvore → Exportar (a exportação da seleção marcada
 * no kind do nó clicado).
 */
export async function contextMenuExport(page: Page, nodeId: string): Promise<void> {
  await nodeRow(page, nodeId).click({ button: 'right' });
  const menu = page.getByRole('menu');
  await expect(menu).toBeVisible();
  await menu.getByRole('menuitem', { name: 'Exportar', exact: true }).click();
}

/**
 * Espera o modal de progresso do export concluir e o fecha. O batch é
 * dirigido por eventos do backend: o modal abre, mostra os steps e termina
 * com "Processamento concluído." + resumo de arquivos.
 */
export async function waitExportDone(
  page: Page,
  timeoutMs = 300_000
): Promise<void> {
  const progress = dialog(page);
  await expect(progress).toBeVisible({ timeout: 30_000 });
  await expect(progress.getByText('Processamento concluído.')).toBeVisible({
    timeout: timeoutMs,
  });
  await expect(progress.getByText(/arquivo\(s\)/)).toBeVisible();
  await progress.getByRole('button', { name: 'Fechar' }).click();
  await expect(progress).toBeHidden();
}
