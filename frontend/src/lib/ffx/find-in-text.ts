// Busca & substituição local na tabela do arquivo aberto — motor PURO
// (sem React, sem backend). A busca casa apenas o TEXTO VISÍVEL das rows:
// tags {…} (chips, cores, itálico) e quebras de linha nunca participam da
// comparação, e a substituição reescreve somente trechos visíveis — por
// construção, nenhuma tag é alterada.
//
// Os offsets são EM CODE POINTS do texto bruto (a mesma régua do
// game-text-parser), então o resultado serve direto para o destaque
// (<mark> via marks) e para o splice do substituir.

/** Opções da busca local (espelha os botões Aa / palavra inteira). */
export interface FindOptions {
  /** Diferenciar maiúsculas/minúsculas (padrão: insensível). */
  matchCase: boolean;
  /** Casa apenas palavras inteiras (bordas não-palavra). */
  wholeWord: boolean;
}

/** Intervalo em CODE POINTS do texto bruto (inclusivo/inclusivo). */
export interface FindRange {
  start: number;
  end: number;
}

/** Trecho VISÍVEL do texto bruto (offsets UTF-16 no bruto). */
interface VisibleSegment {
  start: number; // inclusivo
  end: number; // exclusivo
}

/**
 * Segmentos de texto visível do texto canônico — o espelho da tokenização
 * do game-text-parser: quebras de linha ({TEXT_NEWLINE}/\r\n/\n/\r) e
 * tokens {…} com conteúdo interno interrompem o trecho visível; `{}`/`{ }`
 * (sem conteúdo) o parser mantém como texto, então aqui também são visíveis.
 */
export function visibleSegments(rawText: string): VisibleSegment[] {
  const segments: VisibleSegment[] = [];
  let start = -1; // -1 = fora de trecho visível
  let i = 0;
  while (i < rawText.length) {
    const newline = newlineLengthAt(rawText, i);
    if (newline > 0) {
      if (start >= 0) segments.push({ start, end: i });
      start = -1;
      i += newline;
      continue;
    }
    if (rawText[i] === '{') {
      const close = rawText.indexOf('}', i + 1);
      const open = rawText.indexOf('{', i + 1);
      // Token {…} sem chaves aninhadas (mesma régua do regex do parser).
      if (close >= 0 && (open === -1 || close < open)) {
        const inner = rawText.slice(i + 1, close);
        if (inner.trim() !== '') {
          if (start >= 0) segments.push({ start, end: i });
          start = -1;
          i = close + 1;
          continue;
        }
      }
    }
    if (start < 0) start = i;
    i += 1;
  }
  if (start >= 0) segments.push({ start, end: rawText.length });
  return segments;
}

/** Comprimento do token de quebra de linha na posição (0 = não é quebra). */
function newlineLengthAt(text: string, index: number): number {
  if (
    text[index] === '{' &&
    text.slice(index, index + '{TEXT_NEWLINE}'.length).toUpperCase() ===
      '{TEXT_NEWLINE}'
  ) {
    return '{TEXT_NEWLINE}'.length;
  }
  if (text[index] === '\r') return text[index + 1] === '\n' ? 2 : 1;
  if (text[index] === '\n') return 1;
  return 0;
}

/**
 * Todas as ocorrências de `query` no texto VISÍVEL (não sobrepostas, na
 * ordem do texto). `query` literal — busca local não aceita regex.
 */
export function findMatches(
  rawText: string,
  query: string,
  options: FindOptions
): FindRange[] {
  if (!rawText || !query) return [];
  const needle = options.matchCase ? query : query.toLowerCase();
  if (!needle) return [];

  const matches: FindRange[] = [];
  let cpBefore = 0; // code points do bruto consumidos ANTES do segmento
  let prevEnd = 0; // UTF-16 do fim do segmento anterior
  for (const segment of visibleSegments(rawText)) {
    cpBefore += countCodePoints(rawText.slice(prevEnd, segment.start));
    const text = rawText.slice(segment.start, segment.end);
    const haystack = options.matchCase ? text : text.toLowerCase();
    let from = 0;
    for (;;) {
      const at = haystack.indexOf(needle, from);
      if (at < 0) break;
      const end = at + needle.length;
      if (!options.wholeWord || isWholeWord(text, at, end)) {
        matches.push({
          start: cpBefore + countCodePoints(text.slice(0, at)),
          end: cpBefore + countCodePoints(text.slice(0, end)),
        });
      }
      from = end;
    }
    cpBefore += countCodePoints(text);
    prevEnd = segment.end;
  }
  return matches;
}

/**
 * Substitui os intervalos dados por `replacement` (literal, sem $1). Os
 * matches entram em ordem ASCENDENTE (a chamada é responsável por isso).
 */
export function replaceMatches(
  rawText: string,
  matches: readonly FindRange[],
  replacement: string
): string {
  if (matches.length === 0) return rawText;
  // Corta de trás pra frente: os offsets anteriores não deslocam.
  const cuts = matches
    .map((m) => ({
      start: utf16Index(rawText, m.start),
      end: utf16Index(rawText, m.end),
    }))
    .sort((a, b) => b.start - a.start);
  let out = rawText;
  for (const cut of cuts) {
    out = out.slice(0, cut.start) + replacement + out.slice(cut.end);
  }
  return out;
}

// ---- helpers ----

/** Code points num trecho (surrogates contam como 1). */
function countCodePoints(text: string): number {
  let count = 0;
  for (let i = 0; i < text.length; ) {
    const code = text.codePointAt(i) ?? 0;
    i += code > 0xffff ? 2 : 1;
    count += 1;
  }
  return count;
}

/** Offset em code points -> índice UTF-16 (clamp nas pontas). */
function utf16Index(text: string, codePoints: number): number {
  let count = 0;
  for (let i = 0; i < text.length; ) {
    if (count >= codePoints) return i;
    const code = text.codePointAt(i) ?? 0;
    i += code > 0xffff ? 2 : 1;
    count += 1;
  }
  return text.length;
}

/** Letra/dígito/_ (borda de palavra) — Unicode-aware. */
const WORD_CHAR = /[\p{L}\p{N}_]/u;

/**
 * Match é uma palavra inteira? As bordas precisam ser não-palavra (ou o
 * limite do trecho visível — a tag/quebra ao redor já é fronteira).
 */
function isWholeWord(text: string, start: number, end: number): boolean {
  const before =
    start > 0 ? lastCodePoint(text.slice(Math.max(0, start - 2), start)) : '';
  const after = end < text.length ? firstCodePoint(text.slice(end, end + 2)) : '';
  return !isWordChar(before) && !isWordChar(after);
}

function isWordChar(codePoint: string): boolean {
  return codePoint !== '' && WORD_CHAR.test(codePoint);
}

function lastCodePoint(text: string): string {
  const points = [...text];
  return points.length > 0 ? points[points.length - 1] : '';
}

function firstCodePoint(text: string): string {
  for (const point of text) return point;
  return '';
}
