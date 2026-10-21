#!/usr/bin/env node
/**
 * Sincroniza a fonte única de versão — o arquivo VERSION na raiz — para os
 * dois lugares que precisam dela além do binário:
 *
 *   build/windows/info.json  → file_version / ProductVersion do .exe
 *                               (entra no .syso via `wails3 generate syso -info`)
 *   frontend/package.json    → "version"
 *
 * Idempotente: só regrava quando o valor muda, preservando a indentação de
 * cada arquivo (tabs no info.json, 2 espaços no package.json) e o EOL.
 *
 * Uso:
 *   node scripts/version-sync.mjs      (direto)
 *   wails3 task version:sync           (task; é dep de `build` e `release`)
 *
 * A versão que o UPDATER enxerga NÃO vem daqui: é o -ldflags
 * "-X main.currentVersion=…" que o Taskfile injeta a partir do mesmo VERSION.
 */
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");

const version = readFileSync(join(root, "VERSION"), "utf8").trim();
if (!/^\d+\.\d+\.\d+(?:-[\w.]+)?$/.test(version)) {
  console.error(
    `VERSION inválido: "${version}" — esperado semântico tipo 2.1.1 (sem "v").`
  );
  process.exit(1);
}

const changed = [];

/** Reescreve um JSON só se apply() mexer em algo, mantendo indent e EOL. */
function syncJSON(relPath, apply, indent) {
  const path = join(root, relPath);
  const raw = readFileSync(path, "utf8");
  const data = JSON.parse(raw);
  const before = JSON.stringify(data);
  apply(data);
  if (JSON.stringify(data) === before) return;

  const eol = raw.includes("\r\n") ? "\r\n" : "\n";
  const body = JSON.stringify(data, null, indent).replace(/\n/g, eol);
  writeFileSync(path, body + eol);
  changed.push(relPath);
}

syncJSON(
  "build/windows/info.json",
  (info) => {
    info.fixed = info.fixed ?? {};
    info.fixed.file_version = version;
    info.info = info.info ?? {};
    info.info["0000"] = { ...(info.info["0000"] ?? {}), ProductVersion: version };
  },
  "\t"
);

syncJSON(
  "frontend/package.json",
  (pkg) => {
    pkg.version = version;
  },
  2
);

console.log(
  changed.length
    ? `VERSION ${version} → sincronizado: ${changed.join(", ")}`
    : `VERSION ${version} já sincronizada`
);
