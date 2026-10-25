import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// The console is served by the Go backend under /ui/ and calls /api/v1 on the
// same origin. In development Vite proxies /api to the orchestrator process.
const apiOrigin = process.env.ORCHESTRATOR_API_ORIGIN ?? 'http://127.0.0.1:8080';

export default defineConfig({
  base: '/ui/',
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: { '/api': { target: apiOrigin, changeOrigin: false } },
  },
  build: { outDir: 'dist', emptyOutDir: true },
  test: {
    // The suite drives long, realistic user flows; a loaded host must not turn a
    // slow-but-correct test into a failure.
    testTimeout: 30_000,
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
  },
});
