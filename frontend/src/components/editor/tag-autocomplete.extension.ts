// Autocomplete de tags no editor: `{` abre a sugestão (estágio 1: PC, MCR,
// BUTTON, ICON), `TAG:` + filtro lista os valores do catálogo (estágio 2).
// A conclusão insere sempre um valor atômico (prefixo de texto no estágio 1,
// chip gameTag no estágio 2) — nunca texto solto.
//
// O popup é DOM vanilla montado via props.mount; a própria suggestion
// cuida da posição (floating-ui, âncora no caret) e do container: aqui
// entra o popupContainer — dentro do modal Radix quando houver, para que
// cliques/Escape nele sejam internos ao Dialog (montado no body, o
// DismissableLayer fecha o modal inteiro em qualquer clique no popup).
import { Extension, type JSONContent, type Range } from '@tiptap/core';
import { PluginKey } from '@tiptap/pm/state';
import type { EditorView } from '@tiptap/pm/view';
import { Suggestion, type SuggestionProps } from '@tiptap/suggestion';
import {
  computeSuggestions,
  isPartialTagQuery,
  looksLikeTagAttempt,
  type TagCatalog,
  type TagSuggestionItem,
} from '@/lib/ffx/tag-catalog';
import { createTagSuggestionPopup, type TagSuggestionPopup } from './tag-suggestion-popup';

interface SharedState {
  /** Seleção confirmada: o texto parcial não deve ser apagado no exit. */
  completed: boolean;
  popup: TagSuggestionPopup | null;
  unmount: (() => void) | null;
  query: string;
  items: TagSuggestionItem[];
  /** Último command do suggestion (seleção por teclado). */
  command: ((item: TagSuggestionItem) => void) | null;
}

export function createTagAutocompleteExtension(
  catalog: TagCatalog | null,
  /** Destino de montagem do popup (ex.: conteúdo do modal Radix). */
  popupContainer?: HTMLElement | (() => HTMLElement | null),
): Extension {
  const pluginKey = new PluginKey('tagAutocomplete');
  const shared: SharedState = {
    completed: false,
    popup: null,
    unmount: null,
    query: '',
    items: [],
    command: null,
  };

  const show = (props: SuggestionProps<TagSuggestionItem, TagSuggestionItem>): void => {
    shared.query = props.query;
    shared.items = props.items;
    // Uma sessão nova (ou nova query) nunca herda a confirmação anterior.
    shared.completed = false;
    shared.command = (item) => props.command(item);
    if (!shared.popup) {
      shared.popup = createTagSuggestionPopup();
      shared.unmount = props.mount(shared.popup.element);
    }
    shared.popup.render(shared.items, (item) => props.command(item));
  };

  const teardown = (): void => {
    shared.unmount?.();
    shared.unmount = null;
    shared.popup?.destroy();
    shared.popup = null;
    shared.items = [];
    shared.command = null;
    shared.query = '';
  };

  return Extension.create({
    name: 'tagAutocomplete',

    addProseMirrorPlugins() {
      return [
        Suggestion<TagSuggestionItem, TagSuggestionItem>({
          pluginKey,
          editor: this.editor,
          char: '{',
          // Tag pode conter espaços ("Direcional UP", texto de macro) e
          // começar em qualquer ponto da linha.
          allowSpaces: true,
          allowedPrefixes: null,
          minQueryLength: 0,
          decorationTag: 'span',
          decorationClass: 'tag-suggestion-active',
          decorationEmptyClass: 'is-empty',
          // Texto corrido após `{` não abre popup nem decoração.
          shouldShow: ({ query }) => looksLikeTagAttempt(query),
          // Dismiss (Escape/clique fora) some no próximo caractere digitado.
          shouldResetDismissed: ({ transaction }) => transaction.docChanged,
          items: ({ query }) => computeSuggestions(query, catalog),
          // SuggestionMountOptions.container — resolve no momento em que os
          // plugins nascem (o editor é criado em useEffect, refs já ligados)
          // e de novo a cada recriação (troca de catálogo). undefined volta
          // para document.body, comportamento de quando não há modal Radix.
          container: resolvePopupContainer(popupContainer),
          command: ({ editor, range, props: item }) => {
            shared.completed = true;
            const content: JSONContent | string =
              item.kind === 'value' && item.chip
                ? { type: 'gameTag', attrs: { ...item.chip } }
                : item.insert;
            editor
              .chain()
              .focus()
              .insertContentAt({ from: range.from, to: range.to }, content)
              .run();
          },
          render: () => ({
            onStart: show,
            onUpdate: show,
            onExit: () => {
              // Só o Escape remove o parcial (onKeyDown): fechar o menu por
              // clique/ cursor fora apenas fecha — apagar o que o usuário
              // digitou confundia. O `{…` aberto fica no texto e o gate
              // "Trecho de tag aberto" segura antes de salvar/navegar.
              teardown();
              shared.completed = false;
            },
            onKeyDown: ({ view, event, range }) => {
              if (event.key === 'Escape' || event.key === 'Esc') {
                event.preventDefault();
                removePartial(view, range, shared.query);
                return true;
              }
              if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                const popup = shared.popup;
                if (!popup || popup.itemCount() === 0) return false;
                event.preventDefault();
                popup.move(event.key === 'ArrowDown' ? 1 : -1);
                return true;
              }
              if (event.key === 'Enter' || event.key === 'Tab') {
                const popup = shared.popup;
                if (!popup || popup.itemCount() === 0) return false;
                const item = shared.items[popup.activeIndex()];
                if (!item || !shared.command) return false;
                event.preventDefault();
                shared.command(item);
                return true;
              }
              return false;
            },
          }),
        }),
      ];
    },
  });
}

/**
 * Aceita elemento direto ou thunk (o editor é recriado quando o catálogo
 * muda, o que reexecuta addProseMirrorPlugins; o thunk devolve o nó ligado
 * naquele momento). null/undefined -> document.body.
 */
function resolvePopupContainer(
  source?: HTMLElement | (() => HTMLElement | null),
): string | HTMLElement | undefined {
  const target = typeof source === 'function' ? source() : source;
  return target instanceof HTMLElement ? target : undefined;
}

/**
 * Remove o parcial `{query` deixado pela tentativa abortada. Só apaga se o
 * trecho continuar idêntico ao que o suggestion exibia (chips viram folha
 * e nunca casam), assim não se apaga texto digitado depois do exit.
 */
function removePartial(view: EditorView, range: Range, query: string): void {
  if (!isPartialTagQuery(query)) return;
  const { from, to } = range;
  if (from < 0 || to <= from || to > view.state.doc.content.size) return;

  let text: string;
  try {
    text = view.state.doc.textBetween(from, to, '\n', () => '\uFFFC');
  } catch {
    return;
  }
  if (text !== `{${query}`) return;

  view.dispatch(view.state.tr.delete(from, to).scrollIntoView());
}
