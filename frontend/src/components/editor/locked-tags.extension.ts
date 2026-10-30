import { Extension } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';

/**
 * Protege chips bloqueados (CMD, HEX, UNK e VAR — valores calculados no jogo):
 * Backspace/Delete sobre ou ao redor de um gameTag locked é vetado.
 * Nós atômicos já não permitem edição de texto interna.
 */
export const LockedTagsExtension = Extension.create({
  name: 'lockedTags',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        props: {
          handleKeyDown: (view, event) => {
            if (event.key !== 'Backspace' && event.key !== 'Delete') return false;

            const { selection } = view.state;

            // Seleção em intervalo contendo locked -> veta tudo.
            if (!selection.empty) {
              let found = false;
              view.state.doc.nodesBetween(selection.from, selection.to, (node) => {
                if (node.type.name === 'gameTag' && node.attrs.locked) {
                  found = true;
                  return false;
                }
                return true;
              });
              if (found) {
                event.preventDefault();
                return true;
              }
              return false;
            }

            // Cursor: verifica o nó vizinho na direção da tecla.
            const $pos = selection.$from;
            const neighbor = event.key === 'Backspace' ? $pos.nodeBefore : $pos.nodeAfter;
            if (neighbor && neighbor.type.name === 'gameTag' && neighbor.attrs.locked) {
              event.preventDefault();
              return true;
            }
            return false;
          },
        },
      }),
    ];
  },
});
