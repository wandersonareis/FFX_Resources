import { formatForDisplay } from '@tanstack/react-hotkeys';

/**
 * Catálogo dos ATALHOS DE AÇÃO: qual tecla, o que faz e onde vive o controle.
 * Fonte única — o registro (`useHotkey`/`useHotkeys`) e o texto de UI
 * (tooltip/`title`) leem daqui, então a tecla nunca é escrita em dois lugares.
 *
 * `as const` é obrigatório: `useHotkey` exige o tipo literal `Hotkey`
 * (`string` não compila) e um `Mod+${i}` dinâmico também não serviria.
 *
 * Enter da árvore/tabela e ←/→ do diálogo de tradução ficam de fora: não
 * aparecem em texto nenhum e só fazem sentido dentro do próprio componente.
 * As SETAS estão em `navigation` porque a barra de status as exibe.
 */
export const SHORTCUTS = {
  /** app-shell → grava os rascunhos no binário (botão "Salvar" da aba). */
  save: { keys: 'Mod+S', label: 'Salvar no binário' },
  /** app-shell → abre o ConfigDialog (engrenagem do header). */
  config: { keys: 'Mod+,', label: 'Abrir Configurações' },
  /** app-shell → abre o modal de busca da aba ATIVA (palette Ctrl+K). */
  search: { keys: 'Mod+K', label: 'Buscar' },
  /** app-shell → abre a busca & substituição local na tabela da aba ativa. */
  findInFile: { keys: 'Mod+F', label: 'Buscar neste arquivo' },
  /** app-shell → uma tecla por aba de GAME_VERSIONS (1 = FFX, 2 = EC, ...). */
  versionTabs: {
    keys: ['Mod+1', 'Mod+2', 'Mod+3', 'Mod+4'],
    label: 'Trocar de versão',
  },
  /** image-panel → espelha a pré-visualização (só na tela). */
  imageFlip: { keys: 'Mod+Alt+V', label: 'Espelhar imagem' },
  /** image-panel → ciclo de zoom da pré-visualização. */
  zoomIn: { keys: 'Mod+=', label: 'Aumentar o zoom' },
  zoomOut: { keys: 'Mod+-', label: 'Diminuir o zoom' },
  zoomReset: { keys: 'Mod+0', label: 'Voltar ao ajuste' },
  /**
   * Setas da árvore/tabela: a MESMA tecla muda de sentido com o foco (nó ou
   * linha), então o `key` é a fonte única da tecla, o `label` de cada seta
   * é o rótulo neutro do tooltip e o `label` do grupo é o nome da faixa na
   * barra de status — o que cada tecla faz em cada contexto continua no
   * `meta.name` do registro.
   */
  navigation: {
    label: 'Setas de navegação',
    up: { key: 'ArrowUp', label: 'Item anterior' },
    down: { key: 'ArrowDown', label: 'Próximo item' },
    right: { key: 'ArrowRight', label: 'Expandir ou abrir' },
    left: { key: 'ArrowLeft', label: 'Voltar' },
  },
} as const;

/** Tecla formatada para a plataforma: `Ctrl+S` no Windows, `⌘S` no macOS. */
export function hotkeyLabel(keys: string): string {
  return formatForDisplay(keys);
}

/** Texto pronto para tooltip/`title`: rótulo + tecla formatada. */
export function shortcutTip(label: string, keys: string): string {
  return `${label} · ${hotkeyLabel(keys)}`;
}
