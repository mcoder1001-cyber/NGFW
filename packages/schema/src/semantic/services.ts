import type { ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `services`.
 * TODO(P02c): DHCP pools inside an interface subnet; DNS/SNMP listen addresses exist on an interface.
 * Each entry: `{ name: 'services.<rule>', domains: ['services', ...], validate }` returning `{ pointer, message }[]`
 * built with `jsonPointer()`. Only P02c edits this file.
 */
export const servicesValidators: readonly ValidatorDefinition[] = [];
