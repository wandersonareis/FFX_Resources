import { Schema } from '@tiptap/pm/model';
import { EditorState, TextSelection } from '@tiptap/pm/state';
import { describe, expect, it } from 'vitest';
import { trimDoubleClickSelection } from './double-click-selection.extension';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: {
      content: 'inline*',
      group: 'block',
      toDOM: () => ['p', 0],
      parseDOM: [{ tag: 'p' }],
    },
    text: { group: 'inline' },
  },
});

function stateWith(text: string, from: number, to: number): EditorState {
  const doc = schema.node('doc', null, [schema.node('paragraph', null, [schema.text(text)])]);
  return EditorState.create({
    doc,
    selection: TextSelection.create(doc, from, to),
  });
}

describe('trimDoubleClickSelection', () => {
  it('remove o espaco final que o Chromium inclui no duplo clique', () => {
    // "word next": duplo clique em "word" seleciona 1..6 ("word ").
    const selection = trimDoubleClickSelection(stateWith('word next', 1, 6));
    expect(selection?.from).toBe(1);
    expect(selection?.to).toBe(5);
  });

  it('nao mexe quando a selecao ja termina em texto', () => {
    expect(trimDoubleClickSelection(stateWith('word next', 1, 5))).toBeNull();
  });

  it('nao esvazia uma selecao que e so espaco', () => {
    expect(trimDoubleClickSelection(stateWith('word next', 5, 6))).toBeNull();
  });
});
