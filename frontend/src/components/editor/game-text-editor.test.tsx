import { render, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { TooltipProvider } from '@/components/ui/tooltip';
import { GameTextEditor } from './game-text-editor';

// O catálogo de tags vem do backend Wails — no teste não há runtime.
// `null` = validação só estrutural: o editor nasce assim mesmo.
vi.mock('@/bindings/ffxresources/textservice', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/bindings/ffxresources/textservice')>();
  return { ...actual, GetTagCatalog: vi.fn(async () => null) };
});

/** O contenteditable que o Tiptap cria dentro do host. */
function editorNode(): HTMLElement {
  const node = document.querySelector<HTMLElement>('.tiptap');
  if (!node) throw new Error('o editor não montou');
  return node;
}

function mountEditor(value: string) {
  const ui = (v: string) => (
    <TooltipProvider>
      <GameTextEditor value={v} onValueChange={() => {}} version="ffx" />
    </TooltipProvider>
  );
  const result = render(ui(value));
  return { ...result, setValue: (v: string) => result.rerender(ui(v)) };
}

describe('GameTextEditor', () => {
  it('mantém o mesmo editor quando o valor muda (a tecla não pode levar o foco)', async () => {
    const { setValue } = mountEditor('abc');
    await waitFor(() => expect(document.querySelector('.tiptap')).toBeTruthy());

    const editor = editorNode();
    editor.focus();
    expect(document.activeElement).toBe(editor);

    // O diálogo guarda o rascunho a cada tecla/tecla colada: `value` muda.
    // Se o editor fosse recriado, o DOM do contenteditable sairia do lugar e
    // o foco voltaria para o body — o bug de "digito uma letra e o foco
    // perde, preciso clicar de novo".
    setValue('abcd');

    expect(editorNode()).toBe(editor);
    expect(document.activeElement).toBe(editor);
    // O valor externo chega pelo efeito de sincronia (setContent), sem
    // recriar o editor.
    expect(editor.textContent).toContain('abcd');
  });
});
