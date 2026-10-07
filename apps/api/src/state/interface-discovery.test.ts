import { describe, expect, it, vi } from 'vitest';
import { DesiredState, InterfaceState, type HostNic } from '@ngfw/proto';
import { StateController } from './state.controller.js';
import type { AgentClient } from '../agent/agent.client.js';
import type { RelayService } from '../telemetry/relay.service.js';
import type { CommitService } from '../commit/commit.service.js';
import type { DatastoreService } from '../datastore/datastore.service.js';
import type { SystemEventsService } from '../audit/system-events.service.js';
import type { ValidationService } from '../commit/validation.service.js';

function fixture(
  nics: HostNic[],
  config: Record<string, unknown> = {},
  live: InterfaceState[] = [],
) {
  const agent = {
    retrieve: vi.fn().mockResolvedValue({ desiredState: DesiredState.fromPartial({}) }),
    interfaceState: vi.fn().mockResolvedValue({ interfaces: live }),
    hostNics: vi.fn().mockResolvedValue({ nics }),
    apply: vi.fn(),
  };
  const ds = {
    getRunning: vi.fn().mockResolvedValue({ doc: { interfaces: config } }),
    getCandidate: vi.fn().mockResolvedValue({ interfaces: config }),
  };
  const controller = new StateController(
    agent as unknown as AgentClient,
    {} as RelayService,
    {} as CommitService,
    ds as unknown as DatastoreService,
    {} as SystemEventsService,
    {} as ValidationService,
  );
  vi.spyOn(
    controller as unknown as { statsSnapshot(): Promise<undefined> },
    'statsSnapshot',
  ).mockResolvedValue(undefined);
  return { controller, agent, ds };
}

describe('automatic read-only physical NIC discovery', () => {
  it('lists management and newly attached host NICs without seeding or applying', async () => {
    const { controller, agent, ds } = fixture([
      { pci: '0000:01:00.0', netdev: 'ens192', isManagement: true, linkUp: true },
      { pci: '0000:02:00.0', netdev: 'ens224', driver: 'vmxnet3' },
      { pci: '0000:03:00.0', boundToDpdk: true },
    ]);
    const result = await controller.interfaces();
    expect(result.items.map((i) => i.name)).toEqual(['ens192', 'ens224', 'pci-0000:03:00.0']);
    expect(result.items[0]).toMatchObject({
      inventoryOnly: true,
      hostInventory: { isManagement: true },
      physical: null,
      running: null,
      config: null,
      hasPendingChange: false,
    });
    expect(agent.apply).not.toHaveBeenCalled();
    expect(Object.keys(ds)).toEqual(['getRunning', 'getCandidate']);
  });

  it('correlates physical PCI, exact af-packet name and unique DPDK MAC without duplicates', async () => {
    const { controller } = fixture(
      [
        { pci: '0000:01:00.0', netdev: 'ens192' },
        { pci: '0000:02:00.0', netdev: 'ens224' },
        { pci: '0000:03:00.0', mac: '02:00:00:00:00:03', boundToDpdk: true },
      ],
      { wan: { physical: { pci: '0000:01:00.0', owner: 'dataplane', builtIn: true } } },
      [
        InterfaceState.fromPartial({
          name: 'host-ens224',
          vppName: 'host-ens224',
          type: 'af-packet',
        }),
        InterfaceState.fromPartial({
          name: 'GigabitEthernet0/3/0',
          vppName: 'GigabitEthernet0/3/0',
          type: 'dpdk',
          mac: '02:00:00:00:00:03',
        }),
      ],
    );
    const result = await controller.interfaces();
    expect(result.items).toHaveLength(3);
    expect(result.items.every((i) => i.hostInventory && !i.inventoryOnly)).toBe(true);
  });

  it('never merges ambiguous MACs or unrelated configured Linux names', async () => {
    const { controller } = fixture(
      [
        { pci: '0000:01:00.0', mac: '02:00:00:00:00:01', boundToDpdk: true },
        { pci: '0000:02:00.0', mac: '02:00:00:00:00:01', boundToDpdk: true },
      ],
      {},
      [InterfaceState.fromPartial({ name: 'wan', type: 'dpdk', mac: '02:00:00:00:00:01' })],
    );
    const result = await controller.interfaces();
    expect(result.items).toHaveLength(3);
    expect(result.items.find((i) => i.name === 'wan')?.hostInventory).toBeNull();
  });

  it('keeps an unrelated configured Linux name separate from the discovered host NIC', async () => {
    const { controller } = fixture(
      [{ pci: '0000:01:00.0', netdev: 'ens192', isManagement: true }],
      { ens192: { enabled: false } },
    );
    const result = await controller.interfaces();
    expect(result.items).toHaveLength(2);
    expect(result.items.find((i) => i.name === 'ens192')?.hostInventory).toBeNull();
    expect(result.items.find((i) => i.name === 'pci-0000:01:00.0')).toMatchObject({
      inventoryOnly: true,
      hostInventory: { isManagement: true },
    });
  });

  it('keeps inventory visible with explicit diagnostics when VPP is unavailable', async () => {
    const { controller, agent } = fixture([{ pci: '0000:01:00.0', netdev: 'ens192' }]);
    agent.retrieve.mockRejectedValue(new Error('VPP unavailable'));
    agent.interfaceState.mockRejectedValue(new Error('VPP unavailable'));
    const result = await controller.interfaces();
    expect(result).toMatchObject({
      hostInventoryStatus: 'available',
      dataplaneStatus: 'unavailable',
      observationErrors: [
        { source: 'retrieve', message: 'VPP unavailable' },
        { source: 'live', message: 'VPP unavailable' },
      ],
    });
    expect(result.items[0]?.name).toBe('ens192');
  });

  it('keeps configured/live rows and signals incomplete inventory on older/unavailable agents', async () => {
    const { controller, agent } = fixture([], { loop0: { enabled: true } });
    agent.hostNics.mockRejectedValue(new Error('HostNics unimplemented'));
    const result = await controller.interfaces();
    expect(result.items[0]?.name).toBe('loop0');
    expect(result.hostInventoryStatus).toBe('unavailable');
    expect(result.observationErrors).toContainEqual({
      source: 'hostInventory',
      message: 'HostNics unimplemented',
    });
  });
});
