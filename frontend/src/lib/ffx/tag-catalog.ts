// Catálogo de tags nomeadas (PC/MCR/BUTTON/ICON) para o autocomplete do
// editor e validação das tags digitadas à mão.
//
// A validação tem 2 camadas:
//   1. gramática: espelho de converter.ParseCommand (string_to_bytes.go) —
//      o que o backend NÃO reconhece como comando vira texto literal com
//      chaves no binário (round-trip não garantido);
//   2. valores: os índices de PC/MCR/BUTTON/ICON precisam existir no catálogo
//      da versão — caso contrário o re-import reescreve a tag (o decoder
//      normaliza o índice/nome) e o texto do tradutor muda sozinho.

import type * as services from '@/bindings/ffxresources/backend/services/models';
import { GetTagCatalog } from '@/bindings/ffxresources/textservice';
import { extractChipLabel, isLockedTagInner } from './game-text-tags';
import type { GameVersionId } from './game-version';

export type TagCatalog = services.TagCatalog;

/** Tags nomeadas sugeridas pelo autocomplete (estágio 1). */
export const TAG_NAMES = ['PC', 'MCR', 'BUTTON', 'ICON'] as const;
export type TagName = (typeof TAG_NAMES)[number];

/** Máximo de itens do estágio 2 por query (filtre para ver mais). */
const MAX_VALUES = 200;

/** Itens exibidos no popup do autocomplete. */
export interface TagSuggestionItem {
  kind: 'tag' | 'value';
  /** Texto inserido ao selecionar: prefixo "{PC:" (estágio 1) ou tag completa. */
  insert: string;
  /** Linha principal do item. */
  label: string;
  /** Linha secundária (detalhe/preview). */
  detail: string;
  /** Atributos do chip atômico a inserir (só no estágio 2). */
  chip?: { value: string; label: string; locked: boolean; invalid: boolean };
}

/** true quando a tag interna sobrevive ao round-trip texto→bytes→texto. */
export type ChipValidator = (inner: string) => boolean;

// ---------------------------------------------------------------------------
// carga do catálogo (cacheada por versão)
// ---------------------------------------------------------------------------

const catalogCache = new Map<GameVersionId, Promise<TagCatalog | null>>();

/**
 * Carrega o catálogo da versão uma única vez (promessa cacheada). Falha de
 * binding → null: o editor segue com validação estrutural (gramática) apenas.
 */
export function loadTagCatalog(version: GameVersionId): Promise<TagCatalog | null> {
  let pending = catalogCache.get(version);
  if (!pending) {
    pending = GetTagCatalog(version as Parameters<typeof GetTagCatalog>[0])
      .then((catalog) => catalog ?? null)
      .catch(() => null);
    catalogCache.set(version, pending);
  }
  return pending;
}

// ---------------------------------------------------------------------------
// gramática: espelho de converter.ParseCommand
// ---------------------------------------------------------------------------

const EXACT_TAGS = new Set([
  'PAUSE',
  'BREAK',
  '\\n',
  'TEXT_NEWLINE',
  'TEXT_ITALIC',
  'TEXT_NORMAL',
  'BLANK05',
  'BLANK0C',
  'BLANK0F',
  'BLANK11',
  'CHOICE-END',
]);

// Um padrão por caso do switch do ParseCommand, na mesma ordem de exigência:
// hex de 1–2 dígitos onde o backend faz ParseUint(...,16,8) e o resto
// aceito/ignorado apenas quando o decode re-emite idêntico.
const GRAMMAR_RULES: RegExp[] = [
  /^(?:SPACE|TIME):[0-9A-Fa-f]{1,2}$/,
  /^CLR:/, // qualquer nome de cor: ColorToByte nunca falha
  /^COLOR:/,
  /^(?:ICON|BUTTON):[0-9A-Fa-f]{1,2}:.*$/,
  /^CHOICE:[0-9A-Fa-f]{1,2}$/,
  /^VAR:[0-9A-Fa-f]{1,2}$/,
  /^PC:[0-9A-Fa-f]{1,2}:.*$/,
  /^MCR:s[0-9A-Fa-f]{1,2}:l[0-9A-Fa-f]{1,2}:.*$/,
  /^KEY:[0-9A-Fa-f]{2}$/,
  /^CMD:[0-9A-Fa-f]{1,2}:[0-9A-Fa-f]{1,2}$/,
  /^UNKCHR:[0-9A-Fa-f]{2}$/,
  /^UNKDBLCHR:[0-9A-Fa-f]{2}:[0-9A-Fa-f]{2}$/,
  /^HEX:[0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2})*$/,
  /^PUA:[0-9A-Fa-f]+(?::.*)?$/,
];

const HEX2 = /^[0-9A-Fa-f]{2}$/;

/** O backend importa `inner` como comando (e não como texto literal)? */
function matchesGrammar(inner: string): boolean {
  if (EXACT_TAGS.has(inner)) return true;
  if (GRAMMAR_RULES.some((rule) => rule.test(inner))) return true;
  // MACRO: exige len >= 12 e hex em cmd[7:9] e cmd[10:12] (mesmo corte do
  // backend; o prefixo legado não é emitido pelo decoder).
  if (inner.startsWith('MACRO:')) {
    return inner.length >= 12 && HEX2.test(inner.slice(7, 9)) && HEX2.test(inner.slice(10, 12));
  }
  return false;
}

// ---------------------------------------------------------------------------
// valores: pertence ao catálogo da versão?
// ---------------------------------------------------------------------------

/** Identidade do valor dentro do catálogo: ["PC","01"] / ["MCR","s01:l05"]. */
function semanticKey(inner: string): [tag: string, key: string] | null {
  const named = /^(PC|BUTTON|ICON):([0-9A-Fa-f]{1,2}):/.exec(inner);
  if (named) return [named[1], named[2].toUpperCase()];
  const mcr = /^MCR:s([0-9A-Fa-f]{1,2}):l([0-9A-Fa-f]{1,2}):/.exec(inner);
  if (mcr) return ['MCR', `s${mcr[1].toUpperCase()}:l${mcr[2].toUpperCase()}`];
  return null;
}

function buildKeySets(catalog: TagCatalog): Map<string, Set<string>> {
  const sets = new Map<string, Set<string>>();
  for (const entry of catalog.tags ?? []) {
    const keys = new Set<string>();
    for (const value of entry.values ?? []) keys.add(value.key.toLowerCase());
    sets.set(entry.tag.toUpperCase(), keys);
  }
  return sets;
}

/**
 * Cria o validador de chips para o catálogo. `null` → só gramática (usado
 * quando o catálogo não carregou). A mesma instância deve valer para o parse
 * e para a conversão de tags digitadas, senão o HTML divergiria do valor.
 */
export function createChipValidator(catalog: TagCatalog | null): ChipValidator {
  const sets = catalog ? buildKeySets(catalog) : null;
  return (inner: string) => {
    const trimmed = inner.trim();
    if (!matchesGrammar(trimmed)) return false;
    if (!sets) return true;
    const semantic = semanticKey(trimmed);
    if (!semantic) return true;
    const keys = sets.get(semantic[0]);
    return keys ? keys.has(semantic[1].toLowerCase()) : false;
  };
}

const validatorCache = new WeakMap<TagCatalog, ChipValidator>();
let structuralValidator: ChipValidator | null = null;

/** Validador cacheado por catálogo (uma instância = um documento coerente). */
export function getChipValidator(catalog: TagCatalog | null): ChipValidator {
  if (!catalog) {
    structuralValidator ??= createChipValidator(null);
    return structuralValidator;
  }
  let validator = validatorCache.get(catalog);
  if (!validator) {
    validator = createChipValidator(catalog);
    validatorCache.set(catalog, validator);
  }
  return validator;
}

// ---------------------------------------------------------------------------
// queries do suggestion
// ---------------------------------------------------------------------------

/**
 * A query parece uma tentativa de tag nomeada? Controla o `shouldShow` do
 * suggestion: texto corrido após `{` não abre o popup nem marca a decoração.
 */
export function looksLikeTagAttempt(query: string): boolean {
  if (query === '') return true;
  const colon = query.indexOf(':');
  if (colon >= 0) {
    return (TAG_NAMES as readonly string[]).includes(query.slice(0, colon).toUpperCase());
  }
  const upper = query.toUpperCase();
  return TAG_NAMES.some((name) => name.startsWith(upper));
}

/**
 * A query é um parcial de tag que o autocomplete pode ter gerado e que deve
 * ser removido ao desistir (Escape/clique fora)? `""` (só `{`), prefixo de
 * tag nomeada ("P", "MCR") ou `TAG:` + filtro. Query já fechada com `}` NÃO
 * é parcial: ou o usuário digitou a tag inteira ou ela já virou chip.
 */
export function isPartialTagQuery(query: string): boolean {
  if (query.endsWith('}')) return false;
  return looksLikeTagAttempt(query);
}

/**
 * Itens do popup para a query atual. Query sem `:` → estágio 1 (nomes);
 * `TAG:filtro` → valores do catálogo filtrados por índice ou texto.
 */
export function computeSuggestions(query: string, catalog: TagCatalog | null): TagSuggestionItem[] {
  const colon = query.indexOf(':');

  if (colon < 0) {
    const upper = query.toUpperCase();
    return TAG_NAMES.filter((name) => name.startsWith(upper)).map((name) => {
      const entry = catalog?.tags?.find((t) => t.tag === name);
      return {
        kind: 'tag' as const,
        insert: entry?.prefix ?? `{${name}:`,
        label: name,
        detail: entry?.hint ?? '',
      };
    });
  }

  const name = query.slice(0, colon).toUpperCase();
  if (!(TAG_NAMES as readonly string[]).includes(name)) return [];
  const entry = catalog?.tags?.find((t) => t.tag === name);
  if (!entry) return [];

  const filter = query
    .slice(colon + 1)
    .trim()
    .toLowerCase();
  const validator = getChipValidator(catalog);
  return (entry.values ?? [])
    .filter(
      (value) =>
        !filter ||
        value.key.toLowerCase().includes(filter) ||
        value.label.toLowerCase().includes(filter),
    )
    .slice(0, MAX_VALUES)
    .map((value) => {
      const inner = value.tag.slice(1, -1);
      return {
        kind: 'value' as const,
        insert: value.tag,
        label: value.label,
        // MCR: a tag completa repete o texto; mostra só sXX:lYY como detalhe.
        detail: name === 'MCR' ? value.key : value.tag,
        chip: {
          value: value.tag,
          label: extractChipLabel(inner),
          locked: isLockedTagInner(inner),
          invalid: !validator(inner),
        },
      };
    });
}
