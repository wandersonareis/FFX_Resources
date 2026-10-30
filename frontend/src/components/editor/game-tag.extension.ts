import { Node } from '@tiptap/core';
import { buttonSpriteClasses, extractChipLabel } from '@/lib/ffx/game-text-tags';

export interface GameTagAttrs {
  /** Tag original completa, ex: {MCR:s06:l3F:"Yevon"}. Serializada verbatim. */
  value: string;
  /** Rótulo exibido no chip, ex: Yevon. */
  label: string;
  /** CMD, HEX, UNK ou VAR: valores calculados no jogo, nunca editáveis. */
  locked: boolean;
  /** Tag digitada que não sobrevive ao round-trip (gramática/catálogo). */
  invalid: boolean;
}

/**
 * Nó inline atômico para tags de controle (valor humano do terceiro campo
 * ignorado na serialização — só `value` é emitido, verbatim).
 * Renderização estática via renderHTML (sem NodeView Angular).
 */
export const GameTagExtension = Node.create<GameTagAttrs>({
  name: 'gameTag',
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      value: {
        default: '',
        parseHTML: (element) => element.getAttribute('data-game-tag') || '',
      },
      label: {
        default: '',
        parseHTML: (element) => element.textContent || '',
      },
      locked: {
        default: false,
        parseHTML: (element) => element.hasAttribute('data-locked'),
      },
      invalid: {
        default: false,
        parseHTML: (element) => element.hasAttribute('data-invalid'),
      },
    };
  },

  parseHTML() {
    return [{ tag: 'span[data-game-tag]' }];
  },

  renderHTML({ node }) {
    const value = node?.attrs.value ?? '';
    const label = node?.attrs.label ?? value;
    const locked = !!node?.attrs.locked;
    const invalid = !!node?.attrs.invalid;
    // BUTTON mapeado e sem marcação: ícones direto no texto, sem chip —
    // igual aos botões do jogo. O renderHTML de UM node atômico só aceita
    // UM elemento raiz: o wrapper externo (data-game-tag) é o ponto de
    // re-parse e os tiles ficam ANINHADOS dentro dele (irmãos no mesmo
    // nível seriam empurrados para dentro do primeiro tile, que tem
    // tamanho fixo — sobrepõem no editor). O title é derivado SEMPRE de
    // extractChipLabel(value) — determinístico dos dois lados, pois o
    // textContent do ícone é vazio. Estrutura byte-idêntica à do
    // game-text-parser — a comparação com getHTML() é literal.
    const btnClasses = buttonSpriteClasses(value);
    if (btnClasses.length > 0 && !locked && !invalid) {
      const chipLabel = extractChipLabel(value.replace(/^\{|\}$/g, ''));
      return [
        'span',
        { 'data-game-tag': value },
        ...btnClasses.map((cls) => ['span', { class: `gb-sprite ${cls}`, title: chipLabel }]),
      ];
    }
    const attrs: Record<string, string> = {
      'data-game-tag': value,
      class: `game-tag-chip${locked ? ' locked' : ''}${invalid ? ' invalid' : ''}`,
    };
    if (locked) attrs['data-locked'] = 'true';
    if (invalid) attrs['data-invalid'] = 'true';
    return ['span', attrs, label];
  },
});
