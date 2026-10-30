// Limpeza de marcas de estilo vazias.
//
// `unsetColor()` com o cursor vazio grava a marca `textStyle` com
// `color: null`; o texto digitado depois carrega essa marca e o renderHTML do
// TextStyle emite um `<span>` sem atributos. Além de sujar o documento, esse
// span faz o HTML do editor divergir do parse do valor (que nunca gera span
// vazio) — e qualquer comparação de HTML/`setContent` acaba reconstruindo o
// documento no meio da digitação (cursor/seleção saltando).
//
// O comando `removeEmptyTextStyle` do Tiptap só age na seleção corrente. Este
// plugin varre o documento inteiro a cada mudança e remove as marcas
// `textStyle` cujos atributos estão todos vazios.
import { Extension } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';

function isEmptyTextStyleMark(attrs: Record<string, unknown>): boolean {
  return Object.values(attrs).every(
    (value) => value === null || value === undefined || value === ''
  );
}

export const CleanMarksExtension = Extension.create({
  name: 'cleanMarks',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        appendTransaction: (_transactions, _oldState, newState) => {
          const textStyle = newState.schema.marks['textStyle'];
          if (!textStyle) return null;

          const tr = newState.tr;
          let cleaned = false;
          newState.doc.descendants((node, pos) => {
            if (!node.isText) return true;
            for (const mark of node.marks) {
              if (mark.type !== textStyle) continue;
              if (!isEmptyTextStyleMark(mark.attrs)) continue;
              tr.removeMark(pos, pos + node.nodeSize, mark);
              cleaned = true;
            }
            return true;
          });

          if (!cleaned) return null;
          // Limpeza não é uma edição do usuário: não entra no histórico.
          tr.setMeta('addToHistory', false);
          return tr;
        },
      }),
    ];
  },
});
