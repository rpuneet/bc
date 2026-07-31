import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 9374,
    proxy: {
      // MYCEL_API_PROXY lets dev sessions point at any running daemon
      // (e.g. http://127.0.0.1:8080) without editing this file.
      '/api': process.env.MYCEL_API_PROXY ?? 'http://localhost:9375',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
});
