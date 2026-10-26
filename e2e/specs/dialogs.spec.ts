import { expect, test } from '@playwright/test';
import {
  columnTexts,
  dialog,
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  gotoApp,
  openFirstImageEntry,
  openFirstTextEntry,
} from '../helpers/ui';
import { hasData, hasImages } from '../helpers/fixtures';

/**
 * Todos os DIÁLOGOS do app — abrir, conteúdo mínimo obrigatório, SEM
 * ESTOURO (contidos na viewport; a página nunca rola por causa deles) e
 * fechar sem efeito colateral.
 *
 * Menus nativos (seletores de arquivo/pasta do SO) não existem no modo
 * servidor: os fluxos que dependem deles terminam em cancelamento limpo —
 * e é isso que os testes verificam. O menu do appcore é backend puro, fora
 * do alcance do navegador (nada a testar aqui).
 */
test.describe('Diálogos', () => {
  test.skip(!hasData, 'data/ não existe — sem conteúdo para os diálogos');

  // ---- busca global (Ctrl+K) ----------------------------------------------
  test.describe('Busca global (Ctrl+K)', () => {
    test('abre, busca por nome de arquivo e por texto inexistente', async ({
      page,
    }) => {
      await gotoApp(page);
      const label = await openFirstTextEntry(page);

      await page.keyboard.press('Control+k');
      const search = dialog(page);
      await expect(search).toBeVisible();
      await expect(search.getByRole('textbox')).toBeFocused();
      await expectDialogWithinViewport(page);

      // Busca por NOME: o arquivo aberto é achado pelo próprio label.
      const input = search.getByRole('textbox');
      await input.fill(label.slice(0, Math.max(4, Math.min(label.length, 8))));
      await expect(search.getByText(/arquivo\(s\) por nome/)).toBeVisible({
        timeout: 60_000,
      });

      // Termo inexistente: resultado vazio explícito (não é silêncio).
      await input.fill('zzqqxxww');
      await expect(search.getByText('Nenhum resultado.')).toBeVisible({
        timeout: 60_000,
      });

      await page.keyboard.press('Escape');
      await expect(search).toBeHidden();
      await expectNoHorizontalOverflow(page);
    });
  });

  // ---- busca & substituição no arquivo (Ctrl+F) ---------------------------
  test.describe('Busca no arquivo (Ctrl+F)', () => {
    test('abre com campo e opções; destaca matches; substituição é toggle; fecha', async ({
      page,
    }) => {
      await gotoApp(page);
      await openFirstTextEntry(page);

      await page.keyboard.press('Control+f');
      // O input da barra e o BOTÃO da TableBar compartilham o aria-label:
      // distingue pelo papel (textbox).
      const find = page.getByRole('textbox', { name: 'Buscar neste arquivo' });
      await expect(find).toBeVisible();

      // Opções da barra.
      await expect(
        page.getByRole('button', { name: 'Diferenciar maiúsculas e minúsculas' })
      ).toBeVisible();
      await expect(
        page.getByRole('button', { name: 'Casar palavra inteira' })
      ).toBeVisible();

      // Digita uma palavra que EXISTE na tabela: destaque <mark>. O find
      // casa contra o texto TRADUZIDO exibido (é nele que os <mark> são
      // pintados) — a palavra vem da própria coluna Traduzido, sem tags.
      const translatedTexts = await columnTexts(page, 'Traduzido');
      const words = translatedTexts.flatMap((t) =>
        t.replace(/\{[^}]*\}/g, ' ').split(/\s+/)
      );
      const word =
        words.find((w) => w.length >= 2 && /^[A-Za-zÀ-ÿ0-9]+$/.test(w))?.slice(
          0,
          12
        ) ?? 'the';
      await find.fill(word);
      await expect(page.locator('mark').first()).toBeVisible({ timeout: 30_000 });

      // Substituição escondida por padrão; o toggle a revela.
      await expect(
        page.getByRole('textbox', { name: 'Texto de substituição' })
      ).toBeHidden();
      await page.getByRole('button', { name: 'Mostrar substituição' }).click();
      await expect(
        page.getByRole('textbox', { name: 'Texto de substituição' })
      ).toBeVisible();
      await expect(page.getByPlaceholder('Substituir por…')).toBeVisible();

      await page.getByRole('button', { name: 'Fechar a busca' }).click();
      await expect(
        page.getByRole('textbox', { name: 'Buscar neste arquivo' })
      ).toBeHidden();
      await expectNoHorizontalOverflow(page);
    });
  });

  // ---- menus Exportar/Importar do header ----------------------------------
  test('menu Exportar: escopos por tipo e exportar seleção', async ({ page }) => {
    await gotoApp(page);
    await page.getByRole('button', { name: /^Exportar/ }).click();
    const menu = page.getByRole('menu');
    await expect(menu.getByRole('menuitem', { name: /Exportar todos os tipos/ })).toBeVisible();
    await expect(menu.getByText('Escopo')).toBeVisible();
    await expect(
      menu.getByRole('menuitemcheckbox', { name: 'Eventos' })
    ).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: /Exportar seleção/ })).toBeVisible();
    // O subtítulo é do ESCOPO (tipos marcados), não da seleção da árvore.
    await expect(menu.getByRole('menuitem', { name: /tipos marcados/ })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(menu).toBeHidden();
  });

  test('menu Importar: tipos, lote translated e importar de arquivo', async ({ page }) => {
    await gotoApp(page);
    await page.getByRole('button', { name: /^Importar/ }).click();
    const menu = page.getByRole('menu');
    await expect(menu.getByText('Tipos')).toBeVisible();
    await expect(
      menu.getByRole('menuitemcheckbox', { name: 'Eventos' })
    ).toBeVisible();
    await expect(
      menu.getByRole('menuitem', { name: 'Importar mods/translated' })
    ).toBeVisible();
    await expect(
      menu.getByRole('menuitem', { name: /Importar de… arquivo/ })
    ).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(menu).toBeHidden();
  });

  test('import de arquivo no modo servidor: seletor nativo vira cancelamento limpo', async ({
    page,
  }) => {
    await gotoApp(page);
    await page.getByRole('button', { name: /^Importar/ }).click();
    const menu = page.getByRole('menu');
    await menu.getByRole('menuitem', { name: /Importar de… arquivo/ }).click();

    // Nenhum diálogo de resumo (o seletor do SO não existe no servidor e
    // devolve cancelamento) — e nenhum crash: o app segue de pé.
    await expect(dialog(page)).toHaveCount(0);
    await expect(page.getByText('Conteúdo').first()).toBeVisible();
  });

  test('diálogo do lote mods/translated abre com o resumo e cancela', async ({ page }) => {
    await gotoApp(page);
    await page.getByRole('button', { name: /^Importar/ }).click();
    await page.getByRole('menu').getByRole('menuitem', {
      name: 'Importar mods/translated',
    }).click();

    const importDialog = dialog(page);
    await expect(
      importDialog.getByText('Importar textos de mods/translated')
    ).toBeVisible({ timeout: 60_000 });
    // Sem estouro em janela baixa — listas de aviso podem ser longas.
    await expectDialogWithinViewport(page);

    await importDialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(importDialog).toBeHidden();
  });

  // ---- editor de tradução: cancelar não suja o app -------------------------
  test('editor de linha: Cancelar descarta o rascunho (Salvar segue desabilitado)', async ({
    page,
  }) => {
    await gotoApp(page);
    await openFirstTextEntry(page);

    const save = page.getByRole('button', { name: 'Salvar', exact: true });
    await expect(save).toBeDisabled();

    await page.locator('[data-row-key]').first().locator('.translated-cell').click();
    const editorDialog = dialog(page);
    await expect(editorDialog.getByText(/Linha #\d+/)).toBeVisible();
    await editorDialog.locator('.tiptap').click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('rascunho descartado');
    await editorDialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(editorDialog).toBeHidden();

    // Nada ficou sujo: o botão Salvar do app continua desabilitado.
    await expect(save).toBeDisabled();
  });

  // ---- painel de imagem + seus diálogos (data/ com texturas) --------------
  test.describe('Painel de imagem', () => {
    test.skip(!hasImages, 'sem .dds.phyre em data/ — painel de imagem pulado');

    test('diálogo de extração (.dds + .png em mods/images)', async ({ page }) => {
      await gotoApp(page);
      const label = await openFirstImageEntry(page);

      // Menu de contexto na folha de imagem da árvore: Exportar → diálogo.
      const leaf = page
        .locator('[data-node-kind="images"][data-node-entry="true"]')
        .first();
      await leaf.click({ button: 'right' });
      await page.getByRole('menu').getByRole('menuitem', { name: 'Exportar' }).click();

      const extractDialog = dialog(page);
      await expect(extractDialog.getByText(`Extrair ${label}?`)).toBeVisible({
        timeout: 60_000,
      });
      await expect(
        extractDialog.getByText(/Gera \.dds e \.png em mods\/images/)
      ).toBeVisible();
      await expectDialogWithinViewport(page);
      await extractDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(extractDialog).toBeHidden();
    });

    test('diálogo de exclusão (escopos data/mods/ambos + cópias)', async ({ page }) => {
      await gotoApp(page);
      const label = await openFirstImageEntry(page);

      const leaf = page
        .locator('[data-node-kind="images"][data-node-entry="true"]')
        .first();
      await leaf.click({ button: 'right' });
      await page.getByRole('menu').getByRole('menuitem', { name: 'Deletar' }).click();

      const deleteDialog = dialog(page);
      await expect(deleteDialog.getByText(`Deletar ${label}?`)).toBeVisible({
        timeout: 60_000,
      });
      await expect(deleteDialog.getByText('Irreversível')).toBeVisible();

      // Escopos de exclusão — a ação é EXPLÍCITA, nunca cascata.
      await expect(deleteDialog.getByText('Ambos (data/ e mods/)')).toBeVisible();
      await expect(deleteDialog.getByText('Só o original (data/)')).toBeVisible();
      await expect(deleteDialog.getByText('Só em mods/')).toBeVisible();
      await expectDialogWithinViewport(page);

      await deleteDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(deleteDialog).toBeHidden();
    });

    test('diálogo de importar imagens traduzidas (seleção por imagem)', async ({ page }) => {
      await gotoApp(page);
      await openFirstImageEntry(page);

      // Dropdown Importar do painel → item do lote translated/images.
      await page
        .locator('main')
        .getByRole('button', { name: /^Importar/ })
        .click();
      await page
        .getByRole('menu')
        .getByRole('menuitem', { name: /Importar mods\/translated\/images/ })
        .click();

      const importDialog = dialog(page);
      await expect(
        importDialog.getByText('Importar imagens traduzidas')
      ).toBeVisible({ timeout: 60_000 });
      // O sumário lista caminhos relativos à raiz, com o "Todas" em 3 estados.
      await expect(
        importDialog.getByRole('checkbox', { name: 'Marcar todas as imagens' })
      ).toBeVisible();
      await expectDialogWithinViewport(page);
      await importDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(importDialog).toBeHidden();
    });

    test('replicar para cópias (quando há duplicatas da textura)', async ({ page }) => {
      await gotoApp(page);
      const label = await openFirstImageEntry(page);

      // O botão do painel só existe com cópias; sem cópias o caso não se
      // aplica (única imagem) e o teste registra a ausência.
      const replicate = page.getByRole('button', {
        name: /Replicar para \d+ cópia/,
      });
      if (!(await replicate.isVisible())) {
        test.info().annotations.push({
          type: 'skip',
          description: `textura ${label} sem cópias — diálogo de replicação sem alvo`,
        });
        return;
      }

      await replicate.click();
      const repDialog = dialog(page);
      await expect(repDialog.getByText(/Replicar .+ para \d+ cópia/)).toBeVisible({
        timeout: 60_000,
      });
      await expectDialogWithinViewport(page);
      await repDialog.getByRole('button', { name: 'Cancelar' }).click();
      await expect(repDialog).toBeHidden();
    });
  });

  // ---- export do formato/línguas no header --------------------------------
  test('seletores de formato e idiomas do header', async ({ page }) => {
    await gotoApp(page);

    // Formato: default JSON, alterna para Strings (Ctrl+Alt+F).
    const format = page.getByRole('button', { name: /^JSON$/ });
    await expect(format).toBeVisible();
    await page.keyboard.press('Control+Alt+f');
    await expect(page.getByRole('button', { name: /^Strings$/ })).toBeVisible();

    // Idiomas: default "Todos" com menu.
    await expect(page.getByRole('button', { name: /^Todos$/ })).toBeVisible();
    await page.getByRole('button', { name: /^Todos$/ }).click();
    const menu = page.getByRole('menu');
    await expect(menu.getByText('Idiomas')).toBeVisible();
    await page.keyboard.press('Escape');
  });
});
