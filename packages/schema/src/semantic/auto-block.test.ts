import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { autoBlockValidators } from './auto-block.js';

const run = (doc: RootConfigInput) => autoBlockValidators[0]!.validate(RootConfig.parse(doc));

describe('F-bruteforce-block semantic rules', () => {
  it('accepts a well-formed policy', () => {
    expect(
      run({
        security: {
          autoBlock: {
            enabled: true,
            rules: [{ source: 'webLogin' }],
            allowlist: ['10.0.0.0/8', '2001:db8::/48'],
          },
        },
      }),
    ).toEqual([]);
  });

  it('flags an enabled policy with no enabled rule', () => {
    const issues = run({
      security: { autoBlock: { enabled: true, rules: [{ source: 'ssh', enabled: false }] } },
    });
    expect(issues).toHaveLength(1);
    expect(issues[0]!.pointer).toBe('/security/autoBlock/enabled');
  });

  it('does not flag a disabled policy with no rules', () => {
    expect(run({ security: { autoBlock: { enabled: false } } })).toEqual([]);
  });

  it('flags two allow-list entries covering the same range', () => {
    const issues = run({
      security: {
        autoBlock: {
          enabled: true,
          rules: [{ source: 'webLogin' }],
          allowlist: ['192.0.2.5', '192.0.2.5/32'],
        },
      },
    });
    expect(issues).toHaveLength(1);
    expect(issues[0]!.pointer).toBe('/security/autoBlock/allowlist/1');
  });
});
