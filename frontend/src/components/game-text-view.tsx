'use client';

import { useMemo } from 'react';
import { formatGameTextForDisplay } from '@/lib/ffx/game-text-format';
import type { GameTextMark } from '@/lib/ffx/game-text-parser';

interface GameTextViewProps {
  /** Texto canônico com tags (ex: linha de TextRow.text[lang]). */
  text?: string;
  /** Conteúdo quando o texto está vazio (padrão: nada). */
  fallback?: string;
  className?: string;
  /**
   * Destaques da busca local (code points do texto, MESMA régua do find).
   * Vazio/ausente = HTML byte-idêntico ao de sempre (sem <mark>).
   */
  marks?: readonly GameTextMark[];
  /**
   * Só a TABELA: chip `LF` no fim de cada quebra real de linha — separa a
   * quebra do texto (jogo) do wrap de palavra feito pelo CSS.
   */
  newlineChip?: boolean;
}

/**
 * Exibição somente-leitura do texto do jogo, com a MESMA conversão do editor
 * ({TEXT_NEWLINE}/\n -> parágrafo, {CLR:…} -> cor, {TEXT_ITALIC} -> itálico,
 * demais tags -> chip). Evita mostrar tags cruas ao lado do texto normalizado.
 *
 * Casca fina sobre `formatGameTextForDisplay` (formatador compartilhado com o
 * modal de busca). O parser escapa todo texto/atributo, então o HTML é seguro
 * para dangerouslySetInnerHTML.
 */
export function GameTextView({
  text,
  fallback = '',
  className,
  marks,
  newlineChip,
}: GameTextViewProps) {
  const { paddingSummary, html } = useMemo(
    () => formatGameTextForDisplay(text, marks, { newlineChip }),
    [text, marks, newlineChip],
  );

  if (!text) {
    return fallback ? <div className={className}>{fallback}</div> : null;
  }

  return (
    <div className={className}>
      {paddingSummary !== '' && (
        <div className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
          <span>␍ padding de tela:</span>
          <code className="rounded bg-muted px-1">{paddingSummary}</code>
        </div>
      )}
      <div
        className={
          // Destaque da busca local: <mark> por fora (como no modal de
          // busca), find-current (cor primária) para o match ativo. Sem
          // marks o HTML não tem <mark>, então as regras ficam inertes.
          '[&_mark]:rounded-sm [&_mark]:bg-yellow-200 [&_mark]:px-0.5 dark:[&_mark]:bg-yellow-500/40 ' +
          '[&_mark.find-current]:bg-primary/30 dark:[&_mark.find-current]:bg-primary/40 ' +
          '[&_p]:mb-1 [&_p:last-child]:mb-0'
        }
        // biome-ignore lint/security/noDangerouslySetInnerHtml: HTML montado por formatGameTextForDisplay, que escapa texto e atributos.
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}
