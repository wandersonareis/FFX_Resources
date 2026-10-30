'use client';

import { AlertTriangle } from 'lucide-react';
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

export type ModsGuardDecision = 'save' | 'discard';

/**
 * Guarda do checkbox "Preferir binários traduzidos (mods)" (#EnableMods):
 * abre só quando há edições não salvas (o MESMO boolean que habilita o
 * botão Salvar da tabela — o editDraft na janela principal, a flag
 * HasUnsavedEdits do backend na janela filha de config).
 *
 * Enquanto ela decide, NADA é commitado: o checkbox é controlado pelo
 * store de config e segue exibindo o estado antigo — o cancelamento
 * (Cancelar, Esc, clique fora) devolve exatamente esse estado, sem tocar
 * os rascunhos nem os inputs do diálogo.
 *
 * Salvar/Descartar NÃO executam nada aqui: a decisão viaja pelo evento
 * EnableModsChange (ConfigService.ResolveEnableModsChange) e a janela
 * PRINCIPAL — dona dos rascunhos — salva/descarta e aplica o toggle. O
 * mesmo circuito serve ao modal (browser/server) e à janela filha
 * (desktop): um caminho só, orquestrado no AppShell.
 *
 * ids fixos (#mods-guard-dialog, #ModsGuard*) para os testes e2e — o
 * diálogo fica aninhado sobre o de Configurações e o seletor
 * [data-slot="dialog-content"] passa a casar com os dois.
 */
export function ModsToggleGuardDialog({
  next,
  onDecision,
  onCancel,
}: {
  /** Valor pendente do toggle; null = guarda fechada. */
  next: boolean | null;
  onDecision: (decision: ModsGuardDecision) => void;
  onCancel: () => void;
}) {
  return (
    <Dialog
      open={next !== null}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      {/* Sem X: a escolha é explícita (Esc/clique fora = Cancelar, que
          devolve o checkbox ao estado anterior sem perder rascunho). */}
      <DialogContent id="mods-guard-dialog" showCloseButton={false} className="sm:max-w-105">
        <DialogHeader>
          <DialogTitle>Alterações não salvas</DialogTitle>
          <DialogDescription>
            Há textos editados que ainda não foram salvos no jogo.
          </DialogDescription>
        </DialogHeader>
        <Alert variant="destructive">
          <AlertTriangle />
          <AlertTitle>Todo o conteúdo será recarregado</AlertTitle>
          <AlertDescription>
            <p>
              Escolher <strong>Salvar e recarregar</strong> ou{' '}
              <strong>Descartar e recarregar</strong> aplica a nova preferência de mods e recarrega
              todo o conteúdo — a tradução exibida muda de acordo.
            </p>
            <p>
              Somente <strong>Cancelar</strong> mantém tudo como está.
            </p>
          </AlertDescription>
        </Alert>
        <DialogFooter>
          <Button id="ModsGuardCancel" variant="outline" onClick={onCancel}>
            Cancelar
          </Button>
          <Button id="ModsGuardDiscard" variant="destructive" onClick={() => onDecision('discard')}>
            Descartar e recarregar
          </Button>
          <Button id="ModsGuardSave" onClick={() => onDecision('save')}>
            Salvar e recarregar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
