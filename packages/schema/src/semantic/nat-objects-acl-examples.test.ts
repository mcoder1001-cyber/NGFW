import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';

/**
 * Every `examples/{nat,objects,acl}-*.json` is schema-valid (examples.test.ts already checks that). Here the
 * documents also go through tier (b): `*-semantic-invalid-<rule>.json` must produce exactly the documented
 * pointers, everything else must be clean. Keep this table in sync with docs/contracts/schema-nat-objects-acl.md.
 */
const EXPECTED_POINTERS: Record<string, string[]> = {
  'nat-semantic-invalid-overlapping-pools.json': ['/nat/pools/1/range'],
  'nat-semantic-invalid-port-without-protocol.json': ['/nat/staticMappings/0/protocol'],
  'nat-semantic-invalid-inside-outside.json': ['/nat/outside/0', '/nat/outside/1'],
  'acl-semantic-invalid-unknown-object.json': ['/acl/lists/lan-in/rules/0/source/name'],
  'acl-semantic-invalid-vrf-mismatch.json': ['/acl/attachments/0/vrf'],
  'objects-semantic-invalid-group-cycle.json': ['/objects/addressGroups/b/members/0'],
  'objects-semantic-invalid-zone-interface.json': ['/objects/zones/lan/interfaces/0'],
};

const dir = new URL('../../examples/', import.meta.url);
const files = readdirSync(dir)
  .filter((f) => /^(nat|objects|acl)-.*\.json$/.test(f))
  .sort();

describe('group (b) examples through validateSemantics', () => {
  it('has a fixture for every documented semantic-invalid case', () => {
    expect(files).toEqual(expect.arrayContaining(Object.keys(EXPECTED_POINTERS)));
    expect(files).toEqual(
      expect.arrayContaining([
        'nat-basic.json',
        'nat-cgnat.json',
        'objects-basic.json',
        'acl-basic.json',
      ]),
    );
  });

  for (const file of files) {
    const document: unknown = JSON.parse(readFileSync(new URL(file, dir), 'utf8'));
    const expected = EXPECTED_POINTERS[file];
    if (file.includes('-semantic-invalid-')) {
      it(`${file} is schema-valid and reports ${expected?.join(', ')}`, () => {
        expect(expected).toBeDefined();
        const issues = validateSemantics(RootConfig.parse(document));
        expect(issues.map((i) => i.pointer)).toEqual(expected);
      });
    } else {
      it(`${file} is schema-valid and semantically clean`, () => {
        expect(validateSemantics(RootConfig.parse(document))).toEqual([]);
      });
    }
  }
});
