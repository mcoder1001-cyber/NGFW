import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { mplsLdpValidators } from './mpls-ldp.js';

const run = (doc: RootConfigInput) => mplsLdpValidators[0]!.validate(RootConfig.parse(doc));

const base = (ldp: object, mplsExtra: object = {}): RootConfigInput =>
  ({
    interfaces: { eth0: { enabled: true, ipv4: ['10.0.0.1/32'] }, eth1: { enabled: true } },
    routing: { mpls: { interfaces: ['eth0'], ldp: { routerId: '10.0.0.1', transportAddress: '10.0.0.1', interfaces: ['eth0'], ...ldp }, ...mplsExtra } },
  }) as RootConfigInput;

describe('F-mpls-ldp semantic rules', () => {
  it('accepts a well-formed LDP config', () => {
    expect(run(base({}))).toEqual([]);
  });

  it('flags an LDP interface that is not MPLS-enabled', () => {
    const issues = run(base({ interfaces: ['eth1'] }));
    expect(issues.some((i) => i.message.includes('not MPLS-enabled'))).toBe(true);
  });

  it('flags an unknown LDP interface', () => {
    const issues = run(base({ interfaces: ['ghost0'] }, { interfaces: ['eth0', 'ghost0'] }));
    expect(issues.some((i) => i.message.includes("'ghost0' does not exist"))).toBe(true);
  });

  it('flags a transport address that is not configured locally', () => {
    const issues = run(base({ transportAddress: '192.0.2.99' }));
    expect(issues.some((i) => i.pointer.endsWith('/transportAddress'))).toBe(true);
  });

  it('flags a static label inside the LDP dynamic range', () => {
    const issues = run(
      base(
        { labelRange: { min: 16000, max: 24000 } },
        { labelRoutes: [{ table: 0, label: 17000, eos: true, paths: [{ interface: 'eth0', outLabels: [100] }] }] },
      ),
    );
    expect(issues.some((i) => i.message.includes('LDP dynamic range'))).toBe(true);
  });
});
