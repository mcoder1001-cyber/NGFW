import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `nat` — NAT.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   mode (endpoint-dependent | ...), inside[], outside[], static[], portForwards[], cgnat { ... }
 *
 * Guardrail (vdom.md #1): NAT inside/outside bindings carry `vrf` — never assume VRF 0.
 *
 * TODO(P02b): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/nat.ts`. Only P02b edits this file.
 */
export const NatSchema = withUi(z.looseObject({}), {
  title: 'NAT',
  description: 'NAT44-ED, static NAT, port forwards and CGNAT.',
  order: 60,
});

export type NatConfig = z.infer<typeof NatSchema>;
