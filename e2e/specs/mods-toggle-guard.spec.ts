import { expect, test, type Page } from '@playwright/test';
import {
  closeDialogByX,
  columnTexts,
  dialog,
  entryHeading,
  gotoApp,
  modsGuard,
  openConfigDialog,
  openFirstTextEntry,
  restoreEnableModsInConfigDialog,
  tableRow,
  toast,
} from '../helpers/ui';
import { hasData, modsDir } from '../helpers/fixtures';
import { readEnableMods } from '../helpers/config-file';
import { backupTree, restoreTree, type TreeBackup } from '../helpers/backup';

/**
 * Guarda de edições não salvas do checkbox "Preferir binários traduzidos
 * (mods)" (#EnableMods — o "disable mods" do config).
 *
 * Alternar o checkbox recarrega TODO o conteúdo (o eco invalida os caches
 * do backend e a aba dá coldReload — a tradução liga/desliga de acordo).
 * Com rascunho sujo, a guarda pergunta o que fazer com as edições ANTES de
 * aplicar:
 *
 *  - Cancelar: anula SÓ o toggle — o checkbox volta ao estado anterior, o
 *    config.json fica intocado, o rascunho continua sujo e NADA recarrega;
 *  - Descartar e recarregar: aplica o toggle, mata os rascunhos e recarrega;
 *  - Salvar e recarregar: persiste os rascunhos (mesmo motor do Ctrl+S do
 *    editor-save) e aplica o toggle — o texto salvo em mods/ aparece na
 *    recarga quando o toggle liga a tradução.
 *
 * O critério de "sujo" é o MESMO boolean que habilita o botão Salvar da
 * tabela — sem ele, o toggle aplica direto (spec mods-toggle-navigation).
 */
test.describe('Toggle de mods com edições não salvas (guarda)', () => {
  test.skip(!hasData, 'data/ não existe — sem arquivo para editar');

  let modsBackup: TreeBackup | null = null;

  test.beforeAll(() => {
    // O caminho "Salvar e recarregar" grava de verdade em mods/ — o
    // fixture volta byte a byte no fim (mesma regra do editor-save).
    modsBackup = backupTree(modsDir, 'mods-guard');
  });

  test.afterAll(async () => {
    if (modsBackup) await restoreTree(modsBackup);
  });

  test('Cancelar devolve o checkbox, preserva o rascunho e não recarrega nada', async ({
    page,
  }) => {
    const originalMods = readEnableMods();
    await gotoApp(page);
    const entryLabel = await openFirstTextEntry(page);
    const { testText, rowKey } = await editFirstSimpleRow(page);

    // ---- abre a guarda: rascunho sujo + clique no toggle ----
    await openConfigDialog(page);
    const box = dialog(page).locator('#EnableMods');
    await expect
      .poll(() => box.isChecked(), { timeout: 15_000 })
      .toBe(originalMods);
    await box.click();

    const guard = modsGuard(page);
    await expect(guard).toBeVisible({ timeout: 15_000 });
    // O aviso é a peça central: Sim e NÃO recarregam; só Cancelar mantém.
    await expect(guard.getByText('Todo o conteúdo será recarregado')).toBeVisible();

    // ---- Cancelar ----
    await guard.getByRole('button', { name: 'Cancelar', exact: true }).click();
    await expect(guard).toBeHidden();

    // Checkbox de volta ao estado anterior (o store nunca foi tocado) e
    // config.json intocado — nada chegou ao backend.
    if (originalMods) await expect(box).toBeChecked();
    else await expect(box).not.toBeChecked();
    expect(readEnableMods()).toBe(originalMods);
    await closeDialogByX(page);

    // Nada recarregou: a entrada segue aberta, o rascunho segue sujo
    // (célula editada + botão Salvar habilitado).
    await expect(
      page.getByRole('button', { name: 'Salvar', exact: true })
    ).toBeEnabled();
    await expect(cellOf(page, rowKey)).toContainText(testText);
    await expect(entryHeading(page)).toContainText(`· ${entryLabel}`);
  });

  test('Descartar e recarregar aplica o toggle, limpa os rascunhos e recarrega o conteúdo', async ({
    page,
  }) => {
    const originalMods = readEnableMods();
    const target = !originalMods;
    let toggled = false;
    try {
      await gotoApp(page);
      await openFirstTextEntry(page);
      const { testText, rowKey } = await editFirstSimpleRow(page);

      // ---- abre a guarda e descarta ----
      await openConfigDialog(page);
      const box = dialog(page).locator('#EnableMods');
      await expect
        .poll(() => box.isChecked(), { timeout: 15_000 })
        .toBe(originalMods);
      await box.click();

      const guard = modsGuard(page);
      await expect(guard).toBeVisible({ timeout: 15_000 });
      await guard
        .getByRole('button', { name: 'Descartar e recarregar', exact: true })
        .click();
      await expect(guard).toBeHidden();

      // Aplicou de verdade: checkbox e config.json no novo valor.
      await expect
        .poll(() => box.isChecked(), { timeout: 60_000 })
        .toBe(target);
      await expect
        .poll(() => readEnableMods(), {
          timeout: 15_000,
          message: `config.json com EnableMods=${target}`,
        })
        .toBe(target);
      toggled = true;

      // Recarregou a frio: a seleção caiu (placeholder de nada aberto)…
      await expect(page.getByText('Selecione um arquivo no sidebar.')).toBeVisible({
        timeout: 30_000,
      });
      await closeDialogByX(page);

      // …e os rascunhos morreram: o botão Salvar volta desabilitado.
      await expect(
        page.getByRole('button', { name: 'Salvar', exact: true })
      ).toBeDisabled();

      // Reabre a MESMA entrada: o texto descartado não volta. A row é
      // identificada pela CHAVE (não pelo índice): com o toggle virado a
      // árvore do slot 'us' mudou e o conjunto de rows visíveis também.
      await openFirstTextEntry(page);
      await expect(cellOf(page, rowKey)).not.toContainText(testText);
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

  test('Salvar e recarregar persiste os rascunhos antes de aplicar o toggle', async ({
    page,
  }) => {
    const originalMods = readEnableMods();
    let forced = false;
    try {
      await gotoApp(page);
      // Ponto de partida DESLIGADO: o toggle final (false→true) "ativa a
      // tradução" — o texto salvo em mods/ aparece na recarga. O estado
      // original do fixture volta no fim.
      await restoreEnableModsInConfigDialog(page, false);
      forced = true;

      await openFirstTextEntry(page);
      const { testText, rowKey } = await editFirstSimpleRow(page);

      // ---- abre a guarda e salva ----
      await openConfigDialog(page);
      const box = dialog(page).locator('#EnableMods');
      await expect
        .poll(() => box.isChecked(), { timeout: 15_000 })
        .toBe(false);
      await box.click();

      const guard = modsGuard(page);
      await expect(guard).toBeVisible({ timeout: 15_000 });
      await guard
        .getByRole('button', { name: 'Salvar e recarregar', exact: true })
        .click();
      await expect(guard).toBeHidden();

      // Salvou de verdade (mesmo motor do Ctrl+S do editor-save)…
      await expect(toast(page, 'Alterações salvas no jogo.')).toBeVisible({
        timeout: 60_000,
      });
      // …e aplicou o toggle.
      await expect
        .poll(() => box.isChecked(), { timeout: 60_000 })
        .toBe(true);
      await expect
        .poll(() => readEnableMods(), {
          timeout: 15_000,
          message: 'config.json com EnableMods=true',
        })
        .toBe(true);

      // Recarregou com a preferência NOVA: a seleção caiu…
      await expect(page.getByText('Selecione um arquivo no sidebar.')).toBeVisible({
        timeout: 30_000,
      });
      await closeDialogByX(page);

      // …e a mesma entrada agora exibe o texto salvo em mods/ — na MESMA
      // row (pela chave: o filtro de rows visíveis muda com o toggle).
      await openFirstTextEntry(page);
      await expect(cellOf(page, rowKey)).toContainText(testText);
    } finally {
      if (forced) {
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
 * Célula traduzida da row editada — pela CHAVE data-row-key, não pelo
 * índice: o conjunto de rows visíveis muda quando o toggle de mods troca
 * a árvore do slot 'us' (o filtro de linhas em branco deixa passar rows
 * diferentes), então nth() apontaria outra row após a recarga.
 */
function cellOf(page: Page, rowKey: string) {
  return tableRow(page, rowKey).locator('.translated-cell');
}

/**
 * Suja um rascunho na primeira entrada (editor de linha → commit no
 * rascunho, SEM salvar no binário — o botão Salvar do app habilita).
 * Escolhe uma row com original simples (texto, sem tag protegida).
 * Devolve o texto digitado e a CHAVE da row editada.
 */
async function editFirstSimpleRow(
  page: Page
): Promise<{ testText: string; rowKey: string }> {
  const rows = page.locator('[data-row-key]');
  await expect(rows.first()).toBeVisible();
  const originals = await columnTexts(page, 'Original');
  const targetIndex = originals.findIndex(
    (t) => t !== '' && t !== '—' && !t.includes('{') && t.length <= 120
  );
  expect(
    targetIndex,
    'linha com texto simples para editar'
  ).toBeGreaterThanOrEqual(0);

  const testText = `Guarda mods ${Date.now()}`;
  const editedRow = rows.nth(targetIndex);
  const rowKey = (await editedRow.getAttribute('data-row-key')) ?? '';
  expect(rowKey, 'row editada com data-row-key').not.toBe('');
  await editedRow.locator('.translated-cell').click();
  const editorDialog = dialog(page);
  await expect(editorDialog.getByText(/Linha #\d+/)).toBeVisible();
  const editor = editorDialog.locator('.tiptap');
  await editor.click();
  await page.keyboard.press('Control+a');
  await page.keyboard.type(testText, { delay: 10 });
  await editorDialog
    .getByRole('button', { name: 'Salvar', exact: true })
    .click();
  await expect(editorDialog).toBeHidden();

  // O rascunho sujou: o botão Salvar do app habilita.
  await expect(
    page.getByRole('button', { name: 'Salvar', exact: true })
  ).toBeEnabled();
  return { testText, rowKey };
}
