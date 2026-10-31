'use client';

import { useSelector } from '@tanstack/react-store';
import { AlertTriangle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  cancelDeleteOrphan,
  confirmDeleteOrphan,
  deleteOrphanStore,
} from '@/lib/ffx/delete-orphan-store';

/**
 * Aviso destrutivo da exclusão manual de uma linha órfã: mostra o índice, o
 * campo e o texto COMPLETO que será removido do binário no próximo save.
 * Confirmar só marca no rascunho — nada é gravado até o Salvar.
 */
export function DeleteOrphanDialog() {
  const { request } = useSelector(deleteOrphanStore, (s) => s);
  const row = request?.row;
  const text = row?.text?.us ?? '';

  return (
    <Dialog
      open={request !== null}
      onOpenChange={(next) => {
        if (!next) cancelDeleteOrphan();
      }}
    >
      <DialogContent id="delete-orphan-dialog" className="w-[min(34rem,calc(100%-2rem))]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle size={18} aria-hidden />
            Excluir linha órfã?
          </DialogTitle>
          <DialogDescription>
            Esta linha não existe no original. Confirmar marca a exclusão no rascunho; ao salvar
            (Ctrl+S) ela será removida do binário em <code>mods/</code> — permanente e sem desfazer
            no app.
          </DialogDescription>
        </DialogHeader>
        {row ? (
          <div className="rounded-md border border-border bg-muted/30 p-2 text-sm">
            <div className="font-mono text-xs text-muted-foreground">
              índice {row.index} · {row.name}
            </div>
            <div className="mt-1 wrap-break-word font-mono" data-testid="delete-orphan-text">
              {text.trim() ? text : '(vazio)'}
            </div>
          </div>
        ) : null}
        <DialogFooter>
          <Button variant="ghost" onClick={cancelDeleteOrphan}>
            Cancelar
          </Button>
          <Button variant="destructive" onClick={confirmDeleteOrphan}>
            Excluir linha
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
