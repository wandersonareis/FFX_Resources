// Duplo clique sem o espaço à direita.
//
// O Chromium seleciona a palavra JUNTO com o espaço seguinte em
// contenteditable. Numa seleção de uma linha só isso não traz benefício e
// atrapalha: aplicar cor/itálico no "word " colore o espaço e muda o texto
// canônico sem o tradutor pedir. Ajustamos a seleção nativa para terminar no
// último caractere não-branco.
//
// Regras para não brigar com a seleção nativa:
//   - só age em `dblclick` (detail 2), nunca em triplo clique (parágrafo);
//   - só dentro do MESMO textblock (seleção entre parágrafos fica intacta);
//   - só apara espaços/tabs/NBSP no fim (nunca esvazia a seleção).
import { Extension } from '@tiptap/core';
import { Plugin, TextSelection, type EditorState } from '@tiptap/pm/state';

const TRAILING_WHITESPACE = /[\t \u00A0]+$/;

function selectionToTextEnd(state: EditorState): TextSelection | null {
  const selection = state.selection;
  if (!(selection instanceof TextSelection) || selection.empty) return null;
  const { $from, $to } = selection;
  // Duplo clique nunca atravessa blocos; se atravessar, não mexe.
  if ($from.parent !== $to.parent) return null;

  const text = state.doc.textBetween(selection.from, selection.to, '\n', '\uFFFC');
  const trimmed = text.replace(TRAILING_WHITESPACE, '');
  const removed = text.length - trimmed.length;
  if (removed <= 0 || trimmed.length === 0) return null;

  return TextSelection.create(state.doc, selection.from, selection.to - removed);
}

/** Ajusta a seleção do duplo clique (exportada para teste). */
export function trimDoubleClickSelection(state: EditorState): TextSelection | null {
  return selectionToTextEnd(state);
}

export function createDoubleClickSelectionExtension(): Extension {
  return Extension.create({
    name: 'doubleClickSelection',

    addProseMirrorPlugins() {
      return [
        new Plugin({
          props: {
            handleDOMEvents: {
              dblclick: (view, event) => {
                if (event.detail >= 3) return false;
                // Espera o Chromium terminar de aplicar a seleção de palavra.
                setTimeout(() => {
                  if (view.isDestroyed) return;
                  const next = selectionToTextEnd(view.state);
                  if (next) view.dispatch(view.state.tr.setSelection(next));
                }, 0);
                return false;
              },
            },
          },
        }),
      ];
    },
  });
}
