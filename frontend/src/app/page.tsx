'use client';

import dynamic from 'next/dynamic';

// App de desktop Wails: roda 100% no cliente, sem rede e sem SSR. O
// `ssr: false` dispensa a pré-renderização do Next — e deixa o localStorage
// legível já no initializer de estado (o app-shell lê o layout salvo e a
// aba ativa antes do primeiro paint, sem frame com o padrão piscando).
const AppShell = dynamic(() => import('@/components/app-shell').then((m) => m.AppShell), {
  ssr: false,
});

export default function Home() {
  return <AppShell />;
}
