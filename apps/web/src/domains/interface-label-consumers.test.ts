import { describe, expect, it } from 'vitest';
import { targetText } from './firewall/acl/model';
import { endpoint } from './firewall/nat44-ed-sessions/MappingsTab';
import { labelRouteRows, pathText } from './routing/mpls-srmpls/model';
import { pathsText, routeRows } from './routing/vrf-static-ecmp/model';

const label = (name: string) => (name === 'vmxnet3-0/b/0/0' ? 'Ethernet2' : name);
const translate = (key: string) => (key === 'path.via' ? 'via' : key);

describe('interface labels in network summaries', () => {
  it('formats static and ECMP next hops without changing configuration identities or VRF names', () => {
    const route = {
      prefix: '0.0.0.0/0',
      vrf: 'default',
      blackhole: false,
      distance: 1,
      nextHops: [
        { interface: 'vmxnet3-0/b/0/0', weight: 2 },
        { address: '192.0.2.1', vrf: 'vmxnet3-0/b/0/0', weight: 1 },
      ],
    };
    const before = structuredClone(route);
    expect(pathsText(route, label)).toBe('via Ethernet2 ×2 · 192.0.2.1 in vmxnet3-0/b/0/0 ×1');
    expect(routeRows([route], label)[0]?.paths).toBe(pathsText(route, label));
    expect(route).toEqual(before);
  });

  it('formats MPLS paths and searchable route summaries with the same labels', () => {
    const path = { interface: 'vmxnet3-0/b/0/0', outLabels: [100], weight: 1 };
    expect(pathText(path, translate, label)).toContain('via Ethernet2');
    const config = {
      labelRoutes: [{ table: 'default', label: 20, eos: true, paths: [path] }],
    } as Parameters<typeof labelRouteRows>[0];
    const row = labelRouteRows(config, translate, label)[0]!;
    expect(row.paths).toContain('via Ethernet2');
    expect(row.pathList[0]).toContainEqual({ text: 'Ethernet2', mono: true });
    expect(path.interface).toBe('vmxnet3-0/b/0/0');
  });

  it('formats ACL interfaces and NAT interface endpoints while retaining unrelated names', () => {
    expect(targetText({ kind: 'interface', interface: 'vmxnet3-0/b/0/0' }, translate, label)).toBe(
      'Ethernet2',
    );
    expect(
      targetText(
        { kind: 'zone', zone: 'vmxnet3-0/b/0/0' },
        (_key, args) => String(args?.['zone']),
        label,
      ),
    ).toBe('vmxnet3-0/b/0/0');
    expect(endpoint({ interface: 'vmxnet3-0/b/0/0', port: 443 }, label)).toBe('Ethernet2:443');
    expect(endpoint({ ip: '192.0.2.10', port: 443 }, label)).toBe('192.0.2.10:443');
  });
});
