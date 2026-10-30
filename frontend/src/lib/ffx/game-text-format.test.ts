import { describe, expect, it } from 'vitest';
import { formatGameTextForDisplay, formatGameTextSnippetForSearch } from './game-text-format';
import { gameTextParser } from './game-text-parser';

// A camada de orquestração é a ponte entre o parser (fonte única com a
// tabela, refs e editor) e os dois consumidores: GameTextView e o snippet
// do modal de busca. Estes testos fixam que os dois montam o MESMO parser —
// mudar a formatação muda tudo junto, nunca só um lado.

describe('formatGameTextForDisplay', () => {
  it('é o MESMO HTML do parser sobre o corpo (regressão do GameTextView)', () => {
    const raw = '  \n{CLR:RED}Oi{TEXT_NORMAL}!';
    const formatted = formatGameTextForDisplay(raw);
    expect(formatted.html).toBe(gameTextParser.parseGameTextToHTML('{CLR:RED}Oi{TEXT_NORMAL}!'));
    expect(formatted.paddingSummary).toBe('{TEXT_NEWLINE} ×1 · ␣ ×2');
  });

  it('sem padding o resumo fica vazio e o corpo permanece', () => {
    expect(formatGameTextForDisplay('Oi')).toEqual({
      paddingSummary: '',
      html: '<p>Oi</p>',
    });
    expect(formatGameTextForDisplay(undefined).html).toBe('<p></p>');
  });

  it('marks[] andam junto com o corte do padding (offset em code points)', () => {
    // Padding "  \n" = 3 code points; "Oi" está em [12,14] do texto COM
    // padding. Ao remover o prefixo o destaque precisa deslizar para
    // [9,11] do corpo ({CLR:RED}Oi{TEXT_NORMAL}!).
    const raw = '  \n{CLR:RED}Oi{TEXT_NORMAL}!';
    const html = formatGameTextForDisplay(raw, [{ start: 12, end: 14 }]).html;
    expect(html).toContain('<mark>Oi</mark>');
  });

  it('sem marks[] o HTML permanece sem <mark>', () => {
    expect(formatGameTextForDisplay('Oi', []).html).toBe('<p>Oi</p>');
  });
});

describe('formatGameTextSnippetForSearch', () => {
  it('formata igual à tabela e destaca o hit', () => {
    const full = '{CLR:RED}Tidus sorri{TEXT_NORMAL} hoje';
    const html = formatGameTextSnippetForSearch({
      before: '{CLR:RED}Tidus ',
      hit: 'sorri',
      after: '{TEXT_NORMAL} hoje',
    });
    expect(html).toContain('Tidus <mark>sorri</mark>');
    expect(html).toContain('<span style="color:');
    // Removendo o destaque sobra EXATAMENTE o HTML da tabela — o <mark> é a
    // única diferença.
    expect(html.replace('<mark>', '').replace('</mark>', '')).toBe(
      gameTextParser.parseGameTextToHTML(full),
    );
  });

  it('remove o padding de tela e desloca o destaque junto', () => {
    const html = formatGameTextSnippetForSearch({
      before: '\n\n  Tidus ',
      hit: 'sorri',
      after: ' hoje',
    });
    expect(html).toBe('<p>Tidus <mark>sorri</mark> hoje</p>');
  });

  it('o "…" de truncamento do backend sobrevive como texto', () => {
    const html = formatGameTextSnippetForSearch({
      before: '…Tidus ',
      hit: 'sorri',
      after: '',
    });
    expect(html).toContain('…Tidus <mark>sorri</mark>');
  });

  it('hit dentro de tag chip destaca o chip (busca por conteúdo da tag)', () => {
    const html = formatGameTextSnippetForSearch({
      before: 'x{CMD:01:',
      hit: '02',
      after: '}y',
    });
    expect(html).toContain('<mark><span data-game-tag="{CMD:01:02}"');
  });

  it('janela vazia devolve string vazia (o dialog mostra o placeholder)', () => {
    expect(formatGameTextSnippetForSearch({ before: '', hit: '', after: '' })).toBe('');
  });

  it('quebra de linha vira \\n visível e um único parágrafo (sem unir palavras)', () => {
    const html = formatGameTextSnippetForSearch({
      before: 'Por isso instalamos nosso{TEXT_NEWLINE}',
      hit: 'acampamento',
      after: ' aqui.',
    });
    expect(html).toBe('<p>Por isso instalamos nosso\\n<mark>acampamento</mark> aqui.</p>');
  });

  it('CRLF real do dado vira um único marcador', () => {
    const html = formatGameTextSnippetForSearch({
      before: 'a\r\nb ',
      hit: 'c',
      after: '',
    });
    expect(html).toBe('<p>a\\nb <mark>c</mark></p>');
  });

  it('o <mark> continua no hit mesmo com a compressão das trocas antes dele', () => {
    const html = formatGameTextSnippetForSearch({
      before: 'A {TEXT_NEWLINE}B {TEXT_NEWLINE}C ',
      hit: 'Yuna',
      after: ' fim.',
    });
    expect(html).toBe('<p>A \\nB \\nC <mark>Yuna</mark> fim.</p>');
  });

  it('token que intersecta o hit fica como a quebra real (sem marcador)', () => {
    const html = formatGameTextSnippetForSearch({
      before: 'oi ',
      hit: 'fim{TEXT_NEWLINE}depois',
      after: '!',
    });
    expect(html).toBe('<p>oi <mark>fim</mark></p><p><mark>depois</mark>!</p>');
    expect(html).not.toContain('\\n');
  });
});

describe('formatGameTextForDisplay · chip de quebra (só a tabela)', () => {
  const CHIP = '<span class="newline-chip">LF</span>';

  it('sem a opção o HTML continua sem chip (editor/busca/refs intactos)', () => {
    expect(formatGameTextForDisplay('A{TEXT_NEWLINE}B').html).toBe('<p>A</p><p>B</p>');
  });

  it('com newlineChip a quebra real vira chip LF no FIM da linha', () => {
    expect(
      formatGameTextForDisplay('A{TEXT_NEWLINE}B', undefined, {
        newlineChip: true,
      }).html,
    ).toBe(`<p>A${CHIP}</p><p>B</p>`);
  });

  it('CRLF e \\n do dado contam como quebra igual à tag do jogo', () => {
    expect(formatGameTextForDisplay('A\r\nB\nC', undefined, { newlineChip: true }).html).toBe(
      `<p>A${CHIP}</p><p>B${CHIP}</p><p>C</p>`,
    );
  });

  it('um chip por quebra — a linha vazia entre duas ganha o dela', () => {
    expect(formatGameTextForDisplay('A\n\nB', undefined, { newlineChip: true }).html).toBe(
      `<p>A${CHIP}</p><p>${CHIP}</p><p>B</p>`,
    );
  });

  it('quebra no FIM do texto marca a última linha (o <p> vazio de antes segue)', () => {
    expect(formatGameTextForDisplay('Oi\n', undefined, { newlineChip: true }).html).toBe(
      `<p>Oi${CHIP}</p><p></p>`,
    );
    // Sem a opção continua exatamente como era (regressão).
    expect(formatGameTextForDisplay('Oi\n').html).toBe('<p>Oi</p><p></p>');
  });

  it('padding de tela continua só no resumo (não ganha chip)', () => {
    const formatted = formatGameTextForDisplay('  \nOi', undefined, {
      newlineChip: true,
    });
    expect(formatted.html).toBe('<p>Oi</p>');
    expect(formatted.paddingSummary).toBe('{TEXT_NEWLINE} ×1 · ␣ ×2');
  });

  it('marks[] convivem com o chip (a busca continua destacando)', () => {
    const html = formatGameTextForDisplay('A{TEXT_NEWLINE}B', [{ start: 15, end: 16 }], {
      newlineChip: true,
    }).html;
    expect(html).toBe(`<p>A${CHIP}</p><p><mark>B</mark></p>`);
  });
});
