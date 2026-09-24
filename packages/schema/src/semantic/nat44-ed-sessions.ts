import { jsonPointer } from '../pointer.js';
import { poolRange } from './nat.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-nat44-ed-sessions rules (tier b), in their own file (wave-A-hotspots C2; `semantic/nat.ts` stays P02b's):
 *
 *   nat.nat44-ed-sessions-adjacent-pools  two NAT44 range pools of the same twice-NAT class and tenant VRF that touch
 *                                         (one ends at a.b.c.d, the other starts at a.b.c.d+1)
 *
 * Why: VPP stores pool addresses one by one, and the agent's Retrieve of `nat44-ed.address-pool` merges consecutive
 * addresses of one class and VRF back into maximal ranges (docs/agent/descriptors/nat44-ed.md). Two adjacent pools
 * would come back as ONE range, so the applied state could never equal the configuration and every resync would
 * plan a delete + re-create. Overlaps are `nat.pools-valid`'s; this rule adds only the "touching" case. The fix is to
 * write the two pools as one range (the open question of the task prompt, decided "reject", logged in the task
 * status). A missing `vrf` is the default VRF, like `"default"`.
 */

const vrfKey = (vrf: string | undefined): string => vrf ?? 'default';

export const nat44EdSessionsValidators: readonly ValidatorDefinition[] = [
  {
    name: 'nat.nat44-ed-sessions-adjacent-pools',
    domains: ['nat'],
    validate: ({ nat }) => {
      const issues: SemanticIssue[] = [];
      const seen: { index: number; name: string; start: bigint; end: bigint; key: string }[] = [];
      nat.pools.forEach((pool, index) => {
        if (!('range' in pool)) return; // interface pools: VPP resolves the address at run time
        const r = poolRange(pool.range);
        if (r.start > r.end) return; // reversed: reported by nat.pools-valid
        const key = `${pool.twiceNat ? 'twice' : 'normal'}|${vrfKey(pool.vrf)}`;
        const hit = seen.find(
          (o) => o.key === key && (o.end + 1n === r.start || r.end + 1n === o.start),
        );
        if (hit !== undefined) {
          issues.push({
            pointer: jsonPointer('nat', 'pools', index, 'range'),
            message: `range '${pool.range}' touches pool '${hit.name}' (same twice-NAT class and VRF): VPP keeps pool addresses one by one and reports them back as one range, so write the two pools as one range`,
          });
        }
        seen.push({ index, name: pool.name, start: r.start, end: r.end, key });
      });
      return issues;
    },
  },
];
