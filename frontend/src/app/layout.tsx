import type { Metadata } from 'next';
import { Geist_Mono } from 'next/font/google';
import './globals.css';
import { Providers } from '@/components/providers';

// Sans do projeto é a Malva (arquivo local, @font-face em globals.css) —
// o Geist fica só para o mono (kbd, código, listas técnicas).
const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'FINAL FANTASY X and X-2 HD Remaster Resources Editor',
  description: 'FINAL FANTASY X and X-2 HD Remaster Resources Editor',
};

export default function RootLayout({ children }: LayoutProps<'/'>) {
  return (
    // suppressHydrationWarning: o next-themes escreve a classe `.dark` no
    // <html> antes da hidratação (para não piscar o tema errado).
    <html
      lang="pt-BR"
      className={`${geistMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <body className="min-h-full flex flex-col">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
