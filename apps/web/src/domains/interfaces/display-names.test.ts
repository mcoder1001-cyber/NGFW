import { describe, expect, it } from 'vitest';
import { interfaceDisplayNames } from './display-names';
import { interfaceRows, pageOf } from './InterfacesPage';
import type { InterfaceItem } from './model';

function port(name: string, pci: string, speed: string | null): InterfaceItem {
  return {
    name,
    kind: 'interface',
    parent: null,
    physical: { pci, owner: 'dataplane', builtIn: true },
    state:
      speed === null
        ? null
        : {
            name,
            type: 'dpdk',
            linkSpeedKbps: speed,
            vppName: name,
            swIfIndex: 1,
            adminUp: true,
            linkUp: true,
            mtu: 1500,
            linkMtu: 1500,
            mac: '',
            ipv4: [],
            ipv6: [],
            vrf: 'default',
            tableId: 0,
            parent: '',
            vlanId: 0,
            innerVlanId: 0,
            managed: true,
            rxMode: '',
            description: '',
          },
    running: null,
    config: null,
    counters: null,
    hasPendingChange: false,
  };
}

describe('physical interface display names', () => {
  it('keeps inventory-only ports in PCI numbering before and after dataplane registration', () => {
    const management = {
      ...port('ens192', '', null),
      physical: null,
      inventoryOnly: true,
      hostInventory: {
        netdev: 'ens192',
        pci: '0000:01:00.0',
        driver: 'vmxnet3',
        mac: '',
        isManagement: true,
        boundToDpdk: false,
        linkUp: true,
      },
    };
    const data = port('wan', '0000:02:00.0', '10000000');
    expect(interfaceDisplayNames([data, management]).get('wan')).toBe('Ethernet2-10G');
    expect(interfaceDisplayNames([data, management]).get('ens192')).toBe('Ethernet1-Unknown');
    const registered = {
      ...management,
      inventoryOnly: false,
      physical: { pci: '0000:01:00.0', owner: 'dataplane' as const, builtIn: true },
    };
    expect(interfaceDisplayNames([registered, data]).get('wan')).toBe('Ethernet2-10G');
  });

  it('numbers ports by PCI independently of response order and preserves logical identities', () => {
    const items = [
      port('wan', '0000:0c:00.0', '10000000'),
      port('lan2', '0000:04:00.1', '1000000'),
      port('lan1', '0000:04:00.0', '1000000'),
    ];
    expect([...interfaceDisplayNames(items).values()].sort()).toEqual([
      'Ethernet1-1G',
      'Ethernet2-1G',
      'Ethernet3-10G',
    ]);
    expect(interfaceDisplayNames([...items].reverse()).get('wan')).toBe('Ethernet3-10G');
    const rows = interfaceRows(items);
    expect(rows.map((row) => [row.id, row.name, row.item.name])).toEqual([
      ['lan1', 'Ethernet1-1G', 'lan1'],
      ['lan2', 'Ethernet2-1G', 'lan2'],
      ['wan', 'Ethernet3-10G', 'wan'],
    ]);
    for (const query of ['Ethernet3', 'wan']) {
      expect(
        pageOf(rows, {
          page: 0,
          pageSize: 25,
          sort: [],
          filter: [],
          filterLogic: 'and',
          quickFilter: [query],
        }).rows.map((row) => row.id),
      ).toEqual(['wan']);
    }
    expect(items.map((item) => item.name)).toEqual(['wan', 'lan2', 'lan1']);
  });

  it('includes released or offline ports, handles fractional speeds, and does not invent unknown speeds', () => {
    const released = port('lan1', '0000:01:00.0', null);
    released.physical!.owner = 'host';
    const items = [
      released,
      port('lan2', '0000:02:00.0', '2500000'),
      port('lan3', '0000:03:00.0', '100000'),
      port('lan4', '0000:04:00.0', '0'),
    ];
    expect([...interfaceDisplayNames(items).values()]).toEqual([
      'Ethernet1-Unknown',
      'Ethernet2-2.5G',
      'Ethernet3-100M',
      'Ethernet4-Unknown',
    ]);
  });

  it('uses the parent alias for VLANs and preserves virtual interface names', () => {
    const parent = port('wan', '0000:01:00.0', '10000000');
    const sub = {
      ...port('wan.100', '', null),
      kind: 'subinterface',
      parent: 'wan',
      physical: null,
    } as InterfaceItem;
    const loop = { ...port('loop0', '', null), physical: null };
    expect([...interfaceDisplayNames([sub, loop, parent]).values()]).toEqual([
      'Ethernet1-10G.100',
      'loop0',
      'Ethernet1-10G',
    ]);
  });
});
