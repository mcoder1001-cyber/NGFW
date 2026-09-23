import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `nat`.
 * TODO(P02b): inside/outside interfaces exist; pools do not overlap; port-forward references resolve.
 * Each entry: `{ name: 'nat.<rule>', domains: ['nat', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02b edits this file.
 */
export const natValidators: readonly ValidatorDefinition[] = [];
