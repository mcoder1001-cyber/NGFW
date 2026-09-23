import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `tunnels`.
 * TODO(P02c): tunnel endpoints/VRFs consistent with interfaces and vrfs; names unique per kind.
 * Each entry: `{ name: 'tunnels.<rule>', domains: ['tunnels', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02c edits this file.
 */
export const tunnelsValidators: readonly ValidatorDefinition[] = [];
