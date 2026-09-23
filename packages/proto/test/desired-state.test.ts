// Compile-check for the generated TS contract (task P03).
//
// DesiredState is a 1:1 protobuf projection of the configuration document, so `DesiredState.fromJSON`
// of a parsed packages/schema example must produce a message whose typed fields carry the document's
// values and that survives binary and JSON round trips. `pnpm typecheck` compiles this file too.
import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import {
  ActionOutput,
  ActionRequest,
  ApplyRequest,
  DesiredState,
  type Interface,
  type StaticRoute,
} from '../gen/ts/vrx/v1/dataplane.js';

/** Mirrors ROOT_KEYS in packages/schema/src/index.ts (documented order, docs/04). */
const ROOT_KEYS = [
  'system',
  'dataplane',
  'interfaces',
  'vrfs',
  'routing',
  'nat',
  'objects',
  'acl',
  'vpn',
  'tunnels',
  'services',
  'ha',
  'management',
] as const;

const dir = new URL('../../schema/examples/', import.meta.url);
const files = readdirSync(dir)
  .filter((f) => f.endsWith('.json') && !f.startsWith('invalid-'))
  .sort();

function load(file: string): unknown {
  return JSON.parse(readFileSync(new URL(file, dir), 'utf8'));
}

describe('DesiredState mirrors RootConfig', () => {
  it('has exactly the 13 root keys in ROOT_KEYS order', () => {
    // createBase* lists the fields in declaration (= field number) order.
    expect(Object.keys(DesiredState.fromPartial({}))).toEqual([...ROOT_KEYS]);
  });

  it('includes minimal.json and two-interfaces.json', () => {
    expect(files).toEqual(expect.arrayContaining(['minimal.json', 'two-interfaces.json']));
  });

  for (const file of files) {
    it(`${file} parses with fromJSON and round-trips (binary + JSON)`, () => {
      const ds = DesiredState.fromJSON(load(file));
      const wire = DesiredState.encode(ds).finish();
      expect(DesiredState.decode(wire)).toEqual(ds);
      expect(DesiredState.fromJSON(DesiredState.toJSON(ds))).toEqual(ds);
    });
  }

  it('two-interfaces.json values land in the typed fields', () => {
    const ds = DesiredState.fromJSON(load('two-interfaces.json'));
    expect(ds.system?.hostname).toBe('vrx-a');
    expect(ds.system?.timezone).toBe('UTC');

    const uplink: Interface | undefined = ds.interfaces['TenGigabitEthernet0/0/0'];
    expect(uplink).toBeDefined();
    expect(uplink?.enabled).toBe(true);
    expect(uplink?.description).toBe('uplink');
    expect(uplink?.mtu).toBe(9000);
    expect(uplink?.mac).toBeUndefined();
    expect(uplink?.ipv4).toEqual(['10.0.0.1/24']);
    expect(uplink?.ipv6).toEqual(['2001:db8::1/64']);
    expect(uplink?.vrf).toBe('default');
    expect(uplink?.rxMode).toBe('polling');

    const sub = ds.interfaces['TenGigabitEthernet0/0/1']?.subinterfaces['100'];
    expect(sub?.vlanId).toBe(100);
    expect(sub?.innerVlanId).toBeUndefined();
    expect(sub?.ipv4).toEqual(['192.168.100.1/24']);
    expect(sub?.vrf).toBe('customer-a');

    expect(ds.vrfs['default']?.id).toBe(0);
    expect(ds.vrfs['customer-a']).toEqual({ id: 10, description: 'customer A' });

    const route: StaticRoute | undefined = ds.routing?.static[0];
    expect(route?.prefix).toBe('0.0.0.0/0');
    expect(route?.vrf).toBe('default');
    expect(route?.nextHops).toEqual([{ address: '10.0.0.254', interface: undefined, weight: 1 }]);

    // Domains absent from the document stay absent (Zod prefault happens before Apply, not here).
    expect(ds.nat).toBeUndefined();
    expect(ds.vpn).toBeUndefined();
  });

  it('JSON names are the Zod (lowerCamelCase) names', () => {
    const json = DesiredState.toJSON(
      DesiredState.fromPartial({
        dataplane: { pciWhitelist: ['0000:0b:00.0'], rxQueues: 2 },
        interfaces: { loop700: { rxMode: 'polling', subinterfaces: { '10': { vlanId: 10 } } } },
        routing: { static: [{ prefix: '10.70.0.0/16', vrf: 'w7-a', nextHops: [{ address: '10.7.0.254' }] }] },
        nat: { staticMappings: [{ name: 'web', out2inOnly: true }], insideVrf: 'w7-a' },
        vpn: { ipsec: { tunnels: { 'site-b': { ikeVersion: 2, natT: true, auth: { method: 'psk', secretRef: 'psk/site-b' } } } } },
      }),
    ) as Record<string, unknown>;
    const text = JSON.stringify(json);
    for (const key of ['pciWhitelist', 'rxQueues', 'rxMode', 'vlanId', 'nextHops', 'staticMappings', 'out2inOnly', 'insideVrf', 'ikeVersion', 'natT', 'secretRef']) {
      expect(text).toContain(`"${key}"`);
    }
  });

  it('typed construction compiles for every domain and the envelopes', () => {
    const ds = DesiredState.fromPartial({
      system: { hostname: 'vrx-a', timezone: 'UTC', ntp: {}, dns: {} },
      dataplane: { workers: 2, pciWhitelist: [] },
      interfaces: { loop700: { enabled: true, mtu: 1500, ipv4: ['10.7.0.1/24'], vrf: 'default', rxMode: 'polling' } },
      vrfs: { default: { id: 0 }, 'w7-a': { id: 7001, description: 'slot 7' } },
      routing: { static: [{ prefix: '10.70.0.0/16', vrf: 'w7-a', nextHops: [{ address: '10.7.0.254', weight: 1 }] }], bgp: {} },
      nat: { enabled: true, mode: 'ed', inside: ['loop700'], pools: [{ name: 'p1', range: '10.7.1.1-10.7.1.10' }] },
      objects: { addresses: { h1: { type: 'host', address: '10.7.0.10' } }, services: { https: { protocol: 'tcp', destinationPorts: ['443'] } } },
      acl: {
        lists: { in: { rules: [{ sequence: 10, enabled: true, action: 'permit', ipVersion: 'any', source: { kind: 'object', name: 'h1' } }] } },
        attachments: [{ list: 'in', target: { kind: 'interface', interface: 'loop700' }, direction: 'in', sequence: 1, enabled: true }],
      },
      vpn: { wireguard: { interfaces: { wg0: { instance: 0, listenPort: 51820, privateKeyRef: 'wg/wg0', peers: {} } } } },
      tunnels: { gre: { gre0: {} } },
      services: { dhcp: {} },
      ha: { vrrp: [{}] },
      management: { users: [{ username: 'admin', role: 'admin', scope: '*' }] },
    });
    for (const key of ROOT_KEYS) {
      expect(ds[key], key).toBeDefined();
    }
    const req = ApplyRequest.fromPartial({ txnId: 't1', desiredState: ds, subsystems: ['interfaces', 'vrfs'], confirmTimeoutSec: 120, owner: 'w7' });
    expect(ApplyRequest.decode(ApplyRequest.encode(req).finish()).desiredState?.interfaces['loop700']?.mtu).toBe(1500);
    const action = ActionRequest.fromPartial({ ping: { target: '10.7.0.254', count: 3 } });
    expect(ActionRequest.decode(ActionRequest.encode(action).finish()).ping?.count).toBe(3);
    const done = ActionOutput.fromPartial({ done: { summary: 'ok', exitCode: 0 } });
    expect(ActionOutput.decode(ActionOutput.encode(done).finish()).done?.summary).toBe('ok');
  });
});
