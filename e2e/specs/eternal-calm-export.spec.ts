import { expect, test } from '@playwright/test';
import {
  closeDialogByX,
  dialog,
  expectDialogWithinViewport,
  gotoApp,
  revealFirstTextLeaf,
  switchToTab,
} from '../helpers/ui';
import { gameFiles, hasData, modsDir } from '../helpers/fixtures';
import {
  backupTree,
  restoreTree,
  type TreeBackup,
} from '../helpers/backup';
import {
  artifactEntryIds,
  changedArtifacts,
  changedTreeArtifacts,
  contextMenuExport,
  exportScopeSelection,
  exportAllTypes,
  nodeCheckbox,
  openExportMenu,
  setExportFormat,
  setScope,
  snapshotArtifacts,
  waitExportDone,
} from '../helpers/export';

/**
 * A aba FFX Eternal Calm no export. Ela é uma EXPANSÃO do FFX: divide a
 * mesma árvore de dados (ffx_ps2/ffx/master) e serve UM kind só — Eventos,
 * e só o grupo "sc" (sc/scene*). O contrato tem duas pontas:
 *
 *  1. o export da aba grava somente os arquivos do Eternal Calm. Antes o
 *     export bulk varria o bucket inteiro do datastore (compartilhado com
 *     o FFX) e a aba escrevia TODOS os eventos do FFX;
 *  2. o inverso: os eventos do Eternal Calm não aparecem no export da aba
 *     FFX (mesma régua da listagem da árvore).
 *
 * E a terceira ponta, de layout: com UM kind só, o destino do modal de
 * conclusão é o CAMINHO COMPLETO do artefato (o resumo "N arquivos em
 * <pasta>" só aparece quando sai mais de um). É uma palavra longa, sem
 * espaços, que não quebra linha — e estourava a largura do modal. As
 * demais abas exportam vários kinds e por isso nunca viam o problema.
 *
 * O fixture mods/ é copiado antes e restaurado depois: os artefatos
 * gravados aqui são resultado real do teste.
 */
test.describe('FFX Eternal Calm: export', () => {
  test.skip(!hasData, 'data/ não existe — sem texto para exportar');

  let modsBackup: TreeBackup | null = null;

  test.beforeAll(() => {
    modsBackup = backupTree(modsDir, 'mods');
  });

  test.afterAll(async () => {
    if (modsBackup) await restoreTree(modsBackup);
  });

  test('escopo da aba tem apenas Eventos', async ({ page }) => {
    await gotoApp(page);
    await switchToTab(page, 'eternalcalm');

    const menu = await openExportMenu(page);
    // A aba é uma expansão: um kind só, e por isso um checkbox só.
    const boxes = menu.getByRole('menuitemcheckbox');
    await expect(boxes).toHaveCount(1);
    await expect(boxes.first()).toHaveText('Eventos');
    await page.keyboard.press('Escape');
    await expect(menu).toBeHidden();
  });

  test('exporta apenas os arquivos do Eternal Calm', async ({ page }) => {
    test.setTimeout(600_000);
    await gotoApp(page);
    await switchToTab(page, 'eternalcalm');
    await setExportFormat(page, 'JSON');
    await setScope(page, ['events']);

    const before = snapshotArtifacts();
    await exportScopeSelection(page);
    await waitExportDone(page, 600_000);

    const changed = changedArtifacts(before);
    expect(changed, 'exatamente um artefato').toEqual([
      'events_all_localizations_eternalcalm.json',
    ]);
    expect(
      changedTreeArtifacts(before),
      'o escopo nunca gera artefato tree_'
    ).toEqual([]);

    // O conteúdo é do grupo "sc" — os arquivos do Eternal Calm. Nenhum
    // evento do FFX pode ter vazado para o artefato da expansão.
    const ids = artifactEntryIds(changed[0]);
    expect(ids.length, 'artefato com entradas').toBeGreaterThan(0);
    const leaked = ids.filter((id) => !id.startsWith('sc'));
    expect(leaked, 'nenhum evento de fora do Eternal Calm').toEqual([]);
  });

  test('eventos do Eternal Calm não vazam para o export do FFX', async ({
    page,
  }) => {
    test.setTimeout(600_000);
    await gotoApp(page);
    await setExportFormat(page, 'JSON');
    await setScope(page, ['events']);

    const before = snapshotArtifacts();
    await exportScopeSelection(page);
    await waitExportDone(page, 600_000);

    const changed = changedArtifacts(before);
    expect(changed, 'exatamente um artefato').toEqual([
      'events_all_localizations_ffx.json',
    ]);

    const ids = artifactEntryIds(changed[0]);
    expect(ids.length, 'artefato com entradas').toBeGreaterThan(0);
    const leaked = ids.filter((id) => id.startsWith('sc'));
    expect(leaked, 'nenhum evento do Eternal Calm no FFX').toEqual([]);
  });

  test('menu de contexto da árvore exporta o evento marcado', async ({
    page,
  }) => {
    test.setTimeout(600_000);
    await gotoApp(page);
    await switchToTab(page, 'eternalcalm');
    await setExportFormat(page, 'JSON');

    // A árvore da aba é só o grupo Eternal Calm (sc): desce o kind e o
    // grupo até achar folhas (os dois nascem fechados).
    const leaf = await revealFirstTextLeaf(page);
    const nodeId = (await leaf.getAttribute('data-node-id')) ?? '';
    const id = nodeId.slice('leaf:events:'.length);
    expect(
      nodeId.startsWith('leaf:events:'),
      `folha de events: ${nodeId}`
    ).toBe(true);
    expect(id.startsWith('sc'), `folha da aba é do grupo sc: ${id}`).toBe(
      true
    );

    // O menu de contexto exporta o que está MARCADO na árvore: marca a
    // folha antes de clicar com o botão direito.
    const box = nodeCheckbox(page, nodeId);
    await expect(box).toBeVisible({ timeout: 60_000 });
    await box.click();
    await expect(box).toBeChecked();

    const before = snapshotArtifacts();
    await contextMenuExport(page, nodeId);
    await waitExportDone(page, 600_000);

    const changed = changedArtifacts(before);
    // Events por entrada carrega o prefixo "event_" (regra do
    // eventsExportPath) — diferente dos kinds de tabela, que usam <id>_.
    expect(changed, 'um artefato por entrada marcada').toEqual([
      `event_${id}_all_localizations_eternalcalm.json`,
    ]);
    expect(changedTreeArtifacts(before)).toEqual([]);
  });

  /**
   * O modal de conclusão com UM kind exportado mostra o caminho completo
   * do artefato (o único caso em que o resumo não é "N arquivos em
   * <pasta>"). É uma palavra longa sem espaços: se ela não quebrar linha,
   * a coluna do grid do diálogo estoura o sm:max-w-md do chamador e o
   * conteúdo escapa da tela. Nas demais abas isso nunca aparece.
   */
  test('modal de conclusão não estoura a viewport com um arquivo só', async ({
    page,
  }) => {
    test.setTimeout(600_000);
    test.info().annotations.push({
      type: 'fixture',
      description: `gameFiles=${gameFiles} mods=${modsDir}`,
    });

    await gotoApp(page);
    await switchToTab(page, 'eternalcalm');
    await setExportFormat(page, 'JSON');
    await setScope(page, ['events']);

    await exportScopeSelection(page);

    const progress = dialog(page);
    await expect(progress).toBeVisible({ timeout: 30_000 });
    await expect(progress.getByText('Processamento concluído.')).toBeVisible({
      timeout: 600_000,
    });

    // 1. a caixa inteira cabe na tela.
    await expectDialogWithinViewport(page);

    // 2. a linha do destino NÃO escapa da largura do modal: ela quebra em
    //    vez de empurrar a coluna do grid.
    const destination = progress.getByText(/arquivo\(s\)/);
    await expect(destination).toBeVisible();

    const modalBox = await progress.boundingBox();
    const lineBox = await destination.boundingBox();
    expect(modalBox, 'modal com caixa').toBeTruthy();
    expect(lineBox, 'linha do destino com caixa').toBeTruthy();
    expect(
      lineBox!.x + lineBox!.width,
      'linha do destino não passa da borda direita do modal'
    ).toBeLessThanOrEqual(modalBox!.x + modalBox!.width + 1);

    // 3. e o modal em si não rola horizontalmente (o grid não estourou).
    const scroll = await progress.evaluate((el) => ({
      scrollWidth: el.scrollWidth,
      clientWidth: el.clientWidth,
    }));
    expect(
      scroll.scrollWidth,
      'modal sem estouro horizontal'
    ).toBeLessThanOrEqual(scroll.clientWidth + 1);

    // 4. o caminho realmente quebrou em mais de uma linha: medir o
    //    elemento não basta (um <p> tem UMA caixa por mais grande que
    //    esteja) — o Range sobre o conteúdo devolve um rect por LINHA.
    //    E a garantia de que a quebra é a do CSS (wrap-anywhere), e não
    //    um acerto de tamanho: overflow-wrap declarado no estilo
    //    computado.
    const lines = await destination.evaluate((el) => {
      const range = document.createRange();
      range.selectNodeContents(el);
      return {
        rects: range.getClientRects().length,
        overflowWrap: getComputedStyle(el).overflowWrap,
      };
    });
    expect(
      lines.overflowWrap,
      'destino com overflow-wrap: anywhere'
    ).toBe('anywhere');
    expect(lines.rects, 'destino quebrou em múltiplas linhas').toBeGreaterThan(
      1
    );

    // 5. a página inteira não ganhou rolagem horizontal.
    const docScroll = await page.evaluate(() => ({
      w: document.documentElement.scrollWidth,
      c: document.documentElement.clientWidth,
    }));
    expect(docScroll.w, 'documento sem rolagem horizontal').toBeLessThanOrEqual(
      docScroll.c + 1
    );

    await closeDialogByX(page);
    await expect(progress).toBeHidden();
  });

  /**
   * O caminho "todos os tipos" da aba é o mesmo (um kind só), então o
   * mesmo destino completo aparece — a garantia de layout vale para os
   * dois botões do popover.
   */
  test('modal de conclusão do "todos os tipos" também não estoura', async ({
    page,
  }) => {
    test.setTimeout(600_000);
    await gotoApp(page);
    await switchToTab(page, 'eternalcalm');
    await setExportFormat(page, 'JSON');

    await exportAllTypes(page);

    const progress = dialog(page);
    await expect(progress).toBeVisible({ timeout: 30_000 });
    await expect(progress.getByText('Processamento concluído.')).toBeVisible({
      timeout: 600_000,
    });
    await expectDialogWithinViewport(page);

    const scroll = await progress.evaluate((el) => ({
      scrollWidth: el.scrollWidth,
      clientWidth: el.clientWidth,
    }));
    expect(
      scroll.scrollWidth,
      'modal sem estouro horizontal'
    ).toBeLessThanOrEqual(scroll.clientWidth + 1);

    await closeDialogByX(page);
    await expect(progress).toBeHidden();
  });
});
