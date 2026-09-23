import { defineConfig } from 'vitest/config';

// `pnpm test:coverage` — acceptance for P02 (prompts/P02-schema-package.md): 100 % branch coverage on the primitives
// and the semantic validators. The shared helpers (pointer, json, diff, merge-patch, ui, ip, secrets) are held to the
// same bar. Thresholds are per group (D-048, review M5): the entries below are group (a)'s files; groups (b)/(c) add
// their own entries for their semantic files when they reach the bar.
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
        'src/semantic/index.ts': full,
        'src/semantic/registry.ts': full,
        'src/semantic/unique.ts': full,
        'src/semantic/system.ts': full,
        'src/semantic/dataplane.ts': full,
        'src/semantic/interfaces.ts': full,
        'src/semantic/vrfs.ts': full,
        'src/semantic/routing.ts': full,
        'src/semantic/management.ts': full,
        'src/pointer.ts': full,
        'src/json.ts': full,
        'src/diff.ts': full,
        'src/merge-patch.ts': full,
        'src/ui.ts': full,
        'src/validate.ts': full,
        'src/secrets.ts': full,
      },
    },
  },
});
