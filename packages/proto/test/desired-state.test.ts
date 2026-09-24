// Compile-check and projection tests for the generated TS contract (task P03, review fixes F1/F3/F5).
//
// DesiredState is a 1:1 protobuf projection of the configuration document, so `DesiredState.fromJSON`
// of a valid packages/schema document must produce a message whose typed fields carry the document's
// values, that survives binary and JSON round trips, and whose `toJSON` reproduces the document
// key-for-key (explicit presence, D-039). `pnpm typecheck` compiles this file too.
//
// LIMITATION (documented, F5): ts-proto `fromJSON` is lenient — unknown keys are silently dropped and
// snake_case aliases are accepted. Strictness is therefore asserted structurally here
// (`toJSON(fromJSON(doc))` must deep-equal `doc`, which fails the moment a key is unknown) and
// natively on the Go side (`protojson.Unmarshal`, `DiscardUnknown=false`,
// apps/agent/internal/contracttest). The Zod→proto drift guard is P03b's: JSON Schema ⊆ proto fields
// in Go (contracttest drift_test.go) and `RootConfig.parse()`d documents in parsed-documents.test.ts.
import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import {
  ActionOutput,
  ActionRequest,
  ApplyOperation,
  ApplyRequest,
  DesiredState,
  Event,
  EventKind,
  InterfaceCounters,
  IpsecRekey,
  IssueSeverity,
  ObjectResult,
  ObjectResultCode,
  StatsBatch,
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

/** Document corpora: the schema package's examples plus the proto-local fixtures. */
const corpora = {
  examples: new URL('../../schema/examples/', import.meta.url),
  fixtures: new URL('./fixtures/', import.meta.url),
} as const;

function listValid(dir: URL): string[] {
  return readdirSync(dir)
    .filter((f) => f.endsWith('.json') && !f.startsWith('invalid-'))
    .sort();
}

function load(dir: URL, file: string): unknown {
  return JSON.parse(readFileSync(new URL(file, dir), 'utf8'));
}

const corpus: { name: string; doc: unknown }[] = [
  ...listValid(corpora.examples).map((f) => ({ name: `examples/${f}`, doc: load(corpora.examples, f) })),
  ...listValid(corpora.fixtures).map((f) => ({ name: `fixtures/${f}`, doc: load(corpora.fixtures, f) })),
];

describe('DesiredState mirrors RootConfig', () => {
  it('has exactly the 13 root keys in ROOT_KEYS order', () => {
    // createBase* lists the fields in declaration (= field number) order.
    expect(Object.keys(DesiredState.fromPartial({}))).toEqual([...ROOT_KEYS]);
  });

  it('corpus covers the schema examples and the all-domains fixture', () => {
    expect(corpus.map((c) => c.name)).toEqual(
      expect.arrayContaining(['examples/minimal.json', 'examples/two-interfaces.json', 'fixtures/all-domains.json']),
    );
  });

  for (const { name, doc } of corpus) {
    it(`${name}: fromJSON → toJSON reproduces the document key-for-key and round-trips`, () => {
      const ds = DesiredState.fromJSON(doc);
      // Projection fidelity (the structural strictness check, see the header): every key of the
      // document is a field, every set field is emitted — including explicit `false` and `0`.
      expect(DesiredState.toJSON(ds)).toEqual(doc);
      const wire = DesiredState.encode(ds).finish();
      expect(DesiredState.decode(wire)).toEqual(ds);
      expect(DesiredState.fromJSON(DesiredState.toJSON(ds))).toEqual(ds);
    });
  }

  it('documents the ts-proto leniency: unknown keys are dropped, and the projection check catches it', () => {
    const bad = load(corpora.examples, 'invalid-unknown-root-key.json');
    expect(() => DesiredState.fromJSON(bad)).not.toThrow(); // lenient (the limitation)
    expect(DesiredState.toJSON(DesiredState.fromJSON(bad))).not.toEqual(bad); // `tenants` is gone
    const nested = { system: { hostname: 'vrx-a', bogusField: 1 } };
    expect(DesiredState.toJSON(DesiredState.fromJSON(nested))).toEqual({ system: { hostname: 'vrx-a' } });
  });

  it('explicit presence (D-039): proto3 defaults set by the document survive toJSON — the F3 probe', () => {
    const doc = {
      vrfs: { default: { id: 0 } },
      interfaces: { x: { enabled: false, vrf: 'default', rxMode: 'polling' } },
      acl: { lists: { l: { rules: [{ sequence: 1, enabled: false, action: 'permit', ipVersion: 'any' }] } } },
    };
    const ds = DesiredState.fromJSON(doc);
    expect(ds.vrfs['default']?.id).toBe(0);
    expect(ds.interfaces['x']?.enabled).toBe(false);
    expect(ds.interfaces['x']?.mtu).toBeUndefined();
    expect(ds.interfaces['x']?.promiscuous).toBeUndefined();
    expect(DesiredState.toJSON(ds)).toEqual(doc);
  });

  it('secret-flagged leaves have no field (D-040)', () => {
    const user = { username: 'a', role: 'admin', scope: '*', passwordHash: '$6$x' };
    const back = DesiredState.toJSON(DesiredState.fromJSON({ management: { users: [user] } })) as {
      management: { users: Record<string, unknown>[] };
    };
    expect(back.management.users[0]).toEqual({ username: 'a', role: 'admin', scope: '*' });
    expect('passwordHash' in (back.management.users[0] ?? {})).toBe(false);
  });

  it('two-interfaces.json values land in the typed fields', () => {
    const ds = DesiredState.fromJSON(load(corpora.examples, 'two-interfaces.json'));
    expect(ds.system?.hostname).toBe('vrx-a');
    expect(ds.system?.timezone).toBe('UTC');
    expect(ds.system?.banner).toBeUndefined();

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
    expect(uplink?.promiscuous).toBeUndefined();

    const sub = ds.interfaces['TenGigabitEthernet0/0/1']?.subinterfaces['100'];
    expect(sub?.vlanId).toBe(100);
    expect(sub?.innerVlanId).toBeUndefined();
    expect(sub?.dot1ad).toBeUndefined();
    expect(sub?.ipv4).toEqual(['192.168.100.1/24']);
    expect(sub?.vrf).toBe('customer-a');

    expect(ds.vrfs['default']?.id).toBe(0);
    // proxyArpRanges: repeated (F-neighbors-ra, Vrf 4) — ts-proto fills an absent repeated field with []
    expect(ds.vrfs['customer-a']).toEqual({ id: 10, description: 'customer A', proxyArpRanges: [] });

    const route: StaticRoute | undefined = ds.routing?.static[0];
    expect(route?.prefix).toBe('0.0.0.0/0');
    expect(route?.vrf).toBe('default');
    expect(route?.distance).toBeUndefined();
    expect(route?.nextHops).toEqual([{ address: '10.0.0.254', interface: undefined, weight: 1 }]);

    // Domains absent from the document stay absent (Zod prefault happens before Apply, not here).
    expect(ds.nat).toBeUndefined();
    expect(ds.vpn).toBeUndefined();
  });

  it('all-domains.json: the P02a shapes fixed by the review (D-042)', () => {
    const ds = DesiredState.fromJSON(load(corpora.fixtures, 'all-domains.json'));
    expect(ds.system?.banner).toEqual({ login: 'Authorised access only.', motd: undefined });
    expect(ds.dataplane?.corelist).toEqual([2, 3, 4, 8]);
    expect(ds.dataplane?.txQueues).toBe(2);
    expect(ds.interfaces['loop0']?.rxMode).toBeUndefined();
    expect(ds.interfaces['loop0']?.promiscuous).toBe(true);
    expect(ds.interfaces['TenGigabitEthernet0/0/1']?.subinterfaces['200']?.dot1ad).toBe(true);
    const ifaceRoute = ds.routing?.static.find((r) => r.prefix === '192.0.2.0/24');
    expect(ifaceRoute?.nextHops).toEqual([{ address: undefined, interface: 'loop0', weight: 1 }]);
    expect(ifaceRoute?.distance).toBe(5);
    expect(ds.management?.users[1]).toEqual({
      username: 'auditor',
      role: 'readonly',
      scope: '*',
      sshKeys: [],
      fullName: 'Audit Account',
      disabled: true,
    });
    expect(ds.vpn?.ipsec?.tunnels['site-b']?.rekey?.espBytes).toBe('1073741824');
  });

  it('64-bit integers are strings (forceLong=string, D-039/F1) and survive values above 2^53', () => {
    const big = '18446744073709551615'; // 2^64 − 1: the default `number` mapping threw here
    const counters = InterfaceCounters.fromJSON({ name: 'x', rxBytes: big, txPackets: '9007199254740993' });
    expect(counters.rxBytes).toBe(big);
    expect(typeof counters.rxBytes).toBe('string');
    const decoded = InterfaceCounters.decode(InterfaceCounters.encode(counters).finish());
    expect(decoded.rxBytes).toBe(big);
    expect(decoded.txPackets).toBe('9007199254740993');
    const batch = StatsBatch.fromPartial({ seq: '1', interfaceCounters: [counters] });
    expect(StatsBatch.decode(StatsBatch.encode(batch).finish()).interfaceCounters[0]?.rxBytes).toBe(big);
    // A Zod document carries 64-bit leaves as JSON numbers; fromJSON accepts them and normalises to
    // string, so the API must diff `toJSON(fromJSON(running))` against `toJSON(actual)` (proto.md §5).
    expect(IpsecRekey.toJSON(IpsecRekey.fromJSON({ espBytes: 1073741824 }))).toEqual({ espBytes: '1073741824' });
  });

  it('JSON names are the Zod (lowerCamelCase) names', () => {
    const json = DesiredState.toJSON(
      DesiredState.fromPartial({
        dataplane: { pciWhitelist: ['0000:0b:00.0'], rxQueues: 2, corelist: [2, 3] },
        interfaces: { loop700: { rxMode: 'polling', subinterfaces: { '10': { vlanId: 10 } } } },
        routing: { static: [{ prefix: '10.70.0.0/16', vrf: 'w7-a', nextHops: [{ address: '10.7.0.254' }] }] },
        nat: { staticMappings: [{ name: 'web', out2inOnly: true }], insideVrf: 'w7-a' },
        vpn: { ipsec: { tunnels: { 'site-b': { ikeVersion: 2, natT: true, auth: { method: 'psk', secretRef: 'psk/site-b' } } } } },
        management: { users: [{ username: 'a', sshKeys: ['ssh-ed25519 AAAA test'] }] },
      }),
    ) as Record<string, unknown>;
    const text = JSON.stringify(json);
    for (const key of ['pciWhitelist', 'rxQueues', 'corelist', 'rxMode', 'vlanId', 'nextHops', 'staticMappings', 'out2inOnly', 'insideVrf', 'ikeVersion', 'natT', 'secretRef', 'sshKeys']) {
      expect(text).toContain(`"${key}"`);
    }
  });

  it('typed construction compiles for every domain and the envelopes', () => {
    const ds = DesiredState.fromPartial({
      system: { hostname: 'vrx-a', timezone: 'UTC', banner: { login: 'hi' }, dns: {} },
      dataplane: { workers: 2, corelist: [2, 3], pciWhitelist: [] },
      interfaces: { loop700: { enabled: true, mtu: 1500, ipv4: ['10.7.0.1/24'], vrf: 'default', rxMode: 'polling', promiscuous: false } },
      vrfs: { default: { id: 0 }, 'w7-a': { id: 7001, description: 'slot 7' } },
      routing: { static: [{ prefix: '10.70.0.0/16', vrf: 'w7-a', nextHops: [{ address: '10.7.0.254', weight: 1 }], distance: 1 }], bgp: {} },
      nat: { enabled: true, mode: 'ed', inside: ['loop700'], pools: [{ name: 'p1', range: '10.7.1.1-10.7.1.10' }] },
      objects: { addresses: { h1: { type: 'host', address: '10.7.0.10' } }, services: { https: { protocol: 'tcp', destinationPorts: ['443'] } } },
      acl: {
        lists: { in: { rules: [{ sequence: 10, enabled: true, action: 'permit', ipVersion: 'any', source: { kind: 'object', name: 'h1' } }] } },
        attachments: [{ list: 'in', target: { kind: 'interface', interface: 'loop700' }, direction: 'in', sequence: 1, enabled: true }],
      },
      vpn: { wireguard: { interfaces: { wg0: { instance: 0, listenPort: 51820, privateKeyRef: 'key/wg0', peers: {} } } } },
      tunnels: { gre: { gre0: {} } },
      services: { dhcp: {} },
      ha: { vrrp: { lan: {} } },
      management: { users: [{ username: 'admin', role: 'admin', scope: '*', sshKeys: [], disabled: false }] },
    });
    for (const key of ROOT_KEYS) {
      expect(ds[key], key).toBeDefined();
    }
    const req = ApplyRequest.fromPartial({ txnId: 't1', desiredState: ds, subsystems: ['interfaces', 'vrfs'], confirmTimeoutSec: 120, owner: 'w7' });
    expect(ApplyRequest.decode(ApplyRequest.encode(req).finish()).desiredState?.interfaces['loop700']?.mtu).toBe(1500);
    // Renamed enums (F10) and Event.interface presence (F10).
    const result = ObjectResult.fromPartial({ key: 'interface/loop700', op: ApplyOperation.APPLY_OPERATION_CREATE, code: ObjectResultCode.OBJECT_RESULT_CODE_OK });
    expect(ObjectResult.toJSON(result)).toEqual({ key: 'interface/loop700', op: 'APPLY_OPERATION_CREATE', code: 'OBJECT_RESULT_CODE_OK' });
    expect(IssueSeverity.ISSUE_SEVERITY_ERROR).toBe(1);
    const ev = Event.fromPartial({ kind: EventKind.EVENT_KIND_RECONCILE_DONE, seq: '1' });
    expect(ev.interface).toBeUndefined();
    expect(Event.toJSON(ev)).not.toHaveProperty('interface');
    const action = ActionRequest.fromPartial({ ping: { target: '10.7.0.254', count: 3 } });
    expect(ActionRequest.decode(ActionRequest.encode(action).finish()).ping?.count).toBe(3);
    const done = ActionOutput.fromPartial({ done: { summary: 'ok', exitCode: 0 } });
    expect(ActionOutput.decode(ActionOutput.encode(done).finish()).done?.summary).toBe('ok');
  });
});
