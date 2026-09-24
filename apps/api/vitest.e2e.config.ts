import swc from 'unplugin-swc';
import { defineConfig } from 'vitest/config';

// e2e (host PostgreSQL + Valkey + in-process fake agent) and the agent integration test. Never part of `pnpm test`.
// Files run one after another: they share the slot's database (vrx_<prefix>), which global-setup creates and drops.
export default defineConfig({
  test: {
    include: ['test/e2e/**/*.e2e.test.ts', 'test/integration/**/*.int.test.ts'],
    environment: 'node',
    globalSetup: ['test/support/global-setup.ts'],
    fileParallelism: false,
    testTimeout: 30_000,
    hookTimeout: 60_000,
  },
  plugins: [swc.vite({ module: { type: 'es6' } })],
});
