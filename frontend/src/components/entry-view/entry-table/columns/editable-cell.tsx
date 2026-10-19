import { RotateCcw } from 'lucide-react';
import { GameTextView } from '@/components/game-text-view';
import type { GameTextMark } from '@/lib/ffx/game-text-parser';

/**
 * Célula traduzida comum: texto editável (clique abre o editor) e o botão de
 * reverter para o original quando a linha divergiu.
 */
export function EditableCell({
  displayed,
  edited,
  canRevert,
  marks,
  onOpen,
  onRevert,
}: {
  displayed: string;
  edited: boolean;
  canRevert: boolean;
  /** Destaques da busca local (intervalos em <mark> no texto exibido). */
  marks?: readonly GameTextMark[];
  onOpen: (() => void) | undefined;
  onRevert: () => void;
}) {
  return (
    <div className="group flex items-center gap-1">
      <div
        className={`text-cell translated-cell min-w-0 px-2 flex-1${edited || canRevert ? ' edited' : ''}`}
        onClick={onOpen}
      >
        <GameTextView text={displayed} marks={marks} newlineChip />
      </div>
      {canRevert ? (
        <button
          type="button"
          aria-label="Reverter esta linha para o original"
          title="Reverter esta linha para o original (se for cópia, volta a convergir com a definição)"
          className="shrink-0 rounded p-1 text-muted-foreground opacity-60 transition-opacity hover:bg-muted hover:text-foreground hover:opacity-100 focus:opacity-100 group-hover:opacity-100"
          onClick={(event) => {
            event.stopPropagation();
            onRevert();
          }}
        >
          <RotateCcw size={15} aria-hidden />
        </button>
      ) : null}
    </div>
  );
}
