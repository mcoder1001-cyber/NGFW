import { expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { OspfAuthSchema, Ospf6Schema } from '../domains/routing.js';
import { ospfValidators } from './ospf.js';
const validate = (doc: unknown) => ospfValidators[0]!.validate(RootConfig.parse(doc));
it('checks IPv6 interface family, area, VRF and router ID', () => {
  const issues = validate({
    interfaces: { loop0: { ipv4: ['192.0.2.1/32'] } },
    routing: { ospf6: { areas: { '0': { type: 'stub' } }, interfaces: { loop0: { area: '1' } } } },
  });
  expect(issues.map((i) => i.pointer)).toEqual(
    expect.arrayContaining([
      '/routing/ospf6/areas/0/type',
      '/routing/ospf6/interfaces/loop0',
      '/routing/ospf6/interfaces/loop0/area',
    ]),
  );
  expect(
    validate({ routing: { ospf6: {} } }).some((i) => i.pointer === '/routing/ospf6/routerId'),
  ).toBe(true);
  expect(
    validate({
      interfaces: { loop0: { ipv6: ['2001:db8::1/64'] } },
      routing: {
        ospf6: { routerId: '192.0.2.1', areas: { '0': {} }, interfaces: { loop0: { area: '0' } } },
      },
    }),
  ).toEqual([]);
});
it('requires MD5 key reference and refuses v3 auth and NBMA', () => {
  expect(OspfAuthSchema.safeParse({ type: 'md5' }).success).toBe(false);
  expect(OspfAuthSchema.safeParse({ type: 'md5', keyId: 7, keyRef: 'password/ospf' }).success).toBe(
    true,
  );
  expect(OspfAuthSchema.safeParse({ type: 'none', keyRef: 'password/ospf' }).success).toBe(false);
  expect(
    Ospf6Schema.safeParse({ interfaces: { loop0: { area: '0', auth: { type: 'none' } } } }).success,
  ).toBe(false);
  expect(
    Ospf6Schema.safeParse({ interfaces: { loop0: { area: '0', networkType: 'non-broadcast' } } })
      .success,
  ).toBe(false);
});
