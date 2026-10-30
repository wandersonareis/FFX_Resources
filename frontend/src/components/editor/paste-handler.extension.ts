import { Extension } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';
import type { gameTextParser } from '@/lib/ffx/game-text-parser';
import type { ChipValidator } from '@/lib/ffx/tag-catalog';

/**
 * Extrai o miolo quando o HTML convertido é UM parágrafo só (texto colado
 * sem quebra). O parser sempre devolve `<p>…</p>`; inserir esse bloco no meio
 * de um parágrafo faz o ProseMirror partir o texto em três (uma quebra antes
 * e outra depois do conteúdo colado). Colar uma linha não deve criar quebra
 * nenhuma: o miolo entra inline no parágrafo do cursor.
 *
 * Devolve:
 *   - string  → conteúdo inline para inserir (pode ser vazio);
 *   - null    → tem mais de um parágrafo (quebras reais), insere os blocos.
 *
 * Exportada para teste.
 */
export function unwrapSingleParagraphHtml(html: string): string | null {
  const match = /^<p>([\s\S]*)<\/p>$/.exec(html);
  if (!match) return null;
  const inner = match[1];
  // O parser escapa `<` do texto, então um `<p>` aqui só pode ser outro
  // parágrafo (quebra real). Não achata.
  if (inner.includes('<p>')) return null;
  // `<p></p>`/`<p><br></p>` = colagem sem conteúdo visível.
  if (inner === '' || inner === '<br>') return '';
  return inner;
}

/**
 * Converte texto colado com tags canônicas {…} para o documento,
 * em vez do parse padrão do Tiptap.
 *
 * `validate` é o mesmo validador do parse: mantém o HTML do conteúdo colado
 * idêntico ao que a releitura do valor produziria.
 */
export function createPasteHandlerExtension(
  parser: typeof gameTextParser,
  validate?: ChipValidator,
) {
  return Extension.create({
    name: 'pasteHandler',

    addProseMirrorPlugins() {
      const editor = this.editor;

      return [
        new Plugin({
          props: {
            handlePaste: (_view, event) => {
              const clipboardData = event.clipboardData;
              if (!clipboardData) return false;

              const rawText = clipboardData.getData('text/plain');
              if (!rawText) return false;

              event.preventDefault();
              // Copiar uma linha/bloco costuma trazer `\n` nas pontas (o
              // "texto colado\n" do editor/Bloco de Notas). Colar no meio do
              // parágrafo não deve criar uma quebra antes nem depois.
              const pastedText = rawText.replace(/^(?:\r?\n)+/, '').replace(/(?:\r?\n)+$/, '');
              if (!pastedText) return true;

              const convertedHtml = parser.parseGameTextToHTML(pastedText, validate);
              const inline = unwrapSingleParagraphHtml(convertedHtml);
              if (inline === null) {
                editor.commands.insertContent(convertedHtml);
              } else if (inline) {
                editor.commands.insertContent(inline);
              }
              return true;
            },
          },
        }),
      ];
    },
  });
}
