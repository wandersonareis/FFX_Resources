'use client';

import { ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { DEFAULT_LANG, exportLangs, useExportLangs } from '@/lib/ffx/export-langs';

/**
 * Seletor de idiomas de exportação (header, à direita do seletor de
 * formato). O idioma default fica no topo SEM marcação — sai sempre nos
 * artefatos; os extras (ListLanguages do backend, menos o default) têm
 * checkbox, marcados por padrão e persistidos no localStorage
 * (`ffx.export-langs`): sem storage = todos marcados. A escolha vale para
 * todas as exportações (topo, árvore, VBF).
 */
export function ExportLangsMenu() {
  const { available, checked } = useExportLangs();

  const extras = available.filter((lang) => lang.code !== DEFAULT_LANG);
  const checkedCount = extras.filter((lang) => checked.has(lang.code)).length;
  // Rótulo = valor atual (mesmo espelho do botão de formato): "Todos"
  // enquanto nada foi desmarcado (inclusive antes da hidratar); "us" só
  // com o default; "us + N" no meio do caminho.
  const summary =
    checkedCount === extras.length
      ? 'Todos'
      : checkedCount === 0
        ? DEFAULT_LANG
        : `${DEFAULT_LANG} + ${checkedCount}`;
  const defaultName = available.find((lang) => lang.code === DEFAULT_LANG)?.name ?? DEFAULT_LANG;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-9 gap-1 px-2 text-sm"
          title={`Idiomas da exportação — ${defaultName} (${DEFAULT_LANG}) sai sempre nos artefatos; os extras marcados entram junto. A escolha fica salva e vale para todos os exports.`}
        >
          {summary}
          <ChevronDown size={14} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <DropdownMenuLabel className="text-muted-foreground">Idiomas</DropdownMenuLabel>
        <DropdownMenuItem disabled className="grid grid-cols-[5rem_auto_1fr]">
          <span className="truncate">{defaultName}</span>
          <span className="text-xs opacity-60">{DEFAULT_LANG}</span>
          <span className="justify-self-end text-xs opacity-60">padrão</span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {extras.map((lang) => (
          <DropdownMenuCheckboxItem
            key={lang.code}
            checked={checked.has(lang.code)}
            onCheckedChange={() => exportLangs.toggle(lang.code)}
            onSelect={(event) => event.preventDefault()}
            className="grid grid-cols-[5rem_auto]"
          >
            <span className="truncate">{lang.name}</span>
            <span className="text-xs opacity-60">{lang.code}</span>
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
