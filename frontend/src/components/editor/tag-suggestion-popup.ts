// Popup DOM (vanilla) do autocomplete de tags. Sem framework: o suggestion
// do Tiptap só entrega um elemento via props.mount(element) e o floating-ui
// cuida da posição. O container de montagem vem da extensão (ver lá): dentro
// de um modal Radix é o próprio diálogo — cliques no popup ficam internos e
// não fecham o modal. Mouse (hover/clique) e teclado (↑ ↓ Enter Esc) ambos
// selecionam.
import type { TagSuggestionItem } from '@/lib/ffx/tag-catalog';

export interface TagSuggestionPopup {
  /** Elemento montado (entregue ao props.mount). */
  readonly element: HTMLElement;
  /** Repovoa a lista e define o callback de seleção (mudou a query). */
  render: (items: TagSuggestionItem[], onSelect: (item: TagSuggestionItem) => void) => void;
  /** Move a seleção com as setas (circular); devolve o novo índice. */
  move: (delta: number) => number;
  activeIndex: () => number;
  itemCount: () => number;
  destroy: () => void;
}

let activePopup: TagSuggestionPopup | null = null;

/**
 * Popup montado agora (null quando não há sugestão aberta). O diálogo usa
 * isto para não fechar quando o clique/Escape for do próprio popup.
 */
export function getActiveTagSuggestionPopup(): TagSuggestionPopup | null {
  return activePopup;
}

export function createTagSuggestionPopup(): TagSuggestionPopup {
  const element = document.createElement('div');
  element.className = 'tag-suggestion';
  element.setAttribute('role', 'listbox');
  element.setAttribute('aria-label', 'Sugestões de tag');

  const list = document.createElement('ul');
  list.className = 'tag-suggestion-list';

  const empty = document.createElement('div');
  empty.className = 'tag-suggestion-empty';
  empty.textContent = 'Sem sugestões para o filtro';

  const footer = document.createElement('div');
  footer.className = 'tag-suggestion-footer';
  const note = document.createElement('div');
  note.className = 'tag-suggestion-note';
  footer.append(
    kbd('↑'),
    kbd('↓'),
    label('navegar'),
    kbd('Enter'),
    label('inserir'),
    kbd('Esc'),
    label('cancelar'),
    note,
  );

  element.append(list, empty, footer);

  let items: TagSuggestionItem[] = [];
  let active = 0;
  let onSelect: ((item: TagSuggestionItem) => void) | null = null;

  const paint = (): void => {
    const nodes = list.children;
    for (let i = 0; i < nodes.length; i++) {
      const li = nodes[i] as HTMLElement;
      const on = i === active;
      li.classList.toggle('active', on);
      li.setAttribute('aria-selected', on ? 'true' : 'false');
      if (on) li.scrollIntoView({ block: 'nearest' });
    }
  };

  const setActive = (index: number): void => {
    if (items.length === 0) return;
    active = ((index % items.length) + items.length) % items.length;
    paint();
  };

  // O mousedown com preventDefault mantém foco/caret no editor (o Tiptap não
  // perde a seleção e o suggestion não sai antes do click). O click escolhe
  // ao SOLTO (permite repensar e completar em etapas); hover acompanha.
  element.addEventListener('mousedown', (event) => {
    event.preventDefault();
  });

  list.addEventListener('click', (event) => {
    const li = (event.target as HTMLElement | null)?.closest?.(
      'li[data-index]',
    ) as HTMLElement | null;
    if (!li) return;
    const index = Number(li.dataset.index);
    const item = items[index];
    if (item && onSelect) onSelect(item);
  });

  list.addEventListener('mousemove', (event) => {
    const li = (event.target as HTMLElement | null)?.closest?.(
      'li[data-index]',
    ) as HTMLElement | null;
    if (!li) return;
    const index = Number(li.dataset.index);
    if (!Number.isNaN(index) && index !== active) setActive(index);
  });

  const popup: TagSuggestionPopup = {
    element,
    render(nextItems, nextOnSelect) {
      items = nextItems;
      onSelect = nextOnSelect;
      active = 0;
      list.replaceChildren();
      for (let i = 0; i < items.length; i++) {
        list.appendChild(itemNode(items[i], i));
      }
      const hasItems = items.length > 0;
      empty.hidden = hasItems;
      footer.hidden = !hasItems;
      if (hasItems) paint();
      note.textContent = hasItems ? 'Filtre digitando (ex.: s13 ou texto)' : '';
    },
    move(delta) {
      setActive(active + delta);
      return active;
    },
    activeIndex: () => active,
    itemCount: () => items.length,
    destroy() {
      element.remove();
      items = [];
      onSelect = null;
      if (activePopup === popup) activePopup = null;
    },
  };

  activePopup = popup;
  return popup;
}

function itemNode(item: TagSuggestionItem, index: number): HTMLElement {
  const li = document.createElement('li');
  li.className = 'tag-suggestion-item';
  li.dataset.index = String(index);
  li.setAttribute('role', 'option');
  li.setAttribute('aria-selected', 'false');

  const labelEl = document.createElement('span');
  labelEl.className = 'tag-suggestion-label';
  labelEl.textContent = item.label;

  const detail = document.createElement('span');
  detail.className = 'tag-suggestion-detail';
  detail.textContent = item.detail;

  li.append(labelEl, detail);
  return li;
}

function kbd(text: string): HTMLElement {
  const el = document.createElement('kbd');
  el.textContent = text;
  return el;
}

function label(text: string): HTMLElement {
  const el = document.createElement('span');
  el.textContent = text;
  return el;
}
