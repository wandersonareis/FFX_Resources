import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
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

/** Delete "para frente" (keyCode 46 é o que o ProseMirror consulta). */
function pressDelete(): void {
  const event = new KeyboardEvent('keydown', {
    key: 'Delete',
    code: 'Delete',
    keyCode: 46,
    which: 46,
    bubbles: true,
    cancelable: true,
  } as KeyboardEventInit);
  act(() => editorNode().dispatchEvent(event));
}

/** Cola texto canônico pelo mesmo caminho do clipboard real (handler do editor). */
function pasteIntoEditor(text: string): void {
  const event = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(event, 'clipboardData', {
    value: { getData: (type: string) => (type === 'text/plain' ? text : '') },
  });
  act(() => editorNode().dispatchEvent(event));
}

function mountEditor(value: string, onValueChange: (v: string) => void = () => {}) {
  const ui = (v: string) => (
    <TooltipProvider>
      <GameTextEditor value={v} onValueChange={onValueChange} version="ffx" />
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

  it('a lixeira do padding remove o prefixo do valor emitido', async () => {
    const onValueChange = vi.fn();
    const { setValue } = mountEditor('{TEXT_NEWLINE}  Hello', onValueChange);
    await waitFor(() => expect(document.querySelector('.tiptap')).toBeTruthy());

    // O padding fica fora do conteúdo editável: o editor mostra só o corpo.
    expect(editorNode().textContent).toBe('Hello');
    expect(screen.getByText('{TEXT_NEWLINE} ×1 · ␣ ×2')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Remover padding de tela' }));
    expect(onValueChange).toHaveBeenCalledWith('Hello');

    // O diálogo controla o valor: ao aplicar o texto sem prefixo o aviso some.
    setValue('Hello');
    expect(screen.queryByText('{TEXT_NEWLINE} ×1 · ␣ ×2')).toBeNull();
  });

  it('apagar o bloco inicial não transforma a quebra em padding', async () => {
    const onValueChange = vi.fn();
    const { setValue } = mountEditor('{PC:00:TIDUS}{TEXT_NEWLINE}Hello world', onValueChange);
    await waitFor(() => expect(document.querySelector('.tiptap')).toBeTruthy());
    expect(screen.queryByText('{TEXT_NEWLINE} ×1')).toBeNull();

    // Cursor no início do primeiro bloco: Delete apaga o chip e deixa o
    // parágrafo vazio. A quebra seguinte é sobra de conteúdo, não padding.
    pressDelete();

    const emitted = onValueChange.mock.lastCall?.[0];
    expect(emitted).toBe('Hello world');
    setValue(emitted ?? '');
    expect(screen.queryByText('{TEXT_NEWLINE} ×1')).toBeNull();
  });

  it('a lixeira também limpa o parágrafo vazio que criou o padding na colagem', async () => {
    const onValueChange = vi.fn();
    const { setValue } = mountEditor('', onValueChange);
    await waitFor(() => expect(document.querySelector('.tiptap')).toBeTruthy());

    // Colar texto canônico com {TEXT_NEWLINE} inicial insere um parágrafo
    // vazio à frente: a serialização vira padding e o editor "esconde" o
    // prefixo — mas o parágrafo vazio continua no documento.
    pasteIntoEditor('{TEXT_NEWLINE}linha1{TEXT_NEWLINE}linha2');
    const emitted = '{TEXT_NEWLINE}linha1{TEXT_NEWLINE}linha2';
    expect(onValueChange).toHaveBeenLastCalledWith(emitted);
    setValue(emitted);
    expect(editorNode().querySelectorAll('p')).toHaveLength(3);

    fireEvent.click(screen.getByRole('button', { name: 'Remover padding de tela' }));

    const body = 'linha1{TEXT_NEWLINE}linha2';
    expect(onValueChange).toHaveBeenLastCalledWith(body);
    // Sem normalizar, o próximo onUpdate serializaria o parágrafo vazio de
    // novo e o padding voltaria.
    expect(editorNode().querySelectorAll('p')).toHaveLength(2);

    setValue(body);
    expect(screen.queryByText('{TEXT_NEWLINE} ×1')).toBeNull();
  });
});
