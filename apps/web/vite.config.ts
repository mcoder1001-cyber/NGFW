import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// Slot ports come from the environment (docs/lab/shared-host-rules.md): never hardcode 3000/5173 for a worker.
const apiPort = process.env.VRX_HTTP_PORT ?? '3000';
const webPort = Number(process.env.VRX_WEB_PORT ?? '5173');

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  // Developer demo routes (/dev/*) never ship in a production build unless explicitly requested (review P07a M1).
  define: { __VRX_DEV_ROUTES__: JSON.stringify(mode !== 'production' || process.env.VITE_VRX_DEV_ROUTES === '1') },
  server: {
    host: '127.0.0.1',
    port: webPort,
    strictPort: true,
    proxy: { '/api': { target: `http://127.0.0.1:${apiPort}`, ws: true, changeOrigin: false } },
  },
  preview: { host: '127.0.0.1', port: webPort, strictPort: true },
  build: { sourcemap: true, target: 'es2022' },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    testTimeout: 30_000,
    hookTimeout: 30_000,
  },
}));
