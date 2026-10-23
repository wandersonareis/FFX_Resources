import { expect, test, type Page } from '@playwright/test';
import {
  columnTexts,
  entryHeading,
  findAnotherTextLeaf,
  gotoApp,
  openLeafAndWaitLoad,
  restoreEnableModsInConfigDialog,
  revealFirstTextLeaf,
  toggleEnableModsInConfigDialog,
} from '../helpers/ui';
import { describeFixtures, hasData } from '../helpers/fixtures';
import { readEnableMods } from '../helpers/config-file';

/**
 * Alternar o checkbox "Preferir binários traduzidos (mods)" (#EnableMods —
 * o "disable mods" do config) e continuar navegando os textos:
 *
 *  1. com um texto JÁ aberto: alternar o toggle e clicar em OUTRO texto —
 *     a exibição tem de trocar para o novo arquivo;
 *  2. sem clicar em nada: alternar o toggle ANTES de qualquer carga e só
 *     então clicar num texto — o primeiro load tem de carregar e exibir.
 *
 * O toggle muda a resolução de caminhos no backend (mods-first ↔ data/) na
 * hora (SetEnableMods aplica e persiste) — as cargas seguintes têm de
 * funcionar com a preferência nova, venham de onde vier o binário.
 *
 * EnableMods volta ao valor original no fim (pela UI, para o backend em
 * memória e o config.json ficarem coerentes).
 */
test.describe('Alternar o toggle de mods e a navegação de textos', () => {
  test.skip(!hasData, 'data/ não existe — sem árvore para navegar');

  test('texto aberto → alterna o toggle de mods → clicar em outro texto troca a exibição', async ({
    page,
  }) => {
    test.info().annotations.push({
      type: 'fixture',
      description: describeFixtures(),
    });

    const originalMods = readEnableMods();
    let toggled = false;
    try {
      await gotoApp(page);

      // ---- 1. abre um texto e confere que exibiu ----
      const leafA = await revealFirstTextLeaf(page);
      const labelA = await openLeafAndWaitLoad(page, leafA);
      const heading = entryHeading(page);
      await expect(heading).toContainText(`· ${labelA}`);
      const originalsA = await expectTextDisplayed(page);

      // ---- 2. Configurações: marca/desmarca o toggle de mods ----
      // (inverte o estado atual: ligado desliga, desligado liga)
      const flippedTo = await toggleEnableModsInConfigDialog(page);
      toggled = true;
      test.info().annotations.push({
        type: 'toggle',
        description: `EnableMods: ${originalMods} → ${flippedTo}`,
      });

      // ---- 3. clica em OUTRO texto: tem de carregar e TROCAR ----
      const leafB = await findAnotherTextLeaf(page, labelA);
      const labelB = await openLeafAndWaitLoad(page, leafB);
      await expect(heading).toContainText(`· ${labelB}`);
      // A tabela do arquivo anterior continua visível DURANTE a carga —
      // provar a troca é o conteúdo mudar, não só a row existir.
      await expect
        .poll(
          async () =>
            (await columnTexts(page, 'Original')).join('\u0001') !==
            originalsA.join('\u0001'),
          {
            timeout: 20_000,
            message: `tabela exibindo o conteúdo de "${labelB}" (saía de "${labelA}")`,
          }
        )
        .toBe(true);
      await expectTextDisplayed(page);
    } finally {
      if (toggled) {
        try {
          await restoreEnableModsInConfigDialog(page, originalMods);
        } catch (restoreError) {
          // não mascara a falha original — mas o fixture fica sujo
          console.error(
            `FALHA NA LIMPEZA: EnableMods não restaurado para ${originalMods}`,
            restoreError
          );
        }
      }
    }
  });

  test('sem abrir nada → alterna o toggle de mods → o primeiro texto clicado carrega e exibe', async ({
    page,
  }) => {
    test.info().annotations.push({
      type: 'fixture',
      description: describeFixtures(),
    });

    const originalMods = readEnableMods();
    let toggled = false;
    try {
      await gotoApp(page);

      // ---- 1. SEM clicar em nada na árvore: direto às Configurações ----
      const flippedTo = await toggleEnableModsInConfigDialog(page);
      toggled = true;
      test.info().annotations.push({
        type: 'toggle',
        description: `EnableMods: ${originalMods} → ${flippedTo}`,
      });

      // ---- 2. o PRIMEIRO clique num texto precisa carregar e exibir ----
      const leaf = await revealFirstTextLeaf(page);
      const label = await openLeafAndWaitLoad(page, leaf);
      await expect(entryHeading(page)).toContainText(`· ${label}`);
      await expectTextDisplayed(page);
    } finally {
      if (toggled) {
        try {
          await restoreEnableModsInConfigDialog(page, originalMods);
        } catch (restoreError) {
          console.error(
            `FALHA NA LIMPEZA: EnableMods não restaurado para ${originalMods}`,
            restoreError
          );
        }
      }
    }
  });
});

/**
 * Confere que a tabela EXIBIU conteúdo de verdade: rows no DOM e a coluna
 * Original inteira preenchida (a fonte da verdade é sempre data/ — linhas
 * em branco são filtradas na carga). Devolve os textos da coluna Original
 * para quem precisa comparar antes/depois.
 */
async function expectTextDisplayed(page: Page): Promise<string[]> {
  const rows = page.locator('[data-row-key]');
  const count = await rows.count();
  expect(count, 'tabela com rows').toBeGreaterThan(0);

  const originals = await columnTexts(page, 'Original');
  expect(originals.length, 'uma Original por row').toBe(count);
  const withText = originals.filter((t) => t !== '' && t !== '—');
  expect(withText.length, 'coluna Original preenchida').toBe(originals.length);
  return originals;
}
