/**
 * Tag stacks of VLAN sub-interfaces (F-vlan-qinq). A sub-interface matches exactly one stack (exact-match, routed):
 * one 802.1Q tag, or two tags — an 802.1ad (TPID 0x88a8) or 802.1Q (0x8100) outer tag and an 802.1Q inner tag (QinQ),
 * as VPP's create_subif builds it (ONE_TAG | TWO_TAGS [+ DOT1AD]).
 */

import type { InterfaceItem, LiveState, SubinterfaceConfig } from '../model';

/** One tag of a stack. `dot1q` = IEEE 802.1Q (TPID 0x8100); `dot1ad` = IEEE 802.1ad service tag (TPID 0x88a8). */
export interface VlanTag {
  proto: 'dot1q' | 'dot1ad';
  vlan: number;
}

/** Every field optional and nullable: rows and forms hand over partial or absent values. */
type Loose<T> = { [K in keyof T]?: T[K] | null | undefined };

/**
 * The fields that describe a stack, picked from the schema type (a contract rename breaks the build): the
 * configuration's `vlanId`/`innerVlanId`/`dot1ad`; live state reports 0 = no tag.
 */
export type TagStackFields = Loose<Pick<SubinterfaceConfig, 'vlanId' | 'innerVlanId' | 'dot1ad'>>;

/** The stack, outermost tag first; empty without an outer tag. The inner tag is always 802.1Q. */
export function tagStack(f: TagStackFields | null | undefined): VlanTag[] {
  const outer = f?.vlanId ?? 0;
  if (outer === 0) return [];
  const out: VlanTag[] = [{ proto: f?.dot1ad === true ? 'dot1ad' : 'dot1q', vlan: outer }];
  const inner = f?.innerVlanId ?? 0;
  if (inner !== 0) out.push({ proto: 'dot1q', vlan: inner });
  return out;
}

/**
 * `dot1q 100`, `dot1ad 200 · dot1q 100`: the technical notation (VPP/TNSR CLI; the same in every UI language, shown
 * left-to-right). `label` and `separator` can replace the notation of one tag and the joiner.
 */
export function formatTagStack(
  stack: readonly VlanTag[],
  label: (tag: VlanTag) => string = (tag) => `${tag.proto} ${tag.vlan}`,
  separator = ' · ',
): string {
  return stack.map(label).join(separator);
}

export function sameTagStack(a: readonly VlanTag[], b: readonly VlanTag[]): boolean {
  return (
    a.length === b.length && a.every((t, i) => t.proto === b[i]!.proto && t.vlan === b[i]!.vlan)
  );
}

/** A `/state/interfaces` row (generated client type), reduced to what the live stack needs. */
export type TagStackRow = {
  state?: Pick<LiveState, 'vlanId' | 'innerVlanId'> | null | undefined;
} & Loose<Pick<InterfaceItem, 'config' | 'running'>>;

/**
 * The stack VPP has for a sub-interface row: the tag numbers from the live state (the agent's InterfaceState), the tag
 * type from `config` — what the agent retrieved from VPP — because InterfaceState carries no dot1ad flag (D-105); the
 * running configuration when the agent retrieved nothing. `undefined` when VPP does not have the sub-interface.
 */
export function liveTagStack(row: TagStackRow | null | undefined): VlanTag[] | undefined {
  if (!row?.state) return undefined;
  const typed = (v: unknown) =>
    typeof v === 'object' && v !== null ? (v as TagStackFields) : undefined;
  const dot1ad = typed(row.config)?.dot1ad ?? typed(row.running)?.dot1ad ?? false;
  return tagStack({ vlanId: row.state.vlanId, innerVlanId: row.state.innerVlanId, dot1ad });
}
