import type { JSONContent } from '@tiptap/core';
import {
  COLOR_RESET_TAG,
  buttonSpriteClasses,
  extractChipLabel,
  getColorTagName,
  getResolvedColorFromTag,
  isLockedTagInner,
} from './game-text-tags';
import type { ChipValidator } from './tag-catalog';

/** Um destaque (intervalo) do texto em <mark> — busca. */
export interface GameTextMark {
  /** Início (inclusivo) em CODE POINTS do texto de entrada. */
  start: number;
  /** Fim (exclusivo) em CODE POINTS do texto de entrada. */
  end: number;
  /** Match ATIVO da navegação — destaque distinto (classe find-current). */
  current?: boolean;
}

export interface GameTextMarkOptions {
  /** Um único intervalo — formato legado do snippet da busca geral. */
  markStart?: number;
  markEnd?: number;
  /** Vários intervalos (busca local na tabela); convive com o legado. */
  marks?: readonly GameTextMark[];
}

/** Intervalo já resolvido em UTF-16 (a varredura trabalha em offsets absolutos). */
interface ResolvedMark {
  start: number;
  end: number;
  current: boolean;
}

/** Offset em code points -> índice UTF-16 (clamp nas pontas). */
function toUtf16Index(text: string, codePoints: number): number {
  let count = 0;
  for (let i = 0; i < text.length; ) {
    if (count >= codePoints) return i;
    const code = text.codePointAt(i) ?? 0;
    i += code > 0xffff ? 2 : 1;
    count += 1;
  }
  return text.length;
}

/**
 * Conversão entre o texto canônico do backend (tags {…}) e o HTML do Tiptap.
 *
 * Vira formatação rica: {CLR:X}/{COLOR:X} -> cor, {TEXT_ITALIC}/{TEXT_NORMAL}
 * -> itálico, \n / {TEXT_NEWLINE} -> quebra de parágrafo.
 * Todo o resto vira chip atômico <span data-game-tag="…"> com value = tag
 * original (serialização verbatim, byte-idêntica).
 */
export const gameTextParser = {
  /**
   * Texto canônico com tags -> HTML do Tiptap.
   *
   * `validate` (opcional) marca como inválido o chip da tag digitada que não
   * sobreviveria ao round-trip — a MESMA instância usada na conversão de tags
   * digitadas, senão o HTML divergiria do valor e o setContent perderia o
   * cursor.
   *
   * `options` (opcional) destaca intervalos do texto em <mark> — o snippet
   * da busca geral usa o par legado markStart/markEnd; a busca local da
   * tabela passa `marks[]` (o atual ganha a classe find-current). Sem
   * nenhum dos dois, a saída é byte-idêntica à de sempre.
   */
  parseGameTextToHTML(
    rawText: string,
    validate?: ChipValidator,
    options?: GameTextMarkOptions
  ): string {
    if (!rawText) return '<p></p>';

    // Destaque (busca): markStart/markEnd legado OU vários intervalos da
    // busca local, em CODE POINTS do texto cru, resolvidos uma única vez
    // para UTF-16 — a varredura trabalha em offsets absolutos e nunca corta
    // uma tag no meio.
    const marks = this.resolveMarks(rawText, options);

    // Quebras: \n real ou {TEXT_NEWLINE} viram parágrafos. O split leva
    // grupo de captura (separadores intercalados no resultado), então o
    // cursor absoluto avança exatamente pelo que foi consumido.
    const segments = rawText.split(/(\r?\n|\{TEXT_NEWLINE\})/gi);
    const htmlParagraphs: string[] = [];
    let cursor = 0;

    for (let s = 0; s < segments.length; s += 2) {
      const para = segments[s];
      const paraStart = cursor;
      cursor +=
        para.length + (s + 1 < segments.length ? segments[s + 1].length : 0);

      if (para.trim() === '') {
        htmlParagraphs.push('<p></p>');
        continue;
      }

      let currentColor: string | null = null;
      let italicActive = false;
      let resultHTML = '';
      let lastIndex = 0;

      const tagRegex = /\{([^{}]+)\}/g;
      let match: RegExpExecArray | null;

      while ((match = tagRegex.exec(para)) !== null) {
        const matchIndex = match.index;
        const inner = match[1].trim();

        const colorTag = this.classifyColorTag(inner);
        const fontTag = this.classifyFontTag(inner);

        if (!colorTag && !fontTag && !this.isChipTag(inner)) {
          continue;
        }

        if (matchIndex > lastIndex) {
          const segment = para.slice(lastIndex, matchIndex);
          if (segment) {
            resultHTML += this.wrapSegment(
              segment,
              paraStart + lastIndex,
              currentColor,
              italicActive,
              marks
            );
          }
        }

        if (colorTag) {
          currentColor = colorTag.isReset ? null : colorTag.hex;
        } else if (fontTag) {
          italicActive = fontTag === 'italic';
        } else {
          // Chip atômico genérico: value = tag original, label derivado.
          const fullTag = `{${inner}}`;
          const label = extractChipLabel(inner);
          const locked = isLockedTagInner(inner);
          const invalid = !!validate && !validate(inner);
          // BUTTON com glifo(s) mapeado(s) e sem marcação: os ícones vão
          // DIRETO no texto (sem chip) — igual aos botões do jogo. Chip
          // textual fica para os casos com texto (locked/invalid/dummy).
          const btnClasses = buttonSpriteClasses(inner);
          let chipHTML: string;
          if (btnClasses.length > 0 && !locked && !invalid) {
            // O wrapper externo (data-game-tag) é o ponto de re-parse do
            // node gameTag; os ícones ficam ANINHADOS dentro dele — um node
            // atômico do ProseMirror só aceita UM elemento raiz no
            // renderHTML, e irmãos no mesmo nível virariam filhos do
            // primeiro tile (tile tem tamanho fixo: sobrepõem). Estrutura
            // byte-idêntica à do renderHTML do gameTag.
            chipHTML =
              `<span data-game-tag="${this.escapeAttr(fullTag)}">` +
              btnClasses
                .map(
                  (cls) =>
                    `<span class="gb-sprite ${cls}" title="${this.escapeAttr(
                      label
                    )}"></span>`
                )
                .join('') +
              '</span>';
          } else {
            chipHTML = `<span data-game-tag="${this.escapeAttr(
              fullTag
            )}" class="game-tag-chip${locked ? ' locked' : ''}${
              invalid ? ' invalid' : ''
            }"${locked ? ' data-locked="true"' : ''}${
              invalid ? ' data-invalid="true"' : ''
            }>${this.escapeHtml(label)}</span>`;
          }
          // Destaque que intersecta a TAG inteira: o chip ganha o <mark>
          // (não há texto fora da braces para envolver — senão o hit
          // ficaria invisível). Vários marks podem intersectar; o atual
          // (find-current) tem prioridade na classe.
          const tagStart = paraStart + matchIndex;
          const tagEnd = paraStart + tagRegex.lastIndex;
          const chipMark = this.markOverlapping(marks, tagStart, tagEnd);
          resultHTML += chipMark
            ? `<mark${chipMark.current ? ' class="find-current"' : ''}>${chipHTML}</mark>`
            : chipHTML;
        }

        lastIndex = tagRegex.lastIndex;
      }

      if (lastIndex < para.length) {
        const remaining = para.slice(lastIndex);
        if (remaining) {
          resultHTML += this.wrapSegment(
            remaining,
            paraStart + lastIndex,
            currentColor,
            italicActive,
            marks
          );
        }
      }

      htmlParagraphs.push(`<p>${resultHTML || '<br>'}</p>`);
    }

    return htmlParagraphs.join('');
  },

  /**
   * Documento JSON do Tiptap -> texto canônico com tags.
   * Cores/itálico sincronizados por parágrafo; chips emitidos verbatim.
   * Parágrafos unidos com {TEXT_NEWLINE} (espelha WriteLinebreaksAsCommands).
   */
  serializeJSONToGameText(json: JSONContent): string {
    if (!json || !json.content) return '';

    const paragraphTexts: string[] = [];

    for (const block of json.content) {
      let blockText = '';
      let currentColorTag: string | null = null;
      let italicActive = false;

      if (block.content && block.content.length > 0) {
        for (const inlineNode of block.content) {
          if (inlineNode.type === 'gameTag') {
            const value = (inlineNode.attrs?.['value'] as string) || '';
            if (value) blockText += value;
          } else if (inlineNode.type === 'text') {
            const text = inlineNode.text || '';

            const colorMark = inlineNode.marks?.find(
              (m) => m.type === 'textStyle' && m.attrs?.['color']
            );
            const colorVal = colorMark?.attrs?.['color'] as string | undefined;
            const nodeColorTag = colorVal ? getColorTagName(colorVal) : null;

            const hasItalic = !!inlineNode.marks?.some((m) => m.type === 'italic');

            if (hasItalic !== italicActive) {
              blockText += hasItalic ? '{TEXT_ITALIC}' : '{TEXT_NORMAL}';
              italicActive = hasItalic;
            }

            if (nodeColorTag !== currentColorTag) {
              if (nodeColorTag) {
                blockText += `{CLR:${nodeColorTag}}`;
              } else if (currentColorTag) {
                blockText += `{CLR:${COLOR_RESET_TAG}}`;
              }
              currentColorTag = nodeColorTag;
            }

            blockText += text;
          } else if (inlineNode.type === 'hardBreak') {
            blockText += '{TEXT_NEWLINE}';
          }
        }

        if (currentColorTag) {
          blockText += `{CLR:${COLOR_RESET_TAG}}`;
          currentColorTag = null;
        }
        if (italicActive) {
          blockText += '{TEXT_NORMAL}';
          italicActive = false;
        }
      }

      paragraphTexts.push(blockText);
    }

    return paragraphTexts.join('{TEXT_NEWLINE}');
  },

  classifyColorTag(inner: string): { hex: string | null; isReset: boolean } | null {
    const upper = inner.trim().toUpperCase();
    if (upper.startsWith('CLR:')) {
      return getResolvedColorFromTag(upper.slice(4));
    }
    if (upper.startsWith('COLOR:')) {
      return getResolvedColorFromTag(upper.slice(6));
    }
    return null;
  },

  classifyFontTag(inner: string): 'italic' | 'normal' | null {
    const upper = inner.trim().toUpperCase();
    if (upper === 'TEXT_ITALIC') return 'italic';
    if (upper === 'TEXT_NORMAL') return 'normal';
    return null;
  },

  isChipTag(inner: string): boolean {
    const upper = inner.trim().toUpperCase();
    // TEXT_NEWLINE já foi consumido no split; se sobrar, trata como chip
    // para nunca perder conteúdo.
    return upper.length > 0;
  },

  wrapSegment(
    text: string,
    absoluteStart: number,
    color: string | null,
    italic: boolean,
    marks: readonly ResolvedMark[]
  ): string {
    const absoluteEnd = absoluteStart + text.length;
    const hits = marks
      .filter((mark) => mark.end > absoluteStart && mark.start < absoluteEnd)
      .sort((a, b) => a.start - b.start);
    if (hits.length === 0) {
      return this.wrapStyledText(text, color, italic);
    }
    // Os cortes são ANTES do escape (escapar muda o comprimento); cada trecho
    // é escapado individualmente e o <mark> entra no meio, POR DENTRO de
    // itálico/cor — mesma aparência da tabela.
    let html = '';
    let cursor = 0; // offset relativo ao início do segmento já emitido
    for (const mark of hits) {
      const from = Math.max(mark.start, absoluteStart) - absoluteStart;
      const to = Math.min(mark.end, absoluteEnd) - absoluteStart;
      if (from > cursor) html += this.escapeHtml(text.slice(cursor, from));
      html += `<mark${mark.current ? ' class="find-current"' : ''}>${this.escapeHtml(
        text.slice(from, to)
      )}</mark>`;
      cursor = to;
    }
    if (cursor < text.length) html += this.escapeHtml(text.slice(cursor));
    return this.wrapStyledTextHtml(html, color, italic);
  },

  wrapStyledText(text: string, color: string | null, italic: boolean): string {
    return this.wrapStyledTextHtml(this.escapeHtml(text), color, italic);
  },

  /** Itálico/cor sobre HTML JÁ escapado (pode conter <mark> interno). */
  wrapStyledTextHtml(
    html: string,
    color: string | null,
    italic: boolean
  ): string {
    if (italic) html = `<em>${html}</em>`;
    if (color) html = `<span style="color: ${color}">${html}</span>`;
    return html;
  },

  /** markStart/markEnd legado OU marks[] -> intervalos UTF-16 (pode vir vazio). */
  resolveMarks(
    rawText: string,
    options?: GameTextMarkOptions
  ): ResolvedMark[] {
    const out: ResolvedMark[] = [];
    if (options?.markStart != null && options?.markEnd != null) {
      const start = toUtf16Index(rawText, Math.max(0, options.markStart));
      const end = toUtf16Index(
        rawText,
        Math.max(options.markEnd, options.markStart)
      );
      if (end > start) out.push({ start, end, current: false });
    }
    for (const mark of options?.marks ?? []) {
      const start = toUtf16Index(rawText, Math.max(0, mark.start));
      const end = toUtf16Index(rawText, Math.max(mark.end, mark.start));
      if (end > start) {
        out.push({ start, end, current: mark.current === true });
      }
    }
    return out.sort((a, b) => a.start - b.start || a.end - b.end);
  },

  /** Mark que intersecta o intervalo dado (o atual vence, senão o 1º). */
  markOverlapping(
    marks: readonly ResolvedMark[],
    start: number,
    end: number
  ): ResolvedMark | null {
    const hits = marks.filter((mark) => mark.start < end && mark.end > start);
    return hits.find((mark) => mark.current) ?? hits[0] ?? null;
  },

  /**
   * Escapa texto de nó (mesmo algoritmo da serialização HTML): só `&`, `<`,
   * `>` e nbsp. Escapar aspas (como `&quot;`/`&#039;`) tornaria o HTML
   * diferente do getHTML() — a comparação é literal e o setContent só roda
   * quando o documento realmente divergiu.
   */
  escapeHtml(text: string): string {
    return text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/\u00A0/g, '&nbsp;');
  },

  /**
   * Escapa valor de atributo (idem serialização HTML): `&`, `"`, nbsp e CR.
   */
  escapeAttr(text: string): string {
    return text
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/\u00A0/g, '&nbsp;')
      .replace(/\r/g, '&#13;');
  },
};

