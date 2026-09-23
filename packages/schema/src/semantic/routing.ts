import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `routing`.
 * TODO(P02a): static route next-hop interface exists; route VRF exists; prefix-list / route-map references resolve.
 * Each entry: `{ name: 'routing.<rule>', domains: ['routing', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02a edits this file.
 */
export const routingValidators: readonly ValidatorDefinition[] = [];
