import { expect, it } from 'vitest';
import { RootConfig } from '@ngfw/schema';
import { DesiredState } from '../gen/ts/ngfw/v1/dataplane.js';

it('preserves explicit PD target IDs through schema, JSON and protobuf wire', () => {
  const doc = RootConfig.parse({
    interfaces: {
      wan0: {
        pppoe: {
          username: 'u',
          passwordRef: 'password/isp',
          ipv6: 'dhcpv6',
          delegationTargets: [{ interface: 'lan0', subnetId: 4294967295 }],
        },
      },
      lan0: { enabled: true },
    },
  });
  const message = DesiredState.fromJSON(doc);
  const decoded = DesiredState.decode(DesiredState.encode(message).finish());
  expect(decoded.interfaces.wan0?.pppoe?.delegationTargets).toEqual([
    { interface: 'lan0', subnetId: 4294967295 },
  ]);
  const roundtrip = RootConfig.parse(DesiredState.toJSON(decoded));
  expect(roundtrip.interfaces.wan0?.pppoe?.delegationTargets).toEqual(
    doc.interfaces.wan0?.pppoe?.delegationTargets,
  );
});
