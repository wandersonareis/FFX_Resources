import * as dto from '@/bindings/ffxresources/backend/dto/models';

/**
 * Regras de seleção do modal de importação de imagens — funções puras, para
 * o hook (que aplica) e o dialog (que conta) derivarem o MESMO resultado:
 *
 * - o checkbox da linha controla SÓ a imagem (o estado visual é o agregado
 *   imagem + cópias, daí os 3 estados);
 * - apenas cópias marcadas são importadas;
 * - imagem igual ao binário (`!differs`) com cópias marcadas é reimportada
 *   JUNTO com as cópias — evita o grupo dividido entre o conteúdo traduzido
 *   (nas cópias) e o binário antigo (na imagem);
 * - entrada com erro de validação nunca entra na seleção.
 */

/** A imagem entra na seleção? (a regra acima em uma expressão.) */
export function entryWritesImage(entry: dto.ImageImportPreviewEntry): boolean {
  if (entry.validation_error) return false;
  if (entry.selected) return true;
  return !entry.differs && entryMarkedCopies(entry).length > 0;
}

/** Cópias marcadas de uma entrada (ids). */
export function entryMarkedCopies(entry: dto.ImageImportPreviewEntry): string[] {
  return (entry.copies ?? []).filter((copy) => copy.selected).map((copy) => copy.id);
}

/** Total de gravações que a seleção atual provoca (imagens + cópias). */
export function plannedWrites(preview: dto.ImageImportPreview): number {
  let total = 0;
  for (const entry of preview.entries) {
    if (entryWritesImage(entry)) total += 1;
    total += entryMarkedCopies(entry).length;
  }
  return total;
}

/**
 * Estado do "Todas" (3 estados): marcado só quando TODAS as imagens
 * selecionáveis e todas as cópias estão marcadas. Entradas com erro nunca
 * são selecionáveis.
 */
export function allSelectedState(
  preview: dto.ImageImportPreview
): boolean | 'indeterminate' {
  const selectable = preview.entries.filter((e) => !e.validation_error);
  if (selectable.length === 0) return false;
  const allMarked = selectable.every(
    (e) => e.selected && (e.copies ?? []).every((copy) => copy.selected)
  );
  if (allMarked) return true;
  const noneMarked = selectable.every(
    (e) => !e.selected && (e.copies ?? []).every((copy) => !copy.selected)
  );
  return noneMarked ? false : 'indeterminate';
}

/** Monta o payload do apply a partir do estado do modal. */
export function buildImageImportSelection(
  preview: dto.ImageImportPreview
): dto.ImageImportSelection {
  const entries: dto.ImageImportSelectionEntry[] = [];
  for (const entry of preview.entries) {
    if (entry.validation_error) continue;
    const copies = entryMarkedCopies(entry);
    const image = entryWritesImage(entry);
    if (!image && copies.length === 0) continue;
    entries.push({ id: entry.id, image, copies });
  }
  return { entries };
}

/**
 * Próximo estado do "Todas" no clique: desmarcado ou indeterminate marca
 * tudo; marcado desmarca tudo.
 */
export function nextAllSelected(preview: dto.ImageImportPreview): boolean {
  return allSelectedState(preview) !== true;
}
