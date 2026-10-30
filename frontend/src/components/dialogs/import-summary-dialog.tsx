'use client';

import { useEffect, useState } from 'react';
import { ScrollText } from 'lucide-react';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { EntryKind, KIND_LABELS, resolveEntryLabel } from '@/lib/ffx/display-names';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from 'cn';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ImportSummaryDetailsDialog } from '@/components/dialogs/import-summary-details-dialog';

interface ImportSummaryDialogProps {
  summary: dto.ImportSummary | null;
  open: boolean;
  importing: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

/**
 * Resumo da importação antes de aplicar: estatísticas do arquivo, conferência
 * por entrada (índices vs store), uso do limite binário (uint16), avisos e
 * erros. Entradas com problema são ignoradas (aviso) e o import segue com as
 * demais; só um bloqueador global desabilita o botão Importar.
 *
 * Rows puladas (origem não encontrada / tag divergente) não bloqueiam nada e
 * ficam no modal de DETALHES — o botão que o abre pulsa quando há algo para
 * ver; num import limpo ele fica quieto.
 */
export function ImportSummaryDialog({
  summary,
  open,
  importing,
  onOpenChange,
  onConfirm,
}: ImportSummaryDialogProps) {
  const [detailsOpen, setDetailsOpen] = useState(false);
  // O componente fica montado (open é prop) — sem isto o detalhe reabriria
  // sozinho na PRÓXIMA importação depois de ficar aberto numa sessão anterior.
  useEffect(() => {
    if (!open) setDetailsOpen(false);
  }, [open]);
  const entries = summary?.entries ?? [];
  const usages = summary?.usages ?? [];
  const errors = summary?.errors ?? [];
  const warnings = summary?.warnings ?? [];
  const skipped = summary?.skipped ?? [];
  // Atenção do revisor: rows fora do import OU entradas ignoradas — o
  // "183 atualizados" do toast esconde isso; o pulse é o único sinal.
  const needsAttention = skipped.length > 0 || warnings.length > 0;
  // Bloqueadores globais (dicionário macro): a importação inteira para.
  const blocked = errors.length > 0;
  // Entradas com problema são ignoradas — o import segue com as demais;
  // sem nenhuma entrada restante não há o que aplicar.
  const importable = entries.some((entry) => !entry.error);
  const kind = (summary?.kind ?? 'events') as EntryKind;
  const kindLabel = KIND_LABELS[kind] ?? summary?.kind ?? '';

  return (
    <>
    <Dialog open={open} onOpenChange={(o) => { if (!o || !importing) onOpenChange(o); }}>
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Importar textos</DialogTitle>
          <DialogDescription>
            Confira o resumo antes de aplicar. Somente o texto em inglês
            (&quot;us&quot;) é importado; o formato não aceita entradas novas.
          </DialogDescription>
        </DialogHeader>

        {summary ? (
          <div className="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
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

            <Button
              size="sm"
              variant="outline"
              onClick={() => setDetailsOpen(true)}
              className={cn(
                'gap-1.5',
                needsAttention &&
                  // Chamada para o revisor: só pulsa quando há rows fora do
                  // import ou entradas ignoradas. motion-safe respeita o
                  // prefers-reduced-motion (o Tailwind v4 embute o keyframe).
                  'motion-safe:animate-pulse border-warning bg-warning/10 text-warning hover:bg-warning/20 hover:text-warning'
              )}
              title={
                needsAttention
                  ? 'Há rows/entradas que ficaram fora do import — abra para ver'
                  : 'Resumo detalhado (rows ignoradas, entradas ignoradas)'
              }
            >
              <ScrollText size={14} />
              Detalhes do resumo
              {needsAttention ? (
                <Badge className="ml-1 border-warning/50 bg-warning/10 text-warning">
                  {skipped.length + warnings.length}
                </Badge>
              ) : null}
            </Button>

            {summary.saves_binary ? (
              <div className="rounded-md border border-warning/50 bg-warning/10 p-2 text-sm text-warning">
                Ao confirmar, o texto importado será <strong>salvo em binário</strong> em
                <code className="mx-1">mods/</code> e recarregado dali nas próximas sessões
                (continuidade da tradução).
              </div>
            ) : null}

            {warnings.length > 0 ? (
              <div className="rounded-md border border-warning/50 bg-warning/10 p-2 text-sm text-warning">
                <p className="font-semibold">
                  Entradas ignoradas ({warnings.length}):
                </p>
                <p className="mt-0.5">
                  Estas entradas têm erro e serão <strong>ignoradas</strong> se
                  você continuar — as demais importam normalmente.
                </p>
                <ul className="list-disc pl-5">
                  {warnings.map((w, i) => (
                    <li key={`${i}:${w}`}>{w}</li>
                  ))}
                </ul>
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

            {/* Mesma régua do modal de detalhes: container único rolando nos
                dois eixos, header sticky (thead E th) para o topo travar
                enquanto o corpo rola. */}
            <Table containerClassName="max-h-64 overflow-auto rounded-md border">
              <TableHeader className="sticky top-0 z-[5]">
                <TableRow>
                  <TableHead className="sticky top-0 z-[5] bg-popover">Entrada</TableHead>
                  <TableHead className="sticky top-0 z-[5] bg-popover">Índices</TableHead>
                  <TableHead className="sticky top-0 z-[5] bg-popover">Store</TableHead>
                  <TableHead className="sticky top-0 z-[5] bg-popover">Alterados</TableHead>
                  <TableHead
                    className={cn(
                      'sticky top-0 z-[5] bg-popover',
                      skipped.length > 0 && 'text-warning'
                    )}
                  >
                    Ignoradas
                  </TableHead>
                  <TableHead className="sticky top-0 z-[5] bg-popover">Erro</TableHead>
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
                      className={cn(
                        entry.skipped_rows > 0 && 'font-semibold text-warning'
                      )}
                    >
                      {entry.skipped_rows > 0 ? entry.skipped_rows : '—'}
                    </TableCell>
                    <TableCell
                      className={entry.error ? 'text-destructive' : undefined}
                      title={entry.error || undefined}
                    >
                      {entry.error || '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

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
            disabled={blocked || importing || !summary || (entries.length > 0 && !importable)}
            onClick={onConfirm}
          >
            {importing ? 'Importando…' : 'Importar'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    {/* Detalhe aninhado: abre por cima sem fechar a confirmação — o
        revisor vê o que ficou fora e decide Importar/Cancelar ali. */}
    <ImportSummaryDetailsDialog
      summary={summary}
      open={detailsOpen}
      onOpenChange={setDetailsOpen}
    />
    </>
  );
}
