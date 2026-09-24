import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';
import { nat44EdSessionsValidators } from './nat44-ed-sessions.js';
import { sortIssues, type SemanticIssue } from './registry.js';

// Slot-4 style names and addresses (docs/lab/shared-host-rules.md): the rig's af_packet interfaces, 10.4.0.0/16.
const interfaces = {
  'host-w4l0': { ipv4: ['10.4.1.1/24'] },
  'host-w4w0': { ipv4: ['10.4.2.1/24'] },
};
const vrfs = { cust: { id: 4001 } };

const doc = (nat: unknown) => RootConfig.parse({ interfaces, vrfs, nat });

function adjacent(nat: unknown): SemanticIssue[] {
  const v = nat44EdSessionsValidators[0];
  if (v === undefined) throw new Error('no validator');
  return sortIssues(v.validate(doc(nat)));
}

/** Every finding of the full registry for the NAT44-ED paths the agent projects. */
const all = (nat: unknown) => validateSemantics(doc(nat), ['nat', 'interfaces', 'vrfs']);
const at = (issues: SemanticIssue[]) => issues.map((i) => i.pointer);

describe('nat.nat44-ed-sessions-adjacent-pools', () => {
  it('rejects two range pools that touch in one twice-NAT class and VRF, pointing at the later one', () => {
    const issues = adjacent({
      pools: [
        { name: 'a', range: '10.4.2.100-10.4.2.104' },
        { name: 'b', range: '10.4.2.105-10.4.2.110' },
      ],
    });
    expect(issues).toHaveLength(1);
    expect(issues[0]?.pointer).toBe('/nat/pools/1/range');
    expect(issues[0]?.message).toMatch(/touches pool 'a'.*one range/);
  });

  it('checks both directions and single-address pools; "default" equals a missing vrf', () => {
    expect(
      at(
        adjacent({
          pools: [
            { name: 'hi', range: '10.4.2.105', vrf: 'default' },
            { name: 'lo', range: '10.4.2.100-10.4.2.104' },
          ],
        }),
      ),
    ).toEqual(['/nat/pools/1/range']);
  });

  it('accepts touching pools of different VRFs or twice-NAT classes, gaps, and interface pools', () => {
    expect(
      adjacent({
        pools: [
          { name: 'a', range: '10.4.2.100-10.4.2.104' },
          { name: 'b', range: '10.4.2.105-10.4.2.110', vrf: 'cust' },
          { name: 'tn', range: '10.4.2.111', twiceNat: true },
          { name: 'c', range: '10.4.2.112-10.4.2.120' },
          { name: 'wan', interface: 'host-w4w0' },
        ],
      }),
    ).toEqual([]);
  });

  it('leaves reversed ranges to nat.pools-valid (no second finding)', () => {
    expect(
      adjacent({
        pools: [
          { name: 'a', range: '10.4.2.100-10.4.2.104' },
          { name: 'rev', range: '10.4.2.110-10.4.2.105' },
        ],
      }),
    ).toEqual([]);
  });

  it('is registered in SEMANTIC_VALIDATORS (C2 spread line)', () => {
    const issues = all({
      pools: [
        { name: 'a', range: '10.4.2.100-10.4.2.104' },
        { name: 'b', range: '10.4.2.105' },
      ],
    });
    expect(issues).toContainEqual(expect.objectContaining({ pointer: '/nat/pools/1/range' }));
  });
});

// The existing P02b rules on the NAT44-ED paths the agent projects (task scope 1: tests only, the rules are P02b's).
describe('existing NAT44 rules on the projected ED paths', () => {
  it('inside and outside must be disjoint', () => {
    expect(at(all({ inside: ['host-w4l0'], outside: ['host-w4l0', 'host-w4w0'] }))).toEqual([
      '/nat/outside/0',
    ]);
  });

  it('overlapping range pools are rejected with a pointer at the later pool', () => {
    const issues = all({
      pools: [
        { name: 'a', range: '10.4.2.100-10.4.2.110' },
        { name: 'b', range: '10.4.2.105-10.4.2.120' },
      ],
    });
    expect(issues).toContainEqual({
      pointer: '/nat/pools/1/range',
      message: "range '10.4.2.105-10.4.2.120' overlaps pool 'a'",
    });
  });

  it('an external port without protocol is rejected at /protocol', () => {
    const issues = all({
      pools: [{ name: 'p', range: '10.4.2.100' }],
      staticMappings: [
        {
          name: 'web',
          local: { ip: '10.4.1.2', port: 80 },
          external: { ip: '10.4.2.100', port: 8080 },
        },
      ],
    });
    expect(issues).toContainEqual({
      pointer: '/nat/staticMappings/0/protocol',
      message: 'protocol is required when ports are set',
    });
  });

  it('a clean outbound + 1:1 + port-forward document has no finding', () => {
    expect(
      all({
        inside: ['host-w4l0'],
        outside: ['host-w4w0'],
        pools: [{ name: 'out', range: '10.4.2.100-10.4.2.103' }],
        staticMappings: [
          { name: 'one2one', local: { ip: '10.4.1.3' }, external: { ip: '10.4.2.110' } },
          {
            name: 'web',
            protocol: 'tcp',
            local: { ip: '10.4.1.2', port: 80 },
            external: { ip: '10.4.2.111', port: 8080 },
          },
        ],
      }),
    ).toEqual([]);
  });
});
