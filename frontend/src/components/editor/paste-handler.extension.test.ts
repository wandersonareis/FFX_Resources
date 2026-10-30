import { Editor } from '@tiptap/core';
import { Color } from '@tiptap/extension-color';
import { TextStyle } from '@tiptap/extension-text-style';
import StarterKit from '@tiptap/starter-kit';
import { describe, expect, it } from 'vitest';
import { gameTextParser } from '@/lib/ffx/game-text-parser';
import { GameTagExtension } from './game-tag.extension';
import { LockedTagsExtension } from './locked-tags.extension';
import { createPasteHandlerExtension, unwrapSingleParagraphHtml } from './paste-handler.extension';
import { createTypedTagExtension } from './typed-tag.extension';

function makeEditor(value: string): Editor {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const validate = () => true;
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
      TextStyle,
      Color,
      GameTagExtension,
      LockedTagsExtension,
      createTypedTagExtension(validate),
      createPasteHandlerExtension(gameTextParser, validate),
    ],
    content: gameTextParser.parseGameTextToHTML(value, validate),
  });
}

function paste(editor: Editor, text: string): void {
  const event = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(event, 'clipboardData', {
    value: {
      getData: (type: string) => (type === 'text/plain' ? text : ''),
    },
  });
  editor.view.dom.dispatchEvent(event);
}

describe('unwrapSingleParagraphHtml', () => {
  it('achata um parágrafo só', () => {
    expect(unwrapSingleParagraphHtml('<p>texto</p>')).toBe('texto');
    expect(unwrapSingleParagraphHtml('<p><span data-game-tag="{PAUSE}"></span>oi</p>')).toBe(
      '<span data-game-tag="{PAUSE}"></span>oi',
    );
  });

  it('não achata quando há quebra real (vários parágrafos)', () => {
    expect(unwrapSingleParagraphHtml('<p>a</p><p>b</p>')).toBeNull();
  });

  it('parágrafo vazio não insere nada', () => {
    expect(unwrapSingleParagraphHtml('<p></p>')).toBe('');
    expect(unwrapSingleParagraphHtml('<p><br></p>')).toBe('');
  });
});

describe('paste handler', () => {
  it('colar uma linha não cria quebra antes/depois', () => {
    const editor = makeEditor('um texto');
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
    paste(editor, 'texto colado');

    const json = editor.getJSON();
    expect(json.content).toHaveLength(1);
    expect(json.content?.[0]?.content).toHaveLength(1);
    expect(json.content?.[0]?.content?.[0]?.text).toBe('um textotexto colado');
    expect(json.content?.[0]?.content?.[0]?.type).toBe('text');
    expect(json.content?.[0]?.content?.some((node) => node.type === 'hardBreak')).toBe(false);
    editor.destroy();
  });

  it('mantém chips/cores da colagem no mesmo parágrafo', () => {
    const editor = makeEditor('antes');
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
    paste(editor, '{CLR:BLUE}Oi{PAUSE}');

    const json = editor.getJSON();
    expect(json.content).toHaveLength(1);
    const inline = json.content?.[0]?.content ?? [];
    expect(inline.some((node) => node.type === 'gameTag')).toBe(true);
    // O reset da cor vai para o fim do bloco colorido (mesmo byte do jogo).
    expect(gameTextParser.serializeJSONToGameText(json)).toBe(
      'antes{CLR:BLUE}Oi{PAUSE}{CLR:WHITE}',
    );
    editor.destroy();
  });

  it('texto com quebra real continua virando parágrafos', () => {
    const editor = makeEditor('antes');
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
    paste(editor, 'linha1\nlinha2');

    const json = editor.getJSON();
    expect(json.content).toHaveLength(3);
    expect(gameTextParser.serializeJSONToGameText(json)).toBe(
      'antes{TEXT_NEWLINE}linha1{TEXT_NEWLINE}linha2',
    );
    editor.destroy();
  });

  it('quebras nas pontas do clipboard não criam linha antes/depois', () => {
    const editor = makeEditor('um texto aqui');
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
    paste(editor, '\ntexto colado\n');

    const json = editor.getJSON();
    expect(json.content).toHaveLength(1);
    expect(gameTextParser.serializeJSONToGameText(json)).toBe('um texto aquitexto colado');
    editor.destroy();
  });

  it('clipboard só com quebras não insere nada', () => {
    const editor = makeEditor('um texto');
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
    paste(editor, '\n\r\n');

    const json = editor.getJSON();
    expect(json.content).toHaveLength(1);
    expect(gameTextParser.serializeJSONToGameText(json)).toBe('um texto');
    editor.destroy();
  });
});
