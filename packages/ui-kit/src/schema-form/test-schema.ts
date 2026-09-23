import type { JsonSchema } from './types.js';

/** Exercises every widget the renderer knows. Used by SchemaForm.test.tsx. */
export const WIDGET_SCHEMA: JsonSchema = {
  $schema: 'https://json-schema.org/draft/2020-12/schema',
  type: 'object',
  title: 'Interface',
  properties: {
    name: { type: 'string', title: 'Name', minLength: 1, maxLength: 63, 'x-vrx-ui': { order: 1 } },
    description: { type: 'string', title: 'Description', 'x-vrx-ui': { widget: 'textarea', order: 2 } },
    secret: { type: 'string', title: 'Secret', 'x-vrx-ui': { widget: 'password', group: 'Security' } },
    enabled: { type: 'boolean', title: 'Enabled', default: true },
    monitored: { type: 'boolean', title: 'Monitored', 'x-vrx-ui': { widget: 'checkbox' } },
    mtu: { type: 'integer', title: 'MTU', minimum: 68, maximum: 9216, default: 1500 },
    weight: { type: 'integer', title: 'Weight', minimum: 0, maximum: 10, default: 5, 'x-vrx-ui': { widget: 'slider' } },
    rxMode: { type: 'string', title: 'RX mode', enum: ['polling', 'interrupt', 'adaptive'], default: 'adaptive' },
    duplex: { type: 'string', title: 'Duplex', enum: ['full', 'half'], 'x-vrx-ui': { widget: 'radio' } },
    ipv4: {
      type: 'array',
      title: 'IPv4 addresses',
      items: { type: 'string', format: 'cidrv4', 'x-vrx-ui': { widget: 'cidr' } },
    },
    gateway: {
      anyOf: [{ type: 'string', format: 'ipv4' }, { type: 'string', format: 'ipv6' }],
      title: 'Gateway',
      'x-vrx-ui': { widget: 'ip' },
    },
    mac: { type: 'string', title: 'MAC', pattern: '^([0-9a-f]{2}:){5}[0-9a-f]{2}$', 'x-vrx-ui': { widget: 'mac' } },
    parent: { type: 'string', title: 'Parent interface', 'x-vrx-ui': { widget: 'interface-picker' } },
    tags: { type: 'array', title: 'Tags', items: { type: 'string' }, 'x-vrx-ui': { widget: 'chips' } },
    features: { type: 'array', title: 'Features', uniqueItems: true, items: { type: 'string', enum: ['lldp', 'ipfix', 'nat'] } },
    dns: {
      type: 'array',
      title: 'DNS servers',
      items: {
        type: 'object',
        title: 'DNS server',
        properties: { address: { type: 'string', title: 'Address' }, port: { type: 'integer', title: 'Port', default: 53 } },
        required: ['address'],
      },
    },
    subs: {
      type: 'object',
      title: 'Sub-interfaces',
      additionalProperties: {
        type: 'object',
        properties: { vlanId: { type: 'integer', title: 'VLAN', minimum: 1, maximum: 4094 } },
        required: ['vlanId'],
      },
    },
    auth: {
      title: 'Authentication',
      oneOf: [
        {
          type: 'object',
          title: 'Pre-shared key',
          properties: { kind: { const: 'psk', title: 'Kind' }, psk: { type: 'string', title: 'PSK', 'x-vrx-ui': { widget: 'password' } } },
          required: ['kind', 'psk'],
        },
        {
          type: 'object',
          title: 'Certificate',
          properties: { kind: { const: 'cert', title: 'Kind' }, cert: { type: 'string', title: 'Certificate name' } },
          required: ['kind', 'cert'],
        },
      ],
    },
    extra: { title: 'Extra', 'x-vrx-ui': { widget: 'json' } },
    vrf: { type: 'string', title: 'VRF', 'x-vrx-ui': { dependsOn: { field: 'enabled', value: true } } },
    internal: { type: 'string', 'x-vrx-ui': { widget: 'hidden' } },
  },
  required: ['name', 'mtu'],
  additionalProperties: false,
};

export const WIDGET_VALUE = {
  name: 'eth0',
  mtu: 1500,
  subs: { 'Gig0/0/0.100': { vlanId: 100 } },
  auth: { kind: 'psk', psk: 'VRX_TEST_PSK_ui' },
};
