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
    expect(interfaceDisplayNames([data, management]).get('wan')).toBe('Ethernet2');
    expect(interfaceDisplayNames([data, management]).get('ens192')).toBe('Ethernet1');
    const registered = {
      ...management,
      inventoryOnly: false,
      physical: { pci: '0000:01:00.0', owner: 'dataplane' as const, builtIn: true },
    };
    expect(interfaceDisplayNames([registered, data]).get('wan')).toBe('Ethernet2');
  });

  it('numbers ports by PCI independently of response order and preserves logical identities', () => {
    const items = [
      port('wan', '0000:0c:00.0', '10000000'),
      port('lan2', '0000:04:00.1', '1000000'),
      port('lan1', '0000:04:00.0', '1000000'),
    ];
    expect([...interfaceDisplayNames(items).values()].sort()).toEqual([
      'Ethernet1',
      'Ethernet2',
      'Ethernet3',
    ]);
    expect(interfaceDisplayNames([...items].reverse()).get('wan')).toBe('Ethernet3');
    const rows = interfaceRows(items);
    expect(rows.map((row) => [row.id, row.name, row.item.name])).toEqual([
      ['lan1', 'Ethernet1', 'lan1'],
      ['lan2', 'Ethernet2', 'lan2'],
      ['wan', 'Ethernet3', 'wan'],
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

  it('keeps labels independent of missing, zero or negotiated link speed', () => {
    const released = port('lan1', '0000:01:00.0', null);
    released.physical!.owner = 'host';
    const items = [
      released,
      port('lan2', '0000:02:00.0', '2500000'),
      port('lan3', '0000:03:00.0', '100000'),
      port('lan4', '0000:04:00.0', '0'),
    ];
    expect([...interfaceDisplayNames(items).values()]).toEqual([
      'Ethernet1',
      'Ethernet2',
      'Ethernet3',
      'Ethernet4',
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
      'Ethernet1.100',
      'loop0',
      'Ethernet1',
    ]);
  });
  it('labels native physical ports without treating virtual TAPs as hardware', () => {
    const native = { ...port('uplink', '', '0'), physical: null };
    native.state!.type = 'vmxnet3';
    const tap = { ...port('tap0', '', '0'), physical: null };
    tap.state!.type = 'virtio';
    expect(interfaceDisplayNames([tap, native]).get('uplink')).toBe('Ethernet1');
    expect(interfaceDisplayNames([tap, native]).get('tap0')).toBe('tap0');
  });

  it('resolves VPP and host names without overwriting another logical identity', () => {
    const wan = port('wan', '0000:01:00.0', '0');
    wan.state!.vppName = 'GigabitEthernet0/1/0';
    wan.hostInventory = {
      netdev: 'ens192',
      pci: '0000:01:00.0',
      driver: 'vmxnet3',
      mac: '',
      isManagement: false,
      boundToDpdk: false,
      linkUp: false,
    };
    const vlan = {
      ...port('wan.100', '', '0'),
      physical: null,
      kind: 'subinterface',
      parent: 'wan',
    } as InterfaceItem;
    vlan.state!.vppName = 'GigabitEthernet0/1/0.100';
    const labels = interfaceDisplayNames([wan, vlan]);
    expect(labels.get('ens192')).toBe('Ethernet1');
    expect(labels.get('GigabitEthernet0/1/0')).toBe('Ethernet1');
    expect(labels.get('GigabitEthernet0/1/0.100')).toBe('Ethernet1.100');
    const other = port('ens192', '0000:02:00.0', null);
    expect(interfaceDisplayNames([wan, other]).get('ens192')).toBe('Ethernet2');
  });
});
