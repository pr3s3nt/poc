import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({ base: '/ui/', plugins: [react()], build: { outDir: '/tmp/orchestrator-workload-catalog-mock', emptyOutDir: true, rollupOptions: { input: 'catalog-mock.html' } } });
