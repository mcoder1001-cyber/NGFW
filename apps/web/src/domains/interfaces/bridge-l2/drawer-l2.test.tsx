import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

/**
 * Review F-bridge-l2 #3 / Q10: P08's interface drawer shows `l2` as an opaque JSON field (`drawerSafeL2`). Saving the
 * drawer of a bridged interface after an unrelated edit must not touch the membership: the merge patch carries only the
 * edited field, never `l2` (no `l2: null`, no rewritten leaf).
 */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

const live = (name: string) => ({
  name,
  vppName: name,
  swIfIndex: 5,
  type: 'af-packet',
  adminUp: true,
  linkUp: true,
  mtu: 9000,
  linkMtu: 9000,
  mac: '02:fe:00:00:00:01',
  ipv4: [],
  ipv6: [],
  vrf: 'default',
  tableId: 0,
  parent: '',
  vlanId: 0,
  innerVlanId: 0,
  managed: true,
  linkSpeedKbps: '0',
  rxMode: 'interrupt',
  description: '',
});

const bridged = {
  enabled: true,
  ipv4: [],
  ipv6: [],
  vrf: 'default',
  promiscuous: false,
  subinterfaces: {},
  l2: { bridgeDomain: 'lan', shg: 2, bvi: false, uuFwd: false, macFilter: true },
};

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('interface drawer with an l2 leaf (P08 drawer, F-bridge-l2 Q10)', () => {
  it(
    'an MTU edit saves only the MTU: the bridge membership and the MAC filter stay',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      api.on('GET /api/v1/state/interfaces', {
        body: {
          items: [
            {
              name: 'host-w7l0',
              kind: 'interface',
              parent: null,
              state: live('host-w7l0'),
              config: bridged,
              running: bridged,
              counters: null,
              hasPendingChange: false,
            },
          ],
        },
      });
      api.on('GET /api/v1/config/candidate/interfaces', { body: { 'host-w7l0': bridged } });
      const patches: unknown[] = [];
      api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
        patches.push(body);
        return { body: { pointer: '/interfaces', before: null, after: null } };
      });
      await signIn();
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: 0 } },
      });
      render(
        <App
          router={createTestRouter(['/interfaces'], { devRoutes: false })}
          streamUrl={STREAM}
          queryClient={queryClient}
        />,
      );
      const grid = await screen.findByRole('grid', {}, { timeout: 15_000 });
      fireEvent.click(await within(grid).findByText('host-w7l0'));
      const drawer = await screen.findByRole('region', { name: 'Interface host-w7l0' });
      const mtu = await within(drawer).findByLabelText(/^MTU/);
      fireEvent.change(mtu, { target: { value: '1400' } });
      fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(patches).toHaveLength(1));
      expect(patches[0]).toEqual({ 'host-w7l0': { mtu: 1400 } });
      expect(JSON.stringify(patches[0])).not.toContain('l2');
    },
  );
});
