import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `interfaces`.
 * TODO(P02a): interface references an existing VRF; no overlapping IPv4 on the same VRF; sub-interface VLAN ids unique per parent.
 * Each entry: `{ name: 'interfaces.<rule>', domains: ['interfaces', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02a edits this file.
 */
export const interfacesValidators: readonly ValidatorDefinition[] = [];
