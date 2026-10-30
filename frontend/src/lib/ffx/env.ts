/**
 * Detecta execução dentro do WebView do Wails (desktop) vs browser puro
 * (web / server mode).
 *
 * No v2 era `window.runtime`/`window.go`. No v3 o `@wailsio/runtime` escolhe o
 * transporte em tempo de carga (system.js): WebView2 (`window.chrome.webview`),
 * WKWebView (`window.webkit.messageHandlers.external`) ou fetch HTTP (server
 * mode). Mesma checagem aqui — e a decisão de UX é: janela filha no desktop,
 * modal no browser.
 */
export function isDesktop(): boolean {
  if (typeof window === 'undefined') return false;
  const w = window as Window & {
    chrome?: { webview?: { postMessage?: (data: unknown) => void } };
    webkit?: {
      messageHandlers?: {
        external?: { postMessage?: (data: unknown) => void };
      };
    };
    wails?: { invoke?: unknown };
  };
  return Boolean(
    w.chrome?.webview?.postMessage ||
      w.webkit?.messageHandlers?.external?.postMessage ||
      w.wails?.invoke,
  );
}
