import { createContext, useContext, useMemo, type ReactNode } from 'react';

const EMPTY_LABELS: ReadonlyMap<string, string> = new Map();
const InterfaceLabelsContext = createContext(EMPTY_LABELS);
const EMPTY_HOST_INTERFACES: readonly string[] = [];
const HostInterfacesContext = createContext(EMPTY_HOST_INTERFACES);

/** Presentation only: map keys remain the identities used by configuration and actions. */
export function InterfaceLabelsProvider({ labels, hostInterfaces = EMPTY_HOST_INTERFACES, children }: { labels: ReadonlyMap<string, string>; hostInterfaces?: readonly string[]; children: ReactNode }) {
  return <InterfaceLabelsContext.Provider value={labels}><HostInterfacesContext.Provider value={hostInterfaces}>{children}</HostInterfacesContext.Provider></InterfaceLabelsContext.Provider>;
}

/** Linux netdev choices supplied by the inventory, never inferred from dataplane logical names. */
export function useHostInterfaces(): readonly string[] {
  return useContext(HostInterfacesContext);
}

function lookup(labels: ReadonlyMap<string, string>, name: string): string {
  const exact = labels.get(name);
  if (exact !== undefined) return exact;
  // Candidate VLANs need not have a live row yet; prefer their longest known parent.
  for (let dot = name.lastIndexOf('.'); dot > 0; dot = name.lastIndexOf('.', dot - 1)) {
    const parent = labels.get(name.slice(0, dot));
    if (parent !== undefined && /^(?:\.\d+)+$/.test(name.slice(dot))) return parent + name.slice(dot);
  }
  return name;
}

/** Resolve aliases only within a caller's interface namespace, including candidate VLAN suffixes. */
export function interfaceNameFromOptions(value: string, options: readonly string[], label: (name: string) => string): string {
  const names = new Map<string, string>();
  for (const name of options) if (!names.has(label(name))) names.set(label(name), name);
  return lookup(names, value);
}

/** Known interface label, with identity fallback for unrelated strings. */
export function useInterfaceLabel(): (name: string) => string {
  const labels = useContext(InterfaceLabelsContext);
  return useMemo(() => (name: string) => lookup(labels, name), [labels]);
}

/** Input boundary: first canonical identity wins when several engine/host aliases share one label. */
export function useInterfaceName(): (label: string) => string {
  const labels = useContext(InterfaceLabelsContext);
  return useMemo(() => {
    const names = new Map<string, string>();
    for (const [name, label] of labels) if (!names.has(label)) names.set(label, name);
    return (label: string) => lookup(names, label);
  }, [labels]);
}
