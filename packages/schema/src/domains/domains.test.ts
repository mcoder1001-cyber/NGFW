import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { ROOT_KEYS, RootConfig, type RootKey } from '../index.js';
import { SystemSchema } from './system.js';
import { DataplaneSchema } from './dataplane.js';
import { InterfacesSchema } from './interfaces.js';
import { VrfsSchema } from './vrfs.js';
import { RoutingSchema } from './routing.js';
import { NatSchema } from './nat.js';
import { ObjectsSchema } from './objects.js';
import { AclSchema } from './acl.js';
import { VpnSchema } from './vpn.js';
import { TunnelsSchema } from './tunnels.js';
import { ServicesSchema } from './services.js';
import { HaSchema } from './ha.js';
import { ManagementSchema } from './management.js';

const DOMAINS = {
  system: SystemSchema,
  dataplane: DataplaneSchema,
  interfaces: InterfacesSchema,
  vrfs: VrfsSchema,
  routing: RoutingSchema,
  nat: NatSchema,
  objects: ObjectsSchema,
  acl: AclSchema,
  vpn: VpnSchema,
  tunnels: TunnelsSchema,
  services: ServicesSchema,
  ha: HaSchema,
  management: ManagementSchema,
} satisfies Record<RootKey, z.ZodType>;

describe('domain schemas', () => {
  it('exist for exactly the 13 root keys', () => {
    expect(Object.keys(DOMAINS).sort()).toEqual([...ROOT_KEYS].sort());
  });

  it.each(ROOT_KEYS)('%s is wired into RootConfig as an optional-with-default key', (key) => {
    expect(RootConfig.shape[key].unwrap()).toBe(DOMAINS[key]);
  });

  // Every domain must accept `{}`: RootConfig.parse({}) relies on it (prefault). Field-level defaults are fine.
  it.each(ROOT_KEYS)('%s accepts an empty object', (key) => {
    expect(DOMAINS[key].safeParse({}).success).toBe(true);
  });

  it.each(ROOT_KEYS)('%s carries a title and x-vrx-ui hints for the form renderer', (key) => {
    const js = z.toJSONSchema(DOMAINS[key], { target: 'draft-2020-12', io: 'input' });
    expect(typeof js.title).toBe('string');
    expect(js['x-vrx-ui']).toMatchObject({ order: expect.any(Number) });
  });

  it('orders domains for navigation the same way as ROOT_KEYS', () => {
    const orders = ROOT_KEYS.map((key) => {
      const js = z.toJSONSchema(DOMAINS[key]) as { 'x-vrx-ui'?: { order?: number } };
      return js['x-vrx-ui']?.order ?? Number.NaN;
    });
    expect(orders).toEqual([...orders].sort((a, b) => a - b));
  });
});
