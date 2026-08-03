import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    // Next to the daemon's 9374 rather than on it: taking 9374 meant the dev
    // server and the thing it proxies to wanted the same socket, so the daemon
    // had to be moved aside and mycel appeared to have two ports.
    //
    // Not vite's 5173 default either, which is the most contested port on a
    // machine that runs more than one project — and losing it is silent, so you
    // end up reading another project's app while debugging mycel.
    port: 9375,
    // Fail loudly instead of drifting to 9376 and leaving stale tabs pointed at
    // a port nothing is serving.
    strictPort: true,
    proxy: {
      // The daemon on its default port, so a plain `mycel up` needs no
      // arrangement. MYCEL_API_PROXY overrides it for a daemon elsewhere.
      //
      // 127.0.0.1, not localhost: the daemon binds IPv4 only, while Node
      // resolves localhost to ::1 first and does not fall back. Naming the
      // family the daemon actually listens on is the difference between a
      // working dev server and one stuck on "connecting to the mycel daemon".
      '/api': process.env.MYCEL_API_PROXY ?? 'http://127.0.0.1:9374',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
});
