'use client';

import { useCallback, useState } from 'react';
import { ChevronDown, Download, FileUp, FolderInput, ListChecks, Upload } from 'lucide-react';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { Events } from '@wailsio/runtime';
import * as dto from '@/bindings/ffxresources/backend/dto/models';
import { GAME_VERSIONS, GameVersionId } from '@/lib/ffx/game-version';
import { EntryKind, KIND_LABELS } from '@/lib/ffx/display-names';
import { EXPORT_FORMAT_LABELS, useExportSelection } from '@/lib/ffx/export-selection';
import { exportLangs } from '@/lib/ffx/export-langs';
import { parseError } from '@/lib/ffx/error-handler';
import { progressWasShown } from '@/lib/ffx/progress-store';
import {
  entryKindsFor,
  exportArtifactBatch,
  importFile,
  importTranslated,
  previewImport,
  previewImportTranslated,
  selectImportFile,
} from '@/lib/ffx/tree-data';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ImportSummaryDialog } from '@/components/dialogs/import-summary-dialog';
import { ImportTranslatedDialog } from '@/components/dialogs/import-translated-dialog';

/**
 * Controles Exportar/Importar da barra de abas + diálogo de resumo de import.
 * Única prop: a versão ativa. Tudo o mais é estado interno (escopo por versão,
 * resumo/importando) ou store módulo (`exportSelection`) — zero prop drilling.
 */
export function ExportImportControls({ version }: { version: GameVersionId }) {
  // Escopo de exportação por versão (kinds marcados no dropdown Exportar).
  const [scopes, setScopes] = useState<Record<string, EntryKind[]>>(() =>
    Object.fromEntries(GAME_VERSIONS.map((tab) => [tab.id, entryKindsFor(tab.id)]))
  );
  const [importSummary, setImportSummary] = useState<dto.ImportSummary | null>(
    null
  );
  const [importing, setImporting] = useState(false);

  const servedKinds = entryKindsFor(version);
  const activeScope = scopes[version] ?? servedKinds;

  // Escopo do import em lote (mods/translated), separado do de exportação:
  // escolher o que exportar não decide o que importar.
  const [importScopes, setImportScopes] = useState<Record<string, EntryKind[]>>(
    () =>
      Object.fromEntries(
        GAME_VERSIONS.map((tab) => [tab.id, entryKindsFor(tab.id)])
      )
  );
  const [importBatch, setImportBatch] = useState<dto.ImportBatch | null>(null);
  const activeImportScope = importScopes[version] ?? servedKinds;

  // Formato de exportação vem do BOTÃO DE FORMATO do header (direita do
  // botão de configurações) — a store `exportSelection` é a única fonte;
  // o popover do export não tem seletor próprio.
  const activeFormat = useExportSelection().format;

  const toggleImportScope = (kind: EntryKind) => {
    setImportScopes((prev) => {
      const current = prev[version] ?? entryKindsFor(version);
      const next = current.includes(kind)
        ? current.filter((k) => k !== kind)
        : [...current, kind];
      return { ...prev, [version]: next };
    });
  };

  const toggleScope = (kind: EntryKind) => {
    setScopes((prev) => {
      const current = prev[version] ?? entryKindsFor(version);
      const next = current.includes(kind)
        ? current.filter((k) => k !== kind)
        : [...current, kind];
      return { ...prev, [version]: next };
    });
  };

  // Exporta os kinds no FORMATO SELECIONADO como UMA operação agrupada:
  // o modal de progresso cuida de tudo (um step por kind com barra própria
  // + resumo de arquivos/destino no final) — nada de toast paralelo.
  //
  // O botão Exportar é SÓ DE ESCOPO DE TIPO: cada kind sai por inteiro
  // (ids = [], o contrato de "tudo do kind" do backend). A seleção de
  // itens da árvore não entra aqui — ela é do menu de contexto da árvore,
  // que é onde o usuário espera exportar "o que marquei".
  const runExport = useCallback(
    async (kinds: EntryKind[]) => {
      const format = activeFormat;
      if (kinds.length === 0) {
        toast.info('Nada para exportar — marque um tipo no escopo.');
        return;
      }
      const items = kinds.map((kind) => ({ kind, ids: [] as string[] }));
      try {
        // Idiomas: array do seletor do header (default + extras marcados);
        // `[]` (pré-hidratação) = todos, o contrato do backend.
        const paths = await exportArtifactBatch(items, version, exportLangs.langsOf(), format);
        if ((paths ?? []).length === 0) {
          toast.info('Nada para exportar.');
        }
        // Saiu arquivo: o modal mostrou o resumo — sem toast de sucesso.
      } catch (error) {
        // Erro ANTES do batch abrir (formato/kind inválido, lote vazio):
        // toast. Erro no meio da operação: o modal já exibiu (step falho).
        if (!progressWasShown()) toast.error(parseError(error));
      }
    },
    [version, activeFormat]
  );

  const onImportClick = useCallback(async () => {
    try {
      const path = await selectImportFile();
      if (!path) return;
      setImportSummary(await previewImport(path, version));
    } catch (error) {
      toast.error(parseError(error));
    }
  }, [version]);

  // Import em lote de mods/translated/text: primeiro o resumo (sem aplicar)
  // para o diálogo de confirmação, depois a aplicação arquivo a arquivo.
  const onImportTranslatedClick = useCallback(async () => {
    if (activeImportScope.length === 0) {
      toast.info('Marque pelo menos um tipo para importar.');
      return;
    }
    try {
      setImportBatch(await previewImportTranslated(version, activeImportScope));
    } catch (error) {
      toast.error(parseError(error));
    }
  }, [version, activeImportScope]);

  const onConfirmImportTranslated = useCallback(async () => {
    if (!importBatch) return;
    setImporting(true);
    const toastId = 'import';
    toast.loading('Importando...', { id: toastId });
    try {
      const result = await importTranslated(version, activeImportScope);
      setImportBatch(null);
      if (result.changed_texts === 0) {
        toast.info('Nada a importar.', { id: toastId });
      } else {
        const arquivos =
          result.errors.length > 0
            ? ` (${result.errors.length} arquivo(s) pulado(s))`
            : '';
        const entradas =
          result.warnings.length > 0
            ? ` (${result.warnings.length} entrada(s) ignorada(s))`
            : '';
        toast.success(
          `Importado: ${result.changed_texts} texto(s) alterado(s) em ` +
            `${result.files.length} arquivo(s).${arquivos}${entradas}`,
          { id: toastId }
        );
        // Avisa a aba ativa para recarregar árvore/tabela (rascunhos preservados).
        try {
          Events.Emit('ImportDone');
        } catch {
          // sem runtime Wails (dev no browser) — segue sem recarga automática
        }
      }
    } catch (error) {
      toast.error(parseError(error), { id: toastId });
    } finally {
      setImporting(false);
    }
  }, [importBatch, version, activeImportScope]);

  const onConfirmImport = useCallback(async () => {
    const current = importSummary;
    if (!current) return;
    setImporting(true);
    const toastId = 'import';
    toast.loading('Importando…', { id: toastId });
    try {
      const changed = await importFile(current.path, version);
      setImportSummary(null);
      const ignored = current.warnings?.length ?? 0;
      const skippedRows = current.skipped?.length ?? 0;
      // Sucesso com asterisco: entrada ignorada (aviso) e/ou row pulada
      // (hash stale / tag divergente) continuam existindo depois que o
      // modal fecha — o toast carrega o mesmo número que o botão pulsante.
      const parts: string[] = [];
      if (ignored > 0) parts.push(`${ignored} entrada(s) ignorada(s)`);
      if (skippedRows > 0) parts.push(`${skippedRows} row(s) ignorada(s)`);
      const suffix = parts.length > 0 ? ` (${parts.join(', ')})` : '';
      toast.success(
        `Importação concluída — ${changed} texto(s) atualizado(s) e salvos em binário (mods/).${suffix}`,
        { id: toastId }
      );
      // Avisa a aba ativa para recarregar árvore/tabela (rascunhos preservados).
      try {
        Events.Emit('ImportDone');
      } catch {
        // sem runtime Wails (dev no browser) — segue sem recarga automática
      }
    } catch (error) {
      toast.error(parseError(error), { id: toastId });
    } finally {
      setImporting(false);
    }
  }, [importSummary, version]);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            title={`Exportar textos — ${EXPORT_FORMAT_LABELS[activeFormat]} (formato do seletor ao lado das configurações): todos os tipos, ou só os tipos marcados no escopo`}
          >
            <Download size={16} />
            Exportar
            <ChevronDown size={12} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-70">
          <DropdownMenuItem onSelect={() => void runExport(servedKinds)}>
            <Download size={14} />
            Exportar
            <span className="ml-auto pl-3 text-xs opacity-60">todos os tipos</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="text-muted-foreground">
            Escopo
          </DropdownMenuLabel>
          {servedKinds.map((kind) => (
            <DropdownMenuCheckboxItem
              key={kind}
              checked={activeScope.includes(kind)}
              onCheckedChange={() => toggleScope(kind)}
              onSelect={(event) => event.preventDefault()}
            >
              {KIND_LABELS[kind]}
            </DropdownMenuCheckboxItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => void runExport(activeScope)}>
            <ListChecks size={14} />
            Exportar seleção
            <span className="ml-auto pl-3 text-xs opacity-60">tipos marcados</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            title="Importar textos — em lote de mods/translated ou de arquivo JSON/Strings"
          >
            <Upload size={16} />
            Importar
            <ChevronDown size={12} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-70">
          <DropdownMenuLabel className="text-muted-foreground">
            Tipos
          </DropdownMenuLabel>
          {servedKinds.map((kind) => (
            <DropdownMenuCheckboxItem
              key={kind}
              checked={activeImportScope.includes(kind)}
              onCheckedChange={() => toggleImportScope(kind)}
              onSelect={(event) => event.preventDefault()}
            >
              {KIND_LABELS[kind]}
            </DropdownMenuCheckboxItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => void onImportTranslatedClick()}>
            <FolderInput size={14} />
            Importar
            <span className="ml-auto pl-3 text-xs opacity-60">mods/translated</span>
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => void onImportClick()}>
            <FileUp size={14} />
            Importar de…
            <span className="ml-auto pl-3 text-xs opacity-60">arquivo</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ImportSummaryDialog
        summary={importSummary}
        open={importSummary !== null}
        importing={importing}
        onOpenChange={(open) => {
          if (!open && !importing) setImportSummary(null);
        }}
        onConfirm={() => void onConfirmImport()}
      />
      <ImportTranslatedDialog
        batch={importBatch}
        open={importBatch !== null}
        importing={importing}
        onOpenChange={(open) => {
          if (!open && !importing) setImportBatch(null);
        }}
        onConfirm={() => void onConfirmImportTranslated()}
      />
    </>
  );
}
