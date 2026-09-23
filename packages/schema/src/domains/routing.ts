import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `routing` — Routing.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   static[] { prefix, nextHops[] { address, interface?, weight }, vrf }
 *   prefixLists / routeMaps records (policy skeleton); bgp, ospf, isis, rip, bfd as typed-but-optional objects
 *
 * Guardrail (vdom.md #1): every route carries `vrf`; semantic validators check next-hop interfaces exist.
 *
 * TODO(P02a): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/routing.ts`. Only P02a edits this file.
 */
export const RoutingSchema = withUi(z.looseObject({}), {
  title: 'Routing',
  description: 'Static routes and dynamic routing protocols (FRR).',
  order: 50,
});

export type RoutingConfig = z.infer<typeof RoutingSchema>;
