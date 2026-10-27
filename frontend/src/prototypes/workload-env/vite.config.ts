import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Run from frontend/. This build is separate from the shipped Console.
export default defineConfig({
  base: '/ui/',
  plugins: [react()],
  build: {
    outDir: '/tmp/orchestrator-workload-env-mock',
    emptyOutDir: true,
    rollupOptions: { input: 'workload-mock.html' },
  },
});
