'use client';

import { Editor } from '@tiptap/core';
import { Color } from '@tiptap/extension-color';
import { TextStyle } from '@tiptap/extension-text-style';
import StarterKit from '@tiptap/starter-kit';
import {
  ChevronDown,
  DoorOpen,
  Eraser,
  Italic,
  Palette,
  Redo2,
  RotateCcw,
  Undo2,
} from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Separator } from '@/components/ui/separator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { gameTextParser } from '@/lib/ffx/game-text-parser';
import { extractChipLabel, isLockedTagInner, KNOWN_COLORS } from '@/lib/ffx/game-text-tags';
import type { GameVersionId } from '@/lib/ffx/game-version';
import { splitLeadingPadding, summarizePadding } from '@/lib/ffx/leading-padding';
import { getChipValidator, loadTagCatalog, type TagCatalog } from '@/lib/ffx/tag-catalog';
import { CleanMarksExtension } from './clean-marks.extension';
import { createDoubleClickSelectionExtension } from './double-click-selection.extension';
import { GameTagExtension } from './game-tag.extension';
import { LockedTagsExtension } from './locked-tags.extension';
import { createPasteHandlerExtension } from './paste-handler.extension';
import { createTagAutocompleteExtension } from './tag-autocomplete.extension';
import { createTypedTagExtension } from './typed-tag.extension';

interface TemplateItem {
  /** Tag completa, ex: {BUTTON:31:X}. */
  value: string;
  /** Classes CSS dos tiles na sprite public/pad_icon.png (sequência). */
  cls: string[];
}

/**
 * buttonMap completo exposto ao menu Botões — espelho do Go
 * (backend/core/encoding/encoding_players.go) na arte do Xbox do jogo:
 * 30 TRIANGLE→Y, 31 X→A, 32 CIRCLE→B, 33 SQUARE→X.
 * Dummy/Dummy2 (0x2D, 0x2E) ficam de fora: são dummies legados sem glifo
 * na sprite, e o chip deles continua textual.
 */
const BUTTON_ITEMS: TemplateItem[] = [
  { value: '{BUTTON:30:TRIANGLE}', cls: ['gb-y'] },
  { value: '{BUTTON:31:X}', cls: ['gb-a'] },
  { value: '{BUTTON:32:CIRCLE}', cls: ['gb-b'] },
  { value: '{BUTTON:33:SQUARE}', cls: ['gb-x'] },
  { value: '{BUTTON:34:L1}', cls: ['gb-lb'] },
  { value: '{BUTTON:35:R1}', cls: ['gb-rb'] },
  { value: '{BUTTON:36:L2}', cls: ['gb-lt'] },
  { value: '{BUTTON:37:R2}', cls: ['gb-rt'] },
  { value: '{BUTTON:38:START}', cls: ['gb-start'] },
  { value: '{BUTTON:39:SELECT}', cls: ['gb-back'] },
];

/**
 * buttonMap dos direcionais (0x40–0x4F) completo — sequências de setas na
 * ordem do nome do código (como o jogo renderiza). "Direcional" (0x40) e
 * "All" (0x4F) usam o cursor ✛ do pad.
 */
const DPAD_ITEMS: TemplateItem[] = [
  { value: '{BUTTON:40:Direcional}', cls: ['gb-cursor'] },
  { value: '{BUTTON:41:Direcional UP}', cls: ['gb-arrow-up'] },
  { value: '{BUTTON:42:Direcional RIGHT}', cls: ['gb-arrow-right'] },
  { value: '{BUTTON:43:Direcional Up+Right}', cls: ['gb-arrow-up', 'gb-arrow-right'] },
  { value: '{BUTTON:44:Direcional DOWN}', cls: ['gb-arrow-down'] },
  { value: '{BUTTON:45:Direcional Up+Down}', cls: ['gb-arrow-up', 'gb-arrow-down'] },
  { value: '{BUTTON:46:Direcional Down+Right}', cls: ['gb-arrow-down', 'gb-arrow-right'] },
  {
    value: '{BUTTON:47:Direcional Up+Right+Down}',
    cls: ['gb-arrow-up', 'gb-arrow-right', 'gb-arrow-down'],
  },
  { value: '{BUTTON:48:Direcional LEFT}', cls: ['gb-arrow-left'] },
  { value: '{BUTTON:49:Direcional Up+Left}', cls: ['gb-arrow-up', 'gb-arrow-left'] },
  { value: '{BUTTON:4A:Direcional Left+Right}', cls: ['gb-arrow-left', 'gb-arrow-right'] },
  {
    value: '{BUTTON:4B:Direcional Up+Left+Right}',
    cls: ['gb-arrow-up', 'gb-arrow-left', 'gb-arrow-right'],
  },
  { value: '{BUTTON:4C:Direcional Left+Down}', cls: ['gb-arrow-left', 'gb-arrow-down'] },
  {
    value: '{BUTTON:4D:Direcional Up+Left+Down}',
    cls: ['gb-arrow-up', 'gb-arrow-left', 'gb-arrow-down'],
  },
  {
    value: '{BUTTON:4E:Direcional Left+Down+Right}',
    cls: ['gb-arrow-left', 'gb-arrow-down', 'gb-arrow-right'],
  },
  { value: '{BUTTON:4F:Direcional All}', cls: ['gb-cursor'] },
];

interface IconTemplate {
  label: string;
  value: string;
}

const ICON_TEMPLATES: IconTemplate[] = [
  { label: 'Red Gate', value: '{ICON:80:Red Gate}' },
  { label: 'Green Gate', value: '{ICON:81:Green Gate}' },
  { label: 'Yellow Gate', value: '{ICON:82:Yellow Gate}' },
  { label: 'Blue Gate', value: '{ICON:83:Blue Gate}' },
];

export interface GameTextEditorProps {
  /** Texto canônico com tags (ex: linha de TextRow.text[lang]). */
  value: string;
  /** Emite o texto canônico serializado a cada edição. */
  onValueChange: (value: string) => void;
  /** Versão do jogo: define o catálogo de tags (autocomplete/validação). */
  version: GameVersionId;
}

export function GameTextEditor({ value, onValueChange, version }: GameTextEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  /** Wrapper do editor: container de montagem do popup de sugestões. */
  const popupContainerRef = useRef<HTMLDivElement | null>(null);
  const suppressUpdate = useRef(false);
  const onValueChangeRef = useRef(onValueChange);
  /**
   * Último texto que o PRÓPRIO editor emitiu por onValueChange. O efeito de
   * sincronização só reconstrói o conteúdo quando o `value` veio de fora —
   * comparar HTML literalmente (getHTML × parser) fazia setContent a cada
   * edição em casos de marca vazia/cor, derrubando cursor e seleção.
   */
  const lastEmittedValue = useRef(value);
  /** `value` atual sem depender do closure da criação do editor. */
  const valueRef = useRef(value);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [, setTick] = useState(0);
  // undefined = catálogo ainda carregando (o editor só nasce com ele pronto:
  // parse, colagem e tags digitadas precisam do MESMO validador).
  const [catalog, setCatalog] = useState<TagCatalog | null | undefined>(undefined);

  useEffect(() => {
    onValueChangeRef.current = onValueChange;
  }, [onValueChange]);

  useEffect(() => {
    valueRef.current = value;
  }, [value]);

  useEffect(() => {
    let alive = true;
    loadTagCatalog(version).then((loaded) => {
      if (alive) setCatalog(loaded);
    });
    return () => {
      alive = false;
    };
  }, [version]);

  useEffect(() => {
    const element = hostRef.current;
    if (catalog === undefined || !element) return;
    const validate = getChipValidator(catalog);
    const ed = new Editor({
      element,
      extensions: [
        StarterKit.configure({
          heading: false,
          bulletList: false,
          orderedList: false,
          listItem: false,
          blockquote: false,
          codeBlock: false,
          horizontalRule: false,
          bold: false,
          strike: false,
        }),
        TextStyle,
        Color,
        GameTagExtension,
        LockedTagsExtension,
        CleanMarksExtension,
        createDoubleClickSelectionExtension(),
        createTypedTagExtension(validate),
        createTagAutocompleteExtension(catalog, () => popupContainerRef.current),
        createPasteHandlerExtension(gameTextParser, validate),
      ],
      content: gameTextParser.parseGameTextToHTML(splitLeadingPadding(value).body, validate),
      onUpdate: ({ editor: updated }) => {
        suppressUpdate.current = true;
        try {
          // Padding do valor ATUAL (o editor vive além da linha aberta).
          const { padding } = splitLeadingPadding(valueRef.current);
          const next = padding + gameTextParser.serializeJSONToGameText(updated.getJSON());
          lastEmittedValue.current = next;
          onValueChangeRef.current(next);
        } finally {
          queueMicrotask(() => (suppressUpdate.current = false));
        }
      },
      onTransaction: () => setTick((t) => t + 1),
    });
    lastEmittedValue.current = value;
    setEditor(ed);
    return () => {
      ed.destroy();
      setEditor(null);
    };
    // O editor nasce com o catálogo da versão e nunca é recriado depois.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [catalog, value]);

  useEffect(() => {
    if (!editor || suppressUpdate.current || catalog === undefined) return;
    // Mesma string que o editor acabou de emitir = eco do onValueChange; não
    // reconstrói o documento (preserva cursor/seleção).
    if (value === lastEmittedValue.current) return;
    const html = gameTextParser.parseGameTextToHTML(
      splitLeadingPadding(value).body,
      getChipValidator(catalog),
    );
    editor.commands.setContent(html, { emitUpdate: false });
    lastEmittedValue.current = value;
  }, [editor, value, catalog]);

  const insertControlTag = (template: TemplateItem | IconTemplate) => {
    if (!editor) return;
    const inner = template.value.slice(1, -1);
    editor
      .chain()
      .focus()
      .insertContent({
        type: 'gameTag',
        attrs: {
          value: template.value,
          label: extractChipLabel(inner),
          locked: isLockedTagInner(inner),
          // Mesmo critério do parse: um template que não round-tripa
          // aparece marcado, não "ok" na tela e errado no binário.
          invalid: !getChipValidator(catalog ?? null)(inner),
        },
      })
      .run();
  };

  const colors = Object.values(KNOWN_COLORS).filter((c) => c.name !== 'WHITE');

  return (
    <div ref={popupContainerRef} className="relative flex min-h-0 flex-1 flex-col gap-2">
      <div
        className="flex shrink-0 flex-wrap items-center justify-between gap-1 rounded border border-border px-2 py-1"
        role="toolbar"
        aria-label="Formatação do texto do jogo"
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="data-[active=true]:bg-accent data-[active=true]:text-accent-foreground"
              data-active={editor?.isActive('italic') ?? false}
              onClick={() => editor?.chain().focus().toggleItalic().run()}
              aria-label="Itálico"
            >
              <Italic size={20} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Itálico ({'{TEXT_ITALIC}'})</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => editor?.chain().focus().unsetItalic().run()}
            >
              <Eraser size={20} />
              Normal
            </Button>
          </TooltipTrigger>
          <TooltipContent>Texto normal ({'{TEXT_NORMAL}'})</TooltipContent>
        </Tooltip>

        <Separator orientation="vertical" className="my-1 w-px" />

        <SpriteTemplateDropdown
          ariaLabel="Botões ({BUTTON:…})"
          tooltip="Inserir botão ({BUTTON:…})"
          items={BUTTON_ITEMS}
          triggerCls={['gb-a']}
          onSelect={insertControlTag}
        />
        <SpriteTemplateDropdown
          ariaLabel="Direcionais ({BUTTON:…})"
          tooltip="Inserir direcional ({BUTTON:…})"
          items={DPAD_ITEMS}
          triggerCls={['gb-cursor']}
          onSelect={insertControlTag}
        />
        <TemplateDropdown
          ariaLabel="Ícones ({ICON:…})"
          tooltip="Inserir ícone ({ICON:…})"
          icon={<DoorOpen size={20} />}
          templates={ICON_TEMPLATES}
          onSelect={insertControlTag}
        />

        <Separator orientation="vertical" className="my-1 w-px" />

        <fieldset
          className="m-0 inline-flex items-center gap-0.5 border-0 p-0"
          aria-label="Cores ({CLR:…})"
        >
          <Palette size={20} aria-label="Cores ({CLR:…}, {CLR:WHITE} reseta)" />
          {colors.map((c) => {
            const isActive = editor?.isActive('textStyle', { color: c.hex }) ?? false;
            return (
              <Tooltip key={c.name}>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className="h-5.5 w-5.5 cursor-pointer rounded border-2 p-0"
                    style={{
                      backgroundColor: c.hex,
                      borderColor: isActive ? 'currentColor' : 'transparent',
                      boxShadow: isActive ? '0 0 0 1px currentColor' : 'none',
                    }}
                    aria-label={`Cor ${c.label}`}
                    onClick={() => editor?.chain().focus().setColor(c.hex).run()}
                  />
                </TooltipTrigger>
                <TooltipContent>{`{CLR:${c.name}}`}</TooltipContent>
              </Tooltip>
            );
          })}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Resetar cor"
                onClick={() => editor?.chain().focus().unsetColor().run()}
              >
                <RotateCcw size={20} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Resetar cor ({'{CLR:WHITE}'})</TooltipContent>
          </Tooltip>
        </fieldset>

        <Separator orientation="vertical" className="my-1 w-px" />

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={!editor?.can().undo()}
              aria-label="Desfazer"
              onClick={() => editor?.chain().focus().undo().run()}
            >
              <Undo2 size={20} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Desfazer</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={!editor?.can().redo()}
              aria-label="Refazer"
              onClick={() => editor?.chain().focus().redo().run()}
            >
              <Redo2 size={20} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Refazer</TooltipContent>
        </Tooltip>
      </div>

      {splitLeadingPadding(value).padding !== '' && (
        <div
          className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground"
          title="Espaços/quebras intencionais do jogo, preservados automaticamente"
        >
          <span>␍ padding de tela:</span>
          <code className="rounded bg-muted px-1">
            {summarizePadding(splitLeadingPadding(value).padding)}
          </code>
        </div>
      )}
      <div
        ref={hostRef}
        // Host é flex coluna + rolagem interna: o contenteditable (.tiptap)
        // estica com flex-1 e o ring azul de foco sai com a ALTURA DA CAIXA
        // (não mais a do texto); texto maior que a caixa rola aqui dentro.
        // Respiro do texto: host p-3 (12px até a borda da caixa) + p-1 DENTRO
        // do .tiptap — afasta o texto da borda externa e do ring de foco,
        // que é desenhado na borda do próprio .tiptap.
        className="flex min-h-40 flex-1 flex-col overflow-y-auto rounded border border-border p-3 outline-none [&_.tiptap]:flex-1 [&_.tiptap]:p-1 [&_.tiptap_p]:mb-2 [&_.tiptap_p:last-child]:mb-0"
      />
    </div>
  );
}

/**
 * Menu de templates textuais (Ícones — gates não têm tile na sprite).
 */
function TemplateDropdown({
  ariaLabel,
  icon,
  templates,
  tooltip,
  onSelect,
}: {
  ariaLabel: string;
  icon: React.ReactNode;
  templates: IconTemplate[];
  tooltip: string;
  onSelect: (t: IconTemplate) => void;
}) {
  return (
    <Tooltip>
      <DropdownMenu>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" aria-label={ariaLabel}>
              {icon}
              <ChevronDown size={14} />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <DropdownMenuContent>
          {templates.map((t) => (
            <DropdownMenuItem key={t.value} onSelect={() => onSelect(t)}>
              {t.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Menu só-ícone de controles (Botões / Direcionais): trigger com glifo da
 * sprite e itens ícone + nome do buttonMap.
 */
function SpriteTemplateDropdown({
  ariaLabel,
  tooltip,
  items,
  triggerCls,
  onSelect,
}: {
  ariaLabel: string;
  tooltip: string;
  items: TemplateItem[];
  triggerCls: string[];
  onSelect: (t: TemplateItem) => void;
}) {
  return (
    <Tooltip>
      <DropdownMenu>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" aria-label={ariaLabel}>
              {triggerCls.map((cls) => (
                <span key={cls} className={`gb-sprite gb-trigger-ico ${cls}`} aria-hidden="true" />
              ))}
              <ChevronDown size={14} />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <DropdownMenuContent align="start" className="max-h-85 overflow-y-auto">
          {items.map((t) => (
            <DropdownMenuItem key={t.value} onSelect={() => onSelect(t)}>
              {t.cls.map((cls) => (
                <span key={cls} className={`gb-sprite gb-menu-item ${cls}`} />
              ))}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}
