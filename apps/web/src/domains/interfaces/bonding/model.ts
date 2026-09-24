import type { paths } from '@ngfw/api-client';
import { bondIdOf, type BondConfig, type BondMemberConfig } from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import type { VrxStatus } from '@ngfw/ui-kit';
import { interfaceItemSchema, type InterfaceItem, type InterfacesConfig } from '../model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;

/** `GET /api/v1/state/interfaces/bonds` as generated from the OpenAPI document (never hand-written). */
export type BondsState = Ok<NonNullable<paths['/api/v1/state/interfaces/bonds']['get']>>;
export type BondItem = BondsState['items'][number];
export type LiveBond = NonNullable<BondItem['state']>;
export type LiveMember = LiveBond['members'][number];
export type { BondConfig, BondMemberConfig };

/** `interfaces.<name>.bond` — the one schema (00-CONTEXT rule 5), from the generated domain JSON Schema. */
export function bondSchema(): JsonSchema {
  const props = (interfaceItemSchema().properties ?? {}) as Record<string, JsonSchema>;
  const bond = props['bond'];
  if (!bond || typeof bond !== 'object') throw new Error('interfaces.bond schema not found');
  return bond;
}

/** The bond form edits everything but `members` (they have their own table, like P08's sub-interfaces). */
export function bondFormSchema(): JsonSchema {
  const bond = bondSchema();
  const props = { ...((bond.properties ?? {}) as Record<string, JsonSchema>) };
  delete props['members'];
  const required = Array.isArray(bond.required) ? bond.required.filter((r) => r !== 'members') : undefined;
  return { ...bond, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** `….bond.members.<ifName>` item schema. */
export function memberSchema(): JsonSchema {
  const members = ((bondSchema().properties ?? {}) as Record<string, JsonSchema>)['members'] as { additionalProperties?: JsonSchema } | undefined;
  if (!members?.additionalProperties || typeof members.additionalProperties !== 'object') throw new Error('bond member schema not found');
  return members.additionalProperties;
}

/** A free BondEthernet<id> for "Add bond": the lowest id ≥ base not configured and not on the data plane. */
export function nextBondName(taken: Iterable<string>, base = 0): string {
  const used = new Set<number>();
  for (const n of taken) {
    const id = bondIdOf(n);
    if (id !== undefined) used.add(id);
  }
  let id = base;
  while (used.has(id)) id++;
  return `BondEthernet${id}`;
}

function hasL3(c: InterfacesConfig[string] | undefined): boolean {
  if (!c) return false;
  return (
    (c.ipv4?.length ?? 0) > 0 ||
    (c.ipv6?.length ?? 0) > 0 ||
    c.dhcpClient !== undefined ||
    c.unnumbered !== undefined ||
    (c.vrf !== undefined && c.vrf !== 'default') ||
    Object.keys(c.subinterfaces ?? {}).length > 0
  );
}

/**
 * Interfaces that may join `bond` (the semantic rules interfaces.bonding-member-*): parent interfaces of the data plane or
 * the candidate that are not bonds or loopbacks, not members of another bond and carry no addresses / VRF / DHCP /
 * sub-interfaces of their own. The bond's current members are always included.
 */
export function eligibleMembers(bond: string, candidate: InterfacesConfig, live: readonly InterfaceItem[]): string[] {
  const inOther = new Set<string>();
  for (const [name, c] of Object.entries(candidate)) {
    if (name !== bond) for (const m of Object.keys(c.bond?.members ?? {})) inOther.add(m);
  }
  const names = new Set<string>([...Object.keys(candidate), ...live.filter((i) => i.kind === 'interface').map((i) => i.name)]);
  const own = Object.keys(candidate[bond]?.bond?.members ?? {});
  const out = [...names].filter((n) => {
    if (n === bond || bondIdOf(n) !== undefined || candidate[n]?.bond !== undefined) return false;
    if (/^loop[0-9]+$/.test(n) || live.find((i) => i.name === n)?.state?.type === 'loopback') return false;
    return !inOther.has(n) && !hasL3(candidate[n]);
  });
  return [...new Set([...own, ...out])].sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
}

/** Semantic status of a bond: admin down, up with members transmitting, or up without any active member. */
export function bondStatus(s: LiveBond | null | undefined): VrxStatus | undefined {
  if (!s) return undefined;
  if (!s.adminUp) return 'adminDown';
  if (s.activeMemberCount > 0) return 'up';
  return s.memberCount > 0 ? 'degraded' : 'down';
}

/** Semantic status of one member: its link, or for LACP members whether it is collecting/distributing. */
export function memberStatus(m: LiveMember): VrxStatus {
  if (!m.adminUp) return 'adminDown';
  if (!m.linkUp) return 'down';
  if (m.lacp && m.lacp.muxState !== 'collecting-distributing') return 'degraded';
  return 'up';
}
