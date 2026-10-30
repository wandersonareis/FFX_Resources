'use client';

import { AlertTriangle, Trash2 } from 'lucide-react';
import { useState } from 'react';
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
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { deleteImages } from '@/lib/ffx/tree-data';
import {
  type ActionProps,
  DeleteIrreversibleAlert,
  type DeleteScope,
  DeleteScopeField,
  reportBatch,
  useActionDuplicates,
} from './image-action-shared';

/**
 * Deletar: escopo explícito (nunca cascata implícita) + cópias só se
 * marcadas, com o aviso de irreversível antes de qualquer gravação. Sempre
 * leva junto os .dds/.png extraídos — senão Resolve serviria imagem órfã.
 */
export function DeleteImageDialog({ view, action }: ActionProps) {
  const { version, actions } = view;
  const list = useActionDuplicates(view, action);
  const [scope, setScope] = useState<DeleteScope>('both');
  const [withCopies, setWithCopies] = useState(false);
  const [busy, setBusy] = useState(false);
  const copies = list ?? [];
  const targets = withCopies ? copies.map((d) => d.id) : [];
  const divergent = copies.filter((d) => !d.identical).length;
  const close = () => actions.closeImageAction();

  const run = async () => {
    setBusy(true);
    try {
      const res = await deleteImages(action.id, targets, scope, version);
      reportBatch(res, 'Apagada', 'Apagadas');
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
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Deletar {action.label}?</DialogTitle>
          <DialogDescription>
            Escolha o escopo — o aviso abaixo lista exatamente o que sai do disco.
          </DialogDescription>
        </DialogHeader>

        <DeleteIrreversibleAlert scope={scope} />

        <DeleteScopeField
          scope={scope}
          onChange={setScope}
          legend="Escopo"
          name="image-delete-scope"
        />

        {copies.length > 0 ? (
          <>
            <div className="flex items-center gap-2">
              <Checkbox
                id="delete-copies"
                checked={withCopies}
                onCheckedChange={(value) => setWithCopies(value === true)}
              />
              <Label htmlFor="delete-copies" className="cursor-pointer text-sm">
                Apagar também as {copies.length} cópia
                {copies.length > 1 ? 's' : ''}
              </Label>
            </div>
            {withCopies && divergent > 0 ? (
              <Alert variant="warning">
                <AlertTriangle />
                <AlertTitle>
                  {divergent} cópia{divergent > 1 ? 's' : ''} diverge
                  {divergent > 1 ? 'm' : ''} do original
                </AlertTitle>
                <AlertDescription>
                  Estão divergentes porque foram editadas em separado — a edição delas também será
                  apagada.
                </AlertDescription>
              </Alert>
            ) : null}
          </>
        ) : (
          <p className="text-sm opacity-70">Imagem única: sem cópias.</p>
        )}

        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={close}>
            Cancelar
          </Button>
          <Button variant="destructive" disabled={busy || list === null} onClick={() => void run()}>
            <Trash2 size={16} />
            {busy
              ? 'Apagando…'
              : `Deletar ${targets.length + 1} textura${targets.length > 0 ? 's' : ''}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
