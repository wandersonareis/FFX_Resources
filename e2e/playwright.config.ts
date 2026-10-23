import { execFileSync } from 'node:child_process';
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
 * O servidor é EXCLUSIVO da suíte: o Playwright sobe o exe, espera o
 * /health e ENCERRA o processo no fim do run, mesmo em falha. Porta
 * ocupada no início ABORTA a suíte antes de subir qualquer coisa —
 * servidor aberto é o usuário usando (um run:server manual valeria um
 * binário velho como alvo). A barreira é dupla: a precondition da task
 * `test:e2e` (mensagem em português, ANTES do build) e a probe deste
 * config no runner principal.
 *
 * O caminho do exe é ABSOLUTO e com aspas: o Playwright lança o comando
 * com shell:true (cmd /c no Windows), que não resolve caminho relativo
 * em barras normais.
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

/**
 * Probe síncrona de porta: exit 0 = algo escutando. Via node -e porque
 * não há socket síncrono nativo — e o node já é pré-requisito da suíte.
 */
function portInUse(port: number): boolean {
  const probe = [
    `const net = require('net');`,
    `const s = net.connect(${port}, '127.0.0.1');`,
    `s.on('connect', () => { s.destroy(); process.exit(0); });`,
    `s.on('error', () => process.exit(1));`,
    `setTimeout(() => process.exit(1), 2000).unref();`,
  ].join(' ');
  try {
    execFileSync(process.execPath, ['-e', probe], { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

// A guarda vale SÓ no runner principal: loader e workers são forkados com
// *ProcessEntry.js e reavaliam este config DEPOIS de o webServer já ter
// subido o NOSSO servidor — neles, porta ocupada é o próprio servidor da
// suíte, não um alheio. No runner principal a probe roda antes de qualquer
// webServer: ocupada = servidor do usuário, aborta com a instrução.
if (!/ProcessEntry\.js$/.test(process.argv[1] ?? '') && portInUse(PORT)) {
  throw new Error(
    `Porta ${PORT} ocupada — há um spira-server.exe (ou o app em modo ` +
      `servidor) aberto. A suíte não reusa servidor: encerre o processo e ` +
      `reexecute os testes.`
  );
}

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
    // Readiness pelo /health (e NÃO pela porta crua): o listener aceita
    // conexões ANTES de o app terminar de carregar o config — esperar só
    // a porta já fez os specs rodarem contra um GameFilesRoot default e
    // quebrar na primeira listagem de events. Sem reuso: o processo que
    // o Playwright SOBE ele mesmo ENCERRA no fim do run (mesmo em
    // falha) — porta ocupada DEPOIS dos testes é vazamento.
    url: `http://localhost:${PORT}/health`,
    reuseExistingServer: false,
    timeout: 60_000,
    stdout: 'ignore',
    stderr: 'ignore',
  },
});
