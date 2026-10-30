// Converte `{…}` digitado pelo tradutor em chip atômico, no mesmo formato
// que o parser emite — assim o HTML do documento e o valor serializado nunca
// divergem (o setContent perderia a posição do cursor).
//
// Roda em appendTransaction (não InputRule): não depende da API de InputRule
// e não briga com IME. Tags de formatação (CLR/COLOR/TEXT_*) ficam de fora:
// viram cor/itálico/parágrafo no parser e nunca podem virar chip.
import { Extension } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';
import { extractChipLabel, isLockedTagInner } from '@/lib/ffx/game-text-tags';
import type { ChipValidator } from '@/lib/ffx/tag-catalog';

const TAG_TEXT_RE = /\{([^{}]+)\}/g;

/** Tag que o parser transforma em marca/parágrafo (nunca chip). */
export function isFormattingTagText(inner: string): boolean {
  const upper = inner.trim().toUpperCase();
  if (upper.startsWith('CLR:') || upper.startsWith('COLOR:')) return true;
  return upper === 'TEXT_ITALIC' || upper === 'TEXT_NORMAL' || upper === 'TEXT_NEWLINE';
}

export function createTypedTagExtension(validate: ChipValidator): Extension {
  return Extension.create({
    name: 'typedTag',

    addProseMirrorPlugins() {
      return [
        new Plugin({
          appendTransaction: (_transactions, _oldState, newState) => {
            const gameTag = newState.schema.nodes.gameTag;
            if (!gameTag) return null;

            const targets: Array<{ from: number; to: number; inner: string }> = [];
            newState.doc.descendants((node, pos) => {
              if (!node.isText || !node.text) return true;
              TAG_TEXT_RE.lastIndex = 0;
              for (const match of node.text.matchAll(TAG_TEXT_RE)) {
                const inner = match[1].trim();
                // `{ }`/`{}`: o parser deixa como texto — não criar chip que
                // a próxima leitura rejeitaria.
                if (!inner || isFormattingTagText(inner)) continue;
                const from = pos + match.index;
                targets.push({ from, to: from + match[0].length, inner });
              }
              return true;
            });
            if (targets.length === 0) return null;

            const tr = newState.tr;
            // De trás p/ frente: trocar um trecho não desloca as posições
            // anteriores, então nenhum mapeamento é necessário.
            for (let i = targets.length - 1; i >= 0; i--) {
              const { from, to, inner } = targets[i];
              if (to > tr.doc.content.size) continue;
              tr.replaceWith(
                from,
                to,
                gameTag.create({
                  value: `{${inner}}`,
                  label: extractChipLabel(inner),
                  locked: isLockedTagInner(inner),
                  invalid: !validate(inner),
                }),
              );
            }
            return tr;
          },
        }),
      ];
    },
  });
}
