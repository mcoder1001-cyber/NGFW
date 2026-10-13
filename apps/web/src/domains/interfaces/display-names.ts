import type { InterfaceItem } from './model';

// Native hardware classes also appear without a configured physical marker. Do not infer
// hardware from `virtio`: VPP uses that class for virtual TAP interfaces as well.
const HARDWARE_TYPES = new Set(['dpdk', 'vmxnet3', 'avf', 'rdma', 'af_xdp']);

/** Display aliases only: configuration paths, counters and actions use the original logical name. */
export function interfaceDisplayNames(items: readonly InterfaceItem[]): Map<string, string> {
  const ports = items
    .filter(
      (item) =>
        item.kind === 'interface' &&
        (item.physical ||
          item.hostInventory?.pci ||
          HARDWARE_TYPES.has(item.state?.type?.toLowerCase() ?? '')),
    )
    .sort((a, b) => {
      const pciA = (a.physical?.pci ?? a.hostInventory?.pci)?.toLowerCase();
      const pciB = (b.physical?.pci ?? b.hostInventory?.pci)?.toLowerCase();
      if (pciA && pciB) return pciA.localeCompare(pciB);
      if (pciA || pciB) return pciA ? -1 : 1;
      return (a.state?.vppName ?? a.name).localeCompare(b.state?.vppName ?? b.name, 'en', {
        numeric: true,
      });
    });
  const names = new Map(items.map((item) => [item.name, item.name]));
  ports.forEach((item, index) => {
    names.set(item.name, `Ethernet${index + 1}`);
  });
  for (const item of items) {
    if (item.kind !== 'subinterface' || !item.parent) continue;
    const parent = names.get(item.parent);
    if (parent && parent !== item.parent && item.name.startsWith(`${item.parent}.`)) {
      names.set(item.name, `${parent}${item.name.slice(item.parent.length)}`);
    }
  }
  // State, counters and host-service forms may use different names for the same port.
  // A real logical name always wins over another interface's secondary spelling.
  for (const item of items) {
    const label = names.get(item.name)!;
    for (const alias of [item.state?.vppName, item.hostInventory?.netdev]) {
      if (alias && !names.has(alias)) names.set(alias, label);
    }
  }
  return names;
}
