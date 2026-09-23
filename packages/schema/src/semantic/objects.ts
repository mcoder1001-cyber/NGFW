import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `objects`.
 * TODO(P02b): group members reference existing objects of the same kind; no cycles in address groups.
 * Each entry: `{ name: 'objects.<rule>', domains: ['objects', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02b edits this file.
 */
export const objectsValidators: readonly ValidatorDefinition[] = [];
