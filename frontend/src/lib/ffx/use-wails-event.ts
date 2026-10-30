'use client';

import { Events } from '@wailsio/runtime';
import { useEffect, useRef } from 'react';

/**
 * Registra um listener Wails com cleanup automático.
 *
 * No v2 era `EventsOn(nome, (...data) => cb(...data))`: o handler recebia os
 * argumentos espalhados. No v3 o callback recebe um `WailsEvent` com `.data`,
 * então repassamos `ev.data` — equivalente para os emits do app, que sempre
 * carregam no máximo um argumento.
 */
export function useWailsEvent(eventName: string, callback: (...data: unknown[]) => void): void {
  const ref = useRef(callback);
  useEffect(() => {
    ref.current = callback;
  }, [callback]);
  useEffect(() => Events.On(eventName, (ev) => ref.current(ev.data)), [eventName]);
}
