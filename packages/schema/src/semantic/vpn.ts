import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `vpn`.
 * TODO(P02c): tunnels reference existing proposals, interfaces and VRFs; secretRef present, no inline PSK.
 * Each entry: `{ name: 'vpn.<rule>', domains: ['vpn', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02c edits this file.
 */
export const vpnValidators: readonly ValidatorDefinition[] = [];
