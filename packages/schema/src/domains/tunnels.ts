import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `tunnels` — Tunnels.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   gre { ... }, vxlan { ... }, ipip { ... } — records keyed by tunnel name
 *
 * Guardrail (vdom.md #1): underlay/overlay `vrf` fields are explicit.
 *
 * TODO(P02c): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/tunnels.ts`. Only P02c edits this file.
 */
export const TunnelsSchema = withUi(z.looseObject({}), {
  title: 'Tunnels',
  description: 'GRE, VXLAN and IPIP tunnels.',
  order: 100,
});

export type TunnelsConfig = z.infer<typeof TunnelsSchema>;
