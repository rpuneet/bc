import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    // Vite's own default. 9374 is the daemon's port: taking it here meant the
    // dev server and the thing it proxies to wanted the same socket, so the
    // daemon had to be moved aside to a second port and mycel appeared to have
    // two of them.
    port: 5173,
    proxy: {
      // The daemon on its default port, so a plain `mycel up` needs no
      // arrangement. MYCEL_API_PROXY overrides it for a daemon elsewhere.
      '/api': process.env.MYCEL_API_PROXY ?? 'http://localhost:9374',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
});
