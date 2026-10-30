import { type GameTextMark, gameTextParser } from './game-text-parser';
import { splitLeadingPadding, summarizePadding } from './leading-padding';

/**
 * Orquestração PURA da formatação do texto do jogo — o parser de tags
 * (fonte única compartilhada com tabela, refs e editor) montado nos dois
 * usos da UI:
 *
 * - `formatGameTextForDisplay`: exibição completa (GameTextView) — padding
 *   de tela resumido + corpo formatado, o MESMO resultado de sempre;
 * - `formatGameTextSnippetForSearch`: snippet do modal de busca — a mesma
 *   formatação, com o trecho do hit envolvido em <mark>.
 *
 * Nada de React aqui: string entra, HTML escapado sai.
 */

export interface FormattedGameText {
  /** Resumo do padding ("{TEXT_NEWLINE} ×2 · ␣ ×4"); '' quando não há. */
  paddingSummary: string;
  /** HTML do CORPO (prefixo de padding removido), escapado pelo parser. */
  html: string;
}

export interface DisplayFormatOptions {
  /**
   * Só a TABELA: chip `LF` no fim de cada quebra real de linha, para não
   * confundir com o wrap de palavra do CSS. Desligado (default) = HTML
   * byte-idêntico ao de sempre (refs do diálogo, editor, busca).
   */
  newlineChip?: boolean;
}

/**
 * Exibição completa de um texto canônico, com destaque opcional da busca
 * local (`marks` em code points do texto COM padding — o prefixo é removido
 * aqui e os intervalos andam junto, igual ao snippet da busca geral).
 */
export function formatGameTextForDisplay(
  text?: string,
  marks?: readonly GameTextMark[],
  options?: DisplayFormatOptions,
): FormattedGameText {
  const { padding, body } = splitLeadingPadding(text ?? '');
  const shift = [...padding].length;
  const shifted =
    marks && marks.length > 0 && shift > 0
      ? marks.map((mark) => ({
          ...mark,
          start: Math.max(0, mark.start - shift),
          end: Math.max(0, mark.end - shift),
        }))
      : marks;
  return {
    paddingSummary: summarizePadding(padding),
    // {} (sem marks e sem chip) resolve para o MESMO resultado de passar
    // undefined — resolveMarks vazio e newlineChip false não tocam no HTML.
    html: gameTextParser.parseGameTextToHTML(body, undefined, {
      marks: shifted && shifted.length > 0 ? shifted : undefined,
      newlineChip: options?.newlineChip === true,
    }),
  };
}

/** Janela do snippet devolvida pelo backend (corte em rune, "…" de corte). */
export interface SearchSnippetParts {
  before: string;
  hit: string;
  after: string;
}

/**
 * Snippet da busca formatado com o hit destacado. A remontagem
 * `before + hit + after` é lossless (o backend corta em rune, nunca parte
 * uma tag no meio), então o parser vê a janela ORIGINAL e o destaque é só
 * um intervalo dentro dela — chips e cores saem iguais aos da tabela.
 *
 * Quebra de linha vira `\n` VISÍVEL (2 chars no lugar de até 14 de
 * `{TEXT_NEWLINE}`) fora do hit: o snippet comum vira um único parágrafo e
 * a quebra aparece na lista — sem união de palavras e sem depender de
 * separador CSS entre parágrafos.
 */
export function formatGameTextSnippetForSearch(parts: SearchSnippetParts): string {
  const { before, hit, after } = parts;
  const full = before + hit + after;
  if (!full) return '';

  // Padding de tela sai da linha (a tabela mostra o resumo; o snippet é
  // compacto) — o intervalo do hit anda junto com o corte do prefixo.
  const { padding, body } = splitLeadingPadding(full);
  const shift = [...padding].length;
  const marked = markNewlines(
    [...body],
    Math.max(0, [...before].length - shift),
    Math.max(0, [...(before + hit)].length - shift),
  );
  return gameTextParser.parseGameTextToHTML(marked.chars.join(''), undefined, {
    markStart: marked.start,
    markEnd: marked.end,
  });
}

/** Marcador VISÍVEL de quebra no snippet: barra + n (2 code points). */
const NEWLINE_MARKER = '\\n';

/**
 * Substitui as quebras de linha por `\n` visível FORA do intervalo do hit —
 * token que intersecta o hit fica como está (é o texto que casou, e o
 * parser o trata como a quebra real que é). Trabalha em code points; as
 * trocas antes do hit deslocam o intervalo pelo delta de comprimento.
 */
function markNewlines(
  chars: readonly string[],
  markStart: number,
  markEnd: number,
): { chars: string[]; start: number; end: number } {
  const out: string[] = [];
  let delta = 0;
  let i = 0;
  while (i < chars.length) {
    const token = newlineTokenAt(chars, i);
    if (token) {
      const tokenEnd = i + token.length;
      const outside = tokenEnd <= markStart || i >= markEnd;
      if (outside) {
        // Só as trocas ANTES do hit movem o intervalo (as depois ficam
        // depois — nenhuma das pontas muda).
        if (tokenEnd <= markStart) delta += NEWLINE_MARKER.length - token.length;
        out.push(NEWLINE_MARKER);
        i = tokenEnd;
        continue;
      }
      // Intersecta o hit: cópia literal, token a token (sem marker).
    }
    out.push(chars[i]);
    i += 1;
  }
  return { chars: out, start: markStart + delta, end: markEnd + delta };
}

/** Token de quebra na posição (tag do jogo case-insensitive, CRLF, LF, CR). */
function newlineTokenAt(chars: readonly string[], index: number): string | null {
  const tag = chars
    .slice(index, index + '{TEXT_NEWLINE}'.length)
    .join('')
    .toUpperCase();
  if (tag === '{TEXT_NEWLINE}') return tag;
  if (chars[index] === '\r' && chars[index + 1] === '\n') return '\r\n';
  if (chars[index] === '\n' || chars[index] === '\r') return chars[index];
  return null;
}
