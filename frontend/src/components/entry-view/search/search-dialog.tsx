'use client';

import { useSelector } from '@tanstack/react-store';
import { CornerDownLeft, FileText, Loader2, Search } from 'lucide-react';
import { type KeyboardEvent, startTransition, useEffect, useMemo, useRef, useState } from 'react';
import type * as services from '@/bindings/ffxresources/backend/services/models';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { buildNodeIndex } from '@/lib/ffx/content-tree/model';
import {
  buildSearchItems,
  type SearchListItem,
  searchTreeNames,
} from '@/lib/ffx/content-tree/search';
import { KIND_LABELS } from '@/lib/ffx/display-names';
import { formatGameTextSnippetForSearch } from '@/lib/ffx/game-text-format';
import {
  closeSearchDialog,
  runTextSearch,
  SEARCH_DEBOUNCE_MS,
  SEARCH_MIN_QUERY_RUNES,
  searchStoreFor,
} from '@/lib/ffx/search-store';
import type { EntryView } from '../entry-view-store';

/**
 * Modal de busca (estilo DocSearch) — o ÚNICO lugar com lógica de busca.
 *
 * - rascunho local com debounce (600ms renovado a cada tecla); Enter e o
 *   botão da lupa publicam na hora; Enter sem seleção vai ao primeiro item;
 * - resultados em hierarquia: ARQUIVO (cabeçalho, clique leva ao arquivo com
 *   a primeira row focada) e as rows com match TABULADAS abaixo (clique leva
 *   exatamente àquela linha na tabela, via pendingTableRowFocus);
 * - termo e resultados ficam no store por versão: reabrir o modal mostra
 *   tudo como ficou, inclusive ao trocar de aba e voltar.
 */
export function SearchDialog({ view }: { view: EntryView }) {
  const { version, store, actions } = view;
  const search = useSelector(searchStoreFor(version), (state) => state);
  const roots = useSelector(store, (state) => state.roots);
  const [draft, setDraft] = useState(search.query);
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // O backend devolve só kind/id de cada arquivo: o RÓTULO real (e a árvore
  // onde o reveal vai acontecer) sai do índice de nós já carregado.
  const nodeById = useMemo(() => buildNodeIndex(roots, []), [roots]);
  const nameMatches = useMemo(() => searchTreeNames(roots, search.query), [roots, search.query]);
  const items = useMemo(
    () =>
      buildSearchItems(search.results, nameMatches).map((item) => ({
        ...item,
        label: nodeById.get(item.leafId)?.label ?? item.label,
      })),
    [search.results, nameMatches, nodeById],
  );

  // Debounce: só consulta o backend quando a digitação PARA por 600ms.
  // Igualar o termo já executado curto-circuita (reabrir não re-busca), mas
  // um termo "stale" (base mudou: save/import/reload descartou os resultados)
  // re-executa sozinho — o modal nunca fica mostrando foto velha.
  useEffect(() => {
    const trimmed = draft.trim();
    const stale = [...trimmed].length >= SEARCH_MIN_QUERY_RUNES && search.status === 'idle';
    if (trimmed === search.query.trim() && !stale) return;
    const timeout = setTimeout(() => {
      void runTextSearch(version, draft);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timeout);
  }, [draft, search.query, search.status, version]);

  useEffect(() => {
    setActiveIndex(0);
  }, []);

  // A seta move a seleção; o item ativo rola para a vista.
  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-search-index="${activeIndex}"]`)
      ?.scrollIntoView({ block: 'nearest' });
  }, [activeIndex]);

  const activate = (item: SearchListItem | undefined) => {
    if (!item) return;
    closeSearchDialog(version);
    // Navegação pesada (reveal na árvore + carga da entrada + render da
    // tabela) como transição NÃO urgente: a próxima tecla digitada não
    // espera a renderização terminar.
    startTransition(() => {
      void actions.openSearchResult(item.leafId, item.match);
    });
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActiveIndex((index) => Math.max(0, Math.min(index + 1, items.length - 1)));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActiveIndex((index) => Math.max(index - 1, 0));
    } else if (event.key === 'Enter') {
      event.preventDefault();
      // Flush imediato: Enter não espera o debounce. Se o rascunho ainda não
      // virou consulta, busca AGORA (sem navegar num resultado velho); se já
      // está tudo atual, vai ao item ativo (ou re-busca com lista vazia).
      if (draft.trim() !== search.query.trim() || items.length === 0) {
        void runTextSearch(version, draft);
        return;
      }
      activate(items[activeIndex]);
    }
  };

  let status: string;
  if (search.status === 'pending') {
    status = 'Buscando conteúdo…';
  } else if (items.length === 0) {
    status = search.query.trim()
      ? 'Nenhum resultado.'
      : 'Digite para buscar (conteúdo a partir de 2 caracteres).';
  } else if (search.results.length > 0) {
    const totals = `${search.totalRows} rows em ${search.totalFiles} arquivo(s)`;
    status = search.truncated
      ? `${totals} · mostrando os primeiros ${search.results.length}`
      : totals;
  } else {
    status = `${nameMatches.length} arquivo(s) por nome`;
  }

  return (
    <Dialog
      open={search.open}
      onOpenChange={(open) => {
        if (!open) closeSearchDialog(version);
      }}
    >
      <DialogContent
        className="top-[15%] translate-y-0 gap-0 p-0 sm:max-w-2xl"
        showCloseButton={false}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          inputRef.current?.focus();
        }}
      >
        <DialogTitle className="sr-only">Buscar nome ou texto</DialogTitle>
        <div className="flex items-center gap-2 border-b px-3">
          <Search size={16} className="shrink-0 text-muted-foreground" />
          <Input
            ref={inputRef}
            value={draft}
            onChange={(event) => {
              // Só o rascunho LOCAL: quem escreve `query` no store é o
              // runTextSearch (query = último termo EXECUTADO). Sincronizar
              // aqui mataria o guard do debounce — draft === query sempre.
              setDraft(event.target.value);
            }}
            onKeyDown={onKeyDown}
            placeholder="Buscar nome de arquivo ou texto (us)…"
            aria-label="Buscar nome de arquivo ou texto em inglês/tradução"
            aria-controls="search-results"
            autoComplete="off"
            className="h-12 min-w-0 flex-1 border-0 px-0 shadow-none focus-visible:ring-0 md:text-sm dark:bg-input/0"
          />
          {search.status === 'pending' ? (
            <Loader2
              size={16}
              className="shrink-0 animate-spin text-muted-foreground"
              aria-label="Buscando conteúdo"
            />
          ) : null}
          {/* A lupa publica o termo na hora (fora do debounce): com rascunho
              pendente ela busca na hora; atualizada, abre o item ativo. */}
          <Button
            variant="ghost"
            size="icon"
            aria-label="Buscar agora ou abrir o resultado selecionado"
            onClick={() => {
              if (draft.trim() !== search.query.trim() || items.length === 0) {
                void runTextSearch(version, draft);
                return;
              }
              activate(items[activeIndex]);
            }}
          >
            <CornerDownLeft size={16} />
          </Button>
        </div>
        <div role="status" aria-live="polite" className="px-3 py-1.5 text-xs text-muted-foreground">
          {status}
        </div>
        <div
          id="search-results"
          ref={listRef}
          role="listbox"
          aria-label="Resultados da busca"
          className="max-h-[55vh] min-h-0 overflow-y-auto px-1 pb-2"
        >
          {items.map((item, index) => (
            <div
              key={item.key}
              role="option"
              aria-selected={index === activeIndex}
              tabIndex={-1}
              data-search-index={index}
              onMouseEnter={() => setActiveIndex(index)}
              onClick={() => activate(item)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  activate(item);
                }
              }}
              className={`flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left ${
                item.fileHeader ? 'mt-1 font-medium' : 'pl-8'
              } ${index === activeIndex ? 'bg-accent' : 'hover:bg-accent/50'}`}
            >
              {item.fileHeader ? (
                <>
                  <FileText size={16} className="shrink-0 opacity-70" />
                  <Badge variant="outline" className="shrink-0">
                    {KIND_LABELS[item.kind]}
                  </Badge>
                  <span className="min-w-0 flex-1 truncate">{item.label}</span>
                  {item.rowCount > 0 ? (
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {item.rowCount} row(s)
                    </span>
                  ) : null}
                </>
              ) : (
                <>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    #{item.match?.index}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {item.match ? <SnippetText match={item.match} /> : null}
                  </span>
                  {item.match?.data ? (
                    <Badge variant="secondary" className="shrink-0 text-[10px]">
                      Original
                    </Badge>
                  ) : null}
                  {item.match?.mods ? (
                    <Badge variant="secondary" className="shrink-0 text-[10px]">
                      Traduzido
                    </Badge>
                  ) : null}
                </>
              )}
              {index === activeIndex ? (
                <CornerDownLeft size={14} className="shrink-0 text-muted-foreground" />
              ) : null}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Snippet formatado com a MESMA conversão da tabela/editor (tags {…} viram
 * cor/itálico/chip); o hit ganha <mark> por dentro das marcas. A remontagem
 * é lossless — o parser sempre vê a janela original.
 */
function SnippetText({ match }: { match: services.TextSearchMatch }) {
  const html = formatGameTextSnippetForSearch({
    before: match.snippetBefore ?? '',
    hit: match.snippetHit ?? '',
    after: match.snippetAfter ?? '',
  });
  if (!html) {
    return <span className="italic opacity-70">(sem texto exibível)</span>;
  }
  // O parser escapa todo texto; <mark> é nosso. Estilos do destaque e dos
  // parágrafos ficam no CONTAINER para o snippet caber numa linha da lista.
  return (
    <span
      className="block min-w-0 truncate [&_mark]:rounded-sm [&_mark]:bg-yellow-200 [&_mark]:px-0.5 [&_p]:inline [&_p]:m-0 [&_p+p]:pl-1 dark:[&_mark]:bg-yellow-500/40"
      // biome-ignore lint/security/noDangerouslySetInnerHtml: HTML montado por formatGameTextSnippetForSearch, que escapa texto e atributos.
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
