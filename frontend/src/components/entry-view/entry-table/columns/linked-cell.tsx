import { GitBranch } from 'lucide-react';
import { GameTextView } from '@/components/game-text-view';
import { LINKED_SIDE } from '@/lib/ffx/hash-ref';
import type { GameTextMark } from '@/lib/ffx/game-text-parser';

/**
 * Célula de linha LINKADA (ref dedupada): o texto da def em cor própria, com
 * o original dela no tooltip. O clique abre o editor NA DEF — a tradução é
 * registrada no arquivo da def, nunca aqui.
 */
export function LinkedCell({
  defText,
  original,
  defTranslated,
  readOnly,
  marks,
  onOpen,
  onEditAsDivergent,
}: {
  defText: string;
  original: string;
  defTranslated: boolean;
  readOnly: boolean;
  /** Destaques da busca local (intervalos em <mark> no texto exibido). */
  marks?: readonly GameTextMark[];
  onOpen: (() => void) | undefined;
  onEditAsDivergent: () => void;
}) {
  return (
    <div className="group flex items-center gap-1">
      <div
        className={`text-cell translated-cell px-2 min-w-0 flex-1${defTranslated ? ' edited' : ''}`}
        title={
          defTranslated
            ? `Repetição de: ${original}`
            : 'Repetição (dedup): o texto vive na def de outro ponto'
        }
        onClick={onOpen}
      >
        <GameTextView text={defText} fallback="—" className={LINKED_SIDE} marks={marks} newlineChip />
      </div>
      {!readOnly ? (
        <button
          type="button"
          aria-label="Editar como divergência nesta cópia"
          title="Editar como divergência nesta cópia"
          className="shrink-0 rounded p-1 text-muted-foreground opacity-60 transition-opacity hover:bg-muted hover:text-foreground hover:opacity-100 focus:opacity-100 group-hover:opacity-100"
          onClick={(event) => {
            event.stopPropagation();
            onEditAsDivergent();
          }}
        >
          <GitBranch size={15} aria-hidden />
        </button>
      ) : null}
    </div>
  );
}
