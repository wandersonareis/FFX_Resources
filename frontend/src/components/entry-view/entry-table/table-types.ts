import { createColumnHelper, tableFeatures } from '@tanstack/react-table';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';

// Infra compartilhada da tabela (TanStack v9: features + columnHelper).
export const features = tableFeatures({});

export type F = typeof features;

export const columnHelper = createColumnHelper<F, dto.TextRow>();

/**
 * Célula de uma linha que SÓ EXISTE num dos dois lados (união da tabela):
 * o traço vermelho diz "falta aqui" sem exibir conteúdo do outro lado.
 */
export const MISSING_SIDE = 'text-red-600 dark:text-red-400';
