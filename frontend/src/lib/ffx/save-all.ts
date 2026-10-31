import type * as services from '@/bindings/ffxresources/backend/services/models';
import {
  ApplyTextCollection,
  ApplyVbfTextCollection,
  PreviewSavePrune,
} from '@/bindings/ffxresources/textservice';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import type { EntryKind } from './display-names';
import { editDraft } from './edit-draft';
import { sendErrorNotification } from './error-handler';
import type { GameVersionId } from './game-version';
import { askPruneConfirm } from './prune-confirm-store';
import { invalidateVbfCache } from './vbf';

export const SOURCE_LANG = 'us';

/**
 * Linhas que o save vai EXCLUIR por não existirem no original (ponteiro
 * errado no binário traduzido etc.). O dicionário reconstrói o arquivo
 * inteiro, então a prévia vem do backend; nos demais kinds o save só toca as
 * entradas sujas e o próprio rascunho sabe quais linhas são órfãs.
 */
async function collectPruneRows(): Promise<services.PrunedRowView[]> {
  const out: services.PrunedRowView[] = [];
  const seen = new Set<string>();
  const add = (rows: services.PrunedRowView[]) => {
    for (const row of rows) {
      const key = `${row.file}|${row.block ?? ''}|${row.index}|${row.name}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(row);
    }
  };

  for (const [version, byKind] of editDraft.dirtyBatches()) {
    for (const kind of byKind.keys()) {
      if (kind === 'macro') {
        add(await PreviewSavePrune(kind, version));
        continue;
      }
      add(editDraft.orphanRows(version, kind));
    }
  }
  for (const [version, byRoot] of editDraft.dirtyVbfBatches()) {
    for (const byKind of byRoot.values()) {
      for (const kind of byKind.keys()) add(editDraft.orphanRows(version, kind));
    }
  }
  return out;
}

export async function saveAllDrafts(
  isSaving: () => boolean,
  setSaving: (value: boolean) => void,
): Promise<boolean> {
  if (!editDraft.getSnapshot().hasDirty || isSaving()) return false;
  // Gate de tags protegidas: restaura o que sumiu e aborta se ainda houver
  // valor perdido ou `{…` aberto — o texto não vai ao binário quebrado.
  const report = editDraft.restoreProtectedTags();
  if (report.restored > 0) {
    toast(`Tags protegidas restauradas em ${report.restored} célula(s).`, { duration: 4000 });
  }
  if (report.unresolved > 0 || report.unclosed > 0) {
    const parts: string[] = [];
    if (report.unresolved > 0) {
      parts.push(`${report.unresolved} célula(s) sem tag protegida obrigatória`);
    }
    if (report.unclosed > 0) {
      parts.push(`${report.unclosed} célula(s) com tag aberta ({…)`);
    }
    toast.error(
      `Salvamento bloqueado: ${parts.join(' e ')}. Abra a linha e use Restaurar/Corrigir.`,
      { duration: 8000 },
    );
    return false;
  }

  // Poda do save: avisa (lista completa) antes de excluir qualquer linha.
  // Cancelar aborta o save inteiro — nada é gravado.
  const pruned = await collectPruneRows();
  if (pruned.length > 0) {
    const proceed = await askPruneConfirm(pruned);
    if (!proceed) return false;
  }

  setSaving(true);
  try {
    for (const [version, byKind] of editDraft.dirtyBatches()) {
      for (const kind of byKind.keys()) {
        const collection = editDraft.buildCollection(version as GameVersionId, kind as EntryKind);
        if (Object.keys(collection).length === 0) continue;
        await ApplyTextCollection(
          kind,
          version as Parameters<typeof ApplyTextCollection>[1],
          collection as Parameters<typeof ApplyTextCollection>[2],
        );
      }
    }
    // Tabelas abertas pelo .vbf: mesmo motor, escopo da sessão do
    // container — salvos os binários tocados (lote ou propagação entre
    // cópias abertas).
    for (const [version, byRoot] of editDraft.dirtyVbfBatches()) {
      for (const [root, byKind] of byRoot) {
        for (const kind of byKind.keys()) {
          const collection = editDraft.buildSourceCollection(
            version as GameVersionId,
            kind as EntryKind,
            root,
          );
          if (Object.keys(collection).length === 0) continue;
          await ApplyVbfTextCollection(
            root,
            kind,
            version as Parameters<typeof ApplyVbfTextCollection>[2],
            collection as Parameters<typeof ApplyVbfTextCollection>[3],
          );
        }
      }
    }
    editDraft.markSaved();
    // Tradução nova em mods/: o que o navegador de .vbf serviu pode ter
    // mudado (cópia traduzida vira literal na próxima carga) — as caches
    // de .vbf da sessão caem junto com o reload do backend (carimbo por
    // arquivo) e daqui.
    invalidateVbfCache();
    toast('Alterações salvas no jogo.', { duration: 3000 });
    return true;
  } catch (error) {
    sendErrorNotification(error);
    return false;
  } finally {
    setSaving(false);
  }
}
