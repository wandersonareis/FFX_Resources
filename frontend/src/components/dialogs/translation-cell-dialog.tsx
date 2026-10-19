'use client';

import { useState } from 'react';
import { useHotkey } from '@tanstack/react-hotkeys';
import { ChevronLeft, ChevronRight, ShieldAlert } from 'lucide-react';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { SOURCE_LANG } from '@/lib/ffx/save-all';
import {
  missingProtectedTags,
  newUnclosedFragment,
  removeUnclosedFragment,
  restoreProtectedTags,
} from '@/lib/ffx/protected-tags';
import type { GameVersionId } from '@/lib/ffx/game-version';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { GameTextEditor } from '@/components/editor/game-text-editor';
import { getActiveTagSuggestionPopup } from '@/components/editor/tag-suggestion-popup';
import { GameTextView } from '@/components/game-text-view';

export interface TranslationCellDialogProps {
  open: boolean;
  row: dto.TextRow | null;
  /** Versão do jogo (catálogo de tags do editor). */
  version: GameVersionId;
  /** Idiomas de consulta, sem o idioma de origem (us). */
  languages: Array<{ code: string; name: string }>;
  /** Há linha anterior no ARQUIVO atual (navegação nunca troca de arquivo). */
  hasPrevious: boolean;
  /** Há próxima linha no ARQUIVO atual. */
  hasNext: boolean;
  /** Modal aberto sobre uma ref azul; permite trocar o destino para a cópia. */
  allowDivergence: boolean;
  /** Checkbox inicia marcado quando aberto pelo botão de divergência. */
  initialDivergent: boolean;
  /**
   * Navegação sem fechar o modal. value só vem preenchido quando há edição
   * não salva (aplicada como rascunho antes de trocar de linha).
   */
  onNavigate: (direction: 'prev' | 'next', value?: string) => void;
  /** Fechamento: value != undefined aplica a edição. */
  onClosed: (value?: string, divergent?: boolean) => void;
}

/** Ação interrompida pelo gate (retomada ao restaurar/voltar). */
type PendingAction =
  | { kind: 'save' }
  | { kind: 'navigate'; direction: 'prev' | 'next' };

type GateState =
  | { kind: 'unclosed'; fragment: string; action: PendingAction }
  | { kind: 'protected'; missing: string[]; action: PendingAction };

export function TranslationCellDialog({
  open,
  row,
  version,
  languages,
  hasPrevious,
  hasNext,
  allowDivergence,
  initialDivergent,
  onNavigate,
  onClosed,
}: TranslationCellDialogProps) {
  const [text, setText] = useState(row?.text?.[SOURCE_LANG] ?? '');
  const [prevRow, setPrevRow] = useState(row);
  const [divergent, setDivergent] = useState(initialDivergent);
  const [gate, setGate] = useState<GateState | null>(null);
  if (prevRow !== row) {
    setPrevRow(row);
    setText(row?.text?.[SOURCE_LANG] ?? '');
    setDivergent(initialDivergent);
    setGate(null);
  }

  // Base da edição (o que está salvo no binário atual = row.text). Não é o
  // original pristine: os gates/`changed` comparam contra o salvo, para o
  // rascunho saber o que mudou. O original de data/ está em row.original.
  const baseText = row?.text?.[SOURCE_LANG] ?? '';
  const changed = text !== baseText;
  const referenceLangs = languages.filter((l) => l.code !== SOURCE_LANG);

  const perform = (action: PendingAction, value: string): void => {
    // value === base não gera rascunho (nada mudou em relação ao salvo).
    if (action.kind === 'save') {
      onClosed(value !== baseText ? value : undefined, divergent);
      return;
    }
    onNavigate(action.direction, value !== baseText ? value : undefined);
  };

  /**
   * Gate: edição que removeu tag protegida (ou abriu `{…`) não segue sem o
   * usuário resolver — o texto ainda não foi gravado em lugar nenhum.
   * `value` é explícito para a reavaliação do "Remover trecho" (o estado
   * `text` ainda não foi processado pelo React na hora da chamada).
   */
  const attempt = (action: PendingAction, value: string = text): void => {
    if (value !== baseText) {
      const fragment = newUnclosedFragment(baseText, value);
      if (fragment) {
        setGate({ kind: 'unclosed', fragment, action });
        return;
      }
      const missing = missingProtectedTags(baseText, value);
      if (missing.length > 0) {
        setGate({ kind: 'protected', missing, action });
        return;
      }
    }
    perform(action, value);
  };

  const restoreAndContinue = (): void => {
    if (!gate || gate.kind !== 'protected') return;
    const restored = restoreProtectedTags(baseText, text);
    const action = gate.action;
    setGate(null);
    setText(restored);
    perform(action, restored);
  };

  /** Gate do `{…` aberto: corta o sufixo e reavalia a ação pendente. */
  const fixUnclosedAndContinue = (): void => {
    if (!gate || gate.kind !== 'unclosed') return;
    const action = gate.action;
    const fixed = removeUnclosedFragment(text, baseText);
    setGate(null);
    setText(fixed);
    attempt(action, fixed);
  };

  // Navegação por teclado: ← Anterior / → Próxima. Registrada no document
  // (sem escopo por ref: a ref do Radix muda depois do mount e o listener
  // não re-anexa). Seguro porque o Dialog prende o foco dentro do conteúdo
  // e o ignoreInputs deixa as setas moverem o caret dentro do editor.
  useHotkey('ArrowLeft', () => attempt({ kind: 'navigate', direction: 'prev' }), {
    enabled: open && hasPrevious,
    ignoreInputs: true,
  });
  useHotkey('ArrowRight', () => attempt({ kind: 'navigate', direction: 'next' }), {
    enabled: open && hasNext,
    ignoreInputs: true,
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) onClosed();
      }}
    >
      <DialogContent
        // Altura DEFINIDA (não só max-h): sem ela os flex-1 de dentro não
        // têm espaço livre e a caixa de edição fica colada na altura do
        // texto. O teto continua 90vh; 46rem evita caixa gigante em tela
        // larga/curta.
        className="sm:max-w-180 h-[min(90vh,46rem)] flex flex-col pt-10"
        // O popup do autocomplete agora monta DENTRO deste conteúdo (container
        // da extensão); cliques/Escape nele são internos ao Radix. A guarda
        // abaixo fica como rede de segurança para qualquer elemento que nem
        // sempre esteja sob este conteúdo.
        onInteractOutside={(event) => {
          const popup = getActiveTagSuggestionPopup();
          if (!popup) return;
          const original = event.detail?.originalEvent;
          const target = original?.target ?? event.target;
          if (target instanceof Node && popup.element.contains(target)) {
            event.preventDefault();
          }
        }}
        onEscapeKeyDown={(event) => {
          if (getActiveTagSuggestionPopup()) event.preventDefault();
        }}
      >
        {/* Linha do título + navegação, abaixo do botão X de fechar */}
        <div className="flex items-center justify-between gap-2">
          <DialogTitle className="truncate">
            Linha #{row?.index}
            {row?.name ? <span className="opacity-70"> · {row.name}</span> : null}
          </DialogTitle>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button
              variant="outline"
              size="sm"
              disabled={!hasPrevious}
              onClick={() => attempt({ kind: 'navigate', direction: 'prev' })}
            >
              <ChevronLeft size={16} />
              Anterior
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!hasNext}
              onClick={() => attempt({ kind: 'navigate', direction: 'next' })}
            >
              Próxima
              <ChevronRight size={16} />
            </Button>
          </div>
        </div>
        <DialogDescription className="mt-1">
          Este editor salva apenas o rascunho; o botão Salvar do app grava no binário.
        </DialogDescription>

        {allowDivergence ? (
          <label className="flex items-start gap-2 rounded-md border px-3 py-2 text-sm">
            <input
              type="checkbox"
              checked={divergent}
              onChange={(event) => setDivergent(event.currentTarget.checked)}
              className="mt-0.5 accent-primary"
            />
            <span>
              <span className="font-medium">Salvar como divergência nesta cópia</span>
              <span className="block text-xs text-muted-foreground">
                A edição fica só nesta cópia; as demais continuam ligadas à definição.
              </span>
            </span>
          </label>
        ) : null}

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden">
          {/* Referências: NÃO competem por altura com a caixa de edição
              (sem flex-1, teto menor e min-h-0 = encolhe primeiro quando
              falta espaço — a caixa de edição tem piso próprio e nunca é
              espremida para fora do wrapper). */}
          <div className="grid gap-2.5 overflow-y-auto min-h-0 max-h-[26vh] shrink pr-1">
            {referenceLangs.length === 0 ? (
              <p className="opacity-70">Sem outros idiomas nesta linha.</p>
            ) : (
              referenceLangs.map((lang) => (
                <div key={lang.code}>
                  <span className="text-xs font-semibold opacity-75">
                    {lang.name} ({lang.code})
                  </span>
                  <GameTextView
                    className="mt-0.5"
                    text={row?.original?.[lang.code] ?? ''}
                    fallback="—"
                  />
                </div>
              ))
            )}
          </div>

          {/* Piso da edição (label + toolbar + caixa min-h-40), com teto
              relativo à viewport p/ janela baixa: com piso explícito o flex
              encolhe as REFERÊNCIAS antes, e a caixa clicável nunca sai
              cortada pelo overflow-hidden do wrapper. */}
          <div className="flex min-h-[min(18rem,45vh)] flex-1 flex-col gap-1.5">
            <span className="text-xs font-semibold opacity-75">
              Tradução ({SOURCE_LANG}) — rascunho até salvar em binário
            </span>
            <GameTextEditor
              value={text}
              onValueChange={(next) => {
                setText(next);
                setGate(null);
              }}
              version={version}
            />
          </div>

          {gate ? (
            <div
              className="shrink-0 rounded-lg border border-destructive/45 bg-destructive/10 p-3"
              role="alert"
            >
              <p className="flex items-center gap-2 text-sm font-semibold">
                <ShieldAlert size={16} aria-hidden />
                {gate.kind === 'protected'
                  ? `${gate.missing.length} tag(ns) protegida(s) removida(s)`
                  : 'Trecho de tag aberto'}
              </p>
              <p className="mt-1 text-xs opacity-80">
                {gate.kind === 'protected'
                  ? 'Tags calculadas pelo jogo não podem ser recriadas pela tradução. Restaure para continuar.'
                  : `O trecho "${gate.fragment}" não foi fechado com }. Remova-o ou complete a tag para continuar.`}
              </p>
              {gate.kind === 'protected' ? (
                <ul className="mt-2 flex max-h-24 flex-col gap-0.5 overflow-y-auto font-mono text-xs">
                  {gate.missing.slice(0, 20).map((tag, index) => (
                    <li key={`${tag}-${index}`}>{tag}</li>
                  ))}
                  {gate.missing.length > 20 ? (
                    <li className="opacity-70">
                      … +{gate.missing.length - 20}
                    </li>
                  ) : null}
                </ul>
              ) : null}
              <div className="mt-3 flex justify-end gap-2">
                <Button variant="ghost" size="sm" onClick={() => setGate(null)}>
                  Voltar
                </Button>
                {gate.kind === 'protected' ? (
                  <Button size="sm" onClick={restoreAndContinue}>
                    Restaurar
                  </Button>
                ) : (
                  <Button size="sm" onClick={fixUnclosedAndContinue}>
                    Remover trecho
                  </Button>
                )}
              </div>
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <span className="mr-auto flex items-center gap-1 text-xs text-muted-foreground sm:mr-auto">
            <kbd className="rounded border px-1.5 py-0.5 font-mono">←</kbd>
            <kbd className="rounded border px-1.5 py-0.5 font-mono">→</kbd>
            entre linhas
          </span>
          <Button variant="ghost" onClick={() => onClosed()}>
            Cancelar
          </Button>
          <Button disabled={!changed} onClick={() => attempt({ kind: 'save' })}>
            Salvar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
