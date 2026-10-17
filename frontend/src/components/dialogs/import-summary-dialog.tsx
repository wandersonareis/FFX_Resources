'use client';

import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { EntryKind, KIND_LABELS, resolveEntryLabel } from '@/lib/ffx/display-names';
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
import { Progress } from '@/components/ui/progress';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

interface ImportSummaryDialogProps {
  summary: dto.ImportSummary | null;
  open: boolean;
  importing: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

/**
 * Resumo da importação antes de aplicar: estatísticas do arquivo, conferência
 * por entrada (índices vs store), uso do limite binário (uint16) e erros.
 * Qualquer erro bloqueia a importação inteira (botão Importar desabilitado).
 */
export function ImportSummaryDialog({
  summary,
  open,
  importing,
  onOpenChange,
  onConfirm,
}: ImportSummaryDialogProps) {
  const entries = summary?.entries ?? [];
  const usages = summary?.usages ?? [];
  const errors = summary?.errors ?? [];
  const blocked = errors.length > 0;
  const kind = (summary?.kind ?? 'events') as EntryKind;
  const kindLabel = KIND_LABELS[kind] ?? summary?.kind ?? '';

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o || !importing) onOpenChange(o); }}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Importar textos</DialogTitle>
          <DialogDescription>
            Confira o resumo antes de aplicar. Somente o texto em inglês
            (&quot;us&quot;) é importado; o formato não aceita entradas novas.
          </DialogDescription>
        </DialogHeader>

        {summary ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <Badge variant="outline">{summary.format.toUpperCase()}</Badge>
              <Badge variant="outline">{kindLabel}</Badge>
              <Badge variant="outline">{summary.version}</Badge>
              <span className="text-muted-foreground">
                {summary.entry_count} entrada(s)
              </span>
              <span className="text-muted-foreground">
                {summary.total_indices} índice(s)
              </span>
              <span className="text-muted-foreground">
                {summary.changed_texts} texto(s) alterado(s)
              </span>
              {summary.languages.length > 0 ? (
                <span className="text-muted-foreground">
                  idiomas: {summary.languages.join(', ')}
                </span>
              ) : null}
            </div>
            <p className="text-xs break-all text-muted-foreground">{summary.path}</p>

            {summary.saves_binary ? (
              <div className="rounded-md border border-warning/50 bg-warning/10 p-2 text-sm text-warning">
                Ao confirmar, o texto importado será <strong>salvo em binário</strong> em
                <code className="mx-1">mods/</code> e recarregado dali nas próximas sessões
                (continuidade da tradução).
              </div>
            ) : null}

            {blocked ? (
              <div className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
                <p className="font-semibold">
                  Importação bloqueada ({errors.length}):
                </p>
                <ul className="list-disc pl-5">
                  {errors.map((e, i) => (
                    <li key={`${i}:${e}`}>{e}</li>
                  ))}
                </ul>
              </div>
            ) : null}

            <ScrollArea className="max-h-64 rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Entrada</TableHead>
                    <TableHead>Índices</TableHead>
                    <TableHead>Store</TableHead>
                    <TableHead>Alterados</TableHead>
                    <TableHead>Erro</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell className="font-medium">
                        {resolveEntryLabel(kind, entry.id)}
                      </TableCell>
                      <TableCell>{entry.index_count}</TableCell>
                      <TableCell>{entry.store_index_count}</TableCell>
                      <TableCell>{entry.changed_texts}</TableCell>
                      <TableCell
                        className={entry.error ? 'text-destructive' : undefined}
                      >
                        {entry.error || '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </ScrollArea>

            {usages.length > 0 ? (
              <div className="space-y-2">
                <p className="text-sm font-medium">Limite binário</p>
                {usages.map((usage, i) => {
                  const pct = Math.min(
                    100,
                    Math.round((usage.used / Math.max(1, usage.limit)) * 100)
                  );
                  const label = usage.id
                    ? resolveEntryLabel(usage.kind as EntryKind, usage.id)
                    : 'Dicionário completo';
                  return (
                    <div
                      key={`${usage.kind}:${usage.id || 'all'}:${i}`}
                      className="space-y-1"
                    >
                      <div className="flex justify-between gap-2 text-xs">
                        <span className="truncate">{label}</span>
                        <span
                          className={
                            usage.over
                              ? 'font-semibold text-destructive'
                              : 'text-muted-foreground'
                          }
                        >
                          {usage.used} / {usage.limit} B ({pct}%)
                        </span>
                      </div>
                      <Progress value={pct} />
                    </div>
                  );
                })}
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
            disabled={blocked || importing || !summary}
            onClick={onConfirm}
          >
            {importing ? 'Importando…' : 'Importar'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
