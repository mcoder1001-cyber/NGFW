import { describe, expect, it } from 'vitest';
import { InterfaceSchema, RootConfig, type RootConfigInput } from '../index.js';
import { defaultVppNicsValidators } from './default-vpp-nics.js';

const run = (name: string, doc: RootConfigInput) =>
  defaultVppNicsValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

// A seeded document: one dataplane-owned NIC (ens161 in the whitelist) and management NIC blacklisted.
const seeded = (owner: 'dataplane' | 'host', inWhitelist: boolean): RootConfigInput => ({
  dataplane: {
    managementPci: ['0000:0b:00.0'],
    pciWhitelist: inWhitelist ? ['0000:04:00.0'] : [],
    devices: inWhitelist ? { '0000:04:00.0': { name: 'ens161' } } : {},
  },
  interfaces: {
    ens161: { enabled: true, physical: { pci: '0000:04:00.0', owner, builtIn: true } },
  },
});

describe('interfaces.<name>.physical schema', () => {
  it('defaults owner=dataplane and builtIn=true', () => {
    const itf = InterfaceSchema.parse({ physical: { pci: '0000:04:00.0' } });
    expect(itf.physical).toEqual({ pci: '0000:04:00.0', owner: 'dataplane', builtIn: true });
  });
  it('rejects a bad PCI address and an unknown owner', () => {
    expect(() => InterfaceSchema.parse({ physical: { pci: '04:00.0' } })).toThrow();
    expect(() =>
      InterfaceSchema.parse({ physical: { pci: '0000:04:00.0', owner: 'kernel' } }),
    ).toThrow();
  });
  it('is absent by default (an ordinary interface is not a physical NIC)', () => {
    expect(InterfaceSchema.parse({}).physical).toBeUndefined();
  });
});

describe('dataplane.owner-consistent', () => {
  it('accepts a dataplane-owned NIC in the whitelist', () => {
    expect(run('dataplane.owner-consistent', seeded('dataplane', true))).toEqual([]);
  });
  it('accepts a host-released NIC that is not in the whitelist', () => {
    expect(run('dataplane.owner-consistent', seeded('host', false))).toEqual([]);
  });
  it('reports a dataplane-owned NIC missing from the whitelist', () => {
    expect(run('dataplane.owner-consistent', seeded('dataplane', false))).toEqual([
      {
        pointer: '/interfaces/ens161/physical/owner',
        message:
          'NIC ens161 (0000:04:00.0) is owned by the engine but in neither dataplane.pciWhitelist nor dataplane.devices: add it there, or release it (physical.owner = host)',
      },
    ]);
  });
  it('reports a host-released NIC still in the whitelist', () => {
    expect(run('dataplane.owner-consistent', seeded('host', true))).toEqual([
      {
        pointer: '/interfaces/ens161/physical/owner',
        message:
          'NIC ens161 (0000:04:00.0) is released to the host but still in dataplane.pciWhitelist or dataplane.devices: remove it there, or reclaim it (physical.owner = dataplane)',
      },
    ]);
  });
});

describe('dataplane.physical-name-matches-device', () => {
  it('accepts the seeded shape (device name = interface key)', () => {
    expect(run('dataplane.physical-name-matches-device', seeded('dataplane', true))).toEqual([]);
  });
  it('reports a device logical name that differs from the interface key', () => {
    const doc = seeded('dataplane', true);
    (doc.dataplane as { devices: Record<string, { name: string }> }).devices['0000:04:00.0'] = {
      name: 'lan',
    };
    expect(run('dataplane.physical-name-matches-device', doc)).toEqual([
      {
        pointer: '/dataplane/devices/0000:04:00.0/name',
        message: 'the logical name of 0000:04:00.0 must be ens161 (its interface), not lan',
      },
    ]);
  });
  it('reports two physical rows on one NIC', () => {
    const doc = seeded('dataplane', true);
    (doc.interfaces as Record<string, unknown>)['ens999'] = {
      physical: { pci: '0000:04:00.0', owner: 'dataplane', builtIn: true },
    };
    expect(run('dataplane.physical-name-matches-device', doc)).toEqual([
      {
        pointer: '/interfaces/ens999/physical/pci',
        message: 'NIC 0000:04:00.0 is already the physical row ens161',
      },
    ]);
  });
});
