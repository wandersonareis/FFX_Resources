import { expect, test } from '@playwright/test';
import {
  gotoApp,
  switchToTab,
  waitForKindRoot,
  openFirstTextEntry,
  columnTexts,
  expectTextDisplayed,
  expectNoHorizontalOverflow,
  badgeProgress,
} from '../helpers/ui';
import { hasData, hasDataFFX2, hasDataLastMiss, hasMods } from '../helpers/fixtures';

// Definição local para evitar dependência do alias @/ do frontend
const GAME_VERSIONS = [
  { id: 'ffx', label: 'FFX' },
  { id: 'eternalcalm', label: 'FFX Eternal Calm' },
  { id: 'ffx2', label: 'FFX-2' },
  { id: 'lastmiss', label: 'FFX-2 Last Mission' },
] as const;

type GameVersionId = (typeof GAME_VERSIONS)[number]['id'];

const TABS_FULL_TEST: GameVersionId[] = ['ffx', 'ffx2', 'lastmiss'];
const TABS_TEXT_ONLY: GameVersionId[] = ['eternalcalm'];

/** Verifica se a aba tem dados de texto para testar. */
function hasDataForVersion(version: GameVersionId): boolean {
  switch (version) {
    case 'ffx':
      return hasData;
    case 'ffx2':
      return hasDataFFX2;
    case 'lastmiss':
      return hasDataLastMiss;
    default:
      return hasData;
  }
}

test.describe.configure({ retries: 0 });

// ============================================================
// TESTES COMPLETOS: ffx, ffx2, lastmiss
// ============================================================
for (const version of TABS_FULL_TEST) {
  test.describe(`[${version.toUpperCase()}] Fluxos completos de texto`, () => {
    test.skip(!hasDataForVersion(version), `${version}: data/ não existe — sem árvore para exibir`);

    test('árvore carrega kinds de texto com contagens', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      await waitForKindRoot(page, 'events');
      const tree = page.locator('#entry-tree');
      const eventsKind = tree.locator('[data-node-id="kind:events"]');
      await expect(eventsKind).toHaveAttribute('data-node-label', /Eventos/);
    });

    test('Original obrigatório: toda row visível tem texto original', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      await openFirstTextEntry(page);
      await expectTextDisplayed(page);
    });

    test('Traduzido exibido quando mods/ existe', async ({ page }) => {
      test.skip(!hasMods, 'mods/ não existe — tradução não é exigida');

      await gotoApp(page);
      await switchToTab(page, version);
      await waitForKindRoot(page, 'events');
      const tree = page.locator('#entry-tree');
      await tree.locator('[data-node-id="kind:events"] button[aria-label^="Alternar"]').first().click();
      const group = tree.locator('[data-node-id^="group:events:"]').first();
      await group.locator('button[aria-label^="Alternar"]').first().click();

      const leaves = tree.locator('[data-node-entry="true"]');
      const total = Math.min(await leaves.count(), 5);
      for (let i = 0; i < total; i++) {
        await leaves.nth(i).locator('[data-node-button]').click();
        await expect(page.locator('[data-row-key]').first()).toBeVisible({ timeout: 60_000 });

        const headers = await page.locator('table thead th').allTextContents();
        expect(headers.join('|')).toContain('Traduzido');

        const progress = await badgeProgress(page);
        if (progress && progress.translated > 0 && progress.total > 0) {
          const translated = await columnTexts(page, 'Traduzido');
          const nonEmpty = translated.filter((t) => t !== '' && t !== '—');
          expect(nonEmpty.length).toBeGreaterThan(0);
          return;
        }
      }
      test.info().annotations.push({
        type: 'skip',
        description: `nenhum dos primeiros arquivos tem tradução salva em mods/ para ${version}`,
      });
    });

    test('tabela longa rola sem estouro horizontal', async ({ page }) => {
      await page.setViewportSize({ width: 1024, height: 500 });
      await gotoApp(page);
      await switchToTab(page, version);
      await openFirstTextEntry(page);
      await expectNoHorizontalOverflow(page);
    });
  });

  test.describe(`[${version.toUpperCase()}] Busca global (Ctrl+K)`, () => {
    test.skip(!hasDataForVersion(version), `${version}: data/ não existe`);

    test('abre, busca por nome e termo inexistente', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      const label = await openFirstTextEntry(page);

      await page.keyboard.press('Control+k');
      const search = page.locator('[role="dialog"]').first();
      await expect(search).toBeVisible();
      await expect(search.getByRole('textbox')).toBeFocused();

      const input = search.getByRole('textbox');
      await input.fill(label.slice(0, Math.max(4, Math.min(label.length, 8))));
      await expect(search.getByText(/arquivo\(s\) por nome/)).toBeVisible({ timeout: 60_000 });

      await input.fill('zzqqxxww');
      await expect(search.getByText('Nenhum resultado.')).toBeVisible({ timeout: 60_000 });

      await page.keyboard.press('Escape');
      await expect(search).toBeHidden();
    });
  });

  test.describe(`[${version.toUpperCase()}] Busca no arquivo (Ctrl+F)`, () => {
    test.skip(!hasDataForVersion(version), `${version}: data/ não existe`);

    test('abre, destaca matches, substituição toggle, fecha', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      await openFirstTextEntry(page);

      await page.keyboard.press('Control+f');
      const find = page.getByRole('textbox', { name: 'Buscar neste arquivo' });
      await expect(find).toBeVisible();
      await expect(page.getByRole('button', { name: 'Diferenciar maiúsculas e minúsculas' })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Casar palavra inteira' })).toBeVisible();

      const translatedTexts = await columnTexts(page, 'Traduzido');
      const words = translatedTexts.flatMap((t) =>
        t.replace(/\{[^}]*\}/g, ' ').split(/\s+/)
      );
      const word =
        words.find((w) => w.length >= 2 && /^[A-Za-zÀ-ÿ0-9]+$/.test(w))?.slice(0, 12) ?? 'the';
      await find.fill(word);
      await expect(page.locator('mark').first()).toBeVisible({ timeout: 30_000 });

      await expect(page.getByRole('textbox', { name: 'Texto de substituição' })).toBeHidden();
      await page.getByRole('button', { name: 'Mostrar substituição' }).click();
      await expect(page.getByRole('textbox', { name: 'Texto de substituição' })).toBeVisible();

      await page.getByRole('button', { name: 'Fechar a busca' }).click();
      await expect(find).toBeHidden();
    });
  });

  test.describe(`[${version.toUpperCase()}] Menus Exportar/Importar`, () => {
    test.skip(!hasDataForVersion(version), `${version}: data/ não existe`);

    test('menu Exportar tem escopos por tipo', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      await page.getByRole('button', { name: /^Exportar/ }).click();
      const menu = page.getByRole('menu');
      await expect(menu.getByRole('menuitem', { name: /Exportar todos os tipos/ })).toBeVisible();
      await expect(menu.getByText('Escopo')).toBeVisible();
      await expect(menu.getByRole('menuitemcheckbox', { name: 'Eventos' })).toBeVisible();
      await expect(menu.getByRole('menuitem', { name: /tipos marcados/ })).toBeVisible();
      await page.keyboard.press('Escape');
    });

    test('menu Importar abre lote translated', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      await page.getByRole('button', { name: /^Importar/ }).click();
      const menu = page.getByRole('menu');
      await expect(menu.getByRole('menuitem', { name: 'Importar mods/translated' })).toBeVisible();
      await menu.getByRole('menuitem', { name: 'Importar mods/translated' }).click();

      const importDialog = page.locator('[role="dialog"]').first();
      await expect(importDialog.getByText('Importar textos de mods/translated')).toBeVisible({ timeout: 60_000 });
      await importDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(importDialog).toBeHidden();
    });
  });
}

// ============================================================
// TESTES APENAS EXIBIÇÃO DE TEXTO: eternalcalm
// ============================================================
for (const version of TABS_TEXT_ONLY) {
  test.describe(`[${version.toUpperCase()}] Apenas exibição de texto`, () => {
    test.skip(!hasDataForVersion(version), `${version}: data/ não existe — sem árvore para exibir`);

    test('árvore carrega (sem erro de container .vbf)', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      // eternalcalm compartilha data/ com FFX mas sem container .vbf próprio
      // Verifica que a árvore carrega (não mostra aviso de executável)
      const tree = page.locator('#entry-tree');
      await expect(tree).toBeVisible({ timeout: 60_000 });
      // Pelo menos a raiz "Conteúdo" ou similar está visível
      await expect(page.getByText('Conteúdo').first()).toBeVisible({ timeout: 60_000 });
    });

    test('sem container .vbf próprio (aviso não aparece)', async ({ page }) => {
      await gotoApp(page);
      await switchToTab(page, version);
      // eternalcalm não tem container .vbf próprio — não deve mostrar aviso de executável
      const vbfMissing = page.locator('[data-vbf-missing][role="alert"]');
      await expect(vbfMissing).toHaveCount(0);
    });
  });
}