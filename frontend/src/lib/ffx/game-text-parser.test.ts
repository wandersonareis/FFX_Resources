import { describe, expect, it } from 'vitest';
import { gameTextParser } from './game-text-parser';

// O editor compara parseGameTextToHTML(value) com editor.getHTML() (literal)
// e só chama setContent quando diferem — HTML que não bate com o renderHTML do
// nó gameTag ou com a serialização de texto do browser faria o cursor cair à
// toa. Estes testos fixam essa ordem/escapamento.
describe('parseGameTextToHTML: chips', () => {
  it('emite os atributos na mesma ordem do renderHTML', () => {
    const html = gameTextParser.parseGameTextToHTML('x{CMD:01:02}y');
    expect(html).toBe(
      '<p>x<span data-game-tag="{CMD:01:02}" class="game-tag-chip locked" ' +
        'data-locked="true">{CMD:01:02}</span>y</p>',
    );
  });

  it('marca o chip inválido com data-invalid no fim', () => {
    const html = gameTextParser.parseGameTextToHTML('Oi {PC:ZZ:YUNA}', () => false);
    expect(html).toBe(
      '<p>Oi <span data-game-tag="{PC:ZZ:YUNA}" class="game-tag-chip invalid" ' +
        'data-invalid="true">YUNA</span></p>',
    );
  });

  it('chip válido não ganha data-locked/data-invalid', () => {
    const html = gameTextParser.parseGameTextToHTML('Oi {PC:01:YUNA}', () => true);
    expect(html).toBe(
      '<p>Oi <span data-game-tag="{PC:01:YUNA}" class="game-tag-chip">YUNA</span></p>',
    );
  });

  it('sem validador nenhum chip é invalid', () => {
    const html = gameTextParser.parseGameTextToHTML('{HEX:DEADBEEF}');
    expect(html).toContain('class="game-tag-chip locked"');
    expect(html).not.toContain('data-invalid');
  });

  it('chip BUTTON com glifo vira o ícone direto no texto (sem chip)', () => {
    const html = gameTextParser.parseGameTextToHTML('{BUTTON:31:X}');
    expect(html).toBe(
      '<p><span data-game-tag="{BUTTON:31:X}">' +
        '<span class="gb-sprite gb-a" title="X"></span></span></p>',
    );
  });

  it('chips de direção viram SEQUÊNCIA de setas aninhadas ao wrapper (sem chip)', () => {
    const html = gameTextParser.parseGameTextToHTML('{BUTTON:43:Direcional Up+Right}');
    expect(html).toBe(
      '<p>' +
        '<span data-game-tag="{BUTTON:43:Direcional Up+Right}">' +
        '<span class="gb-sprite gb-arrow-up" title="Direcional Up+Right"></span>' +
        '<span class="gb-sprite gb-arrow-right" title="Direcional Up+Right"></span>' +
        '</span></p>',
    );
    // Triplas: 3 setas aninhadas na mesma ordem
    const html3 = gameTextParser.parseGameTextToHTML('{BUTTON:4B:Direcional Up+Left+Right}');
    expect((html3.match(/gb-arrow/g) ?? []).length).toBe(3);
    expect(html3.indexOf('gb-arrow-up')).toBeLessThan(html3.indexOf('gb-arrow-left'));
    expect(html3.indexOf('gb-arrow-left')).toBeLessThan(html3.indexOf('gb-arrow-right'));
    expect(html3).not.toContain('game-tag-chip');
  });

  it('chips All e Direcional genérico usam o cursor ✛', () => {
    for (const tag of ['{BUTTON:4F:Direcional All}', '{BUTTON:40:Direcional}']) {
      const html = gameTextParser.parseGameTextToHTML(tag);
      expect(html).toContain(`<span data-game-tag="${tag}"><span class="gb-sprite gb-cursor"`);
      expect(html).not.toContain('game-tag-chip');
    }
  });

  it('chip BUTTON sem glifo (Dummy) segue textual', () => {
    const html = gameTextParser.parseGameTextToHTML('{BUTTON:2D:Dummy}');
    expect(html).toContain('>Dummy</span>');
    expect(html).not.toContain('gb-sprite');
  });
});

describe('parseGameTextToHTML: escapamento', () => {
  it('aspas no texto ficam literais (igual à serialização do browser)', () => {
    expect(gameTextParser.parseGameTextToHTML('Ele disse "oi" & saiu')).toBe(
      '<p>Ele disse "oi" &amp; saiu</p>',
    );
  });

  it('apóstrofo no rótulo do chip fica literal', () => {
    const html = gameTextParser.parseGameTextToHTML('{MCR:s01:l05:"Yevon\'s"}');
    expect(html).toContain(">Yevon's</span>");
    expect(html).not.toContain('&#039;');
  });

  it('aspas dentro da tag escapam no atributo', () => {
    const html = gameTextParser.parseGameTextToHTML('{MCR:s01:l05:"Hi"}');
    expect(html).toContain('data-game-tag="{MCR:s01:l05:&quot;Hi&quot;}"');
  });

  it('escapa < e > do texto', () => {
    expect(gameTextParser.parseGameTextToHTML('a < b > c')).toBe('<p>a &lt; b &gt; c</p>');
  });
});

describe('parseGameTextToHTML: formatação não vira chip', () => {
  it('CLR/COLOR/TEXT_* ficam como marca, sem data-game-tag', () => {
    const html = gameTextParser.parseGameTextToHTML(
      '{CLR:RED}oi{TEXT_ITALIC}!{TEXT_NORMAL}{TEXT_NEWLINE}fim',
    );
    expect(html).not.toContain('data-game-tag');
    expect(html.split('<p>').length - 1).toBe(2);
  });
});

describe('parseGameTextToHTML: destaque markStart/markEnd', () => {
  it('sem options a saída é idêntica (refs/tabela/editor não mudam)', () => {
    const raw = '{CLR:RED}Perigli{TEXT_ITALIC}oso{TEXT_NORMAL} do Yuna{CMD:01:02}';
    expect(gameTextParser.parseGameTextToHTML(raw, undefined, {})).toBe(
      gameTextParser.parseGameTextToHTML(raw),
    );
  });

  it('intervalo vazio não gera destaque', () => {
    const raw = 'Tidus e Yuna';
    expect(gameTextParser.parseGameTextToHTML(raw, undefined, { markStart: 5, markEnd: 5 })).toBe(
      gameTextParser.parseGameTextToHTML(raw),
    );
  });

  it('envolve o texto do hit em <mark> por dentro da cor', () => {
    const html = gameTextParser.parseGameTextToHTML('{CLR:RED}Perigloso', undefined, {
      markStart: 13, // 'glo' depois de '{CLR:RED}Peri'
      markEnd: 16,
    });
    expect(html).toContain('Peri<mark>glo</mark>so');
    // O <mark> fica ANINHADO no span de cor, nunca no lugar dele.
    expect(html.indexOf('<mark>')).toBeGreaterThan(html.indexOf('<span style='));
    expect(html.indexOf('</mark>')).toBeLessThan(html.lastIndexOf('</span>'));
  });

  it('hit que intersecta uma tag chip destaca o chip inteiro', () => {
    const html = gameTextParser.parseGameTextToHTML('x{CMD:01:02}y', undefined, {
      markStart: 2, // por dentro de {CMD:01:02} (extensão [1, 12))
      markEnd: 6,
    });
    expect(html).toContain('<mark><span data-game-tag="{CMD:01:02}"');
    expect(html).toContain('</span></mark>');
  });

  it('o destaque atravessa parágrafo ({TEXT_NEWLINE}) sem quebrar', () => {
    const html = gameTextParser.parseGameTextToHTML(
      'Press{TEXT_NEWLINE}Enter agora',
      undefined,
      { markStart: 21, markEnd: 24 }, // 'ter' de Enter
    );
    expect(html).toBe('<p>Press</p><p>En<mark>ter</mark> agora</p>');
  });

  it('os offsets são em code points (emoji não desloca o destaque)', () => {
    // '😀' ocupa 2 unidades UTF-16 mas 1 code point.
    const html = gameTextParser.parseGameTextToHTML('Tidus 😀 Yuna', undefined, {
      markStart: 8,
      markEnd: 12,
    });
    expect(html).toBe('<p>Tidus 😀 <mark>Yuna</mark></p>');
  });
});

describe('parseGameTextToHTML: destaque múltiplo marks[] (busca local)', () => {
  it('sem marks[] a saída é idêntica (nada de highlight fantasma)', () => {
    const raw = 'Tidus e Yuna e Wakka';
    expect(gameTextParser.parseGameTextToHTML(raw, undefined, { marks: [] })).toBe(
      gameTextParser.parseGameTextToHTML(raw),
    );
  });

  it('destaca TODOS os intervalos', () => {
    const html = gameTextParser.parseGameTextToHTML('Tidus e Yuna', undefined, {
      marks: [
        { start: 0, end: 5 },
        { start: 8, end: 12 },
      ],
    });
    expect(html).toBe('<p><mark>Tidus</mark> e <mark>Yuna</mark></p>');
  });

  it('o match atual ganha a classe find-current', () => {
    const html = gameTextParser.parseGameTextToHTML('a b c', undefined, {
      marks: [
        { start: 0, end: 1 },
        { start: 4, end: 5, current: true },
      ],
    });
    expect(html).toBe('<p><mark>a</mark> b <mark class="find-current">c</mark></p>');
  });

  it('convive com o par legado markStart/markEnd', () => {
    const html = gameTextParser.parseGameTextToHTML('a b c', undefined, {
      markStart: 0,
      markEnd: 1,
      marks: [{ start: 4, end: 5 }],
    });
    expect(html).toBe('<p><mark>a</mark> b <mark>c</mark></p>');
  });

  it('destaca chip inteiro quando o mark intersecta a tag', () => {
    const html = gameTextParser.parseGameTextToHTML('x{CMD:01:02}y', undefined, {
      marks: [{ start: 2, end: 6 }],
    });
    expect(html).toContain('<mark><span data-game-tag="{CMD:01:02}"');
  });
});

describe('serializeJSONToGameText', () => {
  it('emite o valor do chip verbatim', () => {
    const json = {
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'Hi ' },
            { type: 'gameTag', attrs: { value: '{CMD:01:02}' } },
          ],
        },
      ],
    };
    expect(gameTextParser.serializeJSONToGameText(json)).toBe('Hi {CMD:01:02}');
  });

  function paragraphWithColor(color: string) {
    return {
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'Hi',
              marks: [{ type: 'textStyle', attrs: { color } }],
            },
          ],
        },
      ],
    };
  }

  it('rgb() de marca vira o NOME da paleta, nunca {CLR:RGB(...)}', () => {
    expect(gameTextParser.serializeJSONToGameText(paragraphWithColor('rgb(37, 99, 235)'))).toBe(
      '{CLR:BLUE}Hi{CLR:WHITE}',
    );
    expect(
      gameTextParser.serializeJSONToGameText(paragraphWithColor('rgba(220, 38, 38, 0.5)')),
    ).toBe('{CLR:RED}Hi{CLR:WHITE}');
  });

  it('hex fora da paleta cai na cor do jogo mais proxima', () => {
    const value = gameTextParser.serializeJSONToGameText(paragraphWithColor('#1d4ed8'));
    expect(value).toBe('{CLR:BLUE}Hi{CLR:WHITE}');
  });

  it('valor que nao e cor nao emite tag de cor', () => {
    expect(gameTextParser.serializeJSONToGameText(paragraphWithColor('inherit'))).toBe('Hi');
  });
});

describe('parseGameTextToHTML: cor corrompida de rascunho antigo', () => {
  it('{CLR:RGB(...)} volta a ser marca de cor (nao vira chip)', () => {
    const html = gameTextParser.parseGameTextToHTML('{CLR:RGB(37, 99, 235)}Oi');
    expect(html).toBe('<p><span style="color: #2563eb">Oi</span></p>');
    expect(html).not.toContain('data-game-tag');
  });

  it('byte canonico do decoder ({CLR:88}) tambem vira cor', () => {
    const html = gameTextParser.parseGameTextToHTML('{CLR:88}Oi');
    expect(html).toBe('<p><span style="color: #2563eb">Oi</span></p>');
  });

  it('byte desconhecido continua chip (byte preservado no round-trip)', () => {
    const html = gameTextParser.parseGameTextToHTML('{CLR:58}Oi');
    expect(html).toContain('data-game-tag="{CLR:58}"');
  });
});
