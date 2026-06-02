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
          if (id.includes('@xyflow') || id.includes('d3') || id.includes('dagre')) return 'vendor-graph';
          if (id.includes('mermaid') || id.includes('cytoscape') || id.includes('katex')) return 'vendor-diagrams';
          if (id.includes('marked') || id.includes('github-markdown-css')) return 'vendor-markdown';
          if (id.includes('antd') || id.includes('@ant-design') || id.includes('rc-')) return 'vendor-antd';
          if (id.includes('react') || id.includes('react-dom')) return 'vendor-react';
          return 'vendor-misc';
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:9705',
      '/ws': { target: 'ws://127.0.0.1:9705', ws: true },
    },
  },
});
