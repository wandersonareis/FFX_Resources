'use client';

import { useEffect, useRef } from 'react';
import { useProgressBatch } from '@/lib/ffx/progress-store';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { useWailsEvent } from '@/lib/ffx/use-wails-event';

/** Cap da fila de toasts de ciclo aberto (descarta o mais antigo). */
const NOTIFY_QUEUE_CAP = 50;

type NotifyPayload = { severity?: string; message?: string };

/** Rende uma notificação por severidade (error: sticky; demais: 4 s). */
function emitNotify(payload: NotifyPayload): void {
  if (payload?.severity === 'error') {
    toast.error(payload?.message ?? 'Notificação');
  } else {
    toast(payload?.message ?? 'Notificação', { duration: 4000 });
  }
}

/**
 * Roteador das notificações Wails (Notify → toast), sem props: o estado do
 * progresso vive na store módulo `progress-store` (o modal é o
 * ProgressModal, irmão deste). Enquanto um modal de progresso está aberto
 * (radix põe pointer-events: none no body), os avisos acumulam numa fila e
 * aparecem quando o usuário fecha o resultado.
 */
export function ProgressNotifier() {
  const batch = useProgressBatch();
  const notifyQueue = useRef<NotifyPayload[]>([]);

  useWailsEvent('Notify', (data) => {
    const payload = data as NotifyPayload;
    if (batch) {
      if (notifyQueue.current.length >= NOTIFY_QUEUE_CAP) {
        notifyQueue.current.shift();
      }
      notifyQueue.current.push(payload ?? { message: 'Notificação' });
      return;
    }
    emitNotify(payload);
  });

  // Modal fechado (usuário soltou o resultado): libera a fila acumulada.
  useEffect(() => {
    if (batch === null && notifyQueue.current.length > 0) {
      const pending = notifyQueue.current;
      notifyQueue.current = [];
      for (const q of pending) emitNotify(q);
    }
  }, [batch]);

  return null;
}
