import type { VrxStatus } from '@ngfw/ui-kit';
import type { InterfaceItem } from '../../interfaces/model';
import { linkStatus } from '../../interfaces/model';

/** The tunnel kinds this screen edits, in the schema's `TUNNEL_KINDS` order, with the engine interface-name prefix. */
export const KINDS = [
  { key: 'gre', vppPrefix: 'gre' },
  { key: 'vxlan', vppPrefix: 'vxlan_tunnel' },
  { key: 'ipip', vppPrefix: 'ipip' },
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
}

/** The engine interface name of a tunnel (`gre7001`), or undefined without a fixed instance. */
export function vppName(kind: KindKey, item: TunnelItem | undefined): string | undefined {
  if (item?.instance === undefined) return undefined;
  const k = KINDS.find((x) => x.key === kind);
  return k ? `${k.vppPrefix}${item.instance}` : undefined;
}

/** Link status of the tunnel interface from `/state/interfaces` (undefined: not in the data plane). */
export function tunnelStatus(items: readonly InterfaceItem[] | undefined, name: string | undefined): VrxStatus | undefined {
  if (!name || !items) return undefined;
  const it = items.find((i) => i.name === name || i.state?.vppName === name);
  return linkStatus(it?.state);
}
