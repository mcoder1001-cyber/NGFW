import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `interfaces` — Interfaces.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   record<vppInterfaceName, { enabled, description, mtu (68–9216), mac?, ipv4[] CIDR, ipv6[] CIDR, vrf,
 *     rxMode (polling|interrupt|adaptive), subinterfaces: record<name, { vlanId, innerVlanId?, ...addresses }>,
 *     unnumbered? }>
 *
 * Guardrail (docs/decisions/vdom.md #1): `vrf` is a first-class field on every interface — never assume VRF 0.
 * VPP interface names contain `/` (TenGigabitEthernet0/0/0): JSON pointers into this record MUST use
 * `jsonPointer()` from `../pointer.js` so the slashes are escaped as `~1`.
 *
 * TODO(P02a): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/interfaces.ts`. Only P02a edits this file.
 */
export const InterfacesSchema = withUi(z.looseObject({}), {
  title: 'Interfaces',
  description: 'Physical, virtual and sub-interfaces keyed by VPP interface name.',
  order: 30,
});

export type InterfacesConfig = z.infer<typeof InterfacesSchema>;
