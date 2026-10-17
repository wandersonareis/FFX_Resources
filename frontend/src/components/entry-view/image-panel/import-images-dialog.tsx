'use client';

import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';

interface ImportImagesDialogProps {
  open: boolean;
  batch: dto.ImageBatch | null;
  selected: string[];
  busy: boolean;
  onToggle: (subdir: string) => void;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

/**
 * Import em lote a partir de <TranslateLocation>/images: lista os
 * diretórios logo após ps3data (events, map, menu…) para marcar e mostra o
 * total de candidatos contra quantos SÃO NOVOS — só entram os que diferem do
 * container já presente em mods/.
 */
export function ImportImagesDialog({
  open,
  batch,
  selected,
  busy,
  onToggle,
  onOpenChange,
  onConfirm,
}: ImportImagesDialogProps) {
  const subdirs = batch?.subdirs ?? [];
  const errors = batch?.errors ?? [];
  const max = batch?.max ?? 0;
  const novas = batch?.new_count ?? 0;
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o || !busy) onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Importar imagens traduzidas</DialogTitle>
          <DialogDescription>
            Somente os .dds.phyre diferentes do que já está em mods/ entram —
            os idênticos são pulados. A árvore original nunca é alterada.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <p className="break-all text-xs text-muted-foreground">
            {batch?.root ?? ''}
          </p>

          {subdirs.length === 0 ? (
            <div className="rounded-md border border-muted bg-muted/40 p-2 text-sm">
              Nenhum .dds.phyre encontrado nessa raiz.
            </div>
          ) : (
            <ScrollArea className="max-h-56 rounded-md border p-2">
              <ul className="space-y-1">
                {subdirs.map((dir) => (
                  <li key={dir} className="flex items-center gap-2">
                    <Checkbox
                      id={`img-dir-${dir}`}
                      checked={selected.includes(dir)}
                      disabled={busy}
                      onCheckedChange={() => onToggle(dir)}
                    />
                    <Label
                      htmlFor={`img-dir-${dir}`}
                      className="cursor-pointer text-sm font-normal"
                    >
                      {dir}
                    </Label>
                  </li>
                ))}
              </ul>
            </ScrollArea>
          )}
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
            <span>{max} candidato(s)</span>
            <span className="font-medium text-foreground">{novas} nova(s)</span>
            <span>{selected.length} diretório(s) marcado(s)</span>
          </div>

          {errors.length > 0 ? (
            <div className="rounded-md border border-warning/50 bg-warning/10 p-2 text-sm text-warning">
              <p className="font-semibold">
                {errors.length} textura(s) serão puladas:
              </p>
              <ScrollArea className="mt-1 max-h-40">
                <ul className="list-disc pl-5">
                  {errors.map((e, i) => (
                    <li key={`${i}:${e}`} className="break-all">
                      {e}
                    </li>
                  ))}
                </ul>
              </ScrollArea>
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => onOpenChange(false)}
          >
            Cancelar
          </Button>
          <Button
            disabled={busy || selected.length === 0 || novas === 0}
            onClick={onConfirm}
          >
            {busy ? 'Importando…' : `Importar ${novas} nova(s)`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}