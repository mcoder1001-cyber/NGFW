import { z } from 'zod';
import { descriptionText, objectName, uint32 } from '../primitives.js';
import { withUi } from '../ui.js';
import { vrfSourceSelect } from './ext/vrf-static-ecmp.js';
import { proxyArpRangesField } from './ext/neighbors-ra.js';

/**
 * `vrfs` — VRF / FIB tables keyed by name (docs/04-api-datamodel.md).
 *
 * Guardrail (vdom.md #1): VRF is first-class on every object that routes; nothing assumes table 0.
 * The `default` VRF is table 0 and always exists on VPP. It MAY be declared here (to give it a description) and
 * then must carry `id: 0`; references to `default` resolve whether or not it is declared (semantic
 * `vrfs.default-is-table-zero`, `interfaces.vrf-exists`, …). Every other VRF must be declared with a unique id
 * (`vrfs.id-unique`). VPP creates both an IPv4 and an IPv6 FIB with that id.
 */
export const VrfSchema = z.strictObject({
  id: withUi(uint32, {
    title: 'Table ID',
    help: 'VPP FIB table id (0 is the default VRF)',
    order: 1,
  }),
  description: withUi(descriptionText.optional(), { title: 'Description', order: 2 }),
  // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
  // wave-A: F-vrf-static-ecmp
  sourceSelect: vrfSourceSelect,
  // wave-A: F-neighbors-ra
  proxyArpRanges: proxyArpRangesField,
});
export type VrfConfig = z.infer<typeof VrfSchema>;

export const VrfsSchema = withUi(z.record(objectName, VrfSchema), {
  title: 'VRFs',
  description: 'VRF / FIB tables keyed by name.',
  order: 40,
});

export type VrfsConfig = z.infer<typeof VrfsSchema>;

/** Name of the always-present VRF (VPP table 0). */
export const DEFAULT_VRF = 'default';

/** True when `name` can be referenced as a VRF: declared under `/vrfs`, or the implicit `default`. */
export function vrfExists(vrfs: VrfsConfig, name: string): boolean {
  return name === DEFAULT_VRF || Object.hasOwn(vrfs, name);
}
