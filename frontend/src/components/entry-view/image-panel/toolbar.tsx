"use client";

import {
  Copy,
  FlipVertical,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { SHORTCUTS, shortcutTip } from "@/lib/ffx/shortcuts";
import { ImageExportMenu } from "./export-menu";
import { ImageExportFormatMenu } from "./image-export-format-menu";
import { ImageImportMenu } from "./import-menu";
import type { ImageBusy } from "./use-image-actions";

/**
 * Barra de ações do painel: exportar (salvar como .dds/.png em pasta livre,
 * ou extrair os dois formatos para mods/images — a extração mora no menu
 * Exportar), importar um .dds editado (repack só do mesmo formato/dimensão
 * nesta etapa), propagar o import para as cópias idênticas, espelhar a
 * pré-visualização, reanalisar o índice e ocultar a coluna de informações
 * (a prévia passa a ocupar toda a largura — o botão é sempre visível).
 */
export function ImageToolbar({
  busy,
  readOnly,
  flipped,
  duplicateCount,
  infoHidden,
  onExport,
  onExtractTo,
  onImport,
  onImportTranslated,
  onReplicate,
  onToggleFlip,
  onToggleInfo,
  onRefresh,
}: {
  busy: ImageBusy;
  readOnly: boolean;
  flipped: boolean;
  duplicateCount: number;
  /** Coluna de informações oculta (estado de sessão, vem do imageInfoStore). */
  infoHidden: boolean;
  /** Exporta no formato selecionado para mods/images (sem diálogo). */
  onExport: (format: "dds" | "png" | "dds+png") => void;
  /** Exporta no formato selecionado para pasta customizada (com diálogo). */
  onExtractTo: (format: "dds" | "png" | "dds+png") => void;
  onImport: () => void;
  onImportTranslated: () => void;
  onReplicate: () => void;
  onToggleFlip: () => void;
  /** Oculta/exibe a coluna de informações ao lado da prévia. */
  onToggleInfo: () => void;
  onRefresh: () => void;
}) {
  const copyTotal = duplicateCount + 1;

  return (
    // justify-between: as ações da textura ficam agrupadas à esquerda (e
    // quebram linha ENTRE SI se faltar largura) e o botão Info fica no
    // canto direito, sempre acessível.
    <div className="flex flex-wrap items-center justify-between gap-1">
      <div className="flex flex-wrap items-center gap-1">
        <ImageExportFormatMenu />
        <ImageExportMenu
          disabled={busy !== null}
          readOnly={readOnly}
          onExport={onExport}
          onExtractTo={onExtractTo}
        />
        <ImageImportMenu
          disabled={busy !== null}
          duplicateCount={duplicateCount}
          onImportFile={onImport}
          onImportTranslated={onImportTranslated}
          busy={busy}
        />
        {duplicateCount > 0 ? (
          <Button
            size="sm"
            variant="outline"
            disabled={busy !== null}
            title={
              readOnly
                ? "Reempacota a imagem aberta em mods/ das cópias do .vbf já identificadas"
                : "Reempacota a imagem aberta em mods/ de cada cópia idêntica — sem escolher arquivo"
            }
            onClick={onReplicate}
          >
            <Copy size={16} />
            Replicar para {duplicateCount} cópia
            {duplicateCount > 1 ? "s" : ""}
          </Button>
        ) : null}
        <Button
          size="sm"
          variant={flipped ? "default" : "outline"}
          aria-pressed={flipped}
          title={`${shortcutTip(SHORTCUTS.imageFlip.label, SHORTCUTS.imageFlip.keys)} — só na tela: não altera nem reenvia a imagem`}
          onClick={onToggleFlip}
        >
          <FlipVertical size={16} />
          {flipped ? "Flip (on)" : "Flip"}
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-1">
        {!readOnly ? (
          <Button
            size="sm"
            variant="ghost"
            disabled={busy !== null}
            title="Reconstrói o índice de duplicatas — para textura editada fora do app (hex editor)"
            onClick={onRefresh}
          >
            <RefreshCw size={16} />
            {busy === "refresh" ? "Reanalisando…" : "Reanalisar"}
          </Button>
        ) : null}
        {/*
          Estado de SESSÃO (imageInfoStore), não de layout: sobrevive à troca
          imagem↔texto e entre abas, mas ao fechar o app a coluna volta aberta
          — por isso não mora no layout-storage e não tem disabled: é só
          exibição, como o Flip. Aceso (azul) = coluna EXIBIDA, apagado =
          coluna oculta — o pressed espelha a coluna, não o clique.
      */}
        <Button
          size="sm"
          variant={infoHidden ? "outline" : "default"}
          aria-pressed={!infoHidden}
          title={
            infoHidden
              ? "Exibe a coluna de informações"
              : "Oculta a coluna de informações"
          }
          onClick={onToggleInfo}
        >
          Info
        </Button>
      </div>
    </div>
  );
}
