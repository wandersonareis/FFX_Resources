import { createStore } from '@tanstack/store';

/**
 * Visibilidade da coluna de informações do painel de texturas.
 *
 * Estado de SESSÃO, deliberadamente fora do localStorage: ocultar precisa
 * sobreviver à troca imagem↔texto (o ImagePanel desmonta) e às abas de
 * versão (que também remontam), mas NÃO ao fechamento do app — ao reabrir,
 * a coluna volta ao padrão (aberta). O módulo vive o tempo da janela: é
 * exatamente esse escopo, sem código de persistência para descartar.
 *
 * A LARGURA da coluna é outro assunto: continua persistida em px/percentual
 * pelo layout-storage (não é afetada por ocultar/exibir).
 */
export const imageInfoStore = createStore<{ hidden: boolean }>({
  hidden: false,
});

/** Alterna a coluna de informações (botão sempre visível na barra do painel). */
export function toggleImageInfo(): void {
  imageInfoStore.setState((prev) => ({ hidden: !prev.hidden }));
}
