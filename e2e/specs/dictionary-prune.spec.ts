import { expect, test } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { dialog, expectDialogWithinViewport, gotoApp, switchToTab } from '../helpers/ui';
import { gameFiles, hasData, hasMods, modsDir } from '../helpers/fixtures';
import { backupTree, restoreTree, type TreeBackup } from '../helpers/backup';

/**
 * Dicionário: o save PODA as linhas que só existem em mods (sem par no
 * original) — o caso do macrodic traduzido com ponteiros "simplified"
 * apontando para dentro do standard. O aviso destrutivo lista TODAS as
 * linhas antes de continuar; a lista rola dentro do modal.
 *
 * O fixture mods/ é copiado antes e restaurado depois (o save grava de
 * verdade o macrodic em mods/).
 */
const MACRO_PATH = path.join(modsDir, 'ffx_ps2/ffx2/master/new_uspc/menu/macrodic.dcp');
const DATA_MACRO_PATH = path.join(gameFiles, 'ffx_ps2/ffx2/master/new_uspc/menu/macrodic.dcp');

/** Ponteiros "simplified" distintos do arquivo inteiro (0 = tudo compartilhado). */
function countDistinctSimplified(file: string): number {
  const b = fs.readFileSync(file);
  const offs = Array.from({ length: 16 }, (_, i) => b.readUInt32LE(i * 4));
  let total = 0;
  for (let c = 0; c < 16; c++) {
    const from = offs[c];
    if (!from) continue;
    let to = b.length;
    for (let j = c + 1; j < 16; j++) {
      if (offs[j] && offs[j] >= from) {
        to = offs[j];
        break;
      }
    }
    if (to - from < 2) continue;
    const count = b.readUInt16LE(from) / 4;
    for (let i = 0; i < count; i++) {
      const nameOff = b.readUInt16LE(from + i * 4);
      const simpOff = b.readUInt16LE(from + i * 4 + 2);
      if (simpOff !== nameOff) total++;
    }
  }
  return total;
}

/** Último log do app (o diagnóstico da poda é gravado nele). */
function newestLog(): string {
  const dir = path.resolve(__dirname, '..', '..', 'build', 'bin', 'logs');
  const files = fs
    .readdirSync(dir)
    .filter((f) => f.endsWith('.log'))
    .map((f) => ({ f, m: fs.statSync(path.join(dir, f)).mtimeMs }))
    .sort((a, b) => b.m - a.m);
  return path.join(dir, files[0].f);
}

async function openMacroChunk(page: import('@playwright/test').Page, label: string) {
  const root = page.locator('[data-node-id="kind:macro"]');
  await expect(root).toBeVisible({ timeout: 60_000 });
  // Mesmo padrão do toggleNode do helper: espera o chevron estabilizar.
  await root.locator('button[aria-label^="Alternar"]').first().click({ timeout: 60_000 });
  // Qualquer folha do dicionário já prova que o kind expandiu.
  await expect(page.locator('[data-node-entry="true"]').first()).toBeVisible({ timeout: 60_000 });
  const leaf = page.locator(`[data-node-entry="true"][data-node-label="${label}"]`);
  await expect(leaf).toBeVisible({ timeout: 60_000 });
  await leaf.locator('[data-node-button]').click();
  await expect(page.locator('[data-row-key]').first()).toBeVisible({ timeout: 60_000 });
}

test.describe('Dicionário: poda no save das linhas sem par no original', () => {
  test.skip(!hasData || !hasMods, 'data/ ou mods/ ausente — sem dicionário para podar');

  let modsBackup: TreeBackup | null = null;

  test.beforeAll(() => {
    modsBackup = backupTree(modsDir, 'mods');
  });

  test.afterAll(async () => {
    if (modsBackup) await restoreTree(modsBackup);
  });

  test('avisa, lista (com rolagem) e exclui as órfãs ao salvar', async ({ page }) => {
    const before = countDistinctSimplified(MACRO_PATH);
    expect(before, 'fixture com órfãs para podar').toBeGreaterThan(0);
    const dataBefore = fs.existsSync(DATA_MACRO_PATH) ? fs.readFileSync(DATA_MACRO_PATH) : null;

    await gotoApp(page);
    await switchToTab(page, 'ffx2');
    await openMacroChunk(page, 'Bloco 07');

    // Edição qualquer só para habilitar o Salvar (o dicionário reconstrói o
    // arquivo inteiro — a poda roda em todos os blocos).
    const firstCell = page.locator('[data-row-key] .translated-cell').first();
    await firstCell.click();
    const editorDialog = dialog(page);
    await expect(editorDialog.getByText(/Linha #\d+/)).toBeVisible();
    await editorDialog.locator('.tiptap').click();
    await page.keyboard.type('Z', { delay: 20 });
    await editorDialog.getByRole('button', { name: 'Salvar', exact: true }).click();
    await expect(editorDialog).toBeHidden();

    // Save pelo atalho: o aviso destrutivo abre ANTES de gravar.
    await page.keyboard.press('Control+s');
    const prune = page.locator('#prune-dialog');
    await expect(prune).toBeVisible({ timeout: 60_000 });
    await expect(prune.getByText(/\d+ linha\(s\) sem par no original serão excluídas/)).toBeVisible();
    // Lista completa com rolagem própria (não estoura o modal).
    const list = prune.getByTestId('prune-list');
    await expect(list).toBeVisible();
    const scrollable = await list.evaluate((el) => el.scrollHeight > el.clientHeight);
    expect(scrollable, 'lista de linhas com rolagem').toBe(true);
    await expectDialogWithinViewport(page);
    await expect(prune.getByTestId('prune-text').first()).toBeVisible();

    // Cancelar não grava nada.
    await prune.getByRole('button', { name: 'Cancelar' }).click();
    await expect(prune).toBeHidden();
    expect(countDistinctSimplified(MACRO_PATH), 'cancelar não altera o binário').toBe(before);

    // Agora confirma: o save poda e grava.
    await page.keyboard.press('Control+s');
    await expect(prune).toBeVisible({ timeout: 60_000 });
    const confirm = prune.getByRole('button', { name: /Excluir \d+ e salvar/ });
    await confirm.click();
    await expect(page.getByText('Alterações salvas no jogo.')).toBeVisible({ timeout: 120_000 });

    // Binário: nenhum ponteiro simplified distinto sobrou; data/ intacto.
    expect(countDistinctSimplified(MACRO_PATH), 'órfãs podadas do mods/').toBe(0);
    if (dataBefore) {
      expect(fs.readFileSync(DATA_MACRO_PATH).equals(dataBefore), 'data/ intocado').toBe(true);
    }

    // Arquivo de diagnóstico: a poda fica registrada com o texto completo.
    const log = fs.readFileSync(newestLog(), 'utf8');
    expect(log).toContain('save-poda/');
    expect(log).toContain('rows_excluidas');

    // Recarga: o bloco 07 volta ao original (as linhas em branco do bloco
    // são filtradas da tabela; o lixo "anarkand Abes" tem de sumir — match
    // EXATO: "Zanarkand Abes" contém o texto procurado como substring).
    await page.reload();
    await switchToTab(page, 'ffx2');
    await openMacroChunk(page, 'Bloco 07');
    await expect(page.getByText('anarkand Abes', { exact: true })).toHaveCount(0, {
      timeout: 60_000,
    });
    await expect(page.locator('[data-row-key]')).toHaveCount(109, { timeout: 60_000 });
  });
});
