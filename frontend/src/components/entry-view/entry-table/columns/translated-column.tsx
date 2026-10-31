import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { askDeleteOrphan } from '@/lib/ffx/delete-orphan-store';
import { rowKey, type useEditDraft } from '@/lib/ffx/edit-draft';
import type { GameTextMark } from '@/lib/ffx/game-text-parser';
import { SOURCE_LANG } from '@/lib/ffx/save-all';
import type { EntryRow } from '@/lib/ffx/tree-data';
import type { EntryView } from '../../entry-view-store';
import { columnHelper, MISSING_SIDE } from '../table-types';
import { EditableCell } from './editable-cell';
import { LinkedCell } from './linked-cell';

/** O que a coluna "Traduzido" precisa para montar accessor e célula. */
export type TranslatedColumnDeps = {
  selectedEntry: EntryRow | null;
  version: EntryView['version'];
  drafts: ReturnType<typeof useEditDraft>['store'];
  actions: EntryView['actions'];
  refLinks: Record<string, dto.RefLink>;
  textLoc: string;
  linkedValueOf: (row: dto.TextRow) => string | undefined;
  /** Destaques da busca local (rowKey → intervalos em <mark>). */
  matchesByRow: Map<string, GameTextMark[]>;
};

/**
 * Coluna "Traduzido": decide entre os três casos — linha ausente no arquivo
 * traduzido (traço vermelho), linha linkada (texto da def) e texto próprio
 * (editável, com reverter).
 */
export function buildTranslatedColumn({
  selectedEntry,
  version,
  drafts,
  actions,
  refLinks,
  textLoc,
  linkedValueOf,
  matchesByRow,
}: TranslatedColumnDeps) {
  return columnHelper.accessor(
    (row) => {
      // Ref dedupada: a célula pinta o TEXTO DA DEF (estado atual,
      // rascunho incluído) — o "$hash" entregue pelo backend é só o
      // ponteiro de collapse, nunca conteúdo exibível.
      const entry = selectedEntry;
      // Rascunho por FONTE: data/ ("") e o .vbf (caminho do container)
      // têm namespaces separados. Ler sem a fonte esconderia a edição
      // feita pela ref (na def de outra entrada — inclusive de .vbf).
      const edited = entry
        ? drafts.editOf(version, entry.kind, entry.id, row, SOURCE_LANG, entry.vbf?.root ?? '')
        : undefined;
      const divergent = entry
        ? drafts.isDivergent(version, entry.kind, entry.id, row, entry.vbf?.root ?? '')
        : false;
      // Enquanto a escolha de divergência está no rascunho, a edição
      // local vence o link e a célula deixa de parecer azul.
      if (divergent && edited !== undefined) return edited;
      const linked = linkedValueOf(row);
      if (linked !== undefined) return linked;
      return edited ?? row.text?.[textLoc] ?? '';
    },
    {
      id: 'translated',
      header: 'Traduzido',
      cell: (info) => {
        const row = info.row.original;
        // União: linha que só existe no original tem o texto vazio e a
        // marcação — nada a traduzir, então o diálogo não abre.
        const missing = row.missingInTranslated === true;
        const entry = selectedEntry;
        // Somente leitura = .vbf de kind fora da lista de kinds
        // editáveis (nenhum texto hoje: events, tabelas eventtable,
        // help, macro, objects e lockit editam pelo .vbf, gravando
        // sempre em mods/). O rascunho é da fonte do container.
        const readOnly = entry?.vbf != null;
        const edited =
          !missing && entry
            ? drafts.editOf(
                version,
                entry.kind,
                entry.id,
                row,
                SOURCE_LANG,
                entry.vbf?.root ?? '',
              ) !== undefined
            : false;
        const divergentDraft =
          !missing && entry
            ? drafts.isDivergent(version, entry.kind, entry.id, row, entry.vbf?.root ?? '')
            : false;
        if (missing) {
          return (
            <div
              className="text-cell translated-cell px-2"
              title="Linha inexistente no arquivo traduzido"
            >
              <span className={MISSING_SIDE}>—</span>
            </div>
          );
        }
        // Ref dedupada (link): texto da def em cor própria, com o
        // original dela no tooltip. O clique abre o editor NA DEF —
        // a tradução é registrada no arquivo da def, nunca aqui.
        const link = divergentDraft ? undefined : refLinks[rowKey(row)];
        if (link) {
          const defText = linkedValueOf(row) ?? link.text ?? '';
          // Fallback: original ausente (def sem pristine em data/)
          // degrada para o texto atual da def em vez de truncar o
          // tooltip ("Repetição de: " sem texto).
          const original = link.original || link.text || '';
          const defTranslated = defText !== '' && defText !== original;
          return (
            <LinkedCell
              defText={defText}
              original={original}
              defTranslated={defTranslated}
              readOnly={readOnly}
              marks={matchesByRow.get(rowKey(row))}
              onOpen={readOnly ? undefined : () => actions.openLinked(row)}
              onEditAsDivergent={() => actions.openLinked(row, true)}
            />
          );
        }
        const displayed = String(info.getValue() ?? '');
        const original = row.original?.[SOURCE_LANG];
        const canRevert =
          !readOnly &&
          textLoc === SOURCE_LANG &&
          original !== undefined &&
          original !== '' &&
          displayed !== original;
        // Linha órfã (só em mods): exclusão manual com aviso destrutivo.
        const onDelete =
          entry && row.missingInOriginal === true
            ? () =>
                askDeleteOrphan({
                  version,
                  kind: entry.kind,
                  id: entry.id,
                  source: entry.vbf?.root ?? '',
                  row,
                })
            : undefined;
        return (
          <EditableCell
            displayed={displayed}
            edited={edited}
            canRevert={canRevert}
            marks={matchesByRow.get(rowKey(row))}
            onOpen={readOnly ? undefined : () => actions.openDialog(row)}
            onRevert={() => actions.revertToOriginal(row)}
            onDelete={onDelete}
          />
        );
      },
    },
  );
}
