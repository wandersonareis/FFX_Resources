import { Extension } from '@tiptap/core';
import type { Node as PMNode } from '@tiptap/pm/model';
import { Plugin } from '@tiptap/pm/state';

/** O bloco ainda existe no documento (algum filho estruturalmente igual)? */
function docContainsBlock(doc: PMNode, block: PMNode): boolean {
  for (let i = 0; i < doc.childCount; i++) {
    if (doc.child(i).eq(block)) return true;
  }
  return false;
}

/**
 * Impede que uma DELEÇÃO vire padding de tela.
 *
 * O padding é o `{TEXT_NEWLINE}` que sobra no início do texto quando o
 * primeiro bloco fica vazio. Isso é desejado quando o usuário cria a quebra
 * (Enter no início, colagem com `{TEXT_NEWLINE}`): o conteúdo antigo é
 * deslocado para o bloco seguinte e continua no documento.
 *
 * Já ao apagar o conteúdo do primeiro bloco (ex.: Delete no chip
 * `{PC:…}` que ocupava a linha sozinho), o parágrafo vazio é apenas sobra e
 * a quebra seguinte NÃO deve ser absorvida como padding. A distinção é
 * estrutural: se o bloco antigo ainda existe em outro lugar, foi um
 * Enter/colagem (mantém); se sumiu, foi deleção (remove os vazios iniciais).
 */
export const LeadingEmptyBlockExtension = Extension.create({
  name: 'leadingEmptyBlock',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        appendTransaction: (transactions, oldState, newState) => {
          if (!transactions.some((tr) => tr.docChanged)) return null;

          const oldFirst = oldState.doc.firstChild;
          const newFirst = newState.doc.firstChild;
          if (!oldFirst || !newFirst) return null;
          // O caso só existe quando havia conteúdo no primeiro bloco e ele
          // terminou vazio.
          if (oldFirst.type.name !== 'paragraph' || oldFirst.content.size === 0) return null;
          if (newFirst.type.name !== 'paragraph' || newFirst.content.size > 0) return null;
          // Enter/colagem deslocam o bloco antigo (ele sobrevive inteiro).
          if (docContainsBlock(newState.doc, oldFirst)) return null;

          const tr = newState.tr;
          // Remove os parágrafos vazios da frente, mas nunca o último bloco
          // do documento (o schema exige ao menos um).
          while (tr.doc.childCount > 1) {
            const first = tr.doc.firstChild;
            if (!first) break;
            if (first.type.name !== 'paragraph' || first.content.size > 0) break;
            tr.delete(0, first.nodeSize);
          }
          if (!tr.docChanged) return null;
          // Limpeza, não edição: não cria passo próprio no histórico.
          tr.setMeta('addToHistory', false);
          return tr;
        },
      }),
    ];
  },
});
