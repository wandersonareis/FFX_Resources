'use client';

import { Download } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { parseError } from '@/lib/ffx/error-handler';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { extractImageGroup } from '@/lib/ffx/tree-data';
import { type ActionProps, reportBatch, useActionDuplicates } from './image-action-shared';
import { DuplicateList } from './image-duplicate-list';

/** "Extrair só esta ou todas as cópias" — a confirmação de duplicatas. */
export function ExtractImageDialog({ view, action }: ActionProps) {
  const { version, actions } = view;
  const list = useActionDuplicates(view, action);
  const [busy, setBusy] = useState(false);
  const copies = list ?? [];
  const divergent = copies.filter((d) => !d.identical).length;
  const identical = copies.length - divergent;
  const close = () => actions.closeImageAction();

  const run = async (targets: string[]) => {
    setBusy(true);
    try {
      const res = await extractImageGroup(action.id, targets, version);
      reportBatch(res, 'Extraída', 'Extraídas');
      await actions.reload();
      close();
    } catch (error) {
      toast.error(parseError(error));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) close();
      }}
    >
      <DialogContent className="sm:max-w-lg" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>Extrair {action.label}?</DialogTitle>
          <DialogDescription>
            Gera .dds e .png em mods/images (a cópia de trabalho, sem tocar em data/ nem em
            mods/*.dds.phyre). Escolha o alcance.
          </DialogDescription>
        </DialogHeader>

        {list === null ? (
          <p className="text-sm opacity-70">Carregando cópias…</p>
        ) : copies.length === 0 ? (
          <p className="text-sm opacity-70">Imagem única: não há cópias para extrair junto.</p>
        ) : (
          <>
            <p className="text-sm">
              Esta imagem tem {copies.length} cópia
              {copies.length > 1 ? 's' : ''}:{' '}
              {identical > 0
                ? `${identical} idêntica${identical > 1 ? 's' : ''}${divergent > 0 ? ' e ' : ''}`
                : ''}
              {divergent > 0
                ? `${divergent} com edição própria (divergente${divergent > 1 ? 's' : ''})`
                : ''}{' '}
              — extrair todas deixa os arquivos lado a lado para conferir a duplicata.
            </p>
            <DuplicateList id={action.id} list={copies} />
          </>
        )}

        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={close}>
            Cancelar
          </Button>
          <Button disabled={busy || list === null} onClick={() => void run([])}>
            <Download size={16} />
            {busy ? 'Extraindo…' : 'Extrair só esta'}
          </Button>
          {copies.length > 0 ? (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void run(copies.map((d) => d.id))}
            >
              Extrair {copies.length + 1} texturas
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
