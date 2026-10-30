import { expect, test } from '@playwright/test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {
  dialog,
  gotoApp,
  restoreEnableModsInConfigDialog,
  toggleEnableModsInConfigDialog,
  toast,
} from '../helpers/ui';
import { hasData, hasMods, modsDir, translateDir } from '../helpers/fixtures';
import { readEnableMods } from '../helpers/config-file';
import { backupTree, changedSince, restoreTree, snapshotMtimes } from '../helpers/backup';
import {
  editsDir,
  exportScopeSelection,
  readArtifact,
  setExportFormat,
  setScope,
  waitExportDone,
} from '../helpers/export';

/**
 * Round-trip CAT no ambiente REAL: exporta um kind (text/source de data/ +
 * target de mods/), coloca o MESMO artefato no lote mods/translated e importa
 * sem edição.
 *
 * O contrato validado:
 *   - o artefato carrega `source` (referência EN) e `target` (tradução de
 *     mods/) — nos quatro formatos o target é o único texto importado;
 *   - reimportar o artefato sem edição é NO-OP: 0 textos alterados e nenhum
 *     binário regravado em mods/ (é a guarda do falso positivo de tag de
 *     controle: o target já é o texto vigente da store);
 *   - o lote não tem arquivo pulado nem entrada ignorada.
 *
 * Isolamento: `mods/translated/text` é a raiz varrida pelo lote. O conteúdo
 * real (artefatos do usuário) é copiado ANTES e restaurado arquivo a arquivo
 * no fim; durante o teste a pasta contém SÓ o artefato deste round-trip (um
 * lote com arquivos alheios importaria coisas fora do escopo do teste).
 */
test.describe('Export → import sem edição (round-trip CAT)', () => {
  test.skip(!hasData, 'data/ não existe — sem texto para exportar');
  test.skip(!hasMods, 'mods/ não existe — sem tradução para o target');

  test('artefato com source/target volta como no-op pelo lote translated', async ({
    page,
  }) => {
    test.setTimeout(600_000);

    const translatedTextDir = path.join(translateDir, 'text');
    const translatedBackup = backupTree(translatedTextDir, 'translated-text-roundtrip');
    const originalEnableMods = readEnableMods();
    let toggledByTest = false;

    try {
      await gotoApp(page);

      // O import casa o target contra a store mods-first: com o toggle
      // desligado a store vira data/ e TUDO pareceria alterado.
      if (!originalEnableMods) {
        toggledByTest = await toggleEnableModsInConfigDialog(page);
        expect(toggledByTest, 'EnableMods ligado para o round-trip').toBe(true);
      }

      // ---- 1. Export de UM kind (Textos de batalha) em JSON ---------------
      await setExportFormat(page, 'JSON');
      await setScope(page, ['battletext']);
      await exportScopeSelection(page);
      await waitExportDone(page);

      const artifactName = 'battletext_all_localizations_ffx.json';
      const artifactPath = path.join(editsDir, artifactName);
      expect(
        fs.existsSync(artifactPath),
        `export gravou ${artifactName} em mods/edits`
      ).toBe(true);

      // ---- 2. O artefato é o formato CAT (source/target) ------------------
      const parsed = JSON.parse(readArtifact(artifactName)) as Record<
        string,
        { rows?: Array<{ text?: Record<string, string>; source?: string; target?: string }> }
      >;
      const rows = Object.values(parsed).flatMap((entry) => entry.rows ?? []);
      expect(rows.length, 'artefato com rows').toBeGreaterThan(0);

      const withSource = rows.filter((r) => (r.source ?? '') !== '');
      const withTarget = rows.filter((r) => (r.target ?? '') !== '');
      expect(withSource.length, 'rows com source (referência de data/)').toBeGreaterThan(0);
      expect(withTarget.length, 'rows com target (tradução de mods/)').toBeGreaterThan(0);
      // Target veio de mods/, não da referência: ao menos uma tradução
      // difere do inglês do source.
      expect(
        rows.some((r) => (r.target ?? '') !== '' && r.target !== r.source),
        'target tem tradução de mods/ (difere do source)'
      ).toBe(true);
      // text continua sendo a referência: literal igual ao source ou ref $hash
      // do dedup (o hash acompanha text, não o target).
      for (const row of rows) {
        if (!row.source) continue;
        const text = row.text?.us ?? '';
        expect(
          text === row.source || /^\$[0-9a-f]{16}$/.test(text),
          `text.us coerente com source na row (text=${JSON.stringify(text)})`
        ).toBe(true);
      }

      // ---- 3. Isola o lote translated: só o artefato deste teste ----------
      if (fs.existsSync(translatedTextDir)) {
        fs.rmSync(translatedTextDir, { recursive: true, force: true });
      }
      fs.mkdirSync(translatedTextDir, { recursive: true });
      fs.copyFileSync(artifactPath, path.join(translatedTextDir, artifactName));

      // ---- 4. Prévia do lote: 1 arquivo, 0 alterações, sem avisos ---------
      const beforeImport = snapshotMtimes(modsDir);
      await page.getByRole('button', { name: /Importar/ }).first().click();
      const importMenu = page.getByRole('menu');
      await expect(importMenu).toBeVisible();
      await importMenu
        .getByRole('menuitem', { name: 'Importar mods/translated' })
        .click();

      const importDialog = dialog(page);
      await expect(
        importDialog.getByText('Importar textos de mods/translated')
      ).toBeVisible({ timeout: 120_000 });

      const body = (await importDialog.textContent()) ?? '';
      expect(body, '1 arquivo no lote').toContain('1 arquivo(s)');
      expect(body, 'lote com entradas').not.toMatch(/\b0 entrada\(s\)/);
      expect(
        body,
        'round-trip sem edição: nenhum texto alterado'
      ).toContain('0 texto(s) alterado(s)');
      // O bug do falso positivo caía em "Entrada(s) ignorada(s)"; arquivo
      // pulado seria erro de formato/parse.
      expect(body, 'sem entrada ignorada no lote').not.toContain(
        'Entrada(s) ignorada(s)'
      );
      expect(body, 'sem arquivo pulado no lote').not.toContain(
        'Arquivo(s) pulado(s)'
      );

      const confirm = importDialog.getByRole('button', { name: /^Importar$/ });
      await expect(confirm).toBeEnabled();
      await confirm.click();
      // Sem edição o lote não tem nada a aplicar: o toast é o informativo
      // "Nada a importar." (0 textos alterados) — o caminho de sucesso
      // ("Importado: N texto(s)...") só existe quando algo muda.
      await expect(toast(page, 'Nada a importar')).toBeVisible({
        timeout: 120_000,
      });
      await expect(importDialog).toBeHidden({ timeout: 30_000 });

      // ---- 5. PreviewImport do MESMO artefato pelo binding ----------------
      // O resumo do import direto é onde o usuário vê "row(s) ignorada(s)".
      // Exportar → importar sem edição NÃO pode ter nenhuma row em skipped
      // (a regressão era "tag de controle divergente" em rows no-op, ex.:
      // MCR de speaker que o original EN não tem). Chamada pelo runtime do
      // Wails no mesmo servidor — sem picker nativo, que o modo server não
      // tem.
      const summary = await page.evaluate(async (artifact: string) => {
        const res = await fetch('/wails/runtime', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'x-wails-client-id': 'e2e-roundtrip',
          },
          body: JSON.stringify({
            object: 0,
            method: 0,
            args: {
              'call-id': `e2e-${Date.now()}`,
              methodName: 'main.TextService.PreviewImport',
              args: [artifact, 'ffx'],
            },
          }),
        });
        if (!res.ok) throw new Error(`${res.status}: ${await res.text()}`);
        return (await res.json()) as Record<string, unknown>;
      }, artifactPath);

      expect(summary['kind'], 'kind do artefato').toBe('battletext');
      expect(summary['format'], 'formato do artefato').toBe('json');
      expect(summary['changed_texts'], 'round-trip sem edição: 0 alterações').toBe(0);
      expect(summary['warnings'], 'nenhuma entrada ignorada').toEqual([]);
      expect(summary['errors'], 'nenhum bloqueador').toEqual([]);
      expect(
        summary['skipped'],
        `nenhuma row ignorada (tag divergente): ${JSON.stringify(summary['skipped'])}`
      ).toEqual([]);

      // ---- 6. Nada regravado na árvore de binários -----------------------
      // edits/ (o artefato exportado) e translated/ (a raiz do lote) são
      // artefatos do teste; o que importa é nenhum BINÁRIO de mods/ mudar.
      const changed = changedSince(modsDir, beforeImport).filter(
        (rel) => !rel.startsWith('edits/') && !rel.startsWith('translated/')
      );
      expect(
        changed,
        `import sem edição regravou binário(s) em mods/: ${changed.join(', ')}`
      ).toEqual([]);
    } finally {
      // Remove o lote isolado ANTES de restaurar: o restore devolve os
      // artefatos reais do usuário (que ficaram no backup).
      if (fs.existsSync(translatedTextDir)) {
        fs.rmSync(translatedTextDir, { recursive: true, force: true });
      }
      await restoreTree(translatedBackup).catch(() => undefined);
      if (toggledByTest) {
        await restoreEnableModsInConfigDialog(page, originalEnableMods).catch(
          () => undefined
        );
      }
    }
  });
});
