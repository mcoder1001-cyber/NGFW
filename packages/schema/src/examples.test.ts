import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from './index.js';

// Every packages/schema/examples/*.json is parsed: `invalid-*` must be rejected, the rest accepted.
// TODO(P02a/b/c): add `invalid-<rule>.json` fixtures for your semantic validators (run them through
// validateSemantics as well).
const dir = new URL('../examples/', import.meta.url);
const files = readdirSync(dir)
  .filter((f) => f.endsWith('.json'))
  .sort();

describe('examples', () => {
  it('include minimal.json and two-interfaces.json', () => {
    expect(files).toEqual(expect.arrayContaining(['minimal.json', 'two-interfaces.json']));
  });

  for (const file of files) {
    const document: unknown = JSON.parse(readFileSync(new URL(file, dir), 'utf8'));
    if (file.startsWith('invalid-')) {
      it(`${file} is rejected`, () => expect(RootConfig.safeParse(document).success).toBe(false));
    } else {
      it(`${file} is accepted`, () => expect(RootConfig.safeParse(document).success).toBe(true));
    }
  }
});
