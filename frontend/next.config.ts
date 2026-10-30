import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  output: 'export',
  // Export com barra final: cada rota sai como <rota>/index.html em vez de
  // <rota>.html. Obrigatório para o asset server do Wails v3 — o export do
  // Next também cria o DIRETÓRIO <rota>/ (metadados RSC __next.*) ao lado do
  // <rota>.html, e o handler do Wails (asset_fileserver.go) responde 500
  // ("a directory has been requested without a trailing slash") quando a
  // URL aponta para diretório sem terminar em "/". As janelas filhas abrem
  // em "/w/#<nome>" (appwindows.go); no dev nada disso aparece porque o
  // next dev serve a rota direto — o erro só existia no exe de release.
  trailingSlash: true,
  reactCompiler: true,
  experimental: {
    turbopackGc: true,
    turbopackRustReactCompiler: true,
    turbopackLazyDynamicImports: true,
    // Os bindings gerados pelo Wails importam com sufixo ESM
    // (`from "./backend/dto/models.js"`) apontando para arquivos .ts.
    // Sem isso o Turbopack não resolve o specifier e o build quebra.
    extensionAlias: {
      '.js': ['.ts', '.tsx', '.js'],
    },
  },
  images: {
    unoptimized: true,
  },
};

export default nextConfig;
