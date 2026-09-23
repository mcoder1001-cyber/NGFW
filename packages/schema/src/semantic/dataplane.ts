import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `dataplane`.
 * TODO(P02a): mainCore not in corelist; workers ≤ cores; pciWhitelist entries unique.
 * Each entry: `{ name: 'dataplane.<rule>', domains: ['dataplane', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02a edits this file.
 */
export const dataplaneValidators: readonly ValidatorDefinition[] = [];
