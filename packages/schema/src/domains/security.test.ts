import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { SecuritySchema, type AutoBlockRule } from './security.js';

const rule = (over: Partial<AutoBlockRule> = {}): AutoBlockRule =>
  SecuritySchema.parse({ autoBlock: { rules: [{ source: 'webLogin', ...over }] } }).autoBlock.rules[0]!;

describe('security domain', () => {
  it('accepts an empty object with autoBlock disabled by default', () => {
    const s = SecuritySchema.parse({});
    expect(s.autoBlock.enabled).toBe(false);
    expect(s.autoBlock.rules).toEqual([]);
    expect(s.autoBlock.maxEntries).toBe(10_000);
  });

  it('is wired into RootConfig with defaults', () => {
    const c = RootConfig.parse({});
    expect(c.security.autoBlock.enabled).toBe(false);
  });

  it('applies rule defaults', () => {
    const r = rule();
    expect(r).toMatchObject({
      source: 'webLogin',
      enabled: true,
      threshold: 5,
      windowSec: 60,
      blockSec: 900,
      escalate: true,
      maxBlockSec: 86_400,
    });
  });

  it('rejects two rules for the same source', () => {
    const doc: RootConfigInput = {
      security: {
        autoBlock: { rules: [{ source: 'ssh' }, { source: 'ssh' }] },
      },
    };
    expect(RootConfig.safeParse(doc).success).toBe(false);
  });

  it('rejects an escalation cap below the first-offence block', () => {
    const doc: RootConfigInput = {
      security: {
        autoBlock: { rules: [{ source: 'ssh', blockSec: 3600, escalate: true, maxBlockSec: 60 }] },
      },
    };
    expect(RootConfig.safeParse(doc).success).toBe(false);
  });

  it('allows a low cap when escalation is off', () => {
    const doc: RootConfigInput = {
      security: {
        autoBlock: { rules: [{ source: 'ssh', blockSec: 3600, escalate: false, maxBlockSec: 60 }] },
      },
    };
    expect(RootConfig.safeParse(doc).success).toBe(true);
  });

  it('accepts allow-list addresses and prefixes', () => {
    const doc: RootConfigInput = {
      security: { autoBlock: { allowlist: ['10.0.0.0/8', '192.0.2.5', '2001:db8::/48'] } },
    };
    expect(RootConfig.safeParse(doc).success).toBe(true);
  });

  it('rejects a bad allow-list entry', () => {
    const doc: RootConfigInput = {
      security: { autoBlock: { allowlist: ['not-an-ip'] } },
    };
    expect(RootConfig.safeParse(doc).success).toBe(false);
  });
});
