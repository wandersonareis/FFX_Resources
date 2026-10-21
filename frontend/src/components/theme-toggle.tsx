'use client';

import { SetTheme } from '@/bindings/ffxresources/configservice';
import { Moon, Sun } from 'lucide-react';
import { useTheme } from 'next-themes';
import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';

/**
 * Toggle light/dark do app. Alterna a classe `.dark` no <html> via
 * next-themes (persistida em localStorage). Renderiza um ícone só depois
 * da hidratação para não divergir entre servidor e cliente (o tema salvo
 * só existe no navegador).
 */
export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  // Espelha o tema no backend (config.json + updater). A janela de
  // atualização é HTML estático em outra webview (sem localStorage) e o Go
  // renderiza o valor que guarda na abertura. O efeito roda no boot (refresca
  // a memória — o config.json pode estar defasado em relação ao
  // localStorage) e a cada troca feita pelo botão. Fire-and-forget: falha de
  // IPC não pode engasgar o clique.
  useEffect(() => {
    if (!mounted) return;
    if (resolvedTheme === 'dark' || resolvedTheme === 'light') {
      SetTheme(resolvedTheme).catch(() => {});
    }
  }, [mounted, resolvedTheme]);

  const isDark = mounted && resolvedTheme === 'dark';

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={isDark ? 'Ativar tema claro' : 'Ativar tema escuro'}
          onClick={() => setTheme(isDark ? 'light' : 'dark')}
        >
          {isDark ? <Sun size={20} /> : <Moon size={20} />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {isDark ? 'Tema claro' : 'Tema escuro'}
      </TooltipContent>
    </Tooltip>
  );
}
