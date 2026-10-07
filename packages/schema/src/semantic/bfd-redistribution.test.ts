import { describe, it, expect } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { bfdRedistributionValidators, redistributionWarnings } from './bfd-redistribution.js';
const run = (doc: RootConfigInput) =>
  bfdRedistributionValidators[0]!.validate(RootConfig.parse(doc));
const session = { interface: 'loop0', localAddress: '10.14.1.1', peerAddress: '10.14.1.2' };
describe('BFD port ownership', () => {
  it('rejects different peers on the same box and AF, including admin down sessions', () => {
    expect(
      run({
        interfaces: { loop0: { ipv4: ['10.14.1.1/24'] } },
        routing: {
          bfd: { sessions: [{ ...session, enabled: false }] },
          bgp: { asn: 65000, neighbors: { '10.14.2.2': { remoteAs: 65001, bfd: true } } },
        },
      }).map((i) => i.pointer),
    ).toContain('/routing/bgp/neighbors/10.14.2.2/bfd');
  });
  it('allows distinct address families', () => {
    expect(
      run({
        interfaces: { loop0: { ipv4: ['10.14.1.1/24'] } },
        routing: {
          bfd: { sessions: [session] },
          bgp: { asn: 65000, neighbors: { '2001:db8::2': { remoteAs: 65001, bfd: true } } },
        },
      }),
    ).toEqual([]);
  });
  it('rejects a local address not assigned to the interface', () => {
    expect(
      run({
        interfaces: { loop0: { ipv4: ['10.14.1.9/24'] } },
        routing: { bfd: { sessions: [session] } },
      })[0]?.pointer,
    ).toBe('/routing/bfd/sessions/0/localAddress');
  });
  it('keeps loop warnings advisory', () => {
    const c = RootConfig.parse({
      routing: {
        bgp: { asn: 65000, redistribute: { ospf: {} } },
        ospf: { redistribute: { bgp: {} } },
      },
    });
    expect(redistributionWarnings(c)).toHaveLength(1);
    expect(bfdRedistributionValidators[0]!.validate(c)).toEqual([]);
  });
});
it('finds inherited peer-group BFD at the group pointer', () => {
  expect(
    run({
      interfaces: { loop0: { ipv4: ['10.14.1.1/24'] } },
      routing: {
        bfd: { sessions: [session] },
        bgp: {
          asn: 65000,
          peerGroups: { upstream: { remoteAs: 65001, bfd: true } },
          neighbors: { '10.14.2.2': { peerGroup: 'upstream' } },
        },
      },
    }).map((i) => i.pointer),
  ).toContain('/routing/bgp/peerGroups/upstream/bfd');
});
it('allows FRR multihop with only VPP single-hop sessions', () => {
  expect(
    run({
      interfaces: { loop0: { ipv4: ['10.14.1.1/24'] } },
      routing: {
        bfd: { sessions: [session] },
        bgp: {
          asn: 65000,
          neighbors: { '10.14.2.2': { remoteAs: 65001, bfd: true, ebgpMultihop: 2 } },
        },
      },
    }),
  ).toEqual([]);
});
