'use client';

import { useSelector } from '@tanstack/react-store';
import { useEffect, useState } from 'react';
import { ListLanguages } from '@/bindings/ffxresources/textservice';
import { TranslationCellDialog } from '@/components/dialogs/translation-cell-dialog';
import type { EntryView } from '@/components/entry-view/entry-view-store';
import { SOURCE_LANG } from '@/lib/ffx/save-all';

/**
 * Fio do diálogo de tradução com o store da view: busca os idiomas de
 * consulta uma única vez e liga navegação/fechamento às actions (que gravam
 * o rascunho via editDraft e avançam a translationRow).
 */
export function TranslationDialog({ view }: { view: EntryView }) {
  const { store, actions } = view;
  const open = useSelector(store, (s) => s.dialogOpen);
  const row = useSelector(store, (s) => s.translationRow);
  const rows = useSelector(store, (s) => s.rows);
  // Alvo override (ref → def em OUTRA entrada): o diálogo está preso na row
  // sintetizada da def — navegar trocaria de linha SEM trocar de arquivo, e
  // o store ignora o movimento (setas que não fazem nada).
  const target = useSelector(store, (s) => s.translationTarget);
  const copyRow = useSelector(store, (s) => s.translationCopyRow);
  const divergent = useSelector(store, (s) => s.translationDivergent);
  const [languages, setLanguages] = useState<Array<{ code: string; name: string }>>([]);

  useEffect(() => {
    ListLanguages()
      .then((v) => setLanguages(v ?? [{ code: SOURCE_LANG, name: 'English' }]))
      .catch(() => setLanguages([{ code: SOURCE_LANG, name: 'English' }]));
  }, []);

  return (
    <TranslationCellDialog
      open={open}
      row={row}
      version={view.version}
      languages={languages}
      hasPrevious={!target && !copyRow && rows.findIndex((r) => r.index === row?.index) > 0}
      hasNext={
        !target && !copyRow && rows.findIndex((r) => r.index === row?.index) < rows.length - 1
      }
      allowDivergence={copyRow !== null}
      initialDivergent={divergent}
      onNavigate={(direction, value) => actions.navigateRow(direction, value)}
      onClosed={(value, isDivergent) => actions.commitRow(value, isDivergent)}
    />
  );
}
