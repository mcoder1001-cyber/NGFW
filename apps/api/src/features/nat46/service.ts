import { Injectable } from '@nestjs/common';
import { ipv6ToBigInt, isPlainObject, NAT46_PREFIX, NAT46_WELL_KNOWN_PREFIX } from '@ngfw/schema';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

/** The running configuration's NAT46 object (tolerates an absent or odd subtree). */
export function nat46Json(runningNat: unknown) {
  const n = isPlainObject(runningNat) ? runningNat['nat46'] : undefined;
  const o: Json = isPlainObject(n) ? n : {};
  const clientPrefix =
    typeof o['clientPrefix'] === 'string' ? o['clientPrefix'] : NAT46_WELL_KNOWN_PREFIX;
  const interfaces = Array.isArray(o['interfaces'])
    ? o['interfaces'].filter((x): x is string => typeof x === 'string')
    : [];
  const raw = Array.isArray(o['mappings']) ? o['mappings'] : [];
  const mappings = raw.filter(isPlainObject).map((m: Json) => {
    const name = typeof m['name'] === 'string' ? m['name'] : '';
    return {
      name,
      domain: NAT46_PREFIX + name,
      ipv4: typeof m['ipv4'] === 'string' ? m['ipv4'] : '',
      ipv6: typeof m['ipv6'] === 'string' ? m['ipv6'] : '',
      mtu: typeof m['mtu'] === 'number' ? m['mtu'] : null,
    };
  });
  return { configured: isPlainObject(n), clientPrefix, interfaces, mappings };
}

/** RFC 6052 /96: the IPv4 address in the last 32 bits of the client prefix (descriptors/nat46.ClientAddress). */
export function nat46ClientAddress(clientPrefix: string, ipv4: string): string | undefined {
  const [addr, len] = clientPrefix.split('/');
  const net = addr === undefined ? undefined : ipv6ToBigInt(addr);
  if (net === undefined || len !== '96') return undefined;
  const v4 = ipv4.split('.').reduce((a, o) => (a << 8n) | BigInt(Number(o)), 0n);
  const full = (net & ~0xffffffffn) | v4;
  const groups: string[] = [];
  for (let i = 7; i >= 0; i--) groups.push(((full >> BigInt(i * 16)) & 0xffffn).toString(16));
  // canonical RFC 5952: compress the longest run (≥ 2) of zero groups
  let best = -1;
  let bestLen = 1;
  for (let i = 0; i < 8; ) {
    let j = i;
    while (j < 8 && groups[j] === '0') j++;
    if (j - i > bestLen) {
      best = i;
      bestLen = j - i;
    }
    i = j === i ? i + 1 : j;
  }
  if (best < 0) return groups.join(':');
  return `${groups.slice(0, best).join(':')}::${groups.slice(best + bestLen).join(':')}`;
}

/** `/state/nat/nat46` and `/state/nat/nat46/client`: running configuration only — NAT46 keeps no state in VPP. */
@Injectable()
export class Nat46Service {
  constructor(private readonly ds: DatastoreService) {}

  async nat46() {
    const running = await this.ds.getRunning();
    return nat46Json(running.doc['nat']);
  }

  async client(ipv4: string) {
    const { clientPrefix } = await this.nat46();
    return { ipv4, clientPrefix, ipv6: nat46ClientAddress(clientPrefix, ipv4) ?? '' };
  }
}
