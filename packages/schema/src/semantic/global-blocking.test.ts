import { describe, expect, it } from 'vitest';
import { validateConfig } from '../index.js';

const cfg = (gb: unknown) => ({
  interfaces: { loop1: { enabled: true } },
  acl: { globalBlocking: gb },
});

describe('F-global-blocking semantics', () => {
  it('accepts a canonical list on an existing interface, and All interfaces', () => {
    expect(validateConfig(cfg({ lists: { bad: { interfaces: ['loop1'], entries: ['192.0.2.7/32', '2001:db8::/48'] } } })).ok).toBe(true);
    expect(validateConfig(cfg({ lists: { any: { allInterfaces: true, entries: [] } } })).ok).toBe(true);
  });
  it('refuses an unknown interface, no interface, non-canonical and duplicate entries — with pointers', () => {
    const r = validateConfig(
      cfg({ lists: { x: { interfaces: ['nope'], entries: ['192.0.2.7/24', '192.0.2.0/24', '192.0.2.0/24', 'garbage/1'] }, y: {} } }),
    );
    expect(r.ok).toBe(false);
    const got = (r as { issues: { pointer: string }[] }).issues.map((i) => i.pointer).sort();
    expect(got).toEqual(
      expect.arrayContaining([
        '/acl/globalBlocking/lists/x/entries/0',
        '/acl/globalBlocking/lists/x/entries/2',
        '/acl/globalBlocking/lists/x/entries/3',
        '/acl/globalBlocking/lists/y/interfaces',
      ]),
    );
  });
  it('checks interface existence once the schema passes', () => {
    const r = validateConfig(cfg({ lists: { x: { interfaces: ['nope'], entries: [] } } }));
    expect(r.ok).toBe(false);
    expect((r as { issues: { pointer: string; message: string }[] }).issues).toEqual([
      expect.objectContaining({ pointer: '/acl/globalBlocking/lists/x/interfaces/0', message: "interface 'nope' does not exist" }),
    ]);
  });
});
