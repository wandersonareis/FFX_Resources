'use client';

import { useCallback, useState } from 'react';
import type { Layout, LayoutChangedMeta } from 'react-resizable-panels';
import { layoutStorage, type PanelLayout } from './layout-storage';

export type LayoutGroup = 'sidebar' | 'imageInfo';

/**
 * Restaura e persiste o layout de UM grupo redimensionável.
 *
 *  - `defaultLayout` vai no `ResizablePanelGroup` (percentuais por id de
 *    painel). `undefined` quando não há nada salvo — aí valem os
 *    `defaultSize` em px declarados nos próprios painéis.
 *  - `onLayoutChanged` só grava quando o usuário mexeu de verdade
 *    (`meta.isUserInteraction`): montagem e recomputação de constraint não
 *    renovam o `savedAt`, então a expiração de 30 dias não é adiada por
 *    abrir o app.
 *
 * Sem SSR (`dynamic ssr:false` na page) o initializer roda já no cliente,
 * então o localStorage é lido antes do primeiro paint — sem frame com o
 * layout padrão piscando antes do restaurado.
 */
export function usePersistedLayout(group: LayoutGroup) {
  const [defaultLayout] = useState<PanelLayout | undefined>(
    () => layoutStorage.load()?.[group] ?? undefined,
  );

  const onLayoutChanged = useCallback(
    (layout: Layout, meta: LayoutChangedMeta) => {
      if (!meta.isUserInteraction) return;
      layoutStorage.saveGroup(group, layout);
    },
    [group],
  );

  return { defaultLayout, onLayoutChanged };
}
