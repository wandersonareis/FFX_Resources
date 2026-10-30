'use client';

import { AlertTriangle, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import { parseError } from '@/lib/ffx/error-handler';
import { exportSelection } from '@/lib/ffx/export-selection';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { deleteImageSelection, imageSelectionCopies } from '@/lib/ffx/tree-data';
import {
  type ActionProps,
  DeleteIrreversibleAlert,
  type DeleteScope,
  DeleteScopeField,
  reportBatch,
} from './image-action-shared';

const EMPTY_IMAGE_IDS: string[] = [];

/** Deletar a SELEÇÃO da árvore: mesmo escopo único, mas cópias próprias por imagem. */
export function DeleteImageSelectionDialog({ view, action }: ActionProps) {
  const { version, actions } = view;
  const ids = action.ids ?? EMPTY_IMAGE_IDS;
  const [scope, setScope] = useState<DeleteScope>('both');
  const [withCopies, setWithCopies] = useState(false);
  const [busy, setBusy] = useState(false);
  const [copyIDs, setCopyIDs] = useState<string[]>([]);
  const [duplicatesReady, setDuplicatesReady] = useState(false);
  const [copyLookupError, setCopyLookupError] = useState<string | null>(null);
  const close = () => actions.closeImageAction();

  useEffect(() => {
    let cancelled = false;
    void imageSelectionCopies(ids, version)
      .then((copies) => {
        if (cancelled) return;
        setCopyIDs(copies ?? []);
        setDuplicatesReady(true);
      })
      .catch((error) => {
        if (cancelled) return;
        setCopyIDs([]);
        setCopyLookupError(parseError(error));
        setDuplicatesReady(true);
        toast.error(parseError(error));
      });
    return () => {
      cancelled = true;
    };
  }, [ids, version]);

  const targets = withCopies ? copyIDs : [];
  const deleteCount = ids.length + targets.length;
  const run = async () => {
    if (ids.length === 0) return;
    setBusy(true);
    try {
      const res = await deleteImageSelection(ids, withCopies, scope, version);
      reportBatch(res, 'Apagada', 'Apagadas');
      exportSelection.setMany(version, 'images', res.done ?? [], false);
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
          <DialogTitle>
            Deletar {ids.length} {ids.length === 1 ? 'imagem selecionada' : 'imagens selecionadas'}?
          </DialogTitle>
          <DialogDescription>
            O escopo escolhido será aplicado a todas as imagens selecionadas. As cópias só serão
            incluídas se a opção abaixo estiver marcada.
          </DialogDescription>
        </DialogHeader>

        <DeleteIrreversibleAlert scope={scope} />

        <DeleteScopeField
          scope={scope}
          onChange={setScope}
          legend="Escopo para todas as selecionadas"
          name="image-delete-selection-scope"
        />

        {duplicatesReady ? (
          copyLookupError ? (
            <Alert variant="warning">
              <AlertTriangle />
              <AlertTitle>Não foi possível verificar as cópias</AlertTitle>
              <AlertDescription>
                A confirmação ainda pode apagar somente as imagens marcadas. Cópias adicionais não
                serão incluídas.
              </AlertDescription>
            </Alert>
          ) : copyIDs.length > 0 ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="delete-selection-copies"
                  checked={withCopies}
                  onCheckedChange={(value) => setWithCopies(value === true)}
                />
                <Label htmlFor="delete-selection-copies" className="cursor-pointer text-sm">
                  Apagar também {copyIDs.length} cópia
                  {copyIDs.length === 1 ? '' : 's'} adicional
                  {copyIDs.length === 1 ? '' : 'is'}
                </Label>
              </div>
              {withCopies ? (
                <Alert variant="warning">
                  <AlertTriangle />
                  <AlertTitle>{targets.length} cópia(s) adicionais serão incluídas</AlertTitle>
                  <AlertDescription>
                    Cópias podem ter sido editadas separadamente. Todas as cópias adicionais serão
                    apagadas com o mesmo escopo.
                  </AlertDescription>
                </Alert>
              ) : null}
            </div>
          ) : (
            <p className="text-sm opacity-70">As imagens selecionadas não têm cópias.</p>
          )
        ) : (
          <p className="text-sm opacity-70">Verificando cópias das selecionadas…</p>
        )}

        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={close}>
            Cancelar
          </Button>
          <Button
            variant="destructive"
            disabled={busy || !duplicatesReady || ids.length === 0}
            onClick={() => void run()}
          >
            <Trash2 size={16} />
            {busy
              ? 'Apagando…'
              : `Deletar ${deleteCount} ${deleteCount === 1 ? 'imagem' : 'imagens'}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
