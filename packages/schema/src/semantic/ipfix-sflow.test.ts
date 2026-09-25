import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { ipfixSflowValidators } from './ipfix-sflow.js';
import { sortIssues } from './registry.js';

const IF = 'host-w1a';
const run = (ipfix: Record<string, unknown>) =>
  sortIssues(
    ipfixSflowValidators.flatMap((v) => v.validate(RootConfig.parse({ services: { ipfix } }))),
  );
const exporter = (address: string, extra: Record<string, unknown> = {}) => ({
  collector: { address, port: 4739 },
  sourceAddress: address.includes(':') ? '2001:db8::1' : '10.1.1.1',
  ...extra,
});
const probe = (extra: Record<string, unknown> = {}) => ({
  interfaces: [{ interface: IF, ip4: true, ip6: false, ...extra }],
});

describe('ipfix-sflow validators', () => {
  it('names are prefixed and unique', () => {
    for (const v of ipfixSflowValidators)
      expect(v.name.startsWith('services.ipfix-sflow-')).toBe(true);
    expect(new Set(ipfixSflowValidators.map((v) => v.name)).size).toBe(ipfixSflowValidators.length);
  });

  it('accepts flowprobe with an enabled IPv4 exporter', () => {
    expect(run({ exporters: { lan: exporter('10.1.1.9') }, flowprobe: probe() })).toEqual([]);
  });

  it('flowprobe interfaces without an enabled IPv4 exporter point at the interface list', () => {
    const want = [
      {
        pointer: '/services/ipfix/flowprobe/interfaces',
        message:
          'flowprobe records are sent through IPFIX exporter 0 only: enable an exporter with an IPv4 collector',
      },
    ];
    expect(run({ flowprobe: probe() })).toEqual(want);
    expect(
      run({ exporters: { lan: exporter('10.1.1.9', { enabled: false }) }, flowprobe: probe() }),
    ).toEqual(want);
    expect(run({ exporters: { v6: exporter('2001:db8::9') }, flowprobe: probe() })).toEqual(want);
  });

  it('exactly one flowprobe variant per interface', () => {
    const issues = run({
      exporters: { lan: exporter('10.1.1.9') },
      flowprobe: {
        interfaces: [{ interface: IF }, { interface: 'loop1', l2: true, ip4: true, ip6: false }],
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      '/services/ipfix/flowprobe/interfaces/0',
      '/services/ipfix/flowprobe/interfaces/1',
    ]);
  });

  it('sFlow header bytes in steps of 32', () => {
    const sflow = (headerBytes: number) => ({
      enabled: true,
      headerBytes,
      collectors: [{ address: '10.1.1.9' }],
      interfaces: [IF],
    });
    expect(run({ sflow: sflow(160) })).toEqual([]);
    expect(run({ sflow: sflow(100) })).toEqual([
      {
        pointer: '/services/ipfix/sflow/headerBytes',
        message: 'sampled header bytes must be a multiple of 32 (VPP rounds 100 silently)',
      },
    ]);
  });

  it('enabled exporters need distinct collector addresses', () => {
    const issues = run({
      exporters: {
        a: exporter('10.1.1.9'),
        b: exporter('10.1.1.9', { collector: { address: '10.1.1.9', port: 2055 } }),
        c: exporter('10.1.1.9', { enabled: false }),
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual(['/services/ipfix/exporters/b/collector/address']);
  });
});
