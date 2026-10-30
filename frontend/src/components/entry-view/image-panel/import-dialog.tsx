'use client';

import { Copy } from 'lucide-react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { DuplicateList } from '@/components/dialogs/image-duplicate-list';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { ImageBusy } from './use-image-actions';

/**
 * Passo de alcance do import: o .dds já está escolhido (fica em
 * `pendingImport` no chamador) e o usuário decide se aplica só à textura
 * aberta ou a todas as cópias idênticas do grupo de payload original.
 *
 * Cópias divergentes serão sobrescritas — por isso a lista mostra o estado
 * de cada uma antes da confirmação.
 */
export function ImageImportDialog({
  open,
  entryId,
  duplicates,
  total,
  busy,
  onCancel,
  onImportOne,
  onImportAll,
}: {
  open: boolean;
  entryId: string;
  duplicates: dto.ImageDuplicate[];
  /** Total a receber o arquivo: esta textura + cópias idênticas. */
  total: number;
  busy: ImageBusy;
  onCancel: () => void;
  onImportOne: () => void;
  onImportAll: () => void;
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Importar em {total} texturas?</DialogTitle>
          <DialogDescription>
            Esta imagem tem {duplicates.length} cópias idênticas — a otimização do DVD repetiu o
            mesmo conteúdo por caminho. O mesmo .dds pode ser reempacotado sobre todas para
            mantê-las sincronizadas. Cópias divergentes serão sobrescritas.
          </DialogDescription>
        </DialogHeader>

        <DuplicateList id={entryId} list={duplicates} />

        <DialogFooter>
          <Button variant="outline" disabled={busy !== null} onClick={onCancel}>
            Cancelar
          </Button>
          <Button variant="outline" disabled={busy !== null} onClick={onImportOne}>
            Só nesta
          </Button>
          <Button disabled={busy !== null} onClick={onImportAll}>
            <Copy size={16} className="mr-1" />
            Importar em {total}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
