// Resolução de refs de dedup ($hash) — espelho da regra do backend
// (unmarshalCollection / helpRefBare): o texto é referência quando começa
// com "$" E o resto casa com o hash PRÓPRIO da row. Prefixo sozinho não
// basta: um texto literal que comece com "$" não pode virar linha oculta.

import type * as dto from '@/bindings/ffxresources/backend/dto/models';

/**
 * Kinds cujo view sai com dedup global (refs "$hash" na entrega): os
 * formatos de extração única/agregada — help (arquivo único com os 6
 * painéis), events (eventos gêmeos com repetição maciça), macro (dicionário
 * único em um artefato), objects (repetição massiva intra-arquivo; refs
 * não propagam entre objetos) e as tabelas eventtable (battletext, cloud,
 * tutorial, menumain — mesma régua dos events).
 */
export const DEDUP_VIEW_KINDS: ReadonlySet<string> = new Set([
  'help',
  'events',
  'macro',
  'objects',
  'battletext',
  'cloud',
  'tutorial',
  'menumain',
]);

/**
 * Linha é referência de dedup de `text` (repetição NÃO traduzida de um
 * original definido em outro ponto) no idioma dado?
 *
 * A ref fica VISÍVEL na tabela como texto linkado (LINKED_SIDE + anotação
 * da def na entrega): o tradutor traduz cada original uma vez e as cópias
 * herdam a tradução da def no salvar (o ponteiro é endereçado pelo
 * ORIGINAL, não pelo texto traduzido). Editar pela ref edita a def —
 * nunca cria texto divergente. Texto divergente da def é carga real de
 * revisão e permanece literal.
 */
export function isRefText(
  text: string | undefined | null,
  hash: string | undefined | null,
): boolean {
  if (!text || !hash) return false;
  return text.startsWith('$') && text.slice(1) === hash;
}

/**
 * Cor do texto LINKADO (ref dedupada): distinta do vermelho ("falta aqui",
 * desalinhamento) — céu indica "repetição: o texto vive na def apontada
 * pela anotação de link". Não é erro: é o dupe resolvido visualmente.
 */
export const LINKED_SIDE = 'text-sky-600 dark:text-sky-300';

/**
 * Texto que a célula de uma row de ref DEVE pintar: o estado ATUAL da def
 * (rascunho incluído) — o "$hash" nunca chega ao DOM.
 *
 * A anotação vem do backend em `FileEntry.refs`, chave = `dto.RowKey` =
 * `${index}:${name}` (a mesma que o rascunho usa). `liveOf(defId, defKey)`
 * resgata o que a def tem AGORA, porque ela pode viver em OUTRA entrada
 * (ref de 236 apontando o 235) e já ter rascunho salvo lá.
 *
 * - sem anotação → `undefined`: a row não é ref e segue o caminho normal;
 * - `entryId` é o da entrada aberta: anotação SEM origem (`sourceId`
 *   vazio) aponta a def dentro da própria entrada.
 */
export function linkedRowText(
  entryId: string,
  link: dto.RefLink | null | undefined,
  liveOf: (defId: string, defKey: string) => string | undefined,
): string | undefined {
  if (!link) return undefined;
  const defId = link.sourceId ? link.sourceId : entryId;
  const defKey = `${link.sourceIndex}:${link.sourceName ?? ''}`;
  return liveOf(defId, defKey) ?? link.text ?? '';
}
