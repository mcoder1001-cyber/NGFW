import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { dataplaneValidators } from './dataplane.js';

const run = (name: string, doc: RootConfigInput) =>
  dataplaneValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

describe('dataplane.workers-match-corelist', () => {
  it('is silent when either side is absent or they agree', () => {
    expect(run('dataplane.workers-match-corelist', {})).toEqual([]);
    expect(run('dataplane.workers-match-corelist', { dataplane: { workers: 2 } })).toEqual([]);
    expect(run('dataplane.workers-match-corelist', { dataplane: { corelist: [2, 3] } })).toEqual(
      [],
    );
    expect(
      run('dataplane.workers-match-corelist', { dataplane: { workers: 2, corelist: [2, 3] } }),
    ).toEqual([]);
  });
  it('reports a mismatch', () => {
    expect(
      run('dataplane.workers-match-corelist', { dataplane: { workers: 4, corelist: [2, 3] } }),
    ).toEqual([
      {
        pointer: '/dataplane/workers',
        message: 'workers (4) must equal the number of cores in corelist (2)',
      },
    ]);
  });
});

describe('dataplane.corelist-unique', () => {
  it('accepts no corelist or distinct cores; reports repeats', () => {
    expect(run('dataplane.corelist-unique', {})).toEqual([]);
    expect(run('dataplane.corelist-unique', { dataplane: { corelist: [1, 2] } })).toEqual([]);
    expect(run('dataplane.corelist-unique', { dataplane: { corelist: [1, 2, 1] } })).toEqual([
      { pointer: '/dataplane/corelist/2', message: 'core 1 is listed more than once' },
    ]);
  });
});

describe('dataplane.main-core-not-worker', () => {
  it('is silent without mainCore / corelist or when they are disjoint', () => {
    expect(run('dataplane.main-core-not-worker', {})).toEqual([]);
    expect(run('dataplane.main-core-not-worker', { dataplane: { mainCore: 1 } })).toEqual([]);
    expect(
      run('dataplane.main-core-not-worker', { dataplane: { mainCore: 1, corelist: [2, 3] } }),
    ).toEqual([]);
  });
  it('reports a main core that is also a worker core', () => {
    expect(
      run('dataplane.main-core-not-worker', { dataplane: { mainCore: 2, corelist: [2, 3] } }),
    ).toEqual([
      { pointer: '/dataplane/mainCore', message: 'main core 2 is also listed as a worker core' },
    ]);
  });
});

describe('dataplane.pci-unique', () => {
  it('accepts distinct devices and reports repeats case-insensitively', () => {
    expect(
      run('dataplane.pci-unique', {
        dataplane: { pciWhitelist: ['0000:0b:00.0', '0000:0b:00.1'] },
      }),
    ).toEqual([]);
    expect(
      run('dataplane.pci-unique', {
        dataplane: { pciWhitelist: ['0000:0b:00.0', '0000:0B:00.0'] },
      }),
    ).toEqual([
      {
        pointer: '/dataplane/pciWhitelist/1',
        message: 'PCI device 0000:0B:00.0 is listed more than once',
      },
    ]);
  });
});

describe('dataplane devices / management / plugins (F-startup-gen)', () => {
  it('parses the new fields with defaults', () => {
    const cfg = RootConfig.parse({});
    expect(cfg.dataplane.managementPci).toEqual([]);
    expect(cfg.dataplane.devices).toEqual({});
    expect(cfg.dataplane.plugins).toBeUndefined();
    expect(RootConfig.parse({ dataplane: { plugins: {} } }).dataplane.plugins).toEqual({
      switches: {},
    });
    const full = RootConfig.parse({
      dataplane: {
        managementPci: ['0000:0b:00.0'],
        devices: { '0000:04:00.0': { name: 'wan', rxQueues: 2, rxDesc: 512 } },
        buffersPerNuma: 32768,
        plugins: { switches: { 'linux_cp_plugin.so': true, 'vxlan-gpe_plugin.so': false } },
      },
    });
    expect(full.dataplane.devices['0000:04:00.0']?.name).toBe('wan');
  });
  it('rejects bad shapes at the schema level', () => {
    for (const dataplane of [
      { devices: { '0000:04:00.0': { name: 'LAN' } } },
      { devices: { '0000:04:00.0': { name: 'lan-' } } },
      { devices: { '0000:04:00.0': { name: 'abcdefghijklmnop' } } },
      { devices: { '0000:04:00.0': { name: 'lan\n}' } } },
      { devices: { '04:00.0': { name: 'lan' } } },
      { devices: { '0000:04:00.0': { nam: 'lan' } } },
      { devices: { '0000:04:00.0': { rxQueues: 0 } } },
      { devices: { '0000:04:00.0': { rxDesc: 32 } } },
      { managementPci: ['0000:0b:00.0 '] },
      { buffersPerNuma: 10 },
      { plugins: { switches: { '../x_plugin.so': true } } },
      { plugins: { switches: { 'acl_plugin.so': 'enable' } } },
      { plugins: { 'acl_plugin.so': true } },
      { plugins: { switches: {}, extra: 1 } },
    ]) {
      expect(RootConfig.safeParse({ dataplane }).success, JSON.stringify(dataplane)).toBe(false);
    }
  });
  it('dataplane.devices-pci-unique', () => {
    expect(
      run('dataplane.devices-pci-unique', {
        dataplane: { devices: { '0000:0c:00.0': {}, '0000:04:00.0': {} } },
      }),
    ).toEqual([]);
    expect(
      run('dataplane.devices-pci-unique', {
        dataplane: { devices: { '0000:0c:00.0': {}, '0000:0C:00.0': {} } },
      }),
    ).toEqual([
      {
        pointer: '/dataplane/devices/0000:0C:00.0',
        message: 'PCI device 0000:0C:00.0 is the same device as 0000:0c:00.0',
      },
    ]);
  });
  it('dataplane.management-not-dpdk', () => {
    expect(
      run('dataplane.management-not-dpdk', {
        dataplane: { managementPci: ['0000:0b:00.0'], devices: { '0000:04:00.0': {} } },
      }),
    ).toEqual([]);
    expect(
      run('dataplane.management-not-dpdk', {
        dataplane: {
          managementPci: ['0000:0B:00.0', '0000:0b:00.0', '0000:1c:00.0'],
          devices: { '0000:0b:00.0': { name: 'lan' } },
          pciWhitelist: ['0000:1C:00.0'],
        },
      }),
    ).toEqual([
      {
        pointer: '/dataplane/managementPci/0',
        message:
          'management NIC 0000:0B:00.0 must never be a DPDK device (remove it from pciWhitelist/devices)',
      },
      {
        pointer: '/dataplane/managementPci/1',
        message: 'management NIC 0000:0b:00.0 is listed more than once',
      },
      {
        pointer: '/dataplane/managementPci/2',
        message:
          'management NIC 0000:1c:00.0 must never be a DPDK device (remove it from pciWhitelist/devices)',
      },
    ]);
  });
  it('dataplane.logical-name-unique', () => {
    expect(
      run('dataplane.logical-name-unique', {
        dataplane: { devices: { '0000:04:00.0': { name: 'wan' }, '0000:0c:00.0': {} } },
      }),
    ).toEqual([]);
    expect(
      run('dataplane.logical-name-unique', {
        dataplane: {
          devices: { '0000:04:00.0': { name: 'lan' }, '0000:0c:00.0': { name: 'lan' } },
        },
      }),
    ).toEqual([
      {
        pointer: '/dataplane/devices/0000:0c:00.0/name',
        message: 'logical name lan is already used by 0000:04:00.0',
      },
    ]);
  });
  it('dataplane.descriptors-power-of-two', () => {
    expect(
      run('dataplane.descriptors-power-of-two', {
        dataplane: {
          devices: { '0000:04:00.0': { rxDesc: 512, txDesc: 1024 }, '0000:0c:00.0': {} },
        },
      }),
    ).toEqual([]);
    expect(
      run('dataplane.descriptors-power-of-two', {
        dataplane: { devices: { '0000:04:00.0': { rxDesc: 1000, txDesc: 600 } } },
      }),
    ).toEqual([
      {
        pointer: '/dataplane/devices/0000:04:00.0/rxDesc',
        message: 'rxDesc 1000 must be a power of two',
      },
      {
        pointer: '/dataplane/devices/0000:04:00.0/txDesc',
        message: 'txDesc 600 must be a power of two',
      },
    ]);
  });
});
