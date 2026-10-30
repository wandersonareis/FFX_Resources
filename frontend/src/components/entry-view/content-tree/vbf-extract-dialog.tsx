'use client';

import { AlertTriangle, FolderOpen, HardDriveDownload, Loader2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { formatBytes } from '@/lib/ffx/bytes';
import { parseError } from '@/lib/ffx/error-handler';
import { progressWasShown } from '@/lib/ffx/progress-store';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { extractVbfSelection, previewVbfExtraction, selectVbfExtractDir } from '@/lib/ffx/vbf';

export interface VbfExtractRequest {
  requestId: number;
  root: string;
  paths: string[];
  /** data = destino padrão do jogo; choose = seletor nativo de pasta. */
  mode: 'data' | 'choose';
}

/** Confirmação + extração da seleção VBF, preservando os caminhos internos. */
export function VbfExtractDialog({
  request,
  onClose,
  onDone,
}: {
  request: VbfExtractRequest | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [destRoot, setDestRoot] = useState<string | null>(request?.mode === 'data' ? '' : null);
  const [preview, setPreview] = useState<dto.VbfExtractPreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [extracting, setExtracting] = useState(false);
  const [runError, setRunError] = useState<string | null>(null);

  // "Extrair para…" abre o diálogo nativo imediatamente; o diálogo de
  // confirmação só aparece depois de a pasta ser escolhida.
  useEffect(() => {
    if (request?.mode !== 'choose') return;
    let cancelled = false;
    void selectVbfExtractDir()
      .then((path) => {
        if (cancelled) return;
        if (!path) {
          onClose();
          return;
        }
        setDestRoot(path);
      })
      .catch((error) => {
        if (cancelled) return;
        toast.error(parseError(error));
        onClose();
      });
    return () => {
      cancelled = true;
    };
  }, [request, onClose]);

  useEffect(() => {
    if (!request || destRoot === null) return;
    let cancelled = false;
    void previewVbfExtraction(request.root, request.paths, destRoot)
      .then((result) => {
        if (!cancelled) setPreview(result);
      })
      .catch((error) => {
        if (!cancelled) setPreviewError(parseError(error));
      });
    return () => {
      cancelled = true;
    };
  }, [request, destRoot]);

  const runExtraction = async () => {
    if (!request || !preview || preview.files === 0 || extracting) return;
    setExtracting(true);
    setRunError(null);
    try {
      const result = await extractVbfSelection(request.root, request.paths, destRoot ?? '');
      // O modal de progresso do backend acompanha arquivo a arquivo e
      // mostra o resultado (falhas contam como issues) — sem toast paralelo.
      void result;
      onDone();
    } catch (error) {
      const message = parseError(error);
      // Erro antes do batch abrir (seleção/destino): toast; durante a
      // operação: o modal de progresso já exibiu.
      if (!progressWasShown()) toast.error(message);
      setRunError(message);
      setExtracting(false);
    }
  };

  const isOpen = request !== null && (request.mode === 'data' || destRoot !== null);
  const loadingPreview = Boolean(
    request && destRoot !== null && preview === null && previewError === null,
  );
  const destination = preview?.destRoot ?? (destRoot || 'data/ do jogo');

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && !extracting && onClose()}>
      <DialogContent className="sm:max-w-sm" showCloseButton={!extracting}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <HardDriveDownload size={18} />
            Confirmar extração
          </DialogTitle>
          <DialogDescription>
            Os binários serão extraídos mantendo os caminhos relativos do .vbf.
          </DialogDescription>
        </DialogHeader>

        {runError ? (
          <p role="alert" className="text-destructive">
            {runError}
          </p>
        ) : loadingPreview ? (
          <div className="flex items-center gap-2 py-3 text-muted-foreground" role="status">
            <Loader2 size={16} className="animate-spin" />
            Calculando arquivos e possíveis sobrescritas…
          </div>
        ) : previewError ? (
          <p role="alert" className="text-destructive">
            {previewError}
          </p>
        ) : preview ? (
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
              <span className="text-muted-foreground">Arquivos</span>
              <span className="font-medium">{preview.files.toLocaleString('pt-BR')}</span>
              <span className="text-muted-foreground">Tamanho total</span>
              <span className="font-medium">{formatBytes(preview.bytes)}</span>
              <span className="text-muted-foreground">Destino</span>
              <span className="flex min-w-0 items-center gap-1 font-medium">
                <FolderOpen size={14} className="shrink-0" />
                <span className="truncate" title={destination}>
                  {destination}
                </span>
              </span>
            </div>
            {preview.existing > 0 ? (
              <div
                role="alert"
                className="flex gap-2 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm"
              >
                <AlertTriangle size={17} className="mt-0.5 shrink-0 text-warning" />
                <span>
                  <strong>{preview.existing.toLocaleString('pt-BR')} arquivo(s)</strong> já existem
                  no destino e serão sobrescritos.
                </span>
              </div>
            ) : null}
            {preview.files === 0 ? (
              <p role="status" className="text-sm text-muted-foreground">
                A seleção não contém arquivos para extrair.
              </p>
            ) : null}
          </div>
        ) : null}

        <DialogFooter>
          <Button variant="outline" disabled={extracting} onClick={onClose}>
            Cancelar
          </Button>
          <Button
            disabled={
              extracting ||
              (!runError &&
                (loadingPreview || Boolean(previewError) || !preview || preview.files === 0))
            }
            onClick={() => {
              if (runError) onClose();
              else void runExtraction();
            }}
          >
            {extracting ? (
              <Loader2 size={16} className="animate-spin" />
            ) : runError ? null : (
              <HardDriveDownload size={16} />
            )}
            {extracting ? 'Extraindo…' : runError ? 'Fechar' : 'Confirmar extração'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
