import { expect, test } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {
  closeDialogByX,
  dialog,
  gotoApp,
  openConfigDialog,
} from '../helpers/ui';
import { CONFIG_PATH, gameFiles, hasData, modsDir } from '../helpers/fixtures';
import { changedSince, snapshotMtimes } from '../helpers/backup';

/** O app grava config.json de forma não atômica; lê com retry. */
function readConfig(): { EnableMods?: boolean } {
  for (let i = 0; i < 40; i++) {
    try {
      const text = fs.readFileSync(CONFIG_PATH, 'utf8').trim();
      if (text.length > 0) return JSON.parse(text);
    } catch {
      // arquivo no meio da escrita — tenta de novo
    }
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 50);
  }
  throw new Error(`config.json ilegível: ${CONFIG_PATH}`);
}

/**
 * Exporta TODO o texto com o toggle "Preferir binários traduzidos (mods)"
 * DESLIGADO na configuração — a extração lê os binários originais de data/,
 * mas o destino continua sendo sempre `<game>/mods/edits` (o backend resolve
 * o caminho por kind; nenhum export escreve fora de edits/).
 *
 * Cobre o caminho real do app em modo servidor (`wails3 run:server`): a UI
 * dispara o batch "Exportar — todos os tipos", o modal de progresso conclui
 * e os artefatos `.strings` aparecem em mods/edits.
 *
 * `EnableMods` é restaurado ao valor original no fim (via UI, para o backend
 * em memória e o config.json ficarem coerentes), mas os artefatos exportados
 * FICAM em mods/edits — é o resultado pedido.
 */
test.describe('Exportar todo o texto com mods desativados', () => {
  test.skip(!hasData, 'data/ não existe — sem texto para exportar');

  test('exporta todos os tipos para mods/edits lendo os originais', async ({
    page,
  }) => {
    test.setTimeout(600_000);

    test.info().annotations.push({
      type: 'fixture',
      description: `gameFiles=${gameFiles} mods=${modsDir}`,
    });

    try {
      await gotoApp(page);

      // ---- 1. garante EnableMods = false pela própria UI ----
      const originalEnableMods = readConfig().EnableMods === true;

      await openConfigDialog(page);
      const modsCheckbox = dialog(page).locator('#EnableMods');
      await expect(modsCheckbox).toBeVisible();

      if (await modsCheckbox.isChecked()) {
        await modsCheckbox.click();
        await expect(modsCheckbox).not.toBeChecked();
      }
      await expect
        .poll(() => readConfig().EnableMods === false, {
          timeout: 15_000,
          message: 'config.json com EnableMods=false',
        })
        .toBe(true);
      await closeDialogByX(page);

      // ---- 2. formato Strings (artefatos .strings) ----
      const formatButton = page.getByRole('button', {
        name: /^(JSON|Strings|CSV|XLSX \(Excel\)|PO \(gettext\))$/,
      });
      await formatButton.click();
      await page.getByRole('menuitemradio', { name: 'Strings' }).click();
      await expect(formatButton).toHaveText('Strings');

      // ---- 3. Exportar → todos os tipos ----
      const before = snapshotMtimes(modsDir);

      await page.getByRole('button', { name: /Exportar/ }).first().click();
      const allTypes = page
        .getByRole('menuitem')
        .filter({ hasText: 'todos os tipos' });
      await expect(allTypes).toBeVisible();
      await allTypes.click();

      // ---- 4. modal de progresso: batch inteiro conclui ----
      const progress = dialog(page);
      await expect(progress).toBeVisible({ timeout: 30_000 });
      await expect(progress.getByText('Processamento concluído.')).toBeVisible({
        timeout: 480_000,
      });
      await expect(progress.getByText(/arquivo\(s\)/)).toBeVisible();
      await progress.getByRole('button', { name: 'Fechar' }).click();
      await expect(progress).toBeHidden();

      // ---- 5. artefatos: TUDO em mods/edits, nada fora dele ----
      const changed = changedSince(modsDir, before);
      expect(changed.length, 'export gravou arquivos').toBeGreaterThan(0);
      expect(
        changed.filter((rel) => !rel.startsWith('edits/')),
        'export não escreve fora de mods/edits'
      ).toEqual([]);

      const artifacts = changed.filter(
        (rel) => rel.startsWith('edits/') && rel.endsWith('.strings')
      );
      expect(
        artifacts.length,
        `artefatos .strings em edits/ (${artifacts.length})`
      ).toBeGreaterThan(0);

      const names = artifacts.map((rel) => path.basename(rel));
      for (const expected of [
        'battletext_all_localizations_ffx.strings',
        'cloudsave_all_localizations_ffx.strings',
        'events_all_localizations_ffx.strings',
        'ffx_loc_kit_ps3_all_localizations_ffx.strings',
        'help_all_localizations_ffx.strings',
        'mainmenu_all_localizations_ffx.strings',
        'objects_all_localizations_ffx.strings',
      ]) {
        expect(names, `artefato ${expected}`).toContain(expected);
      }

      // ---- 6. prova de que leu os ORIGINAIS (mods desligado) ----
      // O menu principal original em `us` é inglês; a tradução em mods/ usa
      // "Rede de Esferas". O export CAT traz `text`/`source` do binário de
      // data/ e `target` do par em mods/ — o toggle não muda essa régua.
      const mainmenu = fs.readFileSync(
        path.join(modsDir, 'edits', 'mainmenu_all_localizations_ffx.strings'),
        'utf8'
      );
      const usLine = mainmenu
        .split(/\r?\n/)
        .find((line) => line.startsWith('menumain:0:us'));
      expect(usLine, 'linha menumain:0:us exportada').toBeTruthy();
      // O VALOR da linha (antes da projeção) é o texto de data/.
      const value = usLine!.split('║source = ')[0];
      expect(value).toContain('Sphere Grid');
      expect(value).not.toContain('Rede de Esferas');
      // source = referência EN (data/); target = tradução de mods/.
      const source = usLine!.split('║source = ')[1]?.split('║target = ')[0] ?? '';
      const target = usLine!.split('║target = ')[1] ?? '';
      expect(source, 'source da linha (data/)').toContain('Sphere Grid');
      expect(source, 'source não é a tradução').not.toContain('Rede de Esferas');
      expect(target, 'target da linha (mods/)').toContain('Rede de Esferas');

      // ---- 7. restaura o toggle original ----
      if (originalEnableMods) {
        await openConfigDialog(page);
        const restore = dialog(page).locator('#EnableMods');
        if (!(await restore.isChecked())) await restore.click();
        await expect(restore).toBeChecked();
        await closeDialogByX(page);
      }
    } finally {
      // Sem backup de arquivo: o único estado tocado é o toggle, restaurado
      // pela UI acima. Os artefatos exportados permanecem em mods/edits.
    }
  });
});
