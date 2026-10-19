import { GameTextView } from '@/components/game-text-view';
import { columnHelper, MISSING_SIDE } from '../table-types';

/**
 * Coluna "Original": o texto do lado de fora, na língua do binário clicado
 * (`textLoc`), nunca o 'us' fixo.
 */
export function buildOriginalColumn(textLoc: string) {
  return columnHelper.accessor((row) => row.original?.[textLoc], {
    id: 'original',
    header: 'Original',
    cell: (info) => {
      const value = info.getValue();
      // União: linha que só existe na tradução fica sem Original e é
      // marcada — o traço vermelho diz "falta aqui" sem mostrar texto
      // do outro lado. Sem contraparte o valor é undefined → "—".
      // NUNCA cai para row.text: seria exibir a tradução como original.
      const missing = info.row.original.missingInOriginal === true;
      return (
        <GameTextView
          text={value ?? ''}
          fallback={value === undefined ? '—' : ''}
          // .text-cell na célula (e não no <td>): com o layout fixo quem
          // precisa do wrap/quebra é o conteúdo — o <td> tem
          // whitespace-nowrap (camada utilities) que venceria o pre-wrap.
          className={`text-cell${missing ? ` ${MISSING_SIDE}` : ''}`}
          // Chip de quebra real: só a tabela marca LF (o resto da UI não).
          newlineChip
        />
      );
    },
  });
}
