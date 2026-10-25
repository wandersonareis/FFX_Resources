import { expect, test } from '@playwright/test';
import { gotoApp, switchToTab, waitForKindRoot } from '../helpers/ui';
import { hasData, hasDataFFX2, modsDir } from '../helpers/fixtures';
import {
  backupTree,
  restoreTree,
  type TreeBackup,
} from '../helpers/backup';
import {
  allLeafIdsOfKind,
  artifactEntryIds,
  artifactUntouched,
  changedArtifacts,
  changedTreeArtifacts,
  checkKindRoot,
  contextMenuExport,
  countLeavesOfKind,
  expandTreeNode,
  exportAllTypes,
  exportScopeSelection,
  firstLeafId,
  firstLeafOfKind,
  nodeCheckbox,
  openExportMenu,
  setExportFormat,
  setScope,
  snapshotArtifacts,
  TREE_PREFIX,
  waitExportDone,
} from '../helpers/export';

/**
 * Os TIPOS de export do app — três caminhos, uma só semântica cada:
 *
 *  1. popover Exportar → "todos os tipos": todos os kinds da versão,
 *     inteiros, nomes canônicos (`events_all_localizations_<v>`…);
 *  2. popover Exportar → "Exportar seleção" (subtítulo "tipos marcados"):
 *     só os tipos marcados no ESCOPO, cada um por inteiro. A marcação da
 *     árvore NÃO entra aqui — é o que o usuário espera quando quer
 *     atualizar só o export de um kind;
 *  3. menu de contexto da árvore → Exportar: o que está MARCADO na
 *     árvore, do kind do nó clicado. Parcial = um arquivo por entrada;
 *     kind inteiro (checkbox da raiz) = artefato bulk ÚNICO com prefixo
 *     `tree_`, que NÃO sobrescreve o canônico.
 *
 * Todos escrevem em <game>/mods/edits; o import reconhece o kind pelo
 * metadata.key do conteúdo (não pelo nome), por isso o prefixo tree_ é
 * cosmético — coberto pelo round-trip em
 * backend/services/export_tree_selection_test.go.
 *
 * O fixture mods/ é copiado antes e restaurado depois: os artefatos
 * gravados aqui são resultado real do teste.
 */
test.describe('Exportar: tipos de export', () => {
  test.skip(!hasData, 'data/ não existe — sem texto para exportar');

  let modsBackup: TreeBackup | null = null;

  test.beforeAll(() => {
    modsBackup = backupTree(modsDir, 'mods');
  });

  test.afterAll(async () => {
    if (modsBackup) await restoreTree(modsBackup);
  });

  // ---- árvore: o que é marcável ------------------------------------------
  test.describe('Árvore: marcação por kind', () => {
    test('Texto e Imagens não têm checkbox; raiz de kind de texto tem', async ({
      page,
    }) => {
      await gotoApp(page);
      await waitForKindRoot(page, 'events');

      // `Texto` é só agrupamento visual (sem kind): nunca selecionável.
      await expect(
        page.locator('[data-node-id="kind:text"]').getByRole('checkbox')
      ).toHaveCount(0);
      // Imagens segue pela marcação de folha/grupo (extrair/deletar).
      await expect(
        page.locator('[data-node-id="kind:images"]').getByRole('checkbox')
      ).toHaveCount(0);
      // Raiz de kind de TEXTO tem checkbox tri-state.
      await expect(
        page.locator('[data-node-id="kind:events"]').getByRole('checkbox')
      ).toHaveCount(1);
    });

    test('checkbox da raiz do kind é tri-state: todos / parcial / nenhum', async ({
      page,
    }) => {
      await gotoApp(page);
      await waitForKindRoot(page, 'battletext');
      const root = 'kind:battletext';
      const box = nodeCheckbox(page, root);
      await expect(box).toBeVisible({ timeout: 60_000 });

      // Marca o kind inteiro.
      await box.click();
      await expect(box).toHaveAttribute('data-state', 'checked');

      // Desmarca UMA folha: a raiz vira parcial.
      await expandTreeNode(page, root);
      const leaf = firstLeafOfKind(page, 'battletext');
      await expect(leaf).toBeVisible({ timeout: 60_000 });
      await leaf.getByRole('checkbox').click();
      await expect(box).toHaveAttribute('data-state', 'indeterminate');

      // Clique no indeterminado marca tudo de novo (regra do Radix).
      await box.click();
      await expect(box).toHaveAttribute('data-state', 'checked');

      // Desmarca a raiz: nada.
      await box.click();
      await expect(box).toHaveAttribute('data-state', 'unchecked');
    });
  });

  // ---- caminho 2: popover Exportar seleção = ESCOPO ----------------------
  test.describe('Popover: Exportar seleção respeita o escopo', () => {
    test('escopo com um tipo exporta só ele, pelo nome canônico', async ({
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
      expect(
        changedTreeArtifacts(before),
        'o escopo nunca gera artefato tree_ (isso é da árvore)'
      ).toEqual([]);
    });

    test('escopo ignora a marcação da árvore', async ({ page }) => {
      test.setTimeout(600_000);
      await gotoApp(page);
      await setExportFormat(page, 'JSON');

      // Marca uma folha de OUTRO kind na árvore: ela não pode vazar para
      // o export do escopo (só Eventos).
      await waitForKindRoot(page, 'battletext');
      await expandTreeNode(page, 'kind:battletext');
      const leaf = firstLeafOfKind(page, 'battletext');
      await expect(leaf).toBeVisible({ timeout: 60_000 });
      await leaf.getByRole('checkbox').click();

      await setScope(page, ['events']);
      const before = snapshotArtifacts();
      await exportScopeSelection(page);
      await waitExportDone(page, 600_000);

      const changed = changedArtifacts(before);
      expect(changed, 'exatamente um artefato').toEqual([
        'events_all_localizations_ffx.json',
      ]);
      // A folha marcada em battletext não gerou artefato por entrada: nada
      // de battletext foi criado ou regravado por este export.
      expect(
        artifactUntouched('battletext_all_localizations_ffx.json', before),
        'marcação da árvore não entra no escopo'
      ).toBe(true);
      expect(
        changedTreeArtifacts(before),
        'marcação da árvore não entra no escopo'
      ).toEqual([]);
    });

    test('menu Exportar: itens, escopo e subtítulo do item de seleção', async ({
      page,
    }) => {
      await gotoApp(page);
      const menu = await openExportMenu(page);
      await expect(
        menu.getByRole('menuitem', { name: /Exportar todos os tipos/ })
      ).toBeVisible();
      await expect(menu.getByText('Escopo')).toBeVisible();
      await expect(
        menu.getByRole('menuitemcheckbox', { name: 'Eventos' })
      ).toBeVisible();
      // O subtítulo deixa claro que é o ESCOPO, não a árvore.
      await expect(
        menu.getByRole('menuitem', { name: /Exportar seleção tipos marcados/ })
      ).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(menu).toBeHidden();
    });
  });

  // ---- caminho 3: menu de contexto da árvore ------------------------------
  test.describe('Árvore: menu de contexto exporta a seleção marcada', () => {
    test('seleção parcial: um arquivo por entrada', async ({ page }) => {
      test.setTimeout(600_000);
      await gotoApp(page);
      await setExportFormat(page, 'JSON');
      await waitForKindRoot(page, 'battletext');
      await expandTreeNode(page, 'kind:battletext');
      const leaf = firstLeafOfKind(page, 'battletext');
      await expect(leaf).toBeVisible({ timeout: 60_000 });
      await leaf.getByRole('checkbox').click();
      const id = await firstLeafId(page, 'battletext');

      const before = snapshotArtifacts();
      await contextMenuExport(page, 'kind:battletext');
      await waitExportDone(page, 600_000);

      const changed = changedArtifacts(before);
      expect(changed, 'um artefato por entrada marcada').toEqual([
        `${id}_all_localizations_ffx.json`,
      ]);
      expect(
        changedTreeArtifacts(before),
        'seleção parcial não gera artefato tree_'
      ).toEqual([]);
    });

    test('seleção completa: um artefato tree_, canônico intacto', async ({
      page,
    }) => {
      test.setTimeout(600_000);
      await gotoApp(page);
      await setExportFormat(page, 'JSON');
      await waitForKindRoot(page, 'battletext');
      await checkKindRoot(page, 'battletext', true);
      await expect(nodeCheckbox(page, 'kind:battletext')).toHaveAttribute(
        'data-state',
        'checked'
      );

      const before = snapshotArtifacts();
      // A árvore nasce fechada: expande a raiz para contar as folhas que
      // ela serve (o universo do kind).
      await expandTreeNode(page, 'kind:battletext');
      const leafCount = await countLeavesOfKind(page, 'battletext');
      await contextMenuExport(page, 'kind:battletext');
      await waitExportDone(page, 600_000);

      const changed = changedArtifacts(before);
      const treeFile = `${TREE_PREFIX}battletext_all_localizations_ffx.json`;
      expect(changed, `o artefato único é ${treeFile}`).toEqual([treeFile]);
      expect(changedTreeArtifacts(before)).toEqual([treeFile]);

      // O canônico NÃO foi tocado: o prefixo existe para não colidir.
      expect(
        artifactUntouched('battletext_all_localizations_ffx.json', before),
        'o artefato canônico não é sobrescrito pela seleção da árvore'
      ).toBe(true);

      // Conteúdo: o universo de entradas do tree_ é o do kind inteiro —
      // a marcação cobriu tudo que a árvore serve.
      expect(leafCount, 'árvore com folhas para comparar').toBeGreaterThan(0);
      const ids = artifactEntryIds(treeFile);
      expect(ids.length).toBe(leafCount);
    });

    test('marcar tudo e desmarcar uma folha volta a ser parcial', async ({
      page,
    }) => {
      test.setTimeout(600_000);
      await gotoApp(page);
      await setExportFormat(page, 'JSON');
      await waitForKindRoot(page, 'battletext');
      await checkKindRoot(page, 'battletext', true);
      await expandTreeNode(page, 'kind:battletext');
      const leaf = firstLeafOfKind(page, 'battletext');
      await expect(leaf).toBeVisible({ timeout: 60_000 });
      await leaf.getByRole('checkbox').click();
      await expect(nodeCheckbox(page, 'kind:battletext')).toHaveAttribute(
        'data-state',
        'indeterminate'
      );

      // O desmarcado sai do lote; as demais folhas continuam marcadas.
      const unmarked = await firstLeafId(page, 'battletext');
      const allLeafIds = await allLeafIdsOfKind(page, 'battletext');
      const expected = allLeafIds
        .filter((id) => id !== unmarked)
        .map((id) => `${id}_all_localizations_ffx.json`)
        .sort();

      const before = snapshotArtifacts();
      await contextMenuExport(page, 'kind:battletext');
      await waitExportDone(page, 600_000);

      expect(
        changedTreeArtifacts(before),
        'seleção parcial (indeterminada) não deduplica'
      ).toEqual([]);
      expect(changedArtifacts(before)).toEqual(expected);
    });
  });

  // ---- formato por tipo de export ----------------------------------------
  test.describe('Formato: extensão do artefato', () => {
    const formats: Array<[string, string]> = [
      ['JSON', '.json'],
      ['Strings', '.strings'],
      ['CSV', '.csv'],
      ['XLSX (Excel)', '.xlsx'],
    ];
    for (const [label, ext] of formats) {
      test(`${label} grava ${ext}`, async ({ page }) => {
        test.setTimeout(600_000);
        await gotoApp(page);
        await setExportFormat(page, label);
        await waitForKindRoot(page, 'battletext');
        await expandTreeNode(page, 'kind:battletext');
        const leaf = firstLeafOfKind(page, 'battletext');
        await expect(leaf).toBeVisible({ timeout: 60_000 });
        await leaf.getByRole('checkbox').click();
        const id = await firstLeafId(page, 'battletext');

        const before = snapshotArtifacts();
        await contextMenuExport(page, 'kind:battletext');
        await waitExportDone(page, 600_000);

        const expected = `${id}_all_localizations_ffx${ext}`;
        expect(changedArtifacts(before)).toContain(expected);
      });
    }
  });

  // ---- caminho 1: popover Exportar tudo ----------------------------------
  test.describe('Popover: Exportar todos os tipos', () => {
    test('nunca gera artefato tree_ (nome canônico por kind)', async ({
      page,
    }) => {
      test.setTimeout(900_000);
      await gotoApp(page);
      await setExportFormat(page, 'JSON');

      const before = snapshotArtifacts();
      await exportAllTypes(page);
      await waitExportDone(page, 600_000);

      const changed = changedArtifacts(before);
      expect(changed.length, 'vários artefatos gravados').toBeGreaterThan(1);
      for (const expected of [
        'events_all_localizations_ffx.json',
        'battletext_all_localizations_ffx.json',
        'objects_all_localizations_ffx.json',
        'cloudsave_all_localizations_ffx.json',
      ]) {
        expect(changed, `artefato ${expected}`).toContain(expected);
      }
      expect(
        changedTreeArtifacts(before),
        'o caminho "todos os tipos" não usa o prefixo da árvore'
      ).toEqual([]);
    });
  });

  // ---- caminho 2 por versão (ffx2: Tutorial é exclusivo de lá) -----------
  test.describe('FFX-2: escopo com Tutorial', () => {
    test.skip(!hasDataFFX2, 'ffx2_data/ não existe');

    test('Tutorial marcado no escopo exporta o canônico tuto', async ({
      page,
    }) => {
      test.setTimeout(600_000);
      await gotoApp(page);
      await switchToTab(page, 'ffx2');
      await setExportFormat(page, 'JSON');
      await setScope(page, ['tutorial']);

      const before = snapshotArtifacts();
      await exportScopeSelection(page);
      await waitExportDone(page, 600_000);

      expect(changedArtifacts(before)).toEqual([
        'tuto_all_localizations_ffx2.json',
      ]);
      expect(changedTreeArtifacts(before)).toEqual([]);
    });
  });
});
