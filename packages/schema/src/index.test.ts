import { describe, expect, it } from 'vitest';
import { RootConfig, ROOT_KEYS } from './index.js';

describe('RootConfig', () => {
  it('accepts an empty document and fills defaults', () => {
    const r = RootConfig.parse({});
    expect(Object.keys(r).sort()).toEqual([...ROOT_KEYS].sort());
  });
  it('rejects unknown top-level keys', () => {
    expect(() => RootConfig.parse({ bogus: 1 })).toThrow();
  });
});
