import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `ha` — High availability.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   vrrp[] { ... }
 *
 * Notes: keepalived/VRRP is a separate GPL process driven by rendered config (00-CONTEXT rule 7).
 *
 * TODO(P02c): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/ha.ts`. Only P02c edits this file.
 */
export const HaSchema = withUi(z.looseObject({}), {
  title: 'High availability',
  description: 'VRRP virtual routers.',
  order: 120,
});

export type HaConfig = z.infer<typeof HaSchema>;
