'use client';

import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { EntryKind, KIND_LABELS } from '@/lib/ffx/display-names';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';

interface ImportTranslatedDialogProps {
  batch: dto.ImportBatch | null;
  open: boolean;
  importing: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

export function ImportTranslatedDialog({
  batch,
  open,
  importing,
  onOpenChange,
  onConfirm,
}: ImportTranslatedDialogProps) {
  const kinds = batch?.kinds ?? [];
  const files = batch?.files ?? [];
  const errors = batch?.errors ?? [];
  const warnings = batch?.warnings ?? [];
  const nada = open && batch !== null && kinds.length === 0 && errors.length === 0;
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o || !importing) onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Importar textos de mods/translated</DialogTitle>
          <DialogDescription>
            Confira os tipos que serão importados antes de aplicar. Somente o
            texto em inglês (&quot;us&quot;) é importado; ao confirmar, os
            binários são reconstruídos e gravados em mods/.
          </DialogDescription>
        </DialogHeader>

        {batch ? (
          <div className="space-y-3 overflow-hidden">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              {kinds.map((k) => (
                <Badge key={k} variant="outline">
                  {KIND_LABELS[k as EntryKind] ?? k}
                </Badge>
              ))}
            </div>

            <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
              <span>{files.length} arquivo(s)</span>
              <span>{batch.entry_count} entrada(s)</span>
              <span>{batch.changed_texts} texto(s) alterado(s)</span>
            </div>

            <p className="break-all wrap-break-word text-xs text-muted-foreground">{batch.root}</p>
            {nada ? (
              <div className="rounded-md border border-muted bg-muted/40 p-2 text-sm wrap-break-word">
                Nenhum arquivo compatível em {batch.root} — exporte para a
                subpasta text/ ou use &quot;Importar de…&quot;.
              </div>
            ) : null}

            {errors.length > 0 ? (
              <div className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
                <p className="font-semibold">
                  Arquivo(s) pulado(s) ({errors.length}):
                </p>
                <ScrollArea className="mt-1 max-h-40">
                  <ul className="list-disc pl-5">
                    {errors.map((e, i) => (
                      <li key={`${i}:${e}`} className="break-all wrap-break-word">
                        {e}
                      </li>
                    ))}
                  </ul>
                </ScrollArea>
              </div>
            ) : null}

            {warnings.length > 0 ? (
              <div className="rounded-md border border-warning/50 bg-warning/10 p-2 text-sm text-warning">
                <p className="font-semibold">
                  Entrada(s) ignorada(s) ({warnings.length}):
                </p>
                <p className="mt-0.5">
                  Só estas entradas ficam de fora — as demais entradas do
                  arquivo importam normalmente.
                </p>
                <ScrollArea className="mt-1 max-h-40">
                  <ul className="list-disc pl-5">
                    {warnings.map((w, i) => (
                      <li key={`${i}:${w}`} className="break-all wrap-break-word">
                        {w}
                      </li>
                    ))}
                  </ul>
                </ScrollArea>
              </div>
            ) : null}
          </div>
        ) : null}

        <DialogFooter>
          <Button
            variant="outline"
            disabled={importing}
            onClick={() => onOpenChange(false)}
          >
            Cancelar
          </Button>
          <Button
            disabled={importing || !batch || batch.kinds.length === 0}
            onClick={onConfirm}
          >
            {importing ? 'Importando...' : 'Importar'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}