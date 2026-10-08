import type { InterfaceConfig, InterfacesConfig } from '../domains/interfaces.js';

/** Resolve the configured leaf, retaining the physical root for exclusivity checks. */
export function resolvePppoeParent(interfaces: InterfacesConfig, name: string) {
  const direct = interfaces[name];
  if (direct) return { rootName: name, root: direct, leaf: direct, vlan: false };
  const match = /^(.+)\.(0|[1-9][0-9]*)$/.exec(name);
  if (!match) return undefined;
  const rootName = match[1]!;
  const root = interfaces[rootName];
  const child = root?.subinterfaces[match[2]!];
  if (!root || !child) return undefined;
  // Root-only attachments must never be inherited onto the selected VLAN leaf.
  const leaf: InterfaceConfig = { ...child, promiscuous: false, subinterfaces: {} };
  return { rootName, root, leaf, vlan: true };
}
