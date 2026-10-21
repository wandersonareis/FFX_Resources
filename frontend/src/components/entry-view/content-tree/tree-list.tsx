'use client';

import type { MouseEvent, RefObject } from 'react';
import { useSelector } from '@tanstack/react-store';
import { Settings } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { configStore } from '@/lib/ffx/config-store';
import { openConfigDialog } from '@/lib/ffx/config-dialog-store';
import { hasOwnVbfContainer } from '@/lib/ffx/game-version';
import type { SideNode } from '@/lib/ffx/content-tree/model';
import { TreeItem } from './tree-item';

/**
 * Erro/aviso exibido no LUGAR da seção de containers quando nenhuma raiz
 * .vbf chegou. O jogo inteiro mora no container — sem ele falta a fonte da
 * verdade — então a ausência precisa ser EXPLÍCITA, não uma seção que
 * apenas some:
 *
 *  - executável NÃO configurado: ERRO (role=alert, cor destrutiva)
 *    pedindo para apontar o FFX.exe/FFX-2.exe nas Configurações, com o
 *    botão abrindo o diálogo direto;
 *  - executável configurado mas sem .vbf achado: aviso neutro — não há o
 *    que configurar além do caminho já escolhido (jogo não instalado?,
 *    container ausente).
 *
 * `data-vbf-missing` marca o bloco para os testes E2E (o mesmo papel dos
 * data-node-* da árvore).
 */
function VbfMissing() {
  const gameExe = useSelector(configStore, (s) => s.gameExeLocation);
  if (gameExe !== '') {
    return (
      <p
        data-vbf-missing=""
        className="mx-2 mt-1 rounded-md border border-border px-2 py-1.5 text-xs text-muted-foreground"
      >
        Nenhum <code>.vbf</code> encontrado ao lado do executável
        configurado.
      </p>
    );
  }
  return (
    <div
      data-vbf-missing=""
      role="alert"
      className="mx-2 mt-1 flex flex-col gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-2 py-1.5 text-xs text-destructive"
    >
      <span className="font-medium">Nenhum container .vbf</span>
      <span>
        Aponte o executável do jogo (FFX.exe ou FFX-2.exe) nas
        Configurações — dele saem os containers exibidos na árvore.
      </span>
      <Button
        size="sm"
        variant="outline"
        className="h-7 w-fit"
        onClick={openConfigDialog}
      >
        <Settings size={14} />
        Abrir Configurações
      </Button>
    </div>
  );
}

/**
 * ScrollArea da árvore com as duas seções e seus separadores. Os containers
 * .vbf vêm PRIMEIRO: são a fonte da verdade e a árvore de data/ (imutável) é
 * o espelho já extraído.
 */
export function TreeList({
  treeRef,
  version,
  vbfRoots,
  vbfStatus,
  roots,
  onNodeContextMenu,
}: {
  treeRef: RefObject<HTMLDivElement | null>;
  /** Versão da aba — só ffx/ffx2 podem ter container próprio. */
  version: string;
  vbfRoots: readonly SideNode[];
  /** 'ready' com lista vazia = aviso "aponte o executável" (ver VbfMissing). */
  vbfStatus: 'loading' | 'ready';
  roots: readonly SideNode[];
  onNodeContextMenu: (event: MouseEvent) => void;
}) {
  // Abas sem container próprio (Eternal Calm, Last Mission) nunca têm
  // raízes — a seção vazia lá é por design e não é erro.
  const vbfExpected = hasOwnVbfContainer(version as Parameters<
    typeof hasOwnVbfContainer
  >[0]);
  return (
    <ScrollArea
      id="entry-tree"
      ref={treeRef}
      className="flex-1 min-h-0"
      onContextMenuCapture={onNodeContextMenu}
    >
      {vbfRoots.length > 0 ? (
        <small className="px-2 pt-1 pb-1 text-xs font-medium text-muted-foreground">
          Containers .vbf
        </small>
      ) : vbfStatus === 'ready' && vbfExpected ? (
        <VbfMissing />
      ) : null}
      {vbfRoots.map((node) => (
        <TreeItem key={node.id} node={node} depth={0} />
      ))}
      {roots.length > 0 && vbfRoots.length > 0 ? (
        <small className="px-2 pt-3 pb-1 text-xs font-medium text-muted-foreground mt-1">
          Arquivos de data/
        </small>
      ) : null}
      {roots.map((node) => (
        <TreeItem key={node.id} node={node} depth={0} />
      ))}
    </ScrollArea>
  );
}
