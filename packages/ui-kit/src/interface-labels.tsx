import { createContext, useContext, useMemo, type ReactNode } from 'react';

const EMPTY_LABELS: ReadonlyMap<string, string> = new Map();
const InterfaceLabelsContext = createContext(EMPTY_LABELS);

/** Presentation only: map keys remain the identities used by configuration and actions. */
export function InterfaceLabelsProvider({ labels, children }: { labels: ReadonlyMap<string, string>; children: ReactNode }) {
  return <InterfaceLabelsContext.Provider value={labels}>{children}</InterfaceLabelsContext.Provider>;
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
    return (label: string) => labels.has(label) ? label : lookup(names, label);
  }, [labels]);
}
