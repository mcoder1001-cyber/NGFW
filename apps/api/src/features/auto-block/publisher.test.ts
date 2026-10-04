import { describe, expect, it } from 'vitest';
import { compileAllowlist } from './engine.js';
import { SnapshotPublisher, runtimeSources } from './publisher.js';
const tick = () => new Promise<void>((r) => setImmediate(r));
describe('runtime snapshot publication', () => {
  it('serializes an updated snapshot behind an in-flight older one', async () => {
    let release!: () => void;
    const sent: number[] = [];
    let value = 1;
    const p = new SnapshotPublisher(
      async () => {
        sent.push(value);
        if (sent.length === 1)
          await new Promise<void>((r) => {
            release = r;
          });
      },
      () => {},
    );
    p.request();
    value = 2;
    p.request();
    expect(sent).toEqual([1]);
    release();
    await tick();
    expect(sent).toEqual([1, 2]);
    p.stop();
  });
  it('retains desired publication after failure and stops cleanly', async () => {
    let tries = 0;
    let failures = 0;
    const p = new SnapshotPublisher(
      async () => {
        if (++tries === 1) throw new Error('offline');
      },
      () => {
        failures++;
      },
    );
    p.request();
    await tick();
    expect(failures).toBe(1);
    p.request();
    await tick();
    expect(tries).toBe(2);
    p.stop();
    p.request();
    await tick();
    expect(tries).toBe(2);
  });
});

it('converts database host prefixes to bare RPC addresses and filters allowlisted hosts', () => {
  const expiry = '2026-10-03T22:00:00.000Z';
  expect(
    runtimeSources(
      [
        { source: '192.0.2.7/32', expiresAt: expiry },
        { source: '2001:db8::7/128', expiresAt: expiry },
        { source: '192.0.2.8/32', expiresAt: expiry },
        { source: '192.0.2.0/24', expiresAt: expiry },
      ],
      compileAllowlist(['192.0.2.8']),
    ),
  ).toEqual([
    { source: '192.0.2.7', expiresAt: new Date(expiry) },
    { source: '2001:db8::7', expiresAt: new Date(expiry) },
  ]);
});
it('deduplicates mapped IPv4 aliases and protects IPv4 loopback/management', () => {
  const entries = [
    { source: '192.0.2.7/32', expiresAt: '2026-10-03T22:00:00Z' },
    { source: '::ffff:c000:207/128', expiresAt: '2026-10-03T23:00:00Z' },
    { source: '::ffff:7f00:1/128', expiresAt: '2026-10-03T23:00:00Z' },
    { source: '::ffff:c633:6407/128', expiresAt: '2026-10-03T23:00:00Z' },
  ];
  expect(runtimeSources(entries, compileAllowlist(['198.51.100.0/24']))).toEqual([
    { source: '192.0.2.7', expiresAt: new Date('2026-10-03T23:00:00Z') },
  ]);
});
