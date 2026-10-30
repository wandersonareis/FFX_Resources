'use client';

import { cn } from 'cn';
import {
  ArrowDown,
  ArrowUp,
  CaseSensitive,
  ChevronDown,
  ChevronRight,
  WholeWord,
  X,
} from 'lucide-react';
import type { KeyboardEvent } from 'react';
import { useEffect, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import type { EntryFind } from './use-entry-find';

/**
 * Barra de busca & substituição local no arquivo aberto (estilo VSCode).
 * Fica logo ABAIXO da save bar, dentro do painel flex-col (shrink-0, fora do
 * rolante). A linha de substituir nasce colapsada (chevron).
 *
 * - digitar já salta e destaca o primeiro match (o hook recalcula e o
 *   activeIndex volta a 0); todos os matches ganham <mark> na tabela;
 * - Enter = próximo match, Shift+Enter = anterior, Esc = fecha e devolve o
 *   foco à linha destacada;
 * - "Substituir" troca o match ativo e segue para o próximo; "Substituir
 *   todos" troca tudo (com desfazer no toast).
 */
export function FindBar({ find }: { find: EntryFind }) {
  const findInputRef = useRef<HTMLInputElement>(null);
  const replaceInputRef = useRef<HTMLInputElement>(null);

  // Abre com o cursor no campo de busca (reabrir = refinar o termo).
  useEffect(() => {
    if (find.open) findInputRef.current?.focus();
  }, [find.open]);

  const onFindKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      if (event.shiftKey) find.prev();
      else find.next();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      find.closeFind();
    }
  };

  const onReplaceKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      find.replaceCurrent();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      find.closeFind();
    }
  };

  const counter =
    find.hasQuery && find.matches.length > 0
      ? `${find.activeIndex + 1}/${find.matches.length}`
      : find.hasQuery
        ? '0/0'
        : '';

  return (
    <div className="flex shrink-0 flex-col gap-1 border-b pb-2">
      {/* Linha 1: busca */}
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={find.showReplace ? 'Ocultar substituição' : 'Mostrar substituição'}
          title={find.showReplace ? 'Ocultar substituição' : 'Mostrar substituição'}
          onClick={find.toggleReplace}
        >
          {find.showReplace ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
        </Button>
        <Input
          ref={findInputRef}
          value={find.query}
          onChange={(event) => find.setQuery(event.target.value)}
          onKeyDown={onFindKeyDown}
          placeholder="Buscar neste arquivo…"
          aria-label="Buscar neste arquivo"
          autoComplete="off"
          className="h-8 w-full max-w-sm"
        />
        <span
          role="status"
          aria-live="polite"
          className={cn(
            'min-w-10 shrink-0 text-xs text-muted-foreground tabular-nums',
            find.hasQuery && find.matches.length === 0 && 'text-destructive',
          )}
        >
          {counter}
        </span>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Diferenciar maiúsculas e minúsculas"
          title="Diferenciar maiúsculas e minúsculas (Aa)"
          aria-pressed={find.matchCase}
          className={cn(find.matchCase && 'bg-muted text-foreground')}
          onClick={find.toggleMatchCase}
        >
          <CaseSensitive size={15} />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Casar palavra inteira"
          title="Casar palavra inteira"
          aria-pressed={find.wholeWord}
          className={cn(find.wholeWord && 'bg-muted text-foreground')}
          onClick={find.toggleWholeWord}
        >
          <WholeWord size={15} />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Match anterior"
          title="Match anterior (Shift+Enter)"
          disabled={find.matches.length === 0}
          onClick={find.prev}
        >
          <ArrowUp size={15} />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Próximo match"
          title="Próximo match (Enter)"
          disabled={find.matches.length === 0}
          onClick={find.next}
        >
          <ArrowDown size={15} />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Fechar a busca"
          title="Fechar (Esc)"
          onClick={find.closeFind}
        >
          <X size={15} />
        </Button>
      </div>

      {/* Linha 2: substituição (colapsável) */}
      {find.showReplace ? (
        <div className="flex items-center gap-1 pl-7">
          <Input
            ref={replaceInputRef}
            value={find.replaceText}
            onChange={(event) => find.setReplaceText(event.target.value)}
            onKeyDown={onReplaceKeyDown}
            placeholder="Substituir por…"
            aria-label="Texto de substituição"
            autoComplete="off"
            className="h-8 w-full max-w-sm"
          />
          <Button
            variant="outline"
            size="sm"
            disabled={find.matches.length === 0}
            onClick={find.replaceCurrent}
          >
            Substituir
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={find.matches.length === 0}
            onClick={() => void find.replaceAll()}
          >
            Substituir todos
          </Button>
        </div>
      ) : null}
    </div>
  );
}
