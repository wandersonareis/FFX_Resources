import { expect, test } from '@playwright/test';
import {
  badgeProgress,
  columnTexts,
  expectNoHorizontalOverflow,
  gotoApp,
  nodeRow,
  openFirstTextEntry,
  waitForKindRoot,
} from '../helpers/ui';
import { hasData, hasMods } from '../helpers/fixtures';

/**
 * Exibição de conteúdo — POR EXISTÊNCIA:
 *  - data/ existe → a árvore carrega e a coluna Original mostra conteúdo
 *    (obrigatório: é a fonte da verdade, sempre em data/);
 *  - mods/ existe → a coluna Traduzido mostra as traduções salvas;
 *  - nenhum dos dois → o spec inteiro é pulado (nada para exibir).
 */
test.describe('Exibição de conteúdo', () => {
  test.skip(!hasData, 'data/ não existe — sem árvore para exibir');

  test('árvore de data/ carrega os kinds de texto com contagens', async ({
    page,
  }) => {
    await gotoApp(page);
    await waitForKindRoot(page, 'events');
    // "Texto" nasce expandido: os kinds ficam visíveis com (N).
    const eventos = nodeRow(page, 'kind:events');
    await expect(eventos).toHaveAttribute('data-node-label', /Eventos \(\d+\)/);
  });

  test('Original obrigatório: toda row visível tem texto original', async ({
    page,
  }) => {
    await gotoApp(page);
    await openFirstTextEntry(page);

    const rows = page.locator('[data-row-key]');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);

    const originals = await columnTexts(page, 'Original');
    expect(originals.length).toBe(count);
    // As rows em branco do original são filtradas na carga: o que sobrou
    // tem texto — o "—" só existe para linha inexistente no lado oposto.
    const withText = originals.filter((t) => t !== '' && t !== '—');
    expect(withText.length).toBe(originals.length);
  });

  test('Traduzido exibido quando mods/ existe', async ({ page }) => {
    test.skip(!hasMods, 'mods/ não existe — tradução não é exigida');

    await gotoApp(page);
    // Abre as primeiras folhas de events procurando um arquivo com
    // tradução salva (badge > 0): é o caso em que a coluna Traduzido
    // mostra o conteúdo vindo de mods/.
    await waitForKindRoot(page, 'events');
    const tree = page.locator('#entry-tree');
    await nodeRow(page, 'kind:events')
      .locator('button[aria-label^="Alternar"]')
      .first()
      .click();
    const group = tree.locator('[data-node-id^="group:events:"]').first();
    await group.locator('button[aria-label^="Alternar"]').first().click();

    const leaves = tree.locator('[data-node-entry="true"]');
    const total = Math.min(await leaves.count(), 5);
    for (let i = 0; i < total; i++) {
      await leaves.nth(i).locator('[data-node-button]').click();
      await expect(page.locator('[data-row-key]').first()).toBeVisible({
        timeout: 60_000,
      });

      const headers = await page.locator('table thead th').allTextContents();
      expect(headers.join('|')).toContain('Traduzido');

      const progress = await badgeProgress(page);
      if (progress && progress.translated > 0 && progress.total > 0) {
        const translated = await columnTexts(page, 'Traduzido');
        const nonEmpty = translated.filter((t) => t !== '' && t !== '—');
        expect(
          nonEmpty.length,
          `badge ${progress.translated}/${progress.total} mas células traduzidas vazias`
        ).toBeGreaterThan(0);
        return;
      }
    }
    test.info().annotations.push({
      type: 'skip',
      description:
        'nenhum dos primeiros arquivos tem tradução salva em mods/ — exibição de tradução não exercitada',
    });
  });

  test('tabela longa rola dentro dela — página sem estouro horizontal', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 500 });
    await gotoApp(page);
    await openFirstTextEntry(page);
    await expectNoHorizontalOverflow(page);
  });
});
