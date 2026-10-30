'use client';

import { AlertTriangle, Info } from 'lucide-react';
import { useState } from 'react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import { replicateImage } from '@/lib/ffx/tree-data';
import { invalidateVbfImage, replicateVbfImage } from '@/lib/ffx/vbf';
import { type ActionProps, reportBatch, useActionDuplicates } from './image-action-shared';
import { DuplicateList } from './image-duplicate-list';

/**
 * Replicar: leva a IMAGEM ABERTA para as cópias em mods/ — o dupe que o app
 * faz sozinho, sem diálogo de arquivo. As contagens avisam o que se perde
 * (divergentes sobrescritas) e o que se cria (pristine viram mods/).
 */
export function ReplicateImageDialog({ view, action }: ActionProps) {
  const { version, actions } = view;
  const list = useActionDuplicates(view, action);
  const [busy, setBusy] = useState(false);
  const copies = list ?? [];
  const divergent = copies.filter((d) => !d.identical).length;
  const pristine = copies.filter((d) => !d.modded).length;
  const close = () => actions.closeImageAction();

  const run = async () => {
    setBusy(true);
    try {
      const res = action.vbf
        ? await replicateVbfImage(
            action.vbf.root,
            action.vbf.path,
            copies.map((d) => d.vbfPath ?? '').filter(Boolean),
          )
        : await replicateImage(
            action.id,
            copies.map((d) => d.id),
            version,
          );
      reportBatch(res, 'Replicada', 'Replicadas');
      if (action.vbf) {
        invalidateVbfImage(action.vbf.root, action.vbf.path);
        const selected = view.store.state.selectedEntry;
        if (selected?.vbf?.root === action.vbf.root && selected.id === action.id) {
          await actions.selectEntry(selected);
        }
      } else {
        await actions.reload();
      }
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
          <DialogTitle>
            Replicar {action.label}
            {list !== null && copies.length > 0
              ? ` para ${copies.length} cópia${copies.length === 1 ? '' : 's'}`
              : ''}
            ?
          </DialogTitle>
          <DialogDescription>
            {action.vbf
              ? 'A imagem lida do .vbf é reempacotada sobre o binário original de cada cópia e gravada em mods/. O container .vbf não é alterado.'
              : 'O conteúdo já carregado é reempacotado sobre o container pristine de cada cópia e gravado em mods/. Nenhum arquivo é escolhido: a fonte é a própria imagem na tela, e a textura de origem não é tocada.'}
          </DialogDescription>
        </DialogHeader>

        {list === null ? (
          <p className="text-sm opacity-70">Carregando cópias…</p>
        ) : copies.length === 0 ? (
          <p className="text-sm opacity-70">Imagem única: não há cópias para replicar.</p>
        ) : (
          <>
            {divergent > 0 ? (
              <Alert variant="warning">
                <AlertTriangle />
                <AlertTitle>
                  {divergent} cópia{divergent > 1 ? 's' : ''} divergente
                  {divergent > 1 ? 's' : ''} serão sobrescrita
                  {divergent > 1 ? 's' : ''}
                </AlertTitle>
                <AlertDescription>
                  Foram editadas em separado e perderão a edição atual.
                </AlertDescription>
              </Alert>
            ) : null}
            {pristine > 0 ? (
              <Alert variant="info">
                <Info />
                <AlertTitle>
                  {pristine} cópia{pristine > 1 ? 's' : ''} pristine receber
                  {pristine > 1 ? 'ão' : ''} arquivo em mods/
                </AlertTitle>
                <AlertDescription>
                  data/ continua intacto — dá para desfazer apagando só mods/.
                </AlertDescription>
              </Alert>
            ) : null}
            <DuplicateList id={action.id} list={copies} />
          </>
        )}

        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={close}>
            Cancelar
          </Button>
          <Button
            disabled={busy || list === null || copies.length === 0}
            onClick={() => void run()}
          >
            {busy
              ? 'Replicando…'
              : list === null
                ? 'Carregando…'
                : `Replicar para ${copies.length} cópia${copies.length === 1 ? '' : 's'}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
