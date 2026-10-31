'use client';

import { useSelector } from '@tanstack/react-store';
import { AlertTriangle } from 'lucide-react';
import type * as services from '@/bindings/ffxresources/backend/services/models';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { pruneConfirmStore, resolvePruneConfirm } from '@/lib/ffx/prune-confirm-store';

/**
 * Aviso destrutivo do Save: lista TODAS as linhas que serão excluídas por
 * não existirem no original (dicionário com ponteiro "simplified" errado,
 * por exemplo). O binário não aceita menos linhas que o original; o save
 * poda só as órfãs, então as linhas originais (e o preenchimento da
 * tradução parcial) ficam.
 *
 * Largura/altura EXPLÍCITAS (w-[/max-w-[min(...)]) e lista com rolagem
 * própria: variantes sm: já foram aplicadas em viewport não-mobile nas
 * betas do Wails 3 e estouravam o modal — aqui não dependemos delas.
 */
export function PruneOrphansDialog() {
  const { open, rows } = useSelector(pruneConfirmStore, (s) => s);
  const byFile = groupByFile(rows);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) resolvePruneConfirm(false);
      }}
    >
      <DialogContent
        id="prune-dialog"
        showCloseButton={false}
        className="flex h-[min(88vh,46rem)] w-[min(64rem,calc(100%-2rem))] max-w-[min(64rem,calc(100%-2rem))] flex-col gap-3 overflow-hidden"
      >
        <DialogHeader className="shrink-0">
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle size={18} aria-hidden />
            {rows.length} linha(s) sem par no original serão excluídas
          </DialogTitle>
          <DialogDescription>
            Ao salvar, estas linhas serão removidas dos binários em <code>mods/</code> porque não
            existem no original. A gravação é permanente e não há desfazer no app. As linhas do
            original são preservadas (o binário não aceita menos linhas que o original).
          </DialogDescription>
        </DialogHeader>

        <div
          className="min-h-0 flex-1 overflow-y-auto rounded-md border border-border bg-muted/30"
          data-testid="prune-list"
        >
          {byFile.map(([file, fileRows]) => (
            <section key={file} className="border-b border-border last:border-b-0">
              <h3 className="sticky top-0 z-10 truncate bg-muted px-2 py-1 font-mono text-xs font-semibold">
                {file}
              </h3>
              <ul className="divide-y divide-border/60">
                {fileRows.map((row) => (
                  <li
                    key={`${file}-${row.block ?? ''}-${row.index}-${row.name}`}
                    className="flex flex-col gap-0.5 px-2 py-1.5"
                  >
                    <span className="font-mono text-xs text-muted-foreground">
                      {row.block != null ? `bloco ${pad(row.block)} · ` : ''}índice {row.index} ·{' '}
                      {row.name}
                      {row.langs && row.langs.length > 0 ? ` · ${row.langs.join(', ')}` : ''}
                    </span>
                    <span className="wrap-break-word font-mono text-sm" data-testid="prune-text">
                      {row.text?.trim() ? row.text : '(vazio)'}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>

        <DialogFooter className="shrink-0">
          <Button variant="ghost" onClick={() => resolvePruneConfirm(false)}>
            Cancelar
          </Button>
          <Button variant="destructive" onClick={() => resolvePruneConfirm(true)}>
            Excluir {rows.length} e salvar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function pad(block: number): string {
  return String(block).padStart(2, '0');
}

/** Ordena por arquivo, bloco e índice — a leitura do log bate com a lista. */
function groupByFile(rows: services.PrunedRowView[]): Array<[string, services.PrunedRowView[]]> {
  const byFile = new Map<string, services.PrunedRowView[]>();
  for (const row of rows) {
    const list = byFile.get(row.file) ?? [];
    list.push(row);
    byFile.set(row.file, list);
  }
  return [...byFile.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([file, list]) => [
      file,
      [...list].sort(
        (a, b) =>
          (a.block ?? -1) - (b.block ?? -1) || a.index - b.index || a.name.localeCompare(b.name),
      ),
    ]);
}
