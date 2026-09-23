import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    // The shared CI host runs up to 12 workers; jsdom + MUI renders are slow there.
    testTimeout: 30_000,
    hookTimeout: 30_000,
  },
});
