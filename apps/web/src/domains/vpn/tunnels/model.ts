import type { paths } from '@ngfw/api-client';
import type { NgfwStatus } from '@ngfw/ui-kit';
import type { InterfaceItem } from '../../interfaces/model';
import { linkStatus } from '../../interfaces/model';

/** The tunnel kinds this screen edits, in the schema's `TUNNEL_KINDS` order, with the engine interface-name prefix. */
export const KINDS = [
  { key: 'gre', vppPrefix: 'gre' },
  { key: 'vxlan', vppPrefix: 'vxlan_tunnel' },
  { key: 'ipip', vppPrefix: 'ipip' },
  { key: 'vxlanGpe', vppPrefix: 'vxlan_gpe_tunnel' },
  { key: 'gtpu', vppPrefix: 'gtpu_tunnel' },
  { key: 'l2tpv3', vppPrefix: 'l2tpv3_tunnel' },
  { key: 'pppoe', vppPrefix: 'pppoe_session' },
] as const;

export type KindKey = (typeof KINDS)[number]['key'];

/** `tunnels.<kind>.<name>` members the list shows (the form edits every member of the one schema). */
export interface TunnelItem {
  instance?: number;
  src?: string;
  dst?: string;
  vni?: number;
  type?: string;
  mode?: string;
  decap?: string;
  enabled?: boolean;
  sixrd?: unknown;
  ourAddress?: string;
  clientAddress?: string;
  clientIp?: string;
  teid?: number;
  sessionId?: number;
}

/** The engine interface name of a tunnel (`gre7001`), or undefined without a fixed instance. */
export function vppName(kind: KindKey, item: TunnelItem | undefined): string | undefined {
  if (
    item?.instance === undefined ||
    item.sixrd !== undefined ||
    !['gre', 'ipip', 'vxlan'].includes(kind)
  )
    return undefined;
  const k = KINDS.find((x) => x.key === kind);
  return k ? `${k.vppPrefix}${item.instance}` : undefined;
}

/** Link status of the tunnel interface from `/state/interfaces` (undefined: not in the data plane). */
export function tunnelStatus(
  items: readonly InterfaceItem[] | undefined,
  name: string | undefined,
): NgfwStatus | undefined {
  if (!name || !items) return undefined;
  const it = items.find((i) => i.name === name || i.state?.vppName === name);
  return linkStatus(it?.state);
}

export const ADVANCED: readonly KindKey[] = ['gtpu', 'l2tpv3', 'pppoe'];
export type TunnelLive = NonNullable<
  paths['/api/v1/state/tunnels']['get']
>['responses'][200]['content']['application/json']['items'][number];
/** Config names are metadata identities; engine allocations and candidate edits never determine live identity. */
export function liveTunnel(
  items: readonly TunnelLive[] | undefined,
  kind: KindKey,
  name: string,
): TunnelLive | undefined {
  return items?.find((item) => item.kind === kind && item.name === name);
}
export function liveStatus(item: TunnelLive | undefined): NgfwStatus | undefined {
  if (!item) return undefined;
  return !item.adminUp ? 'adminDown' : item.linkUp ? 'up' : 'down';
}
