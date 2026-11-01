import { splitLeadingPadding } from './leading-padding';

/**
 * Texto canônico do jogo -> texto PURO para o clipboard.
 *
 * O display (GameTextView) converte tags em chips, cores e itálico; aqui é o
 * oposto: tudo que é tag de controle sai, sobrando só o que se lê. Quebras
 * ({TEXT_NEWLINE} e \r\n/\r/\n) viram `\n` para a formatação sobreviver ao
 * copiar/colar.
 *
 * O padding de tela do início (espaços e quebras usados para alinhar) fica
 * de fora, igual à exibição — senão o texto copiado começaria com linhas em
 * branco. Detalhe: o parser trata QUALQUER `{…}` como chip, então remover
 * todos os `{…}` é fiel ao que a área mostra (nada de tag "crua" sobra).
 */
export function gameTextToPlainText(text?: string): string {
  const { body } = splitLeadingPadding(text ?? '');
  return body
    .replace(/\{TEXT_NEWLINE\}/gi, '\n')
    .replace(/\r\n?/g, '\n')
    .replace(/\{[^{}]+\}/g, '');
}

/** Uma referência (idioma) do diálogo de tradução. */
export interface ReferenceForCopy {
  /** Nome de exibição do idioma (ex.: "Alemão"). */
  name: string;
  /** Código do idioma (ex.: "de"). */
  code: string;
  /** Texto canônico da linha naquele idioma (row.original[code]). */
  text?: string;
}

/**
 * Monta o conteúdo copiado da área de referências: todas as línguas na ordem
 * exibida, cada uma com o MESMO cabeçalho da tela (`Nome (código)`) seguido do
 * texto puro. Referências sem texto (vazias ou só com tags) ficam de fora —
 * um cabeçalho solto não ajuda quem vai colar.
 */
export function formatReferencesForCopy(references: readonly ReferenceForCopy[]): string {
  const blocks: string[] = [];
  for (const reference of references) {
    const plain = gameTextToPlainText(reference.text);
    if (plain.trim() === '') continue;
    blocks.push(`${reference.name} (${reference.code})\n${plain}`);
  }
  return blocks.join('\n\n');
}
