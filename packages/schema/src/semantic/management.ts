import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `management`.
 * TODO(P02a): at least one admin user; usernames unique; role assignments carry scope.
 * Each entry: `{ name: 'management.<rule>', domains: ['management', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02a edits this file.
 */
export const managementValidators: readonly ValidatorDefinition[] = [];
