import swc from 'unplugin-swc';
import { defineConfig } from 'vitest/config';

// NestJS needs emitDecoratorMetadata, which esbuild cannot do — swc handles the transform.
export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
    // Load-tolerant (TD-12): defaults (5 s test, 10 s hook) fired on the quick merge gate under host
    // load; raised to match apps/web.
    testTimeout: 30_000,
    hookTimeout: 30_000,
    teardownTimeout: 30_000,
    // "[vitest-worker]: Timeout calling 'fetch'/'transform'" (birpc's own 60 s default, not exposed as
    // a test.*Timeout) is the worker pool asking the main thread to transform a module and getting
    // starved of CPU before it answers — reproduced verbatim on this host at load ~20 with the default
    // (vitest's own) maxForks of "cpus - 1". Up to 12 worker slots run this suite at once on the shared
    // host (docs/lab/shared-host-rules.md), so an unbounded fork count multiplies into hundreds of
    // processes contending for 32 cores. Capping it keeps each fork's transform call answering well
    // inside the 60 s RPC timeout.
    maxWorkers: 4,
  },
  plugins: [swc.vite({ module: { type: 'es6' } })],
});
