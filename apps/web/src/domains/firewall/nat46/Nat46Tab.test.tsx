import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { natTabs } from '../nat44-ed-sessions/tabs';
import { nat46Schema } from './model';

/** F-nat46 tab of the NAT screen in jsdom against a scripted stand-in of the API. */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const LONG = { timeout: 120_000 };

function app(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App
      router={createTestRouter([path], { devRoutes: false })}
      streamUrl={STREAM}
      queryClient={queryClient}
    />
  );
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('nat46 model', () => {
  it('registers the NAT46 tab after NPTv6, before the CGNAT group', () => {
    const ids = natTabs.map((t) => t.id);
    expect(ids.indexOf('nat46')).toBe(ids.indexOf('nptv6') + 1);
    expect(ids.indexOf('cgnat')).toBe(ids.indexOf('nat46') + 1);
  });
  it('the schema has the NAT46 fields', () => {
    expect(Object.keys(nat46Schema().properties ?? {}).sort()).toEqual([
      'clientPrefix',
      'interfaces',
      'mappings',
    ]);
  });
});

describe('NAT screen: NAT46 tab', () => {
  it('shows the form and the client address', LONG, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/nat', {
      body: {
        nat46: {
          clientPrefix: 'fd00:8:46::/96',
          interfaces: ['host-w8l0'],
          mappings: [{ name: 'web', ipv4: '10.8.2.80', ipv6: 'fd00:8:2::80' }],
        },
      },
    });
    api.on('GET /api/v1/config/candidate/interfaces', {
      body: { 'host-w8l0': { enabled: true }, 'host-w8w0': { enabled: true } },
    });
    api.on('GET /api/v1/state/nat/nat46/client', {
      body: { ipv4: '10.8.1.2', clientPrefix: 'fd00:8:46::/96', ipv6: 'fd00:8:46::a08:102' },
    });
    await signIn();
    render(app('/firewall/nat?tab=nat46'));
    expect(
      await screen.findByDisplayValue('fd00:8:46::/96', {}, { timeout: 30_000 }),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('IPv4 client'), { target: { value: '10.8.1.2' } });
    fireEvent.click(screen.getByRole('button', { name: 'Show' }));
    expect(await screen.findByTestId('nat46-client')).toHaveTextContent(
      '10.8.1.2 appears as fd00:8:46::a08:102',
    );
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === '/api/v1/state/nat/nat46/client')?.search,
      ).toContain('ipv4=10.8.1.2'),
    );
  });
});
