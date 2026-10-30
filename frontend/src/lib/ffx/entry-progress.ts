import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { isBlankText } from './blank-us';
import { isRefText } from './hash-ref';
import { SOURCE_LANG } from './save-all';

/** Progresso de tradução por row da entrada aberta — todos os formatos
 * (events/objects/macro/lockit/help). */
export interface EntryProgress {
  /** Rows traduzidas (us != original e não vazio). */
  translated: number;
  /** Rows comparáveis (original us não vazio). */
  total: number;
  /** 0..100 (0 quando total = 0). */
  pct: number;
}

/**
 * Progresso por row da entrada: conta sobre as rows COM original de data/
 * (comparável). Traduzida = text.us difere do original e não vazio.
 *
 * Rows de referência dedup ($hash) ficam FORA da contagem: o texto delas
 * vive na def (em outro ponto) — o progresso do arquivo é sobre o que ele
 * mesmo carrega.
 *
 * Rows em branco no 'us' (vazio/espaço/"-") também ficam FORA: não são
 * traduzíveis (o próprio export as descarta) — não podem minguar o
 * percentual como "nunca traduzidas".
 *
 * A comparação é TEXTE vs ORIGINAL: o hash da row vem do original após o
 * merge (ponteiro do domínio display), então não serve para detectar o
 * estado da célula.
 */
export function entryProgress(entry: dto.FileEntry | null | undefined): EntryProgress {
  const rows = entry?.rows ?? [];
  let translated = 0;
  let total = 0;
  for (const row of rows) {
    const original = row.original?.[SOURCE_LANG] ?? '';
    if (original === '') continue;
    if (isRefText(row.text?.[SOURCE_LANG], row.hash?.[SOURCE_LANG])) continue;
    if (isBlankText(row.text?.[SOURCE_LANG])) continue;
    total++;
    const text = row.text?.[SOURCE_LANG] ?? '';
    if (text !== '' && text !== original) translated++;
  }
  return {
    translated,
    total,
    pct: total > 0 ? Math.round((translated / total) * 100) : 0,
  };
}
