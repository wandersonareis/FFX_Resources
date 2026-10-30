// Tags protegidas: valores calculados no jogo que o texto editado não pode
// regenerar sozinho. Sumiram do editado → o salvamento é BLOQUEADO até o
// usuário restaurar (ou editar de novo) — fecha a lacuna do lock de teclado,
// que só cobre Backspace/Delete e não cut/colar por cima.
import { isLockedTagInner } from './game-text-tags';

const TAG_RE = /\{[^{}]+\}/g;

/**
 * A tag interna é protegida? CMD/HEX/UNK* e VAR (valores do jogo, chips
 * bloqueados) + PUA (slot duplicado do encoding: o byte exato importa).
 */
export function isProtectedTagInner(inner: string): boolean {
  const upper = inner.trim().toUpperCase();
  if (upper.startsWith('PUA:') || upper.startsWith('UNK')) return true;
  return isLockedTagInner(inner);
}

/** Contagem das tags protegidas por string exata (multiplicidade importa). */
function protectedCounts(text: string): Map<string, number> {
  const counts = new Map<string, number>();
  for (const match of text.matchAll(TAG_RE)) {
    if (!isProtectedTagInner(match[0].slice(1, -1))) continue;
    counts.set(match[0], (counts.get(match[0]) ?? 0) + 1);
  }
  return counts;
}

/**
 * Tags protegidas do original ausentes no editado, em ordem do original e
 * repetindo as ocorrências (duas iguais, uma sumiu → uma faltando).
 * Lista vazia = nada protegido foi removido → salvamento liberado.
 */
export function missingProtectedTags(original: string, edited: string): string[] {
  const remaining = protectedCounts(edited);
  const missing: string[] = [];
  for (const match of original.matchAll(TAG_RE)) {
    const tag = match[0];
    if (!isProtectedTagInner(tag.slice(1, -1))) continue;
    const left = remaining.get(tag) ?? 0;
    if (left > 0) remaining.set(tag, left - 1);
    else missing.push(tag);
  }
  return missing;
}

// ---------------------------------------------------------------------------
// restauração: reinsere as tags protegidas que sumiram
// ---------------------------------------------------------------------------

/**
 * Máximo de células da DP de LCS (~10 MB de Uint32Array). Acima disso o
 * texto é grande demais para alinhamento palavra-a-palavra → heurística de
 * âncoras.
 */
const MAX_LCS_CELLS = 2_500_000;

/** Tokeniza preservando tags como átomos e cada espaço individual. */
function tokenize(text: string): string[] {
  const tokens: string[] = [];
  let last = 0;
  const tags = /\{[^{}]+\}/g;
  for (let match = tags.exec(text); match !== null; match = tags.exec(text)) {
    pushWords(tokens, text.slice(last, match.index));
    tokens.push(match[0]);
    last = match.index + match[0].length;
  }
  pushWords(tokens, text.slice(last));
  return tokens;
}

function pushWords(tokens: string[], chunk: string): void {
  if (!chunk) return;
  // Espaços um a um (alinhamento estável quando um chip some entre eles);
  // runs de não-espaço como token único.
  const parts = chunk.match(/\s|[^\s]+/g);
  if (parts) tokens.push(...parts);
}

function isProtectedToken(token: string): boolean {
  return (
    token.length > 2 &&
    token.startsWith('{') &&
    token.endsWith('}') &&
    isProtectedTagInner(token.slice(1, -1))
  );
}

/** Alinhamento LCS (reverso na DP) reinserindo protegidos não casados. */
function restoreByLCS(original: string[], edited: string[]): string {
  const n = original.length;
  const m = edited.length;
  const row = m + 1;
  const dp = new Uint32Array((n + 1) * row);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i * row + j] =
        original[i] === edited[j]
          ? dp[(i + 1) * row + (j + 1)] + 1
          : Math.max(dp[(i + 1) * row + j], dp[i * row + (j + 1)]);
    }
  }

  const out: string[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (original[i] === edited[j]) {
      out.push(edited[j]);
      i++;
      j++;
      continue;
    }
    // Empate → avança o original: protegido ainda não casado é reinserido
    // antes do próximo token sobrevivente (posição de origem).
    if (dp[(i + 1) * row + j] >= dp[i * row + (j + 1)]) {
      if (isProtectedToken(original[i])) out.push(original[i]);
      i++;
    } else {
      out.push(edited[j]);
      j++;
    }
  }
  while (i < n) {
    if (isProtectedToken(original[i])) out.push(original[i]);
    i++;
  }
  while (j < m) {
    out.push(edited[j]);
    j++;
  }
  return out.join('');
}

/** Fallback para textos grandes: ancora no trecho vizinho sobrevivente. */
function restoreByAnchors(original: string, edited: string): string {
  let out = edited;
  const parts = original.split(/(\{[^{}]+\})/g);
  for (let k = 0; k < parts.length; k++) {
    const part = parts[k];
    if (!part.startsWith('{') || !part.endsWith('}')) continue;
    if (!isProtectedTagInner(part.slice(1, -1))) continue;
    if (missingProtectedTags(original, out).length === 0) break;
    if (out.includes(part)) continue;

    let at = -1;
    const tail = (parts[k - 1] ?? '').slice(-24);
    if (tail) {
      const idx = out.lastIndexOf(tail);
      if (idx >= 0) at = idx + tail.length;
    }
    if (at < 0) {
      const head = (parts[k + 1] ?? '').slice(0, 24);
      if (head) {
        const idx = out.indexOf(head);
        if (idx >= 0) at = idx;
      }
    }
    out = at >= 0 ? out.slice(0, at) + part + out.slice(at) : out + part;
  }
  return out;
}

/**
 * Reinsere no editado as tags protegidas que existiam no original, na
 * posição alinhada ao texto ao redor (LCS palavra-a-palavra). Se nada faltar,
 * devolve `edited` intacto. A conferência é sempre via missingProtectedTags.
 */
export function restoreProtectedTags(original: string, edited: string): string {
  if (missingProtectedTags(original, edited).length === 0) return edited;
  const originalTokens = tokenize(original);
  const editedTokens = tokenize(edited);
  if (originalTokens.length * editedTokens.length > MAX_LCS_CELLS) {
    return restoreByAnchors(original, edited);
  }
  return restoreByLCS(originalTokens, editedTokens);
}

// ---------------------------------------------------------------------------
// fragmentos parciais ({… sem fechar)
// ---------------------------------------------------------------------------

/** Trecho final aberto com `{` (parcial de tag que vazou no texto). */
const UNCLOSED_FRAGMENT_RE = /\{[^{}]*$/;

export function findUnclosedFragment(text: string): string | null {
  const match = UNCLOSED_FRAGMENT_RE.exec(text);
  return match ? match[0] : null;
}

/**
 * Fragmento aberto que NÃO já existia no original — só a digitação/edição
 * introduziu. O gate de salvamento bloqueia por ele (o original intocado
 * não pode travar a célula).
 */
export function newUnclosedFragment(original: string, edited: string): string | null {
  const now = findUnclosedFragment(edited);
  if (now === null) return null;
  if (now === findUnclosedFragment(original)) return null;
  return now;
}

/**
 * Remove o fragmento aberto (o botão "Remover trecho" do gate). A regex é
 * ancorada no fim, então ele é sempre sufixo — nada além dele é cortado.
 *
 * `original`: quando o texto salvo já terminava em espaço/tab, ele é preservado;
 * caso contrário some também o espaço que o usuário digitou junto com a
 * tentativa abortada (senão sobraria `Olá ` no lugar de `Olá`).
 */
export function removeUnclosedFragment(text: string, original?: string): string {
  const match = UNCLOSED_FRAGMENT_RE.exec(text);
  if (!match) return text;
  const before = text.slice(0, match.index);
  if (original !== undefined && /[ \t]$/.test(original)) return before;
  return before.replace(/[ \t]+$/, '');
}
