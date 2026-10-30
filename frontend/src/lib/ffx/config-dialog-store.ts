import { createStore } from '@tanstack/store';
import { OpenWindow } from '@/bindings/ffxresources/windowservice';
import { isDesktop } from './env';
import { loggedToast } from './toast-logged';

/**
 * Diálogo de Configurações — estado de MÓDULO (mesmo padrão do search-store
 * e do find-store): o AppShell só assina o `open` e renderiza, e qualquer
 * ponto do app abre as Configurações sem prop drilling do onOpenChange.
 *
 * Nasceu com o aviso "aponte o executável" da seção de containers da árvore:
 * sem ele, o clique vindo da sidebar precisaria atravessar ContentTree →
 * GameVersionTab → AppShell como callbacks empilhados.
 *
 * ABERTURA UNIFICADA: no desktop vira JANELA FILHA frameless
 * (WindowService.OpenWindow("config") — arrastável, com botões de janela);
 * no browser/server mode cai no modal Radix de sempre. O botão do header, o
 * Ctrl+, e o aviso da árvore passam todos por openConfigDialog().
 */
export const configDialogStore = createStore<{ open: boolean }>({
  open: false,
});

/** Abre/fecha o MODAL (AppShell é o único RENDERIZADOR, não o dono). */
export function setConfigDialogOpen(open: boolean): void {
  configDialogStore.setState((prev) => ({ ...prev, open }));
}

/** Abre as Configurações (atalho Ctrl+, header ou aviso da árvore). */
export function openConfigDialog(): void {
  if (isDesktop()) {
    void OpenWindow('config').catch((error: unknown) => {
      const message = error instanceof Error ? error.message : String(error ?? 'Erro');
      loggedToast.error(message, { duration: 4000 });
    });
    return;
  }
  setConfigDialogOpen(true);
}
