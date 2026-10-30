'use client';

import { ThemeProvider } from 'next-themes';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { UpdateBanner } from '@/components/update-banner';

/**
 * Providers globais. O ThemeProvider do next-themes é o dono do tema:
 * escreve a classe `.dark` no <html> (o Tailwind v4 resolve via
 * `@custom-variant dark`), persiste a escolha em localStorage e evita
 * flash na hidratação. `defaultTheme="dark"` = code_dark.html; o toggle
 * no header alterna para "light" (code.html).
 */
export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="dark"
      enableSystem={false}
      disableTransitionOnChange
    >
      <TooltipProvider delayDuration={300}>
        {children}
        <Toaster position="bottom-center" />
        {/* Estado do auto-update: acorda só com eventos wails:updater:* */}
        <UpdateBanner />
      </TooltipProvider>
    </ThemeProvider>
  );
}
