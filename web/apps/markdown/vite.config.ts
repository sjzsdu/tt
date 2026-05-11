import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:9595',
      '/raw': 'http://127.0.0.1:9595',
      '/save': 'http://127.0.0.1:9595',
      '/delete': 'http://127.0.0.1:9595',
      '/images': 'http://127.0.0.1:9595',
      '/ws': { target: 'ws://127.0.0.1:9595', ws: true },
    },
  },
});
