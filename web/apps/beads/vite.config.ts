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
          if (id.includes('@antv/g6') || id.includes('@antv')) return 'vendor-graph';
          if (id.includes('antd') || id.includes('@ant-design') || id.includes('rc-')) return 'vendor-antd';
          if (id.includes('react') || id.includes('react-dom')) return 'vendor-react';
          return 'vendor-misc';
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:9720',
    },
  },
});
