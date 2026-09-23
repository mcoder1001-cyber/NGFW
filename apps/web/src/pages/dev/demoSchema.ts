import { hostname, ipAddress, ipv4Cidr, ipv6Cidr, macAddress, objectName, vppInterfaceName, withUi, type UiMeta } from '@ngfw/schema';
import { z } from 'zod';

/**
 * DEMO ONLY (/dev/schema-form): a schema that exercises every widget of `<SchemaForm>`, composed from the
 * real primitives and `withUi()` of packages/schema so the hints travel exactly as in production (D-019).
 * It is not a product schema; the product's domain schemas are generated from packages/schema.
 */
const DemoInterface = withUi(
  z.object({
    name: withUi(vppInterfaceName, { order: 1 }),
    description: withUi(z.string().max(255).optional(), { title: 'Description', widget: 'textarea', order: 2 }),
    enabled: withUi(z.boolean().default(true), { title: 'Enabled', order: 3 }),
    mtu: withUi(z.number().int().min(68).max(9216).default(1500), { title: 'MTU', order: 4, help: 'Bytes, 68–9216' }),
    mac: withUi(macAddress.optional(), { order: 5 }),
    rxMode: withUi(z.enum(['polling', 'interrupt', 'adaptive']).default('adaptive'), { title: 'RX mode', widget: 'radio', group: 'Dataplane', order: 10 }),
    rxQueues: withUi(z.number().int().min(1).max(16).default(1), { title: 'RX queues', widget: 'slider', group: 'Dataplane', order: 11 }),
    ipv4: withUi(z.array(ipv4Cidr).default([]), { title: 'IPv4 addresses', group: 'Addressing', order: 20 }),
    ipv6: withUi(z.array(ipv6Cidr).default([]), { title: 'IPv6 addresses', widget: 'chips', group: 'Addressing', order: 21 }),
    gateway: withUi(ipAddress.optional(), { title: 'Gateway', group: 'Addressing', order: 22 }),
    vrf: withUi(objectName.default('default'), { title: 'VRF', group: 'Addressing', order: 23 }),
    dnsServer: withUi(hostname.optional(), { title: 'DNS server', group: 'Addressing', order: 24 }),
    features: withUi(z.array(z.enum(['lldp', 'ipfix', 'nat'])).default([]), { title: 'Features', widget: 'multiselect', group: 'Services', order: 30 }),
    subinterfaces: withUi(
      z.record(
        vppInterfaceName,
        z.object({
          vlanId: withUi(z.number().int().min(1).max(4094), { title: 'VLAN ID' }),
          innerVlanId: withUi(z.number().int().min(1).max(4094).optional(), { title: 'Inner VLAN (QinQ)' }),
          ipv4: withUi(z.array(ipv4Cidr).default([]), { title: 'IPv4 addresses' }),
        }),
      ),
      { title: 'Sub-interfaces', group: 'Sub-interfaces', order: 40 },
    ),
    auth: withUi(
      z
        .discriminatedUnion('kind', [
          z.object({ kind: z.literal('none') }).meta({ title: 'None' }),
          z.object({ kind: z.literal('psk'), psk: withUi(z.string().min(8), { title: 'Pre-shared key', widget: 'password' }) }).meta({ title: 'Pre-shared key' }),
          z.object({ kind: z.literal('cert'), certificate: withUi(objectName, { title: 'Certificate' }) }).meta({ title: 'Certificate' }),
        ])
        .default({ kind: 'none' }),
      { title: 'Authentication', group: 'Security', order: 50 },
    ),
    bfd: withUi(z.boolean().default(false), { title: 'BFD', widget: 'checkbox', group: 'Security', order: 51 }),
    bfdInterval: withUi(z.number().int().min(50).max(60000).default(300), {
      title: 'BFD interval (ms)',
      group: 'Security',
      order: 52,
      dependsOn: { field: 'bfd', value: true },
    } as UiMeta),
    neighbours: withUi(
      z
        .array(
          z.object({
            address: withUi(ipAddress, { title: 'Address' }),
            description: withUi(z.string().optional(), { title: 'Description' }),
          }),
        )
        .default([]),
      { title: 'Static neighbours', group: 'Neighbours', order: 60 },
    ),
    notes: withUi(z.looseObject({}).optional(), { title: 'Vendor extensions', widget: 'json', group: 'Advanced', order: 90 }),
  }),
  { title: 'Interface (demo)', description: 'Demo schema exercising every SchemaForm widget; not a product schema.' },
);

export const demoSchema = z.toJSONSchema(DemoInterface, { target: 'draft-2020-12', io: 'input' });

/** Example starting value (RFC 5737 documentation addresses). */
export const demoValue = {
  name: 'GigabitEthernet0/8/0',
  enabled: true,
  mtu: 1500,
  ipv4: ['192.0.2.1/24'],
  subinterfaces: { 'GigabitEthernet0/8/0.100': { vlanId: 100, ipv4: [] } },
  auth: { kind: 'none' },
};
