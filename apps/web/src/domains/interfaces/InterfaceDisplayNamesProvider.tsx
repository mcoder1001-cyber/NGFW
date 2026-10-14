import { InterfaceLabelsProvider } from '@ngfw/ui-kit';
import { useQuery } from '@tanstack/react-query';
import { useMemo, type ReactNode } from 'react';
import { useAuth } from '../../auth/AuthProvider';
import { interfaceDisplayNames } from './display-names';
import { fetchInterfacesState, ifaceKeys, STATE_POLL_MS } from './queries';

/** One inventory and naming map for all screens, including configuration reference pickers. */
export function InterfaceDisplayNamesProvider({ children }: { children: ReactNode }) {
  const { state } = useAuth();
  const authenticated = state.status === 'authenticated';
  const inventory = useQuery({
    queryKey: ifaceKeys.state,
    queryFn: ({ signal }) => fetchInterfacesState(signal),
    enabled: authenticated,
    refetchInterval: STATE_POLL_MS,
  });
  const labels = useMemo(
    () => interfaceDisplayNames(authenticated ? (inventory.data?.items ?? []) : []),
    [authenticated, inventory.data],
  );
  const hostInterfaces = useMemo(
    () =>
      authenticated
        ? (inventory.data?.items ?? []).flatMap((item) =>
            item.hostInventory?.netdev ? [item.hostInventory.netdev] : [],
          )
        : [],
    [authenticated, inventory.data],
  );
  return (
    <InterfaceLabelsProvider labels={labels} hostInterfaces={hostInterfaces}>
      {children}
    </InterfaceLabelsProvider>
  );
}
