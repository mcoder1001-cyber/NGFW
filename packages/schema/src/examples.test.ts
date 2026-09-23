import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from './index.js';
import { validateSemantics } from './semantic/index.js';

/**
 * Every `packages/schema/examples/*.json` is exercised:
 * - `invalid-semantic-<rule>.json`: schema-valid, but `validateSemantics` reports the pointer listed below;
 * - other `invalid-*.json`: rejected by `RootConfig` (tier a);
 * - everything else: accepted, semantically clean, and idempotent under re-parse.
 */
const dir = new URL('../examples/', import.meta.url);
const files = readdirSync(dir)
  .filter((f) => f.endsWith('.json'))
  .sort();

/** The first pointer each semantic fixture must be reported at. Add a row when adding a fixture. */
const SEMANTIC_POINTERS: Record<string, string> = {
  'invalid-semantic-vrf-missing.json': '/interfaces/TenGigabitEthernet0~10~10/vrf',
  'invalid-semantic-ipv4-overlap.json': '/interfaces/TenGigabitEthernet0~10~11/ipv4/0',
  'invalid-semantic-vlan-duplicate.json':
    '/interfaces/TenGigabitEthernet0~10~10/subinterfaces/101/vlanId',
  'invalid-semantic-nexthop-interface.json': '/routing/static/0/nextHops/0/interface',
  'invalid-semantic-no-admin.json': '/management/users',
};

const load = (file: string): unknown => JSON.parse(readFileSync(new URL(file, dir), 'utf8'));

describe('examples', () => {
  it('include minimal.json, two-interfaces.json and every listed semantic fixture', () => {
    expect(files).toEqual(
      expect.arrayContaining([
        'minimal.json',
        'two-interfaces.json',
        ...Object.keys(SEMANTIC_POINTERS),
      ]),
    );
  });

  for (const file of files) {
    if (file.startsWith('invalid-semantic-')) {
      it(`${file} passes the schema but fails semantic validation`, () => {
        const parsed = RootConfig.safeParse(load(file));
        expect(parsed.success).toBe(true);
        const pointers = validateSemantics(parsed.data!).map((i) => i.pointer);
        expect(pointers).toContain(SEMANTIC_POINTERS[file]);
      });
    } else if (file.startsWith('invalid-')) {
      it(`${file} is rejected by the schema`, () => {
        expect(RootConfig.safeParse(load(file)).success).toBe(false);
      });
    } else {
      it(`${file} is accepted, semantically clean and idempotent`, () => {
        const parsed = RootConfig.safeParse(load(file));
        expect(parsed.error?.issues).toBeUndefined();
        expect(validateSemantics(parsed.data!)).toEqual([]);
        expect(RootConfig.parse(parsed.data!)).toEqual(parsed.data);
      });
    }
  }
});
