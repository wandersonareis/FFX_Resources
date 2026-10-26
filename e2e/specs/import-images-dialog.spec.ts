import { expect, test, type Locator, type Page } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  gotoApp,
  openFirstImageEntry,
  toast,
} from '../helpers/ui';
import {
  firstPhyreIds,
  hasImages,
  hasTranslatedImages,
  modsDir,
  translatedImagesDir,
} from '../helpers/fixtures';
import {
  backupTree,
  changedSince,
  restoreTree,
  snapshotMtimes,
  type TreeBackup,
} from '../helpers/backup';

/**
 * Modal de importação de imagens traduzidas (mods/translated/images) — o
 * sumário que o usuário avalia ANTES de gravar: uma linha por .dds.phyre
 * encontrado (caminho relativo à raiz, COM extensão, semelhante ao interno
 * do .vbf), checkbox "Todas" em 3 estados, cópias ausentes no collapse da
 * primeira imagem do grupo e aplicação SÓ do que está marcado.
 *
 * Roda contra o app em MODO SERVIDOR (wails3 task test:e2e builda e sobe o
 * spira-server). Quando a máquina NÃO tem translated/images, o spec semeia
 * a pasta COM texturas reais de data/ (uma nova, uma igual ao binário) —
 * assim o fluxo de seleção roda em qualquer fixture; o semeio e os gravados
 * em mods/ são restaurados no fim.
 */

/** Árvore da aba aberta por openFirstImageEntry (default = FFX). */
const TREE = 'ffx_data';

/** Id semeado como "novo" (sem container em mods/) e como "igual ao binário". */
interface Seed {
  nova: { id: string; abs: string };
  igual: { id: string; abs: string };
}

/**
 * Semeia a pasta traduzida: uma textura nova (cópia do original de data/) e
 * uma igual ao binário (o container gravado em mods/ é o mesmo arquivo). Só
 * semeia quando a pasta está vazia/ausente — fixture do usuário intacta.
 */
function seedTranslatedImages(): Seed | null {
  const [nova, igual] = firstPhyreIds(TREE, 2);
  if (!nova || !igual || nova.id === igual.id) return null;

  const dest = path.join(translatedImagesDir, `${nova.id}.dds.phyre`);
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.copyFileSync(nova.abs, dest);

  // "Igual ao binário": mesmo bytes no container de mods/ e no traduzido.
  const modsDest = path.join(modsDir, TREE, `${igual.id}.dds.phyre`);
  fs.mkdirSync(path.dirname(modsDest), { recursive: true });
  fs.copyFileSync(igual.abs, modsDest);
  const destIgual = path.join(translatedImagesDir, `${igual.id}.dds.phyre`);
  fs.mkdirSync(path.dirname(destIgual), { recursive: true });
  fs.copyFileSync(igual.abs, destIgual);

  return { nova, igual };
}

/** Diálogo do modal (escopado pelo título — durante a escrita aparece o
 *  modal de progresso, que também é [role="dialog"]). */
function importImagesDialog(page: Page) {
  return page
    .getByRole('dialog')
    .filter({ hasText: 'Importar imagens traduzidas' });
}

/** Abre o modal pela toolbar do painel (dropdown Importar). */
async function openImportImagesDialog(page: Page) {
  await page.locator('main').getByRole('button', { name: /^Importar/ }).click();
  await page
    .getByRole('menu')
    .getByRole('menuitem', { name: /Importar mods\/translated\/images/ })
    .click();
  const importDialog = importImagesDialog(page);
  await expect(importDialog).toBeVisible({ timeout: 60_000 });
  return importDialog;
}

/** Conta "N nova(s)" exibida no cabeçalho do modal (null se não achou). */
async function novasCount(importDialog: Locator): Promise<number | null> {
  const text = await importDialog
    .getByText(/^\d+ nova\(s\)$/)
    .textContent()
    .catch(() => null);
  if (!text) return null;
  return Number(text.replace(/\D/g, ''));
}

test.describe('Importar imagens traduzidas (modal de seleção)', () => {
  test.skip(!hasImages, 'data/ sem texturas — painel de imagem pulado');

  test('abre com origem, Todas em 3 estados e legenda; cancela sem gravar', async ({
    page,
  }) => {
    await gotoApp(page);
    await openFirstImageEntry(page);
    const before = snapshotMtimes(modsDir);

    const importDialog = await openImportImagesDialog(page);

    // Origem exibida completa (as linhas, não).
    await expect(importDialog.getByText('Origem:')).toBeVisible();
    await expect(importDialog.getByText(translatedImagesDir)).toBeVisible({
      timeout: 15_000,
    });

    // Todas (3 estados) e as contagens.
    await expect(
      importDialog.getByRole('checkbox', { name: 'Marcar todas as imagens' })
    ).toBeVisible();

    // Legenda explica a cor das imagens iguais ao binário (texto exato: o
    // mesmo rótulo aparece na explicação das cópias).
    await expect(
      importDialog.getByText('igual ao binário', { exact: true })
    ).toBeVisible();
    await expect(importDialog.getByText(/regravando o binário/)).toBeVisible();

    // Nada escapa da viewport (listas longas rolam dentro do modal).
    await expectDialogWithinViewport(page);

    // Cancelar fecha e NÃO gravou nada em mods/.
    await importDialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(importDialog).toBeHidden();
    expect(changedSince(modsDir, before), 'cancelar não grava em mods/').toEqual(
      []
    );
    await expectNoHorizontalOverflow(page);
  });

  // ---- seleção, 3 estados e escrita (com semeado quando não há fixture) --
  test.describe('seleção e aplicação', () => {
    let backupMods: TreeBackup | null = null;
    let backupTranslated: TreeBackup | null = null;
    let seed: Seed | null = null;

    test.beforeAll(() => {
      backupMods = backupTree(modsDir, 'mods-import-images');
      backupTranslated = backupTree(translatedImagesDir, 'translated-import-images');
      if (!hasTranslatedImages) seed = seedTranslatedImages();
    });

    test.afterAll(async () => {
      if (backupTranslated) await restoreTree(backupTranslated);
      if (backupMods) await restoreTree(backupMods);
    });

    test('lista plana com caminhos relativos; novas marcadas, iguais desmarcadas', async ({
      page,
    }) => {
      await gotoApp(page);
      await openFirstImageEntry(page);
      const importDialog = await openImportImagesDialog(page);

      const primeiraLinha = importDialog.locator('[data-identical]').first();
      await expect(primeiraLinha).toBeVisible({ timeout: 30_000 });
      // Caminho relativo à raiz COM extensão — nunca o absoluto.
      await expect(primeiraLinha).not.toContainText(translatedImagesDir);
      await expect(primeiraLinha).toContainText('gamedata/ps3data/');
      await expect(primeiraLinha).toContainText('.dds.phyre');

      const novas = await novasCount(importDialog);

      if (seed) {
        // Fixture semeado: a nova nasce marcada, a igual ao binário nasce
        // desmarcada e marcada como tal (a cor cinza vem do data-identical).
        await expect(
          importDialog.getByRole('checkbox', {
            name: `Importar ${seed.nova.id}.dds.phyre`,
          })
        ).toBeChecked();
        const igualBox = importDialog.getByRole('checkbox', {
          name: `Importar ${seed.igual.id}.dds.phyre`,
        });
        await expect(igualBox).toBeChecked({ checked: false });
        await expect(
          importDialog.locator(`[data-identical="true"]`, {
            has: page.getByRole('checkbox', {
              name: `Importar ${seed.igual.id}.dds.phyre`,
            }),
          })
        ).toHaveCount(1);
        expect(novas).toBe(1);
      } else if (novas === 0) {
        // Pasta real só com imagens iguais ao binário: aviso pulsante.
        await expect(
          importDialog.getByText('Nada novo para importar.')
        ).toBeVisible();
      }

      await importDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(importDialog).toBeHidden();
    });

    test('Todas em 3 estados consolida a seleção', async ({ page }) => {
      await gotoApp(page);
      await openFirstImageEntry(page);
      const importDialog = await openImportImagesDialog(page);

      const all = importDialog.getByRole('checkbox', {
        name: 'Marcar todas as imagens',
      });
      const state = await all.getAttribute('aria-checked');
      //Desmarcado/indeterminate → o clique marca TUDO (inclusive as iguais
      // ao binário, que passam a regravar); marcado → desmarca tudo.
      await all.click();
      const depois = await all.getAttribute('aria-checked');
      expect(depois).not.toBe(state);
      if (depois === 'true') {
        const linhas = importDialog.locator('[data-identical] [role="checkbox"]');
        const total = await linhas.count();
        for (let i = 0; i < total; i++) {
          await expect(linhas.nth(i)).toBeChecked();
        }
      } else {
        const linhas = importDialog.locator('[data-identical] [role="checkbox"]');
        const total = await linhas.count();
        for (let i = 0; i < total; i++) {
          await expect(linhas.nth(i)).toBeChecked({ checked: false });
        }
      }

      await importDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(importDialog).toBeHidden();
    });

    test('confirmar grava só a seleção em mods/ (com restauração)', async ({
      page,
    }) => {
      const before = snapshotMtimes(modsDir);

      await gotoApp(page);
      await openFirstImageEntry(page);
      const importDialog = await openImportImagesDialog(page);

      const confirm = importDialog.getByRole('button', { name: /^Importar \d+$/ });
      if (!(await confirm.isEnabled())) {
        // Nada gravável com a seleção padrão (tudo igual ao binário):
        // "Todas" força a regravação — é o que o usuário faria.
        await importDialog
          .getByRole('checkbox', { name: 'Marcar todas as imagens' })
          .click();
      }
      await expect(confirm).toBeEnabled();
      await confirm.click();

      await expect(toast(page, /gravada\(s\) em mods\//)).toBeVisible({
        timeout: 120_000,
      });
      // O modal fecha (o de progresso, que também é dialog, some sozinho).
      await expect(importImagesDialog(page)).toBeHidden({ timeout: 30_000 });

      // Prova da escrita: containers .dds.phyre novos/regravados em mods/.
      const changed = changedSince(modsDir, before);
      const gravados = changed.filter((f) => f.endsWith('.dds.phyre'));
      expect(
        gravados.length,
        `containers .dds.phyre regravados: ${gravados.slice(0, 5).join(', ')}`
      ).toBeGreaterThan(0);
    });
  });
});
