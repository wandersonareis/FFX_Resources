import { expect, test } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {
  columnTexts,
  dialog,
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  gotoApp,
  openFirstTextEntry,
  toast,
} from '../helpers/ui';
import { hasData, hasMods, modsDir } from '../helpers/fixtures';
import {
  backupTree,
  changedSince,
  restoreTree,
  snapshotMtimes,
  withRetry,
  type TreeBackup,
} from '../helpers/backup';

/**
 * Editor de texto — o ciclo completo de ESCRITA:
 *
 *  1. editar uma linha e SALVAR: o texto alterado vai para o binário em
 *     mods/ (nunca data/), provado em duas pontas — arquivo regravado no
 *     disco e a UI recarregada exibindo o texto salvo;
 *  2. EXPORTAR a entrada marcada: artefatos em mods/edits com o modal de
 *     progresso;
 *  3. IMPORTAR o lote de mods/translated: o binário regravado em mods/.
 *
 * O fixture mods/ é copiado antes e restaurado depois (byte a byte) — o
 * teste valida o save real sem deixar sujeira na árvore de tradução. Sem
 * mods/ no início, o teste de escrita continua valendo (é ele quem prova o
 * "salvar em binário", criando mods/ na hora; a limpeza remove o que
 * criou).
 */
test.describe('Editor: escrita, save em mods/, export e import', () => {
  test.skip(!hasData, 'data/ não existe — sem arquivo para editar');

  let modsBackup: TreeBackup | null = null;

  test.beforeAll(() => {
    modsBackup = backupTree(modsDir, 'mods');
  });

  test.afterAll(async () => {
    if (modsBackup) await restoreTree(modsBackup);
  });

  test('texto alterado é salvo no binário (mods/) e volta na recarga', async ({
    page,
  }) => {
    await gotoApp(page);
    const entryLabel = await openFirstTextEntry(page);

    // ---- escolhe a row alvo: original com texto, sem tag protegida ----
    const rows = page.locator('[data-row-key]');
    await expect(rows.first()).toBeVisible();
    const originals = await columnTexts(page, 'Original');
    const targetIndex = originals.findIndex(
      (t) => t !== '' && t !== '—' && !t.includes('{') && t.length <= 120
    );
    expect(targetIndex, 'linha com texto simples para editar').toBeGreaterThanOrEqual(0);
    const originalText = originals[targetIndex];

    // O texto TRADUZIDO atual (é para ele que a célula volta na limpeza).
    const translatedBefore = (await columnTexts(page, 'Traduzido'))[targetIndex];

    // ---- abre o editor da linha ----
    await rows.nth(targetIndex).locator('.translated-cell').click();
    const editorDialog = dialog(page);
    await expect(editorDialog.getByText(/Linha #\d+/)).toBeVisible();
    const editor = editorDialog.locator('.tiptap');
    await expect(editor).toBeVisible();
    // Sem estouro: o diálogo cabe na viewport e a página não rola.
    await expectDialogWithinViewport(page);

    // ---- digita o texto de teste por cima ----
    const testText = `E2E ${Date.now()}`;
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type(testText, { delay: 10 });
    await editorDialog.getByRole('button', { name: 'Salvar', exact: true }).click();
    await expect(editorDialog).toBeHidden();

    // A célula reflete o rascunho e o Salvar do app habilita (sujo).
    const cell = rows.nth(targetIndex).locator('.translated-cell');
    await expect(cell).toContainText(testText);
    const saveButton = page.getByRole('button', { name: 'Salvar', exact: true });
    await expect(saveButton).toBeEnabled();

    // ---- salva no binário (Ctrl+S, mesmo caminho do botão) ----
    const before = snapshotMtimes(modsDir);
    await page.keyboard.press('Control+s');
    await expect(toast(page, 'Alterações salvas no jogo.')).toBeVisible({
      timeout: 60_000,
    });

    // ---- prova 1: o binário foi regravado/criado em mods/ ----
    await expect
      .poll(
        async () => changedSince(modsDir, before).length,
        { timeout: 30_000, message: 'arquivos mudando em mods/' }
      )
      .toBeGreaterThan(0);
    const changed = changedSince(modsDir, before);
    // O "salvar em binário" só escreve na árvore mods/ (data/ é imutável).
    for (const rel of changed) {
      expect(rel, 'gravação dentro de mods/').toMatch(/^(?!edits\/|images\/|translated\/)/);
    }

    // ---- prova 2: a UI recarregada exibe o texto salvo ----
    await page.reload();
    await expect(page.getByText('Conteúdo').first()).toBeVisible();
    await openFirstTextEntry(page);
    const cellAfter = page.locator('[data-row-key]').nth(targetIndex).locator('.translated-cell');
    await expect(cellAfter).toContainText(testText);

    // ---- limpeza: devolve o binário original e confere a UI ----
    await restoreChanged(modsBackup, changed);
    await page.reload();
    await expect(page.getByText('Conteúdo').first()).toBeVisible();
    await openFirstTextEntry(page);
    const cellRestored = page
      .locator('[data-row-key]')
      .nth(targetIndex)
      .locator('.translated-cell');
    await expect(cellRestored).toContainText(translatedBefore || originalText);
    test.info().annotations.push({
      type: 'entry',
      description: `entrada editada: ${entryLabel} (linha ${targetIndex})`,
    });
  });

  test('export da entrada marcada grava artefatos em mods/edits', async ({
    page,
  }) => {
    await gotoApp(page);
    const entryLabel = await openFirstTextEntry(page);

    // Marca a entrada aberta (checkbox da folha na árvore).
    const leaf = page.locator('[data-node-entry="true"]').first();
    await leaf
      .getByRole('checkbox', { name: `Selecionar ${entryLabel}` })
      .click();

    const before = snapshotMtimes(modsDir);

    // Exportar ▾ → Exportar seleção (marcados) — o formato é o do seletor
    // do header (default JSON).
    await page.getByRole('button', { name: /Exportar/ }).first().click();
    await page.getByRole('menuitem', { name: /Exportar seleção/ }).click();

    // Modal de progresso: abre, conclui e é fechado pelo usuário.
    const progress = dialog(page);
    await expect(progress).toBeVisible({ timeout: 30_000 });
    await expect(progress.getByText('Processamento concluído.')).toBeVisible({
      timeout: 90_000,
    });
    await expect(progress.getByText(/arquivo\(s\)/)).toBeVisible();
    await progress.getByRole('button', { name: 'Fechar' }).click();
    await expect(progress).toBeHidden();

    // Artefatos novos sob mods/edits (o destino de todo export).
    const newArtifacts = changedSince(modsDir, before).filter((rel) =>
      rel.startsWith('edits/')
    );
    expect(
      newArtifacts.length,
      'export gravou em mods/edits'
    ).toBeGreaterThan(0);
  });

  test('import do lote mods/translated regrava o binário em mods/', async ({
    page,
  }) => {
    test.skip(
      !hasMods || !fs.existsSync(path.join(modsDir, 'translated')),
      'mods/translated não existe — lote de import indisponível'
    );

    await gotoApp(page);

    // Importar ▾ — estreita o escopo para Textos de batalha (lote pequeno e
    // determinístico) e dispara a prévia.
    await page.getByRole('button', { name: /Importar/ }).first().click();
    const menu = page.getByRole('menu');
    for (const kind of ['Eventos', 'Sistema', 'Dicionário', 'Loc Kit', 'Painel de ajuda', 'Cloud', 'Texto menu principal']) {
      const item = menu.getByRole('menuitemcheckbox', { name: kind });
      if (await item.count()) {
        await item.first().click();
      }
    }
    await menu.getByRole('menuitem', { name: 'Importar mods/translated' }).click();

    // Diálogo de resumo do lote.
    const importDialog = dialog(page);
    await expect(
      importDialog.getByText('Importar textos de mods/translated')
    ).toBeVisible({ timeout: 60_000 });

    const before = snapshotMtimes(modsDir);
    const confirm = importDialog.getByRole('button', { name: /^Importar$/ });

    // Sem candidatos a aplicar: o diálogo é o próprio teste (exibição do
    // resumo) — nada é gravado.
    const body = await importDialog.textContent();
    if (!body || !/\d+\s+texto\(s\)\s+alterado\(s\)/.test(body) || /0\s+texto\(s\)\s+alterado\(s\)/.test(body)) {
      test.info().annotations.push({
        type: 'skip',
        description: 'lote traduzido sem textos diferentes de mods/ — apply não exercitado',
      });
      await importDialog.getByRole('button', { name: 'Cancelar' }).click();
      return;
    }

    await confirm.click();
    await expect(toast(page, 'Importação concluída')).toBeVisible({
      timeout: 120_000,
    });

    // Binários regravados na árvore mods/ (fora de edits/translated).
    const changed = changedSince(modsDir, before).filter(
      (rel) => !rel.startsWith('edits/') && !rel.startsWith('translated/')
    );
    expect(changed.length, 'import regravou binários em mods/').toBeGreaterThan(0);
  });

  test('sem estouro: dialog de edição com texto longo contido na viewport', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 500 });
    await gotoApp(page);
    await openFirstTextEntry(page);

    // A row com o maior texto original: a pior caso de estouro do editor.
    const originals = await columnTexts(page, 'Original');
    const longest = originals.reduce(
      (best, t, i) => (t.length > originals[best].length ? i : best),
      0
    );
    await page.locator('[data-row-key]').nth(longest).locator('.translated-cell').click();
    const editorDialog = dialog(page);
    await expect(editorDialog.getByText(/Linha #\d+/)).toBeVisible();

    // Digita um texto longo por cima do existente — o editor precisa
    // rolar DENTRO do diálogo, sem esticar a página.
    await editorDialog.locator('.tiptap').click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('texto longo '.repeat(60), { delay: 5 });
    await expectDialogWithinViewport(page);
    await expectNoHorizontalOverflow(page);

    // Descarta: o rascunho não pode vazar para o fixture (sem Salvar).
    await editorDialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(editorDialog).toBeHidden();
  });
});

/** Restaura só os arquivos mudados a partir do backup integral. */
async function restoreChanged(
  backup: TreeBackup | null,
  relPaths: string[]
): Promise<void> {
  if (!backup || relPaths.length === 0) return;
  for (const rel of relPaths) {
    const from = path.join(backup.dir, rel);
    const to = path.join(backup.target, rel);
    fs.mkdirSync(path.dirname(to), { recursive: true });
    await withRetry(() => fs.copyFileSync(from, to));
  }
}
