import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `ha`.
 * TODO(P02c): VRRP interfaces exist; VRIDs unique per interface.
 * Each entry: `{ name: 'ha.<rule>', domains: ['ha', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02c edits this file.
 */
export const haValidators: readonly ValidatorDefinition[] = [];
