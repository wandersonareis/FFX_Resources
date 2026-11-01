/**
 * Escrita no clipboard com fallback para o WebView.
 *
 * `navigator.clipboard.writeText` é o caminho normal (exige contexto seguro e
 * documento focado); quando a permissão/foco nega, cai no textarea +
 * `execCommand('copy')`, que continua funcionando no WebView2. Devolve
 * `false` quando NENHUMA via aceitou — o chamador decide o aviso.
 */
export async function copyTextToClipboard(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Sem permissão/foco: tenta o fallback abaixo.
  }

  if (typeof document === 'undefined') return false;

  try {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    // readOnly evita teclado virtual e seleção visível no WebView.
    textarea.setAttribute('readonly', '');
    textarea.style.position = 'fixed';
    textarea.style.top = '-9999px';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    textarea.setSelectionRange(0, text.length);
    const copied = document.execCommand('copy');
    textarea.remove();
    return copied;
  } catch {
    return false;
  }
}
