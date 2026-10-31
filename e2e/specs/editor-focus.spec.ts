import { expect, test } from '@playwright/test';
import { columnTexts, dialog, gotoApp, openFirstTextEntry } from '../helpers/ui';
import { hasData } from '../helpers/fixtures';

/**
 * Editor de texto — FOCO no campo de edição.
 *
 * O editor (Tiptap) nasce UMA vez e sobrevive às edições: o diálogo guarda
 * o rascunho a cada tecla/colagem, então `value` muda o tempo todo. Se o
 * editor fosse RECRIADO a cada mudança de `value`, o contenteditable sairia
 * do DOM e o foco iria para o body — o usuário teria que clicar de novo a
 * cada letra ("digito uma letra e o foco perde").
 *
 * O spec fecha com Cancelar: nenhum rascunho é salvo no binário.
 */
test.describe('Editor: foco na digitação e na colagem', () => {
  test.skip(!hasData, 'data/ não existe — sem arquivo para editar');

  test('o foco fica no editor ao digitar várias letras e ao colar', async ({ page }) => {
    await gotoApp(page);
    await openFirstTextEntry(page);

    // Linha com texto simples (sem tag): o editor abre com o rascunho atual
    // (vazio ou não) — o que importa aqui é o FOCO.
    const originals = await columnTexts(page, 'Original');
    const targetIndex = originals.findIndex(
      (t) => t !== '' && t !== '—' && !t.includes('{') && !t.includes('FOCO123')
    );
    expect(targetIndex, 'linha com texto simples para editar').toBeGreaterThanOrEqual(0);

    const rows = page.locator('[data-row-key]');
    await rows.nth(targetIndex).locator('.translated-cell').click();

    const editorDialog = dialog(page);
    await expect(editorDialog.getByText(/Linha #\d+/)).toBeVisible();
    const editor = editorDialog.locator('.tiptap');
    await expect(editor).toBeVisible();

    // O Chromium só deixa o teste escrever na área de transferência com a
    // permissão concedida.
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);

    await editor.click();
    await expect(editor, 'foco no editor ao abrir').toBeFocused();

    // Várias letras seguidas: com o editor recriado a cada tecla, só a
    // primeira entraria — as outras iriam para o body.
    await page.keyboard.type('FOCO123', { delay: 30 });
    await expect(editor, 'as letras digitadas ficaram no editor').toContainText('FOCO123');
    await expect(editor, 'foco mantido depois de digitar').toBeFocused();

    // Colar: mesmo caminho do bug (onUpdate → rascunho novo → value novo).
    await page.evaluate(() => navigator.clipboard.writeText('COLADO456'));
    await page.keyboard.press('Control+v');
    await expect(editor, 'o texto colado entrou no editor').toContainText('COLADO456');
    await expect(editor, 'foco mantido depois de colar').toBeFocused();

    // Descarta: o rascunho não pode vazar para o fixture (sem Salvar).
    await editorDialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(editorDialog).toBeHidden();
  });
});
