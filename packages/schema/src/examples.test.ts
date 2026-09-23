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

/** File-name prefixes of the other groups' fixtures (optionally after `invalid-`). */
const SIBLING = /^(?:invalid-)?(?:nat|objects|acl|vpn|tunnels|services|ha)-[a-z0-9-]+\.json$/;

const load = (file: string): unknown => JSON.parse(readFileSync(new URL(file, dir), 'utf8'));

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
