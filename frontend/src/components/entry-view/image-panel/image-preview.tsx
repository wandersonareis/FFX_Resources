'use client';

import { Maximize2, ZoomIn } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { imagePreviewHotkeyHint } from './use-image-preview';

export type PreviewZoomLevel = 'fit' | number;

/**
 * Área de exibição da textura + os controles flutuando sobre ela.
 *
 * A lupa anda no ciclo 1× (tamanho real) → 4× e volta; ajuste fino no
 * slider. A altura do contêiner muda com o zoom: no ajuste ele ocupa o
 * disponível, em zoom fixo cresce e rola em ambas as direções
 * (comportamento de visualizador de imagem), por escolha do usuário.
 *
 * O menu flutuante só aparece com hover sobre a área da imagem (ou ao
 * focar um dos botões via teclado): fora disso ele some e a textura fica
 * limpa. As opacidades mudam, nunca `hidden`/`display` — os controles
 * continuam no DOM, acessíveis e revelados por `group-hover`/
 * `group-focus-within`.
 */
export function ImagePreview({
  src,
  alt,
  width,
  height,
  zoomLevel,
  flipped,
  pixelated,
  onZoomIn,
  onResetZoom,
  onSetZoomLevel,
  onTogglePixelated,
}: {
  src: string | null;
  alt: string;
  width: number;
  height: number;
  zoomLevel: PreviewZoomLevel;
  flipped: boolean;
  pixelated: boolean;
  onZoomIn: () => void;
  onResetZoom: () => void;
  onSetZoomLevel: (level: number) => void;
  onTogglePixelated: () => void;
}) {
  return (
    // `group` na área da imagem: é o hover aqui (ou o focus em qualquer
    // controle, via focus-within) que revela o menu flutuante.
    <div className="group relative">
      {/* Área de exibição com altura fixa: no zoom a imagem cresce
          além do container e rola em ambas as direções (comportamento
          de visualizador de imagem na web), por escolha do usuário. */}
      <div
        className={
          zoomLevel === 'fit'
            ? 'flex min-h-40 items-center justify-center overflow-auto rounded-md border bg-muted/40 p-4'
            : 'flex h-[65vh] overflow-auto rounded-md border bg-muted/40 p-4'
        }
      >
        {src ? (
          // eslint-disable-next-line @next/next/no-img-element -- data URL gerado pelo Go (next/image não otimiza nem precisa)
          <img
            src={src}
            alt={alt}
            // Só visual: o backend já envia na orientação do jogo; o
            // espelhamento, o zoom e o pixelado não tocam no dado nem
            // geram chamada ao Go. No zoom o scroll do container navega.
            // Escala UNIFORME explícita: largura e altura multiplicadas
            // pelo mesmo nível — o aspecto nunca muda, nem em 1× (tamanho
            // real). maxWidth: 'none' anula o preflight do Tailwind
            // (img { max-width: 100%; height: auto }), que reencaparía a
            // imagem de volta ao container e quebraria a rolagem.
            style={{
              transform: flipped ? 'scaleY(-1)' : undefined,
              width: zoomLevel === 'fit' ? undefined : `${width * zoomLevel}px`,
              height: zoomLevel === 'fit' ? undefined : `${height * zoomLevel}px`,
              maxWidth: zoomLevel === 'fit' ? undefined : 'none',
              imageRendering: pixelated ? 'pixelated' : undefined,
              transition: 'transform 120ms ease-out',
            }}
            className={
              zoomLevel === 'fit'
                ? 'max-h-[65vh] max-w-full object-contain'
                : // m-auto centraliza quando cabe e zera quando estoura —
                  // sem isso o flex centrado cortaria a rolagem; shrink-0
                  // impede o flex de encolher a imagem de volta.
                  'm-auto shrink-0'
            }
          />
        ) : (
          <p className="opacity-70">Sem pré-visualização disponível.</p>
        )}
      </div>
      {/* Controles de zoom flutuando sobre a imagem: a lupa anda no
          ciclo 1× (tamanho real) → 4× e volta; ajuste fino no slider.
          Ocultos por opacidade até o hover na área (ou o focus em um dos
          botões) — a imagem fica sem menu quando o mouse está longe. */}
      <div className="pointer-events-none absolute inset-x-0 top-0 z-10 flex flex-col items-end gap-1 p-2 opacity-0 transition-opacity duration-150 ease-out group-hover:opacity-100 group-focus-within:opacity-100 motion-reduce:transition-none">
        <div className="pointer-events-auto flex items-center gap-1 rounded-md border bg-background/85 p-1 shadow-sm backdrop-blur">
          <Button
            size="icon"
            variant="ghost"
            className="size-7"
            title={`Lupa: tamanho real → 2× → 3× → 4× → tamanho real (${imagePreviewHotkeyHint})`}
            onClick={onZoomIn}
          >
            <ZoomIn size={16} />
          </Button>
          <Button
            size="sm"
            variant={zoomLevel === 'fit' ? 'default' : 'ghost'}
            className="h-7 px-2 text-xs"
            title="Encaixa a textura no painel (escala suave do navegador)"
            onClick={onResetZoom}
          >
            <Maximize2 size={14} />
            Ajustar
          </Button>
          <Button
            size="sm"
            variant={pixelated ? 'default' : 'ghost'}
            className="h-7 px-2 text-xs"
            title="Pixelado (nearest-neighbor) — cada pixel como é; distingue embaçamento da exibição do embaçamento do dado"
            onClick={onTogglePixelated}
          >
            Pixelado
          </Button>
        </div>
        {zoomLevel !== 'fit' ? (
          <div className="pointer-events-auto flex items-center gap-2 rounded-md border bg-background/85 px-2 py-1 shadow-sm backdrop-blur">
            <span className="text-xs opacity-60">real</span>
            <input
              type="range"
              min={1}
              max={4}
              step={0.25}
              value={zoomLevel}
              aria-label="Zoom da pré-visualização"
              className="w-36 accent-primary"
              onChange={(e) => onSetZoomLevel(Number(e.target.value))}
            />
            <span className="w-9 text-right text-xs tabular-nums opacity-60">
              {zoomLevel}×
            </span>
          </div>
        ) : null}
      </div>
    </div>
  );
}
