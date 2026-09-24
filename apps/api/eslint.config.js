// apps/api: the root config plus decorator awareness. Nest resolves constructor parameters through
// emitDecoratorMetadata, so a class used only as a constructor type is a runtime import; with these parser options
// typescript-eslint's consistent-type-imports stops asking for `import type` there (which would break DI).
import root from '../../eslint.config.js';

export default [
  ...root,
  {
    files: ['**/*.ts'],
    languageOptions: {
      parserOptions: { emitDecoratorMetadata: true, experimentalDecorators: true },
    },
  },
  {
    // e2e tests walk untyped JSON responses; `any` is confined to test/ (never in src/)
    files: ['test/**/*.ts'],
    rules: { '@typescript-eslint/no-explicit-any': 'off' },
  },
];
