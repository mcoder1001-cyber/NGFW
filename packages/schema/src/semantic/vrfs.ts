import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `vrfs`.
 * TODO(P02a): VRF ids unique; the `default` VRF present.
 * Each entry: `{ name: 'vrfs.<rule>', domains: ['vrfs', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02a edits this file.
 */
export const vrfsValidators: readonly ValidatorDefinition[] = [];
