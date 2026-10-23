import * as fs from 'node:fs';
import * as path from 'node:path';
import { defineConfig } from '@playwright/test';

/**
 * E2E contra o app em MODO SERVIDOR (Wails v3, -tags server): o mesmo
 * binário do desktop sobe como HTTP puro em http://localhost:9246 (ver
 * ServerOptions no main.go) servindo o export do frontend, com bindings e
 * eventos atravessando /wails/runtime no mesmo origem — o navegador É a
 * janela.
 *
 * O servidor é o próprio `build/bin/spira-server.exe` (construído por
 * `wails3 task build:server` — a task `test:e2e` faz o build antes). O
 * webServer sobe o exe e espera o /health do Wails.
 *
 * O caminho do exe é ABSOLUTO e com aspas: o Playwright lança o comando
 * com shell:true (cmd /c no Windows), que não resolve caminho relativo em
 * barras normais.
 */
const PORT = Number(process.env.E2E_PORT ?? 9246);
const BASE_URL = `http://localhost:${PORT}`;
const SERVER_EXE_PROD = path.resolve(__dirname, '..', 'build', 'bin', 'spira-server.exe');
// `wails3 run:server` (DEV=true) grava <app>-server-dev.exe; o build de
// produção grava <app>-server.exe. Usa o que existir.
const SERVER_EXE_DEV = path.resolve(
  __dirname,
  '..',
  'build',
  'bin',
  'spira-server-dev.exe'
);
const SERVER_EXE = fs.existsSync(SERVER_EXE_PROD) ? SERVER_EXE_PROD : SERVER_EXE_DEV;

export default defineConfig({
  testDir: './specs',
  // O app é UM backend compartilhado (caches, config, .vbf abertos): os
  // specs precisam rodar em série para um não invalidar o estado do outro.
  fullyParallel: false,
  workers: 1,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  retries: 0,
  reporter: [['list']],
  outputDir: '../.e2e-artifacts/test-results',
  use: {
    baseURL: BASE_URL,
    locale: 'pt-BR',
    // Mesmas dimensões da janela nativa do main.go.
    viewport: { width: 1280, height: 800 },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    actionTimeout: 15_000,
  },
  webServer: {
    command: `"${SERVER_EXE}"`,
    url: `http://localhost:${PORT}/health`,
    // Reusa um `wails3 task run:server` já no ar (o build precisa estar
    // atualizado por conta de quem o subiu).
    reuseExistingServer: true,
    timeout: 60_000,
    stdout: 'ignore',
    stderr: 'ignore',
  },
});
