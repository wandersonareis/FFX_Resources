import { expect, test } from '@playwright/test';
import {
  closeDialog,
  dialog,
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  gotoApp,
  openConfigDialog,
} from '../helpers/ui';
import { gameFiles, translateDir, gameExe } from '../helpers/fixtures';

test.describe('Diálogo de Configurações', () => {
  test('abre pelo botão do header, pelo Ctrl+, e fecha pelo Fechar', async ({
    page,
  }) => {
    await gotoApp(page);

    await openConfigDialog(page);
    await expect(dialog(page).getByText('Configurações')).toBeVisible();
    await closeDialog(page, 'Fechar');

    // Atalho global Ctrl+, abre as mesmas Configurações.
    await page.keyboard.press('Control+,');
    await expect(dialog(page).getByText('Configurações')).toBeVisible();
    await closeDialog(page, 'Fechar');
  });

  test('exibe os campos de localização e o toggle mods', async ({ page }) => {
    await gotoApp(page);
    await openConfigDialog(page);

    const exe = dialog(page).locator('#GameExeLocation');
    const files = dialog(page).locator('#gameFiles');
    const translated = dialog(page).locator('#translate');

    await expect(dialog(page).getByText('Executável do jogo')).toBeVisible();
    await expect(
      dialog(page).getByText('Arquivos originais do jogo')
    ).toBeVisible();
    await expect(
      dialog(page).getByText('Arquivos traduzidos (reimport)')
    ).toBeVisible();

    // Espelho do config.json (o backend responde com o que realmente
    // persistiu) — a carga é assíncrona, então espera o valor chegar.
    if (gameFiles) {
      await expect(files).toHaveValue(gameFiles, { timeout: 10_000 });
    }
    if (translateDir) {
      await expect(translated).toHaveValue(translateDir, { timeout: 10_000 });
    }
    if (gameExe) {
      await expect(exe).toHaveValue(gameExe, { timeout: 10_000 });
    }

    await expect(dialog(page).locator('#EnableMods')).toBeVisible();
    await expect(
      dialog(page).getByText('Preferir binários traduzidos (mods)')
    ).toBeVisible();
    await closeDialog(page, 'Fechar');
  });

  test('sem estouro: diálogo contido na viewport em janela baixa', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 800, height: 500 });
    await gotoApp(page);
    await openConfigDialog(page);
    await expectDialogWithinViewport(page);
    await expectNoHorizontalOverflow(page);
    await closeDialog(page, 'Fechar');
  });

  test('seletores nativos não existem no modo servidor: nada quebra', async ({
    page,
  }) => {
    await gotoApp(page);
    await openConfigDialog(page);

    // No modo servidor os pickers nativos devolvem "" (cancelamento):
    // nenhum diálogo do SO, nenhum crash — o app segue utilizável.
    await dialog(page)
      .getByRole('button', { name: 'Selecionar o executável do jogo' })
      .click();
    await dialog(page)
      .getByRole('button', { name: 'Selecionar Arquivos originais do jogo' })
      .click();
    await dialog(page)
      .getByRole('button', { name: 'Selecionar Arquivos traduzidos (reimport)' })
      .click();

    // O diálogo segue aberto e os valores continuam os do config.
    await expect(dialog(page).getByText('Configurações')).toBeVisible();
    await closeDialog(page, 'Fechar');
  });
});
