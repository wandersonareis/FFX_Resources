'use client';

import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { type EntryKind, KIND_LABELS, resolveEntryLabel } from '@/lib/ffx/display-names';

interface ImportSummaryDetailsDialogProps {
  summary: dto.ImportSummary | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Detalhe do resumo de importação: as ROWS que ficaram fora (hash não
 * encontrado / tag de controle divergente) e as entradas ignoradas. É o
 * "olho" do revisor — o botão que abre pulsa quando há algo aqui; num
 * import limpo não há motivo para abrir (e nada pulsa).
 */
export function ImportSummaryDetailsDialog({
  summary,
  open,
  onOpenChange,
}: ImportSummaryDetailsDialogProps) {
  const skipped = summary?.skipped ?? [];
  const warnings = summary?.warnings ?? [];
  const kind = (summary?.kind ?? 'events') as EntryKind;
  const kindLabel = KIND_LABELS[kind] ?? summary?.kind ?? '';
  const empty = skipped.length === 0 && warnings.length === 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Detalhes do resumo</DialogTitle>
          <DialogDescription>
            Rows do arquivo que ficaram fora do import e entradas ignoradas. Nada aqui bloqueia a
            importação — é o registro do que <strong>não</strong> entra para você revisar no
            artefato.
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant="outline">{kindLabel}</Badge>
            {skipped.length > 0 ? (
              <Badge className="border-warning/50 bg-warning/10 text-warning">
                {skipped.length} row(s) ignorada(s)
              </Badge>
            ) : null}
            {warnings.length > 0 ? (
              <Badge className="border-warning/50 bg-warning/10 text-warning">
                {warnings.length} entrada(s) ignorada(s)
              </Badge>
            ) : null}
          </div>

          {empty ? (
            <div className="rounded-md border p-4 text-sm text-muted-foreground">
              Nenhuma row ignorada — importação limpa. O log completo fica em{' '}
              <code className="mx-1">build/bin/logs</code>.
            </div>
          ) : null}

          {skipped.length > 0 ? (
            <section className="space-y-2">
              <p className="text-sm font-medium">Rows ignoradas ({skipped.length})</p>
              <p className="text-xs text-muted-foreground">
                O hash da row não casa com nada no binário atual (o conteúdo mudou desde o export)
                ou a tradução divergiu das tags de controle do original. A row é pulada; o resto
                importa.
              </p>
              {/* Container ÚNICO rolando nos dois eixos (containerClassName):
                  o header gruda no topo dele (sticky) e rola horizontalmente
                  JUNTO com o corpo — separar em dois containers exigiria
                  sincronizar o arrasto horizontal. Mesma régua do
                  entry-table: sticky no thead E em cada th, porque com
                  border-collapse o Chromium pinta o thead atrás das linhas. */}
              <Table containerClassName="max-h-72 overflow-auto rounded-md border">
                <TableHeader className="sticky top-0 z-[5]">
                  <TableRow>
                    <TableHead className="sticky top-0 z-[5] bg-popover">Entrada</TableHead>
                    <TableHead className="sticky top-0 z-[5] bg-popover">Índice</TableHead>
                    <TableHead className="sticky top-0 z-[5] bg-popover">Hash</TableHead>
                    <TableHead className="sticky top-0 z-[5] bg-popover">Motivo</TableHead>
                    <TableHead className="sticky top-0 z-[5] bg-popover">Texto</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {skipped.map((issue) => (
                    <TableRow key={`${issue.id}:${issue.index}`}>
                      <TableCell className="font-medium">
                        {resolveEntryLabel(kind, issue.id)}
                      </TableCell>
                      <TableCell>{issue.index}</TableCell>
                      <TableCell>
                        <code className="text-xs" title={`$${issue.hash}`}>{`$${issue.hash}`}</code>
                      </TableCell>
                      <TableCell>{issue.reason}</TableCell>
                      {/* Sem truncate/max-w: nowrap natural da célula deixa
                          a linha crescer e o texto INTEIRO fica visível no
                          arrasto horizontal do container. */}
                      <TableCell title={issue.text}>{issue.text}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </section>
          ) : null}

          {warnings.length > 0 ? (
            <section className="space-y-2">
              <p className="text-sm font-medium">Entradas ignoradas ({warnings.length})</p>
              <ul className="list-disc space-y-1 pl-5 text-sm text-warning">
                {warnings.map((w) => (
                  <li key={w}>{w}</li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}
