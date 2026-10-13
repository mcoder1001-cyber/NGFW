import type { InterfaceItem } from './model';

/** Display aliases only: configuration paths, counters and actions use the original logical name. */
export function interfaceDisplayNames(items: readonly InterfaceItem[]): Map<string, string> {
  const ports = items
    .filter((item) => item.kind === 'interface' && (item.physical || item.state?.type === 'dpdk'))
    .sort((a, b) => {
      const pciA = a.physical?.pci?.toLowerCase();
      const pciB = b.physical?.pci?.toLowerCase();
      if (pciA && pciB) return pciA.localeCompare(pciB);
      if (pciA || pciB) return pciA ? -1 : 1;
      return (a.state?.vppName ?? a.name).localeCompare(b.state?.vppName ?? b.name, 'en', { numeric: true });
    });
  const names = new Map(items.map((item) => [item.name, item.name]));
  ports.forEach((item, index) => {
    const speed = Number(item.state?.linkSpeedKbps ?? 0);
    const suffix = Number.isFinite(speed) && speed > 0
      ? speed >= 1_000_000 ? `${speed / 1_000_000}G` : `${speed / 1_000}M`
      : 'Unknown';
    names.set(item.name, `Ethernet${index + 1}-${suffix}`);
  });
  for (const item of items) {
    if (item.kind !== 'subinterface' || !item.parent) continue;
    const parent = names.get(item.parent);
    if (parent && parent !== item.parent && item.name.startsWith(`${item.parent}.`)) {
      names.set(item.name, `${parent}${item.name.slice(item.parent.length)}`);
    }
  }
  return names;
}
