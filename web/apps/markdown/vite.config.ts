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
          if (id.includes('mermaid') || id.includes('cytoscape') || id.includes('katex')) return 'vendor-diagrams';
          if (id.includes('canvg') || id.includes('@resvg')) return 'vendor-export';
          if (id.includes('antd') || id.includes('@ant-design') || id.includes('rc-')) return 'vendor-antd';
          if (id.includes('marked') || id.includes('github-markdown-css')) return 'vendor-markdown';
          if (id.includes('react') || id.includes('react-dom')) return 'vendor-react';
          return 'vendor-misc';
        },
      },
    },
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
