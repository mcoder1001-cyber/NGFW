import { describe, expect, it } from 'vitest';
import {
  blockDurationSec,
  canonicalSource,
  compileAllowlist,
  isAllowlisted,
  parseIp,
  parsePrefix,
  SlidingWindows,
} from './engine.js';

describe('auto-block IP helpers', () => {
  it('parses IPv4 and IPv6 addresses', () => {
    expect(parseIp('192.0.2.1')).toEqual({ family: 4, value: (192n << 24n) | (2n << 8n) | 1n });
    expect(parseIp('::1')).toEqual({ family: 6, value: 1n });
    expect(parseIp('2001:db8::1')?.family).toBe(6);
    expect(parseIp('nope')).toBeUndefined();
    expect(parseIp('256.0.0.1')).toBeUndefined();
  });

  it('canonicalises a source to a single host prefix', () => {
    expect(canonicalSource('192.0.2.5')).toBe('192.0.2.5/32');
    expect(canonicalSource('2001:db8::1')).toBe('2001:db8::1/128');
    expect(canonicalSource('2001:0db8:0000:0000:0000:0000:0000:0001')).toBe('2001:db8::1/128');
    expect(canonicalSource('bad')).toBeUndefined();
  });

  it('parses prefixes and bare addresses', () => {
    const p = parsePrefix('10.0.0.0/8')!;
    expect(p.family).toBe(4);
    expect(parsePrefix('10.1.2.3')!.first).toBe(parseIp('10.1.2.3')!.value);
    expect(parsePrefix('10.0.0.0/33')).toBeUndefined();
    expect(parsePrefix('x/8')).toBeUndefined();
  });

  it('masks host bits in a prefix range', () => {
    const p = parsePrefix('10.1.2.3/24')!;
    expect(p.first).toBe(parseIp('10.1.2.0')!.value);
    expect(p.last).toBe(parseIp('10.1.2.255')!.value);
  });
});

describe('auto-block allow-list', () => {
  it('always allow-lists loopback', () => {
    const allow = compileAllowlist([]);
    expect(isAllowlisted(allow, '127.0.0.1')).toBe(true);
    expect(isAllowlisted(allow, '::1')).toBe(true);
    expect(isAllowlisted(allow, '192.0.2.1')).toBe(false);
  });

  it('matches configured prefixes and single addresses', () => {
    const allow = compileAllowlist(['10.0.0.0/8', '192.0.2.7', '2001:db8::/48']);
    expect(isAllowlisted(allow, '10.9.9.9')).toBe(true);
    expect(isAllowlisted(allow, '192.0.2.7')).toBe(true);
    expect(isAllowlisted(allow, '192.0.2.8')).toBe(false);
    expect(isAllowlisted(allow, '2001:db8:0:1::5')).toBe(true);
    expect(isAllowlisted(allow, '2001:dead::1')).toBe(false);
  });

  it('does not cross address families', () => {
    const allow = compileAllowlist(['0.0.0.0/0']);
    expect(isAllowlisted(allow, '10.0.0.1')).toBe(true);
    expect(isAllowlisted(allow, '2001:db8::1')).toBe(false);
  });

  it('ignores an unparsable allow-list entry', () => {
    const allow = compileAllowlist(['not-an-ip', '10.0.0.0/8']);
    expect(isAllowlisted(allow, '10.0.0.1')).toBe(true);
  });
});

describe('auto-block escalation', () => {
  const rule = { blockSec: 900, escalate: true, maxBlockSec: 86_400 };
  it('holds the first offence at blockSec', () => {
    expect(blockDurationSec(1, rule)).toBe(900);
  });
  it('doubles per repeat offence', () => {
    expect(blockDurationSec(2, rule)).toBe(1800);
    expect(blockDurationSec(3, rule)).toBe(3600);
  });
  it('caps at maxBlockSec', () => {
    expect(blockDurationSec(20, rule)).toBe(86_400);
  });
  it('does not escalate when escalate is off', () => {
    expect(blockDurationSec(5, { blockSec: 900, escalate: false, maxBlockSec: 86_400 })).toBe(900);
  });
  it('never exceeds the cap even on the first offence', () => {
    expect(blockDurationSec(1, { blockSec: 5000, escalate: true, maxBlockSec: 60 })).toBe(60);
  });
});

describe('auto-block sliding windows', () => {
  it('trips once the threshold is reached inside the window', () => {
    const w = new SlidingWindows();
    let last = w.observe('webLogin', '1.1.1.1/32', 0, 60, 3);
    expect(last.tripped).toBe(false);
    last = w.observe('webLogin', '1.1.1.1/32', 1000, 60, 3);
    expect(last.tripped).toBe(false);
    last = w.observe('webLogin', '1.1.1.1/32', 2000, 60, 3);
    expect(last).toEqual({ count: 3, tripped: true });
  });

  it('forgets samples that fall out of the window', () => {
    const w = new SlidingWindows();
    w.observe('ssh', 'a/32', 0, 10, 3);
    w.observe('ssh', 'a/32', 1000, 10, 3);
    // 20s later the first two are outside the 10s window
    const r = w.observe('ssh', 'a/32', 20_000, 10, 3);
    expect(r).toEqual({ count: 1, tripped: false });
  });

  it('keeps sources and detectors independent', () => {
    const w = new SlidingWindows();
    w.observe('webLogin', 'a/32', 0, 60, 2);
    const other = w.observe('webLogin', 'b/32', 0, 60, 2);
    expect(other.tripped).toBe(false);
    const ssh = w.observe('ssh', 'a/32', 0, 60, 2);
    expect(ssh.tripped).toBe(false);
  });

  it('clear() resets a source', () => {
    const w = new SlidingWindows();
    w.observe('webLogin', 'a/32', 0, 60, 2);
    w.clear('webLogin', 'a/32');
    const r = w.observe('webLogin', 'a/32', 100, 60, 2);
    expect(r.count).toBe(1);
  });

  it('prune() drops stale windows', () => {
    const w = new SlidingWindows();
    w.observe('webLogin', 'a/32', 0, 60, 5);
    expect(w.size).toBe(1);
    w.prune(1_000_000, 86_400_000);
    // still within a day
    expect(w.size).toBe(1);
    w.prune(200_000_000, 86_400_000);
    expect(w.size).toBe(0);
  });
});
