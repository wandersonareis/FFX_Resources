'use client';

import { useHotkey } from '@tanstack/react-hotkeys';
import { useState } from 'react';
import { hotkeyLabel, SHORTCUTS } from '@/lib/ffx/shortcuts';

/**
 * Níveis discretos do ciclo da lupa: tamanho real (1×) até 4× — depois de
 * 4× volta ao tamanho real. O slider permite valores intermediários.
 */
const ZOOM_LEVELS = [1, 2, 3, 4];

/**
 * Estado de EXIBIÇÃO da pré-visualização: espelhamento, zoom e renderização
 * pixelada. Nenhum byte muda e nada é mandado de volta ao backend — é tudo
 * um transform/estilo no <img>.
 *
 * Cada valor guarda o id da textura (não um booleano/nível solto) para cada
 * textura voltar ao seu estado de pé ao reabrir, sem useEffect de
 * sincronização. Os atalhos do painel vêm daqui, ligados por `enabled`.
 */
export function useImagePreview({
  entryId,
  enabled,
}: {
  entryId: string;
  /** Painel ocioso: sem diálogo/menu aberto e sem gravação em andamento. */
  enabled: boolean;
}) {
  /**
   * Textura com o preview espelhado (id) — SÓ VISUAL: o backend já entrega a
   * imagem na orientação correta (flip feito ao decodificar).
   */
  const [flippedId, setFlippedId] = useState<string | null>(null);
  /**
   * Zoom manual do preview: id da textura + nível ('fit' = encaixar no
   * painel). O nível é o multiplicador do tamanho NATIVO da textura (1× = 1
   * pixel da textura por pixel da tela).
   */
  const [zoom, setZoom] = useState<{ id: string; level: 'fit' | number } | null>(null);
  /**
   * Renderização pixelada (nearest-neighbor) — preferência da sessão. É o
   * jeito de distinguir embaçamento da EXIBIÇÃO (escala bilinear do browser
   * ao encaixar texturas pequenas no painel) do embaçamento do DADO (DXT
   * comprimido / arte de baixa resolução): em 1× com pixelado, o que se vê
   * é exatamente o que o decoder do jogo produz.
   */
  const [pixelated, setPixelated] = useState(false);

  const flipped = flippedId === entryId;
  const zoomLevel = zoom && zoom.id === entryId ? zoom.level : 'fit';

  /** Lupa: próximo nível do ciclo — depois de 4× volta ao tamanho real. */
  const zoomIn = () => {
    const current = zoomLevel === 'fit' ? 0 : zoomLevel;
    setZoom({
      id: entryId,
      level: ZOOM_LEVELS.find((l) => l > current) ?? ZOOM_LEVELS[0],
    });
  };
  /** Passo atrás no ciclo; sem efeito no ajuste automático. */
  const zoomOut = () => {
    if (zoomLevel === 'fit') return;
    const prev = [...ZOOM_LEVELS].reverse().find((l) => l < zoomLevel);
    if (prev) setZoom({ id: entryId, level: prev });
  };
  const resetZoom = () => setZoom(null);
  const setZoomLevel = (level: 'fit' | number) => setZoom({ id: entryId, level });
  const togglePixelated = () => setPixelated((p) => !p);
  const toggleFlip = () => setFlippedId((id) => (id === entryId ? null : entryId));

  // Zoom por teclado (TanStack Hotkeys): Mod+= / Mod+- andam no ciclo,
  // Mod+0 volta ao ajuste; 1–4 vão direto ao nível SEM Mod (Mod+1/2/3 já
  // trocam de aba no app-shell). As três teclas de ciclo vêm do catálogo —
  // elas aparecem no `title` da lupa.
  useHotkey(SHORTCUTS.zoomIn.keys, zoomIn, {
    enabled,
    ignoreInputs: true,
    meta: { name: SHORTCUTS.zoomIn.label, group: 'Painel de imagem' },
  });
  useHotkey(SHORTCUTS.zoomOut.keys, zoomOut, {
    enabled,
    ignoreInputs: true,
    meta: { name: SHORTCUTS.zoomOut.label, group: 'Painel de imagem' },
  });
  useHotkey(SHORTCUTS.zoomReset.keys, resetZoom, {
    enabled,
    ignoreInputs: true,
    meta: { name: SHORTCUTS.zoomReset.label, group: 'Painel de imagem' },
  });
  useHotkey('1', () => setZoomLevel(1), { enabled, ignoreInputs: true });
  useHotkey('2', () => setZoomLevel(2), { enabled, ignoreInputs: true });
  useHotkey('3', () => setZoomLevel(3), { enabled, ignoreInputs: true });
  useHotkey('4', () => setZoomLevel(4), { enabled, ignoreInputs: true });
  // Ctrl+Alt+V: espelha a pré-visualização — a mesma ação do botão Flip.
  useHotkey(SHORTCUTS.imageFlip.keys, toggleFlip, {
    enabled,
    ignoreInputs: true,
    meta: { name: SHORTCUTS.imageFlip.label, group: 'Painel de imagem' },
  });

  return {
    flipped,
    zoomLevel,
    pixelated,
    zoomIn,
    zoomOut,
    resetZoom,
    setZoomLevel,
    togglePixelated,
    toggleFlip,
  };
}

export const imagePreviewHotkeyHint =
  `teclado: ${hotkeyLabel(SHORTCUTS.zoomIn.keys)} / ${hotkeyLabel(SHORTCUTS.zoomOut.keys)}; ` +
  `${hotkeyLabel(SHORTCUTS.zoomReset.keys)} volta ao ajuste`;
