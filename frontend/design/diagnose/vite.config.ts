import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  publicDir: false,
  plugins: [react()],
  build: { outDir: '/tmp/diagnose-mock-dist', emptyOutDir: true },
  server: { host: '127.0.0.1', port: 18496, strictPort: true },
});
