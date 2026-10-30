import * as dto from '@/bindings/ffxresources/backend/dto/models';
import type { EntryKind } from './display-names';
import { editDraft, rowKey } from './edit-draft';
import type { GameVersionId } from './game-version';
import { linkedRowText } from './hash-ref';
import { SOURCE_LANG } from './save-all';
import type { EntryRow } from './tree-data';

/**
 * Régua da coluna "Traduzido" extraída para um módulo: a MESMA lógica que o
 * accessor da tabela pinta (divergente > link > rascunho > texto do binário)
 * e o MESMO roteamento do editor (ref dedupada edita a DEF) — a busca local
 * enxerga e substitui exatamente o que a célula mostra, sem divergir do
 * comportamento de edição manual.
 */

/** Onde o rascunho registra a edição da célula (self ou DEF da ref). */
export interface TranslatedEditTarget {
  kind: EntryKind;
  /** Entrada dona do texto: a aberta ou a da def (possivelmente outra). */
  id: string;
  /** "" (data/) ou caminho do .vbf — a fonte do rascunho. */
  source: string;
  /** Row endereçada no rascunho (def sintetizada quando vive em outra entry). */
  row: dto.TextRow;
  /** Idioma exibido/escrito ('us' no fluxo normal; textLoc no .vbf). */
  lang: string;
}

export interface TranslatedTextDeps {
  version: GameVersionId;
  entry: EntryRow | null;
  refLinks: Record<string, dto.RefLink>;
  /** Idioma do binário exibido (SOURCE_LANG no fluxo normal de data/). */
  textLoc: string;
}

/**
 * Texto da célula LINKADA (row de ref): estado atual da def — rascunho dela
 * inclusive. O "$hash" do backend nunca é pintado. `undefined` = não é ref.
 */
export function linkedValueOf(row: dto.TextRow, deps: TranslatedTextDeps): string | undefined {
  const link = deps.refLinks[rowKey(row)];
  if (!link) return undefined;
  const entry = deps.entry;
  if (!entry) return link.text ?? '';
  return linkedRowText(entry.id, link, (defId, defKey) =>
    editDraft.editTextOf(
      deps.version,
      entry.kind,
      defId,
      defKey,
      SOURCE_LANG,
      entry.vbf?.root ?? '',
    ),
  );
}

/**
 * Valor EXIBIDO da coluna Traduzido — fonte única do accessor da tabela e
 * da busca local (os dois enxergam o mesmo texto, na mesma ordem de
 * precedência).
 */
export function displayedTranslatedText(row: dto.TextRow, deps: TranslatedTextDeps): string {
  const entry = deps.entry;
  const edited = entry
    ? editDraft.editOf(deps.version, entry.kind, entry.id, row, SOURCE_LANG, entry.vbf?.root ?? '')
    : undefined;
  const divergent = entry
    ? editDraft.isDivergent(deps.version, entry.kind, entry.id, row, entry.vbf?.root ?? '')
    : false;
  // Enquanto a escolha de divergência está no rascunho, a edição local
  // vence o link (mesma régua da coluna).
  if (divergent && edited !== undefined) return edited;
  const linked = linkedValueOf(row, deps);
  if (linked !== undefined) return linked;
  return edited ?? row.text?.[deps.textLoc] ?? '';
}

/**
 * Onde registrar a edição desta célula — o MESMO roteamento do editor:
 * row de ref dedupada edita a DEF (mesmo quando ela vive em OUTRA entrada;
 * a row da def é sintetizada igual à do openLinked — o rascunho endereça
 * por rowKey e resolve o original pela base). Divergente edita a própria
 * row. `null` = sem entrada aberta.
 */
export function translatedEditTarget(
  row: dto.TextRow,
  deps: TranslatedTextDeps,
): TranslatedEditTarget | null {
  const entry = deps.entry;
  if (!entry) return null;
  const divergent = editDraft.isDivergent(
    deps.version,
    entry.kind,
    entry.id,
    row,
    entry.vbf?.root ?? '',
  );
  if (!divergent) {
    const link = deps.refLinks[rowKey(row)];
    if (link) {
      // Def da MESMA entrada ou sem origem anotada: a row é a própria def,
      // endereçada pelo índice/nome do link (caso exista na tabela ou não).
      return {
        kind: entry.kind,
        id: link.sourceId ?? entry.id,
        source: entry.vbf?.root ?? '',
        row: dto.TextRow.createFrom({
          index: link.sourceIndex,
          name: link.sourceName,
        }),
        // O texto da def é SEMPRE o 'us' (o link aponta o slot canônico).
        lang: SOURCE_LANG,
      };
    }
  }
  // Row comum (ou divergente): o texto é DESTA row. O idioma exibido é o
  // do binário, mas rascunho em 'us' vence a exibição — quando já existe
  // edição 'us', é nela que a substituição precisa cair para aparecer.
  const editedUs =
    deps.textLoc !== SOURCE_LANG
      ? editDraft.editOf(
          deps.version,
          entry.kind,
          entry.id,
          row,
          SOURCE_LANG,
          entry.vbf?.root ?? '',
        )
      : undefined;
  return {
    kind: entry.kind,
    id: entry.id,
    source: entry.vbf?.root ?? '',
    row,
    lang: deps.textLoc !== SOURCE_LANG && editedUs === undefined ? deps.textLoc : SOURCE_LANG,
  };
}
