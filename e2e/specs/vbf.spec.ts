import * as fs from 'node:fs';
import { expect, test, type Page } from '@playwright/test';
import * as path from 'node:path';
import {
  columnTexts,
  dialog,
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  gotoApp,
  openConfigDialog,
} from '../helpers/ui';
import {
  CONFIG_PATH,
  hasVbf,
  vbfFiles,
} from '../helpers/fixtures';
import { backupFile, restoreFile, type FileBackup } from '../helpers/backup';

/**
 * Containers .vbf — o mínimo requerido do fixture (o jogo inteiro mora no
 * container). POR EXISTÊNCIA:
 *
 *  - executável configurado e .vbf achado: as raízes aparecem na sidebar e a
 *    navegação abre TEXTO e TEXTURA direto do container (somente leitura);
 *  - executável NÃO configurado: a árvore mostra o ERRO pedindo para
 *    apontar o executável nas Configurações (com atalho de abertura);
 *  - executável configurado mas SEM .vbf ao lado: aviso neutro.
 *
 * Os dois últimos mutam o config e o DEVOLVEM pelo mesmo caminho (o diálogo
 * de Configurações) — o config.json ainda é restaurado byte a byte no fim.
 *
 * A descida no container usa o CAMINHO interno (data-node-vbf-path, em
 * minúsculas como o índice do .vbf indexa): os labels não distinguem raiz de
 * filho ("FFX_Data" e o diretório ffx_data casam o mesmo texto).
 */
test.describe('Containers .vbf', () => {
  let cfgBackup: FileBackup;
  /**
   * Executável ORIGINAL do config, lido do BACKUP do beforeAll — não do
   * snapshot de module-load: entre um e outro o app desktop pode ter
   * regravado o config, e é a versão do início DESTE spec que precisa
   * voltar.
   */
  let originalExe = '';

  test.beforeAll(() => {
    cfgBackup = backupFile(CONFIG_PATH, 'config');
    try {
      originalExe = (
        JSON.parse(fs.readFileSync(cfgBackup.copy, 'utf8')) as {
          GameExeLocation?: string;
        }
      ).GameExeLocation?.trim() ?? '';
    } catch {
      originalExe = '';
    }
  });

  test.afterAll(() => {
    restoreFile(cfgBackup);
  });

  test('raízes .vbf listadas na sidebar (FFX_Data na aba FFX)', async ({
    page,
  }) => {
    test.skip(!hasVbf, 'nenhum .vbf ao lado do executável — navegação pulada');

    await gotoApp(page);
    const section = page.getByText('Containers .vbf');
    await expect(section).toBeVisible({ timeout: 60_000 });

    const root = page.locator('[data-node-vbf="true"]').first();
    await expect(root).toBeVisible();
    // Um container por versão: a aba FFX mostra o FFX_Data (sem extensão).
    const ffxVbf = vbfFiles.find((file) => /ffx_data\.vbf$/i.test(file));
    await expect(root.locator('[data-node-button]')).toContainText(
      ffxVbf ? path.basename(ffxVbf).replace(/\.vbf$/i, '') : 'FFX_Data',
      { ignoreCase: true }
    );
  });

  test('arquivo de TEXTO do container abre em tabela somente leitura', async ({
    page,
  }) => {
    test.skip(!hasVbf, 'nenhum .vbf ao lado do executável — navegação pulada');

    await gotoApp(page);
    await openVbfTextEntry(page);

    const rows = page.locator('[data-row-key]');
    await expect(rows.first()).toBeVisible({ timeout: 60_000 });
    const originals = await columnTexts(page, 'Original');
    const withText = originals.filter((t) => t !== '' && t !== '—');
    expect(withText.length, 'linhas com texto original no container').toBeGreaterThan(0);

    // Somente leitura: nada de coluna Traduzido editável — o .vbf nunca
    // é escrito; o save das tabelas de .vbf grava em mods/.
    await expectNoHorizontalOverflow(page);
  });

  test('textura do container abre com prévia (data URL)', async ({ page }) => {
    test.skip(!hasVbf, 'nenhum .vbf ao lado do executável — navegação pulada');

    await gotoApp(page);
    await openVbfImageEntry(page);
    await expect(page.locator('img[src^="data:"]').first()).toBeVisible({
      timeout: 60_000,
    });
  });

  test('diálogo de extração do container (extrair binário em data/)', async ({
    page,
  }) => {
    test.skip(!hasVbf, 'nenhum .vbf ao lado do executável — extração pulada');

    await gotoApp(page);
    // Menu de contexto em um ARQUIVO do container: arquivo clicado sem
    // marcação já é alvo (pastas precisam ser marcadas para a subárvore).
    // O lockit é pequeno e determinístico.
    await toggleVbfRoot(page);
    for (const dir of VBF_LOCKIT_DIRS) {
      await expandVbfPath(page, dir);
    }
    const fileRow = vbfRow(
      page,
      'ffx_data/gamedata/ps3data/lockit/ffx_loc_kit_ps3_us.bin'
    );
    await fileRow.click({ button: 'right' });

    const menu = page.getByRole('menu');
    await menu
      .getByRole('menuitem', { name: 'Extrair binário em data/' })
      .click();

    const extractDialog = dialog(page);
    // O título e o botão de confirmar têm o mesmo texto: o título é o
    // heading do diálogo.
    await expect(
      extractDialog.getByRole('heading', { name: 'Confirmar extração' })
    ).toBeVisible({ timeout: 60_000 });
    await expect(extractDialog.getByText('Os binários serão extraídos')).toBeVisible();
    await expect(extractDialog.getByText('Destino')).toBeVisible();
    await expectDialogWithinViewport(page);
    // Cancelar: nada é extraído para o fixture.
    await extractDialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(extractDialog).toBeHidden();
  });

  test('sem executável configurado: erro pedindo para apontar o .vbf', async ({
    page,
  }) => {
    await gotoApp(page);

    // Limpa o executável pelo diálogo (commit vazio é válido: desliga as
    // raízes VBF).
    await openConfigDialog(page);
    const exe = dialog(page).locator('#GameExeLocation');
    await exe.fill('');
    await exe.press('Enter');
    await dialog(page).getByRole('button', { name: 'Fechar' }).click();

    // Recarrega a árvore: a descoberta roda de novo.
    await page.getByRole('button', { name: 'Recarregar' }).click();

    const alert = page.locator('[data-vbf-missing][role="alert"]');
    await expect(alert).toBeVisible({ timeout: 60_000 });
    await expect(alert).toContainText('Nenhum container .vbf');
    await expect(alert).toContainText(
      'Aponte o executável do jogo (FFX.exe ou FFX-2.exe)'
    );

    // O aviso leva direto às Configurações.
    await alert.getByRole('button', { name: 'Abrir Configurações' }).click();
    await expect(dialog(page).getByText('Configurações')).toBeVisible();
    await dialog(page).getByRole('button', { name: 'Fechar' }).click();

    await restoreGameExe(page, originalExe);
  });

  test('executável sem .vbf ao lado: aviso neutro (não é erro de config)', async ({
    page,
  }) => {
    await gotoApp(page);

    await openConfigDialog(page);
    const exe = dialog(page).locator('#GameExeLocation');
    await exe.fill('Z:\\e2e-sem-vbf\\FFX.exe');
    await exe.press('Enter');
    await dialog(page).getByRole('button', { name: 'Fechar' }).click();
    await page.getByRole('button', { name: 'Recarregar' }).click();

    const missing = page.locator('[data-vbf-missing]');
    await expect(missing).toBeVisible({ timeout: 60_000 });
    await expect(missing).toContainText('Nenhum .vbf encontrado');
    // Neutro: sem role=alert e sem botão de ação.
    await expect(page.locator('[data-vbf-missing][role="alert"]')).toHaveCount(0);

    await restoreGameExe(page, originalExe);
  });
});

// ---- navegação no container ------------------------------------------------

/** Caminhos internos até o lockit (o espelho de data/ extraído do .vbf). */
const VBF_LOCKIT_DIRS = [
  'ffx_data',
  'ffx_data/gamedata',
  'ffx_data/gamedata/ps3data',
  'ffx_data/gamedata/ps3data/lockit',
];

/** Caminhos internos até a primeira textura do evento bltz0005. */
const VBF_IMAGE_DIRS = [
  'ffx_data',
  'ffx_data/gamedata',
  'ffx_data/gamedata/ps3data',
  'ffx_data/gamedata/ps3data/event',
  'ffx_data/gamedata/ps3data/event/us',
  'ffx_data/gamedata/ps3data/event/us/obj',
  'ffx_data/gamedata/ps3data/event/us/obj/bl',
  'ffx_data/gamedata/ps3data/event/us/obj/bl/bltz0005',
  'ffx_data/gamedata/ps3data/event/us/obj/bl/bltz0005/tex',
  'ffx_data/gamedata/ps3data/event/us/obj/bl/bltz0005/tex/d3d11',
];

/** Escapa um valor para dentro de [attr="..."]. */
function attr(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

/** Linha do nó .vbf pelo caminho interno (raiz = ""). */
function vbfRow(page: Page, vbfPath: string) {
  // data-node-vbf="true" é OBRIGATÓRIO: as rows de data/ também renderizam
  // data-node-vbf-path vazio e casariam o seletor da raiz.
  return page
    .locator(`[data-node-vbf="true"][data-node-vbf-path="${attr(vbfPath)}"]`)
    .first();
}

/** Expande a raiz do container (primeira carga do índice de diretórios). */
async function toggleVbfRoot(page: Page): Promise<void> {
  const root = vbfRow(page, '');
  await expect(root).toBeVisible({ timeout: 60_000 });
  await root.locator('button[aria-label^="Alternar"]').first().click();
}

/** Expande um diretório do container pelo caminho interno. */
async function expandVbfPath(page: Page, vbfPath: string): Promise<void> {
  const row = vbfRow(page, vbfPath);
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.locator('button[aria-label^="Alternar"]').first().click();
}

/** Desce até o lockit do container e abre o binário de texto us. */
async function openVbfTextEntry(page: Page): Promise<void> {
  await toggleVbfRoot(page);
  for (const dir of VBF_LOCKIT_DIRS) {
    await expandVbfPath(page, dir);
  }
  // O lockit exibe o label amigável ("Kit de localização (FFX)") — o
  // caminho interno preserva o nome bruto do arquivo.
  const leaf = vbfRow(
    page,
    'ffx_data/gamedata/ps3data/lockit/ffx_loc_kit_ps3_us.bin'
  );
  await expect(leaf).toBeVisible({ timeout: 60_000 });
  await leaf.locator('[data-node-button]').click();
}

/** Desce até a primeira textura .dds.phyre do container e abre a prévia. */
async function openVbfImageEntry(page: Page): Promise<void> {
  await toggleVbfRoot(page);
  for (const dir of VBF_IMAGE_DIRS) {
    await expandVbfPath(page, dir);
  }
  // texlist.txt também é folha visível sob tex/: a textura é a folha que o
  // app SABE SERVIR (data-node-entry — o .dds.phyre; o label no container
  // vem SEM a extensão).
  const leaf = page.locator('[data-node-vbf="true"][data-node-entry="true"]').first();
  await expect(leaf).toBeVisible({ timeout: 60_000 });
  await leaf.locator('[data-node-button]').click();
}

// ---- config: devolve o executável original pelo diálogo -------------------

/** Devolve o executável original pelo mesmo diálogo e confere a árvore. */
async function restoreGameExe(page: Page, exe: string): Promise<void> {
  await openConfigDialog(page);
  const input = dialog(page).locator('#GameExeLocation');
  // Mesmo valor ainda no campo: o fill não dispara onChange (rascunho
  // nasce só na primeira edição) e o Enter não commita — a recarga abaixo
  // já reflete o estado persistido, nada a fazer.
  if (await input.inputValue() !== exe) {
    await input.fill(exe);
    await input.press('Enter');
  }
  await dialog(page).getByRole('button', { name: 'Fechar' }).click();
  await page.getByRole('button', { name: 'Recarregar' }).click();
  if (exe !== '' && hasVbf) {
    await expect(vbfRow(page, '')).toBeVisible({ timeout: 60_000 });
  } else if (exe === '') {
    await expect(
      page.locator('[data-vbf-missing][role="alert"]')
    ).toBeVisible({ timeout: 60_000 });
  }
}
