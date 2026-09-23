import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `acl`.
 * TODO(P02b): attachments reference existing lists and interfaces; rules reference existing objects.
 * Each entry: `{ name: 'acl.<rule>', domains: ['acl', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02b edits this file.
 */
export const aclValidators: readonly ValidatorDefinition[] = [];
