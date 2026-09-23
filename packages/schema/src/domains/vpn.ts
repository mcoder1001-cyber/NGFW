import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `vpn` — VPN.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   ipsec { tunnels { ... }, proposals { ... } }, wireguard { ... }
 *
 * Guardrail (vdom.md #1): IPsec tunnels carry `vrf`. Secrets rule (00-CONTEXT #10): PSKs and private keys are
 * referenced by `secretRef`, never stored inline in this document, never returned by GET, never logged.
 *
 * TODO(P02c): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/vpn.ts`. Only P02c edits this file.
 */
export const VpnSchema = withUi(z.looseObject({}), {
  title: 'VPN',
  description: 'IPsec (strongSwan) and WireGuard.',
  order: 90,
});

export type VpnConfig = z.infer<typeof VpnSchema>;
