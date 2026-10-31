/// <reference types="vitest" />

import path from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  resolve: {
    // A ordem importa: o alias é prefixo e o PRIMEIRO que casa vence — com
    // '@' antes, '@/bindings/...' viraria 'src/bindings/...' (inexistente) e
    // qualquer import de runtime dos bindings (o catálogo de tags do editor,
    // por exemplo) quebrava o teste.
    alias: {
      '@/bindings': path.resolve('./bindings'),
      '@': path.resolve('./src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    alias: {
      '@/bindings': path.resolve('./bindings'),
      '@': path.resolve('./src'),
    },
    deps: {
      inline: ['@/lib/ffx/**'],
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      exclude: [
        '**/node_modules/**',
        '**/*.d.ts',
        '**/*.config.*',
        'vitest.setup.ts',
        '**/bindings/**',
        '**/*.generated.ts',
      ],
    },
  },
});
