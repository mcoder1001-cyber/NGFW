import { useInterfaceLabel } from '@ngfw/ui-kit';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { InterfaceDisplayNamesProvider } from './InterfaceDisplayNamesProvider';

const auth = vi.hoisted(() => ({ status: 'anonymous' }));
const fetchInventory = vi.hoisted(() => vi.fn());
vi.mock('../../auth/AuthProvider', () => ({ useAuth: () => ({ state: auth }) }));
vi.mock('./queries', () => ({
  ifaceKeys: { state: ['state', 'interfaces'] },
  STATE_POLL_MS: 3_000,
  fetchInterfacesState: fetchInventory,
}));

function PortReference() {
  const label = useInterfaceLabel();
  return <span data-testid="port">{label('uplink')}</span>;
}

afterEach(() => {
  auth.status = 'anonymous';
  fetchInventory.mockReset();
});

describe('application interface labels', () => {
  it('loads names only after authentication and clears visible labels on sign-out', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    fetchInventory.mockResolvedValue({
      items: [{ name: 'uplink', kind: 'interface', physical: { pci: '0000:01:00.0' } }],
    });
    const view = () => (
      <QueryClientProvider client={queryClient}>
        <InterfaceDisplayNamesProvider>
          <PortReference />
        </InterfaceDisplayNamesProvider>
      </QueryClientProvider>
    );
    const mounted = render(view());
    await act(async () => {});
    expect(fetchInventory).not.toHaveBeenCalled();
    auth.status = 'authenticated';
    mounted.rerender(view());
    await waitFor(() => expect(screen.getByTestId('port')).toHaveTextContent('Ethernet1'));
    expect(fetchInventory).toHaveBeenCalledTimes(1);
    auth.status = 'anonymous';
    mounted.rerender(view());
    expect(screen.getByTestId('port')).toHaveTextContent('uplink');
    mounted.unmount();
    queryClient.clear();
  });

  it('shares a cached full inventory with a newly mounted screen', async () => {
    auth.status = 'authenticated';
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    queryClient.setQueryData(['state', 'interfaces'], {
      items: [
        { name: 'uplink', kind: 'interface', physical: { pci: '0000:02:00.0' } },
        { name: 'management', kind: 'interface', hostInventory: { pci: '0000:01:00.0' } },
      ],
    });
    const mounted = render(
      <QueryClientProvider client={queryClient}>
        <InterfaceDisplayNamesProvider>
          <PortReference />
        </InterfaceDisplayNamesProvider>
      </QueryClientProvider>,
    );
    expect(screen.getByTestId('port')).toHaveTextContent('Ethernet2');
    expect(fetchInventory).not.toHaveBeenCalled();
    mounted.unmount();
    queryClient.clear();
  });
});
