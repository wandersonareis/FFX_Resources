'use client';

import { AlertTriangle, CheckCircle2, CircleDashed, Loader2, XCircle } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import {
  clearProgressBatch,
  type ProgressBatch,
  type ProgressStep,
  progressEnded,
  progressStarted,
  progressUpdated,
  useProgressBatch,
} from '@/lib/ffx/progress-store';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';

const fmt = (n: number): string => n.toLocaleString('pt-BR');

/** Contagem da ação: "301/301" quando o total é conhecido. */
function stepCount(step: ProgressStep): string {
  return step.total > 0 ? `${fmt(step.processed)}/${fmt(step.total)}` : fmt(step.processed);
}

/** Uma ação da lista: ícone por estado + barra própria + contagem. */
function StepRow({ step }: { step: ProgressStep }) {
  const dim = step.state === 'pending';
  return (
    <div className={`flex items-center gap-2.5 ${dim ? 'opacity-50' : ''}`} role="status">
      <span className="shrink-0">
        {step.state === 'running' ? (
          <Loader2 className="size-4 animate-spin" aria-label="Executando" />
        ) : step.state === 'done' ? (
          step.issueCount > 0 ? (
            <AlertTriangle className="size-4 text-warning" aria-label="Concluída com falhas" />
          ) : (
            <CheckCircle2 className="size-4 text-success" aria-label="Concluída" />
          )
        ) : step.state === 'error' ? (
          <XCircle className="size-4 text-destructive" aria-label="Falhou" />
        ) : (
          <CircleDashed className="size-4" aria-label="Aguardando" />
        )}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{step.label}</p>
        {step.state === 'running' ? (
          <Progress
            value={step.total > 0 ? Math.min(100, (step.processed / step.total) * 100) : 0}
            className="mt-1 h-1.5"
          />
        ) : null}
        {step.state === 'error' && step.error ? (
          <p className="mt-0.5 truncate text-xs text-destructive" title={step.error}>
            {step.error}
          </p>
        ) : null}
      </div>
      {step.state !== 'pending' ? (
        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
          {stepCount(step)}
        </span>
      ) : null}
    </div>
  );
}

/** Estado final do batch: resultado geral + resumo de arquivos/destino. */
function BatchResult({ batch }: { batch: ProgressBatch }) {
  const failed = batch.failed ?? false;
  const withIssues = batch.issueCount > 0;
  return (
    // min-w-0: o BatchResult é um ITEM do grid do DialogContent, e item de
    // grid nasce com min-width:auto (mínimo = min-content). Sem isso o
    // destino longo abaixo empurraria a coluna e o modal "cresceria" além
    // do sm:max-w-md do chamador.
    <div className="min-w-0 space-y-2 py-2" role="status">
      <div className="flex justify-center">
        {failed ? (
          <XCircle className="size-14 text-destructive" aria-label="Operação falhou" />
        ) : withIssues ? (
          <AlertTriangle className="size-14 text-warning" aria-label="Concluído com falhas" />
        ) : (
          <CheckCircle2 className="size-14 text-success" aria-label="Concluído" />
        )}
      </div>
      <p className="text-center text-sm text-muted-foreground">
        {failed
          ? 'Operação interrompida.'
          : withIssues
            ? `Concluído com ${batch.issueCount} falha(s).`
            : 'Processamento concluído.'}
      </p>
      {batch.written && batch.written > 0 ? (
        // wrap-anywhere (e não o wrap-break-word usado nos diálogos de
        // import): o destino pode ser UM caminho absoluto sem espaços —
        // export de um kind só, o caso da aba Eternal Calm, que exporta
        // apenas Eventos e por isso mostra o path completo do artefato em
        // vez de "N arquivos em <pasta>". Palavra sem espaço não quebra com
        // break-word sem estourar o grid: anywhere quebra em qualquer
        // carácter E reduz o min-content, então o modal mantém a largura e
        // o texto desce para a linha de baixo.
        <p className="text-center text-xs tabular-nums text-muted-foreground wrap-anywhere">
          {fmt(batch.written)} arquivo(s)
          {batch.destination ? ` — ${batch.destination}` : ''}
        </p>
      ) : null}
    </div>
  );
}

/**
 * O modal de progresso: escuta os eventos do wails (ProgressStart abre,
 * ProgressUpdate ticka, ProgressEnd congela no resultado) e renderiza o
 * snapshot da store. Um batch agrupado = título + "Ação k/N" + um step por
 * ação com a própria barra (p.ex. "Eventos 0→301/301").
 */
export function ProgressModal() {
  const batch = useProgressBatch();
  // Fechamento é do usuário: a caixa fica aberta no estado de resultado
  // até o Fechar (ou o clique fora, liberado só quando concluído).
  const [dismissed, setDismissed] = useState(false);

  useWailsEvent('ProgressStart', (data) => {
    setDismissed(false);
    progressStarted(data as ProgressBatch);
  });
  useWailsEvent('ProgressUpdate', (data) => progressUpdated(data as ProgressBatch));
  useWailsEvent('ProgressEnd', (data) => progressEnded(data as ProgressBatch));

  // Batch novo (start) reseta o fechado; store limpa quando fecha.
  useEffect(() => {
    if (batch && !batch.done) setDismissed(false);
  }, [batch]);

  const open = batch !== null && !dismissed;
  const done = batch?.done ?? false;
  const action = batch
    ? `${Math.min(batch.current + 1, batch.steps.length)} de ${batch.steps.length}`
    : '';
  const anyIssue = (batch?.issueCount ?? 0) > 0 || (batch?.failed ?? false);

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && done) {
          setDismissed(true);
          clearProgressBatch();
        }
      }}
    >
      <DialogContent
        className="sm:max-w-md"
        onInteractOutside={(e) => e.preventDefault()}
        showCloseButton={done}
        onEscapeKeyDown={(e) => {
          if (!done) e.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{batch?.title ?? 'Processando…'}</DialogTitle>
          <DialogDescription>
            {done
              ? anyIssue
                ? 'Concluído com falhas — confira as ações marcadas.'
                : 'Todas as ações concluídas.'
              : `Ação ${action} em andamento.`}
          </DialogDescription>
        </DialogHeader>
        {batch ? (
          done ? (
            <BatchResult batch={batch} />
          ) : (
            <div className="space-y-2.5 py-1">
              {batch.steps.map((step) => (
                <StepRow key={step.label} step={step} />
              ))}
            </div>
          )
        ) : null}
        {done ? (
          <div className="flex justify-end">
            <Button
              onClick={() => {
                setDismissed(true);
                clearProgressBatch();
              }}
            >
              Fechar
            </Button>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
