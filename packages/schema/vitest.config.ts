import { defineConfig } from 'vitest/config';

// `pnpm test:coverage` — acceptance for P02 (prompts/P02-schema-package.md): 100 % branch coverage on the primitives
// and the semantic validators. The shared helpers (pointer, json, diff, merge-patch, ui, ip) are held to the same bar.
const full = { branches: 100, functions: 100, lines: 100, statements: 100 };

export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      include: ['src/**/*.ts'],
      exclude: ['src/**/*.test.ts', 'src/gen.ts'],
      reporter: ['text', 'json-summary'],
      thresholds: {
        'src/primitives.ts': full,
        'src/ip.ts': full,
        'src/semantic/**/*.ts': full,
        'src/pointer.ts': full,
        'src/json.ts': full,
        'src/diff.ts': full,
        'src/merge-patch.ts': full,
        'src/ui.ts': full,
        'src/validate.ts': full,
      },
    },
  },
});
