import { expect, test, type Locator, type Page } from '@playwright/test';
import {
  GET_TEXT_ENTRY_METHOD_ID,
  gotoApp,
  nodeRow,
  switchToTab,
} from '../helpers/ui';

/**
 * Invariante de EXIBIÇÃO: a coluna "Traduzido" nunca pinta uma ref de dedup
 * ("$" + 16 hex). O backend entrega "$hash" + a anotação `refs` e o frontend
 * pinta o texto da DEF; um "$hash" no DOM significa que a ref virou TEXTO no
 * binário (apply de objects/lockit sem expandir a ref) ou que a anotação se
 * perdeu — o bug que corrompeu o lm_accesary (57 rows) e o command (2 rows).
 *
 * Varre TODAS as abas e TODAS as entradas de texto. O DOM da tabela é
 * construído completo ao abrir a entrada (a árvore não virtualiza rows), então
 * ler as células é suficiente — não precisa rolar.
 */

/** Formatos de texto servidos pela árvore (imagens ficam de fora). */
const TEXT_KINDS = [
  'events',
  'objects',
  'macro',
  'lockit',
  'help',
  'battletext',
  'cloud',
  'tutorial',
  'menumain',
] as const;

const REF_IN_DOM = /^\$[0-9a-f]{16}$/;

/** Chevrão "para a direita" = nó colapsado (o app usa lucide). */
async function isCollapsed(page: Page, nodeId: string): Promise<boolean> {
  const row = nodeRow(page, nodeId);
  const chevron = row.locator('button[aria-label^="Alternar"] svg').first();
  const cls = (await chevron.getAttribute('class')) ?? '';
  return cls.includes('chevron-right');
}

/** Expande um nó da árvore (idempotente: só clica se estiver colapsado). */
async function expandNode(page: Page, nodeId: string): Promise<void> {
  const row = nodeRow(page, nodeId);
  await expect(row).toBeVisible({ timeout: 60_000 });
  if (!(await isCollapsed(page, nodeId))) return;
  await row.locator('button[aria-label^="Alternar"]').first().click({
    timeout: 60_000,
  });
}

/**
 * Abre a folha e espera a resposta do GetTextEntry + o cabeçalho trocar para
 * a entrada. Não exige `[data-row-key]`: entrada sem linha visível também é
 * varrida (nada a pintar).
 */
async function openEntry(page: Page, leaf: Locator): Promise<string> {
  const label = (await leaf.getAttribute('data-node-label')) ?? '';
  const loaded = page.waitForResponse(
    (res) =>
      res.request().method() === 'POST' &&
      res.url().includes('/wails/runtime') &&
      (res.request().postData() ?? '').includes(
        `"methodID":${GET_TEXT_ENTRY_METHOD_ID}`
      ),
    { timeout: 120_000 }
  );
  await leaf.locator('[data-node-button]').click();
  const res = await loaded;
  expect(
    res.ok(),
    `GetTextEntry de "${label}" respondeu HTTP ${res.status}`
  ).toBeTruthy();
  if (label !== '') {
    await expect(page.locator('main h3')).toContainText(label, {
      timeout: 30_000,
    });
  }
  return label;
}

/** Refs de dedup pintadas como TEXTO na coluna Traduzido (vazio = ok). */
async function hashesInDom(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const hits: string[] = [];
    for (const cell of Array.from(
      document.querySelectorAll('main .translated-cell')
    )) {
      const text = (cell.textContent ?? '').trim();
      if (/^\$[0-9a-f]{16}$/.test(text)) hits.push(text);
    }
    return hits;
  });
}

/** Raízes de kind de texto presentes na aba (a árvore só serve o que existe). */
async function textKindsOfTab(page: Page): Promise<string[]> {
  const ids = await page
    .locator('[data-node-id^="kind:"]')
    .evaluateAll((nodes) =>
      nodes.map((n) => n.getAttribute('data-node-id') ?? '')
    );
  return ids
    .map((id) => id.slice('kind:'.length))
    .filter((kind) => (TEXT_KINDS as readonly string[]).includes(kind));
}

/**
 * Varre uma aba inteira: expande cada kind (e os grupos de events), abre cada
 * entrada e confere o DOM. Devolve os problemas encontrados.
 */
async function sweepTab(page: Page): Promise<string[]> {
  const problems: string[] = [];
  const kinds = await textKindsOfTab(page);

  for (const kind of kinds) {
    const root = nodeRow(page, `kind:${kind}`);
    await expect(root).toBeVisible({ timeout: 60_000 });
    await expandNode(page, `kind:${kind}`);

    // events agrupa as entradas (group:events:xx): abre todos os grupos.
    const groups = page.locator(`[data-node-id^="group:${kind}:"]`);
    for (let round = 0; round < 200; round++) {
      const collapsed = groups.filter({
        has: page.locator(
          'button[aria-label^="Alternar"] svg[class*="chevron-right"]'
        ),
      });
      if ((await collapsed.count()) === 0) break;
      const groupId = await collapsed
        .first()
        .getAttribute('data-node-id');
      if (!groupId) break;
      await expandNode(page, groupId);
    }

    const leaves = page.locator(`[data-node-id^="leaf:${kind}:"]`);
    const count = await leaves.count();
    for (let i = 0; i < count; i++) {
      const leaf = leaves.nth(i);
      const nodeId = (await leaf.getAttribute('data-node-id')) ?? '';
      const id = nodeId.slice(`leaf:${kind}:`.length);
      const label = await openEntry(page, leaf);
      const hits = await hashesInDom(page);
      if (hits.length > 0) {
        problems.push(
          `${kind}/${id} (${label}): ${hits.length} "$hash" na coluna Traduzido (ex.: ${hits[0]})`
        );
      }
    }
  }
  return problems;
}

// A varredura abre centenas de entradas (FFX: ~450): cada uma espera a
// resposta do binding e varre o DOM. Tempo próprio, bem acima do default.
test.describe.configure({ mode: 'serial' });

for (const version of ['ffx', 'eternalcalm', 'ffx2', 'lastmiss'] as const) {
  test(`${version}: nenhum $hash de dedup exibido na coluna Traduzido`, async ({
    page,
  }) => {
    test.setTimeout(1_800_000);
    await gotoApp(page);
    await switchToTab(page, version);

    const problems = await sweepTab(page);

    expect(
      problems,
      `Ref de dedup ("$hash") pintada como TEXTO — o binário da entrada foi ` +
        `gravado com a ref em vez do literal (apply de objects/lockit sem ` +
        `expandir a ref). Repare reimportando o artefato e re-salve:\n` +
        problems.join('\n')
    ).toEqual([]);
  });
}
