import type { RootConfig, SetupInput } from '@ngfw/schema';
import type { fetchInterfacesState } from '../../interfaces/queries';
import type { RouteItem } from '../../routing/vrf-static-ecmp/api';

type Inventory = Awaited<ReturnType<typeof fetchInterfacesState>>;
type WanValues = Pick<SetupInput, 'wan' | 'wanMode' | 'wanAddress' | 'wanGateway' | 'wanPppoe'>;

/** Infer only from actual configuration or observations; ambiguous uplinks require selection. */
export function wanDefaults(
  doc: RootConfig,
  names: string[],
  inventory?: Inventory,
  routes: RouteItem[] = [],
  selected?: string,
): Partial<WanValues> {
  const logical = doc.interfaces['setup-pppoe']?.pppoe;
  const resolve = (name: string) => (name === 'setup-pppoe' ? logical?.parent : name);
  const configuredRoutes = doc.routing.static.filter(
    (r) => r.prefix === '0.0.0.0/0' && !r.blackhole,
  );
  const liveRoutes = routes.filter((r) => r.prefix === '0.0.0.0/0');
  const onSubnet = (cidr: string, address: string) => {
    const [ip, prefix] = cidr.split('/');
    if (!ip || !prefix || address.includes(':')) return false;
    const number = (value: string) =>
      value.split('.').reduce((n, octet) => n * 256 + Number(octet), 0);
    const size = 2 ** (32 - Number(prefix));
    return Math.floor(number(ip) / size) === Math.floor(number(address) / size);
  };
  const hopInterface = (hop: { interface?: string; address?: string }, vrf: string) => {
    if (hop.interface) return hop.interface;
    const matching = names.filter(
      (name) =>
        (doc.interfaces[name]?.vrf ?? 'default') === vrf &&
        (
          doc.interfaces[name]?.ipv4 ??
          inventory?.items.find((item) => item.name === name)?.state?.ipv4 ??
          []
        ).some((cidr) => hop.address && onSubnet(cidr, hop.address)),
    );
    return matching.length === 1 ? matching[0] : undefined;
  };
  let wan = selected;
  const candidateGroups = [
    configuredRoutes
      .filter((r) => r.vrf === 'default')
      .flatMap((r) => r.nextHops.map((hop) => hopInterface(hop, r.vrf))),
    liveRoutes
      .filter((r) => r.vrf === 'default')
      .flatMap((r) => r.nextHops.map((hop) => hopInterface(hop, r.vrf))),
    doc.nat.outside,
    names.filter((name) => doc.interfaces[name]?.dhcpClient),
    names.filter(
      (name) =>
        ((doc.interfaces[name]?.ipv4.length ||
          inventory?.items.find((item) => item.name === name)?.state?.ipv4?.length) ??
          0) > 0,
    ),
    names,
  ];
  if (!wan) {
    for (const candidates of candidateGroups) {
      const valid = [
        ...new Set(
          candidates
            .map((name) => name && resolve(name))
            .filter((name): name is string => !!name && names.includes(name)),
        ),
      ];
      if (valid.length > 1) return {};
      if (valid.length === 1) {
        wan = valid[0];
        break;
      }
    }
  }
  if (!wan) return {};
  const config = doc.interfaces[wan];
  if (logical?.parent === wan)
    return {
      wan,
      wanMode: 'pppoe',
      wanPppoe: { username: logical.username, passwordRef: logical.passwordRef },
    };
  const address =
    config?.ipv4[0] ?? inventory?.items.find((item) => item.name === wan)?.state?.ipv4?.[0];
  const gateway =
    configuredRoutes
      .filter((r) => r.vrf === (config?.vrf ?? 'default'))
      .flatMap((r) => r.nextHops.map((hop) => ({ ...hop, interface: hopInterface(hop, r.vrf) })))
      .find((hop) => hop.interface === wan)?.address ??
    liveRoutes
      .filter((r) => r.vrf === (config?.vrf ?? 'default'))
      .flatMap((r) => r.nextHops.map((hop) => ({ ...hop, interface: hopInterface(hop, r.vrf) })))
      .find((hop) => hop.interface === wan)?.address;
  return {
    wan,
    wanMode: config?.dhcpClient ? 'dhcp' : address ? 'static' : 'dhcp',
    wanAddress: address,
    wanGateway: gateway,
    wanPppoe: undefined,
  };
}
