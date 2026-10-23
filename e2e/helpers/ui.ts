import { expect, type Locator, type Page } from '@playwright/test';
import { readEnableMods } from './config-file';

/**
 * Seletores e fluxos compartilhados — tudo por papel/aria/rótulo (a UI é
 * pt-BR) e pelos data-* que a árvore já expõe ([data-node-id] e cia).
 */

export const HEADER_TITLE = 'FINAL FANTASY X and X-2 HD Remaster Resources Editor';
export const TAB_LABELS = ['FFX', 'FFX Eternal Calm', 'FFX-2', 'FFX-2 Last Mission'];

/** Abre o app e espera a casca hidratar (AppShell é client-only). */
export async function gotoApp(page: Page): Promise<void> {
  await page.goto('/');
  await expect(page.getByText(HEADER_TITLE).first()).toBeVisible();
}

/** Escapa um valor para dentro de [attr="..."] (ids de nó têm | e :). */
function attr(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

export function nodeRow(page: Page, nodeId: string) {
  return page.locator(`[data-node-id="${attr(nodeId)}"]`);
}

/** Botão que expande/colapsa um nó da árvore (chevron). */
async function toggleNode(page: Page, nodeId: string): Promise<void> {
  const row = nodeRow(page, nodeId);
  await expect(row).toBeVisible();
  // Timeout próprio (maior que o actionTimeout global): no boot frio do
  // servidor, o kind pode ainda estar carregando (spinner no lugar do
  // chevron) — o clique espera o botão existir e estabilizar.
  await row.locator('button[aria-label^="Alternar"]').first().click({
    timeout: 60_000,
  });
}

/** Espera a árvore montar a raiz de um kind de data/ (ex.: kind:events). */
export async function waitForKindRoot(page: Page, kind: string): Promise<void> {
  await expect(nodeRow(page, `kind:${kind}`)).toBeVisible({ timeout: 60_000 });
}

/**
 * Abre a PRIMEIRA folha de entrada de texto de data/ (Texto nasce expandido;
 * Eventos é agrupado — desce o primeiro grupo até achar folhas) e espera a
 * tabela renderizar rows. Devolve o label da entrada aberta.
 */
export async function openFirstTextEntry(page: Page): Promise<string> {
  const leaf = await revealFirstTextLeaf(page);
  const label = (await leaf.getAttribute('data-node-label')) ?? '';
  await leaf.locator('[data-node-button]').click();
  await expect(page.locator('[data-row-key]').first()).toBeVisible({ timeout: 60_000 });
  return label;
}

/**
 * Navega a árvore até a primeira folha de texto ficar VISÍVEL (kind events
 * expandido + primeiro grupo aberto) e devolve o locator da folha — sem
 * clicar. É a parte de navegação de openFirstTextEntry, separada para quem
 * precisa controlar o clique (ex.: sincronizar com a resposta da carga).
 */
export async function revealFirstTextLeaf(page: Page): Promise<Locator> {
  await waitForKindRoot(page, 'events');
  await toggleNode(page, 'kind:events');
  const tree = page.locator('#entry-tree');
  for (let i = 0; i < 4; i++) {
    if (await tree.locator('[data-node-entry="true"]').first().isVisible()) break;
    const group = tree.locator('[data-node-id^="group:events:"]').first();
    await group.locator('button[aria-label^="Alternar"]').first().click({
      timeout: 60_000,
    });
  }
  const leaf = tree.locator('[data-node-entry="true"]').first();
  await expect(leaf).toBeVisible();
  return leaf;
}

/**
 * Acha uma folha de texto DIFERENTE da indicada (label distinto): procura
 * entre as folhas já visíveis e, se só existe uma, expande o próximo grupo
 * de events colapsado (chevron à direita) até achar outra. Devolve o
 * locator da folha — sem clicar.
 */
export async function findAnotherTextLeaf(
  page: Page,
  firstLabel: string
): Promise<Locator> {
  const tree = page.locator('#entry-tree');
  for (let round = 0; round < 10; round++) {
    const leaves = tree.locator('[data-node-entry="true"]');
    const count = await leaves.count();
    for (let i = 0; i < count; i++) {
      const leaf = leaves.nth(i);
      const label = (await leaf.getAttribute('data-node-label')) ?? '';
      if (label !== '' && label !== firstLabel) return leaf;
    }
    // Nenhuma folha diferente visível: expande o próximo grupo colapsado
    // (o chevron aponta à direita quando o grupo está fechado).
    const collapsed = tree
      .locator('[data-node-id^="group:events:"]')
      .filter({
        has: page.locator('button[aria-label^="Alternar"] svg[class*="chevron-right"]'),
      });
    if ((await collapsed.count()) === 0) break;
    await collapsed.first().locator('button[aria-label^="Alternar"]').first().click({
      timeout: 60_000,
    });
  }
  throw new Error(`nenhuma folha de texto além de "${firstLabel}" na árvore de events`);
}

/**
 * ID do binding TextService.GetTextEntry (frontend/bindings) — é a chamada
 * que carrega o conteúdo de uma entrada de texto clicada na árvore.
 * O corpo do POST do runtime carrega "methodID":<id> (ver @wailsio/runtime).
 */
const GET_TEXT_ENTRY_METHOD_ID = 826447931;

/**
 * Clica uma folha de texto e espera a carga dela RESPONDER — não basta a
 * row visível: a tabela do arquivo ANTERIOR continua exibida durante o
 * load, então rows visíveis não provam que trocou. A resposta HTTP do
 * binding (200 = carregou; erro = a UI limpa a seleção e mostra toast) é o
 * ponto exato em que o conteúdo novo existe. Devolve o label da folha.
 */
export async function openLeafAndWaitLoad(page: Page, leaf: Locator): Promise<string> {
  const label = (await leaf.getAttribute('data-node-label')) ?? '';
  const loaded = page.waitForResponse(
    (res) =>
      res.request().method() === 'POST' &&
      res.url().includes('/wails/runtime') &&
      (res.request().postData() ?? '').includes(
        `"methodID":${GET_TEXT_ENTRY_METHOD_ID}`
      ),
    { timeout: 60_000 }
  );
  await leaf.locator('[data-node-button]').click();
  const res = await loaded;
  expect(res.ok(), `GetTextEntry de "${label}" respondeu HTTP ${res.status}`).toBeTruthy();
  await expect(page.locator('[data-row-key]').first()).toBeVisible({ timeout: 30_000 });
  return label;
}

/** Barra de título da tabela (main h3): "Eventos · <arquivo>" + badge. */
export function entryHeading(page: Page): Locator {
  return page.locator('main h3');
}

/**
 * Abre a primeira textura de data/ — a árvore de imagens tem profundidade
 * fixa (kind:images → categoria → diretório → folha): dois cliques de
 * chevron chegam às folhas. Espera o painel com a prévia (img data URL).
 * Devolve o label da textura aberta.
 */
export async function openFirstImageEntry(page: Page): Promise<string> {
  const imageRoot = nodeRow(page, 'kind:images');
  await expect(imageRoot).toBeVisible({ timeout: 60_000 });
  await imageRoot.locator('button[aria-label^="Alternar"]').first().click();

  const tree = page.locator('#entry-tree');
  // Categoria: o primeiro grupo visível (só categorias existem até aqui).
  const category = tree.locator('[data-node-id^="group:images:"]').first();
  await expect(category).toBeVisible();
  await category.locator('button[aria-label^="Alternar"]').first().click();

  // Diretório: o primeiro grupo com "/" no id (categoria não tem barra).
  const dirGroup = tree
    .locator('[data-node-id^="group:images:"][data-node-id*="/"]')
    .first();
  await expect(dirGroup).toBeVisible();
  await dirGroup.locator('button[aria-label^="Alternar"]').first().click();

  const leaf = tree
    .locator('[data-node-kind="images"][data-node-entry="true"]')
    .first();
  await expect(leaf).toBeVisible();
  const label = (await leaf.getAttribute('data-node-label')) ?? '';
  await leaf.locator('[data-node-button]').click();
  // Painel de imagem: a prévia é um <img> com data URL (src="data:...").
  await expect(page.locator('img[src^="data:"]').first()).toBeVisible({
    timeout: 60_000,
  });
  return label;
}

/** Progresso fixado na abertura: "t/m linhas traduzidas (p%)". */
export async function badgeProgress(
  page: Page
): Promise<{ translated: number; total: number } | null> {
  const text = await page
    .locator('header, main, div')
    .getByText(/linhas traduzidas \(/)
    .first()
    .textContent()
    .catch(() => null);
  if (!text) return null;
  const m = text.match(/(\d+)\/(\d+)\s+linhas traduzidas/);
  if (!m) return null;
  return { translated: Number(m[1]), total: Number(m[2]) };
}

/** Textos das células de uma coluna pela legenda do cabeçalho. */
export async function columnTexts(
  page: Page,
  header: 'Original' | 'Traduzido'
): Promise<string[]> {
  return page.evaluate((h) => {
    const ths = [...document.querySelectorAll('table thead th')] as HTMLTableCellElement[];
    const th = ths.find((cell) => (cell.textContent ?? '').trim().startsWith(h));
    if (!th) return [];
    const idx = th.cellIndex;
    const rows = [...document.querySelectorAll('table tbody tr')] as HTMLTableRowElement[];
    return rows.map((row) => (row.cells[idx]?.textContent ?? '').trim());
  }, header);
}

/** Toast do sonner contendo o texto pedido. */
export function toast(page: Page, text: string) {
  return page.locator('[data-sonner-toast]', { hasText: text }).first();
}

/** A página nunca ganha rolagem horizontal (estouro de conteúdo). */
export async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => ({
    doc: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    body: document.body.scrollWidth - document.body.clientWidth,
  }));
  expect(overflow.doc, 'documentElement sem scroll horizontal').toBeLessThanOrEqual(1);
  expect(overflow.body, 'body sem scroll horizontal').toBeLessThanOrEqual(1);
}

/** Diálogo aberto atual (Radix renderiza um por vez nos fluxos testados). */
export function dialog(page: Page) {
  return page.locator('[data-slot="dialog-content"]');
}

/** Diálogo contido na viewport — conteúdo nada escapa da tela. */
export async function expectDialogWithinViewport(page: Page): Promise<void> {
  const box = await dialog(page).boundingBox();
  expect(box, 'diálogo aberto com caixa').toBeTruthy();
  const vp = page.viewportSize();
  expect(vp, 'viewport definida').toBeTruthy();
  expect(box!.x, 'diálogo não sai à esquerda').toBeGreaterThanOrEqual(-1);
  expect(box!.y, 'diálogo não sai acima').toBeGreaterThanOrEqual(-1);
  expect(box!.x + box!.width, 'diálogo não sai à direita').toBeLessThanOrEqual(
    vp!.width + 1
  );
  expect(box!.y + box!.height, 'diálogo não sai abaixo').toBeLessThanOrEqual(
    vp!.height + 1
  );
}

/** Fecha o diálogo atual pelo botão de texto dado (Cancelar/Fechar/…). */
export async function closeDialog(page: Page, label: string): Promise<void> {
  await dialog(page).getByRole('button', { name: label }).click();
  await expect(dialog(page)).toBeHidden();
}

/**
 * Fecha o diálogo atual pelo X do canto (id fixo do ui/dialog.tsx — todo
 * DialogContent o carrega). É o fechamento dos diálogos de efeito
 * imediato, como o de Configurações: as mudanças commitam na hora, não
 * há o que Cancelar/Salvar — fechar é só sair.
 */
export async function closeDialogByX(page: Page): Promise<void> {
  await dialog(page).locator('#dialog-close').click();
  await expect(dialog(page)).toBeHidden();
}

/** Abre as Configurações pelo botão do header. */
export async function openConfigDialog(page: Page): Promise<void> {
  // exact: o aviso "aponte o executável" da árvore também tem um botão de
  // Configurações ("Abrir Configurações") — o do header é o único exato.
  await page
    .getByRole('button', { name: 'Configurações', exact: true })
    .click();
  await expect(dialog(page)).toBeVisible();
}

/**
 * Alternador do checkbox "Preferir binários traduzidos (mods)"
 * (#EnableMods, o "disable mods" do config): INVERTE o estado atual —
 * marcado desmarca, desmarcado marca. Espera o espelho do config.json
 * chegar ao diálogo ANTES de clicar (a carga do store é assíncrona e o
 * default é ligado) e confirma DEPOIS que o backend persistiu o novo
 * valor. Devolve o estado que ficou (o inverso do encontrado).
 */
export async function toggleEnableModsInConfigDialog(page: Page): Promise<boolean> {
  const target = !readEnableMods();
  await openConfigDialog(page);
  const box = dialog(page).locator('#EnableMods');
  await expect(box).toBeVisible();
  // O checkbox precisa espelhar o config.json antes do clique — sem isso
  // um clique no meio da hidratação alternaria a partir do default errado.
  await expect
    .poll(() => box.isChecked(), { timeout: 15_000 })
    .toBe(!target);
  await box.click();
  await expect
    .poll(() => box.isChecked(), { timeout: 15_000 })
    .toBe(target);
  await expect
    .poll(() => readEnableMods(), {
      timeout: 15_000,
      message: `config.json com EnableMods=${target}`,
    })
    .toBe(target);
  await closeDialogByX(page);
  return target;
}

/**
 * Devolve o toggle de mods ao estado pedido (original do fixture):
 * só clica se divergir, e sempre confirma o config.json no fim. Usada na
 * limpeza dos specs que alternaram o toggle pela UI — o backend em
 * memória e o arquivo ficam coerentes.
 */
export async function restoreEnableModsInConfigDialog(
  page: Page,
  target: boolean
): Promise<void> {
  await openConfigDialog(page);
  const box = dialog(page).locator('#EnableMods');
  await expect(box).toBeVisible();
  const current = readEnableMods();
  await expect
    .poll(() => box.isChecked(), { timeout: 15_000 })
    .toBe(current);
  if (current !== target) {
    await box.click();
    await expect
      .poll(() => box.isChecked(), { timeout: 15_000 })
      .toBe(target);
  }
  await expect
    .poll(() => readEnableMods(), {
      timeout: 15_000,
      message: `config.json restaurado com EnableMods=${target}`,
    })
    .toBe(target);
  await closeDialogByX(page);
}
