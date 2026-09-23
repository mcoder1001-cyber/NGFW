import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `system`.
 * TODO(P02a): timezone is a known IANA zone; hostname per RFC 1123 (structural rules live in the Zod schema).
 * Each entry: `{ name: 'system.<rule>', domains: ['system', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02a edits this file.
 */
export const systemValidators: readonly ValidatorDefinition[] = [];
