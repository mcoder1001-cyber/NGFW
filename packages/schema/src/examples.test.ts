import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from './index.js';
import { secretPointers } from './secrets.js';
import { validateSemantics } from './semantic/index.js';

/**
 * Group (a) fixtures in `packages/schema/examples/` (review H1, D-048). Each group tests its own files: this suite
 * claims only the files listed in {@link GROUP_A}; nat/objects/acl fixtures are exercised by
 * `semantic/nat-objects-acl-examples.test.ts` (P02b), vpn/tunnels/services/ha ones by P02c's suites. A file that
 * belongs to no group fails here, so nothing lands in the directory untested.
 *
 * Valid documents are also the corpus of the protobuf drift tests (packages/proto/test, apps/agent contracttest):
 * they must not contain secret-flagged leaves (`passwordHash`, D-040) — asserted below.
 */
const dir = new URL('../examples/', import.meta.url);
const files = readdirSync(dir)
  .filter((f) => f.endsWith('.json'))
  .sort();

type Expectation = 'valid' | 'schema' | { semantic: string };

/** Every group (a) fixture and what it must do. `semantic`: the pointer a validator must report. */
const GROUP_A: Record<string, Expectation> = {
  'minimal.json': 'valid',
  'two-interfaces.json': 'valid',
  'group-a-full.json': 'valid',
  'invalid-mtu-out-of-range.json': 'schema',
  'invalid-static-route-family.json': 'schema',
  'invalid-unknown-field.json': 'schema',
  'invalid-unknown-root-key.json': 'schema',
  'invalid-semantic-vrf-missing.json': { semantic: '/interfaces/TenGigabitEthernet0~10~10/vrf' },
  'invalid-semantic-ipv4-overlap.json': {
    semantic: '/interfaces/TenGigabitEthernet0~10~11/ipv4/0',
  },
  'invalid-semantic-vlan-duplicate.json': {
    semantic: '/interfaces/TenGigabitEthernet0~10~10/subinterfaces/101/vlanId',
  },
  'invalid-semantic-nexthop-interface.json': {
    semantic: '/routing/static/0/nextHops/0/interface',
  },
  'invalid-semantic-no-admin.json': { semantic: '/management/users' },
};

/** Group prefixes of the P02b/P02c fixtures. */
const GROUPS = ['nat', 'objects', 'acl', 'vpn', 'tunnels', 'services', 'ha'];

/**
 * Feature rows' example files are named `<slug>-*.json` after their board id without `F-` (tech-debt: F-vrf-static-ecmp
 * Q4, F-vlan-qinq Q3, F-bridge-l2 Q4, F-neighbors-ra Q5, F-loopback Q2; TD-22). The first word of a slug is accepted
 * too (`loopback-*.json` for F-loopback-bvi-gso-lldp-span). Each feature tests its own fixtures; a file matching
 * neither a group, a feature nor {@link GROUP_A} still fails.
 */
const FEATURE_SLUGS = [
  'aaa', 'ab-upgrade', 'acl', 'backup-restore', 'bfd-redistribution', 'bonding', 'bridge-l2', 'capture-trace',
  'dashboard-prom-alarms', 'det44-map-dslite-cnat', 'ha-state-sync', 'hardening-lite', 'host-acl-nftables', 'host-stack',
  'igmp-mfib', 'ikev2-native', 'images', 'ipfix-sflow', 'isis-rip', 'kea-dhcp-relay', 'lb', 'licensing', 'lisp',
  'loopback-bvi-gso-lldp-span', 'mpls-ldp', 'mpls-srmpls', 'nat44-ed-sessions', 'nat44-ei-64-66-nptv6', 'neighbors-ra',
  'object-model', 'ospf', 'pki', 'qos-flat', 'ra-vpn', 'restconf-yang', 'rpf-adl-pbr', 'sdk-terraform-ansible', 'snmp',
  'srv6', 'startup-apply', 'startup-gen', 'tunnels', 'unbound-chrony-syslog', 'vlan-qinq', 'vpp-debs', 'vrf-static-ecmp',
  'vrrp-config-sync', 'wireguard',
];

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const prefixes = [...new Set([...GROUPS, ...FEATURE_SLUGS, ...FEATURE_SLUGS.map((s) => s.split('-')[0]!)])];
/** File names of the other groups' and the feature rows' fixtures (optionally after `invalid-`). */
const SIBLING = new RegExp(`^(?:invalid-)?(?:${prefixes.map(escape).join('|')})-[a-z0-9-]+\\.json$`);

const load = (file: string): unknown => JSON.parse(readFileSync(new URL(file, dir), 'utf8'));

describe('SIBLING (TD-22)', () => {
  it('accepts group and feature fixture names, rejects unknown prefixes and odd names', () => {
    for (const f of ['nat-basic.json', 'invalid-vpn-inline-psk.json', 'vrf-static-ecmp-basic.json', 'vlan-qinq-semantic-invalid-tpid.json',
      'bridge-l2-bvi.json', 'invalid-neighbors-ra-lifetime.json', 'loopback-bvi.json', 'loopback-bvi-gso-lldp-span-full.json'])
      expect(SIBLING.test(f), f).toBe(true);
    for (const f of ['typo-basic.json', 'nat.json', 'Nat-basic.json', 'nat-basic.JSON', 'vrfx-basic.json', '../nat-basic.json', 'nat-basic.json.bak'])
      expect(SIBLING.test(f), f).toBe(false);
  });
});

describe('examples (group a)', () => {
  it('every listed group (a) fixture exists', () => {
    expect(files).toEqual(expect.arrayContaining(Object.keys(GROUP_A)));
  });

  it('every other file belongs to a sibling group', () => {
    expect(files.filter((f) => !(f in GROUP_A) && !SIBLING.test(f))).toEqual([]);
  });

  for (const [file, expectation] of Object.entries(GROUP_A)) {
    if (expectation === 'valid') {
      it(`${file} is accepted, semantically clean, idempotent and free of secret leaves`, () => {
        const doc = load(file);
        const parsed = RootConfig.safeParse(doc);
        expect(parsed.error?.issues).toBeUndefined();
        expect(validateSemantics(parsed.data!)).toEqual([]);
        expect(RootConfig.parse(parsed.data!)).toEqual(parsed.data);
        expect(secretPointers(doc)).toEqual([]);
      });
    } else if (expectation === 'schema') {
      it(`${file} is rejected by the schema`, () => {
        expect(RootConfig.safeParse(load(file)).success).toBe(false);
      });
    } else {
      it(`${file} passes the schema but fails semantic validation at ${expectation.semantic}`, () => {
        const parsed = RootConfig.safeParse(load(file));
        expect(parsed.success).toBe(true);
        const pointers = validateSemantics(parsed.data!).map((i) => i.pointer);
        expect(pointers).toContain(expectation.semantic);
      });
    }
  }
});
