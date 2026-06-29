import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined;
          if (id.includes('mermaid') || id.includes('@terrastruct/d2')) return 'vendor-diagrams';
          if (id.includes('canvg') || id.includes('@resvg')) return 'vendor-export';
          if (id.includes('reveal.js')) return 'vendor-reveal';
          if (id.includes('marked') || id.includes('highlight.js')) return 'vendor-markdown';
          if (id.includes('react') || id.includes('react-dom')) return 'vendor-react';
          return 'vendor-misc';
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:9596',
      '/raw': 'http://127.0.0.1:9596',
      '/images': 'http://127.0.0.1:9596',
      '/ws': { target: 'ws://127.0.0.1:9596', ws: true },
    },
  },
});
