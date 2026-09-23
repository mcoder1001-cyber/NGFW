import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `services` — Services.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   dhcp { ... }, dns { ... }, snmp { ... }, lldp { ... }, ipfix { ... }
 *
 * Guardrail (vdom.md #5): model so that "one Kea/Unbound instance per <vrf|interface>" is a parameter, not an
 * assumption baked into the shape.
 *
 * TODO(P02c): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/services.ts`. Only P02c edits this file.
 */
export const ServicesSchema = withUi(z.looseObject({}), {
  title: 'Services',
  description: 'Network services: DHCP (Kea), DNS (Unbound), SNMP, LLDP, IPFIX.',
  order: 110,
});

export type ServicesConfig = z.infer<typeof ServicesSchema>;
