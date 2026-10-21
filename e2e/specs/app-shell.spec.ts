import { expect, test } from '@playwright/test';
import {
  gotoApp,
  expectNoHorizontalOverflow,
  HEADER_TITLE,
  TAB_LABELS,
} from '../helpers/ui';
import { CONFIG_PATH, describeFixtures } from '../helpers/fixtures';
import { backupFile, restoreFile } from '../helpers/backup';

test.describe('AppShell (casca do app)', () => {
  // Trocar de aba persiste a versão no config.json: o arquivo é devolvido
  // no fim do spec (mesmo se um teste falhar no meio da troca).
  let configBackup: ReturnType<typeof backupFile>;

  test.beforeAll(() => {
    // eslint-disable-next-line no-console
    console.log(`[e2e] ${describeFixtures()}`);
    configBackup = backupFile(CONFIG_PATH, 'app-shell-config');
  });

  test.afterAll(() => {
    restoreFile(configBackup);
  });

  test('carrega com título, header e abas de versão', async ({ page }) => {
    await gotoApp(page);
    await expect(page).toHaveTitle(HEADER_TITLE);
    await expect(page.getByText(HEADER_TITLE).first()).toBeVisible();

    // Ctrl+1..4 — uma aba por versão do jogo (exact: "FFX" casa com todas
    // as outras por substring).
    for (const label of TAB_LABELS) {
      await expect(page.getByRole('tab', { name: label, exact: true })).toBeVisible();
    }
  });

  test('sidebar "Conteúdo" com recarregar e busca, estado vazio da tabela', async ({
    page,
  }) => {
    await gotoApp(page);
    await expect(page.getByText('Conteúdo').first()).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Recarregar' })
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: /Buscar nome ou texto/ })
    ).toBeVisible();

    // Sem arquivo aberto: a área principal pede a seleção no sidebar.
    await expect(page.getByText('Selecione um arquivo no sidebar.')).toBeVisible();
  });

  test('troca de aba por atalho (Ctrl+2/FFX Eternal Calm e volta ao FFX)', async ({ page }) => {
    await gotoApp(page);
    const ffx = page.getByRole('tab', { name: 'FFX', exact: true });
    const eternalcalm = page.getByRole('tab', {
      name: 'FFX Eternal Calm',
      exact: true,
    });
    await expect(ffx).toHaveAttribute('aria-selected', 'true');

    // Ctrl+2 é a SEGUNDA versão (FFX Eternal Calm — árvore leve); trocar de
    // aba também persiste a versão no config (o afterAll devolve o arquivo).
    await page.keyboard.press('Control+2');
    await expect(eternalcalm).toHaveAttribute('aria-selected', 'true');
    await expect(ffx).toHaveAttribute('aria-selected', 'false');

    // Volta à aba inicial — o config do app permanece no FFX.
    await page.keyboard.press('Control+1');
    await expect(ffx).toHaveAttribute('aria-selected', 'true');
  });

  test('alternador de tema claro/escuro', async ({ page }) => {
    await gotoApp(page);
    // Default do app: tema escuro (classe .dark no <html>).
    await expect(page.locator('html')).toHaveClass(/dark/);

    await page.getByRole('button', { name: 'Ativar tema claro' }).click();
    await expect(page.locator('html')).not.toHaveClass(/dark/);

    await page.getByRole('button', { name: 'Ativar tema escuro' }).click();
    await expect(page.locator('html')).toHaveClass(/dark/);
  });

  test('barra de status presente no rodapé', async ({ page }) => {
    await gotoApp(page);
    await expect(page.locator('footer')).toBeVisible();
    // Grupo de setas de navegação da árvore (esquerda da barra).
    await expect(
      page.getByRole('group', { name: 'Setas de navegação' })
    ).toBeVisible();
  });

  test('página sem estouro horizontal (janela baixa e estreita)', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 600 });
    await gotoApp(page);
    await expectNoHorizontalOverflow(page);
  });
});
