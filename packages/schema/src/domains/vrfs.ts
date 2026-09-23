import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `vrfs` — VRFs.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   record<name, { id (0..2^32-1), description }>
 *
 * Guardrail (vdom.md #1): VRF is first-class everywhere; the `default` VRF is an ordinary entry, not an implicit 0.
 *
 * TODO(P02a): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/vrfs.ts`. Only P02a edits this file.
 */
export const VrfsSchema = withUi(z.looseObject({}), {
  title: 'VRFs',
  description: 'VRF / FIB tables keyed by name.',
  order: 40,
});

export type VrfsConfig = z.infer<typeof VrfsSchema>;
