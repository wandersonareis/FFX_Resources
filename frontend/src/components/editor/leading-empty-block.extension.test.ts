import { Editor } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { describe, expect, it } from 'vitest';
import { gameTextParser } from '@/lib/ffx/game-text-parser';
import { LeadingEmptyBlockExtension } from './leading-empty-block.extension';

function makeEditor(value: string): Editor {
  const host = document.createElement('div');
  document.body.appendChild(host);
  return new Editor({
    element: host,
    extensions: [
      StarterKit.configure({
        heading: false,
        bulletList: false,
        orderedList: false,
        listItem: false,
        blockquote: false,
        codeBlock: false,
        horizontalRule: false,
        bold: false,
        strike: false,
      }),
      LeadingEmptyBlockExtension,
    ],
    content: gameTextParser.parseGameTextToHTML(value),
  });
}

function serialized(editor: Editor): string {
  return gameTextParser.serializeJSONToGameText(editor.getJSON());
}

describe('LeadingEmptyBlockExtension', () => {
  it('apagar o conteúdo do primeiro bloco remove o vazio inicial (sem virar padding)', () => {
    const editor = makeEditor('primeira{TEXT_NEWLINE}segunda');
    // Deleta o texto do primeiro parágrafo ("primeira"), deixando-o vazio.
    editor.view.dispatch(editor.state.tr.delete(1, 9));

    expect(editor.state.doc.childCount).toBe(1);
    expect(serialized(editor)).toBe('segunda');
    editor.destroy();
  });

  it('quebra criada no início (Enter) mantém o parágrafo vazio e vira padding', () => {
    const editor = makeEditor('Hello');
    editor.view.dispatch(editor.state.tr.split(1));

    expect(editor.state.doc.childCount).toBe(2);
    expect(serialized(editor)).toBe('{TEXT_NEWLINE}Hello');
    editor.destroy();
  });

  it('documento só com um parágrafo vazio não é quebrado', () => {
    const editor = makeEditor('Hello');
    // Apaga todo o conteúdo: resta um parágrafo vazio e o schema continua
    // válido (o plugin nunca remove o último bloco).
    editor.view.dispatch(editor.state.tr.delete(1, 6));

    expect(editor.state.doc.childCount).toBe(1);
    expect(serialized(editor)).toBe('');
    editor.destroy();
  });
});
