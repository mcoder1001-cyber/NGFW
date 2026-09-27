import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { natTabs } from '../nat44-ed-sessions/tabs';
import { det44Plan, subtreeSchema } from './model';

/** F-det44-map-dslite-cnat tabs of the NAT screen in jsdom against a scripted stand-in of the API. */
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

function withNat(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/nat', {
    body: {
      det44: {
        enabled: true,
        inside: ['host-w8l0'],
        outside: ['host-w8w0'],
        mappings: [{ inside: '10.8.1.0/24', outside: '10.8.2.200/30' }],
      },
      cnat: {
        translations: [],
        snat: { policy: 'none', addresses: {}, interfaces: [], excludePrefixes: [] },
      },
    },
  });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: { 'host-w8l0': { enabled: true }, 'host-w8w0': { enabled: true } },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('det44-map-dslite-cnat model', () => {
  it('registers CGNAT, MAP, CNAT and PNAT after the other NAT tabs', () => {
    expect(natTabs.map((t) => t.id).slice(-4)).toEqual(['cgnat', 'map', 'cnat', 'pnat']);
  });
  it('port-block calculator follows VPP (ratio, ports per host, block of a host)', () => {
    const p = det44Plan('10.8.1.0/24', '10.8.2.200/30');
    if (typeof p === 'string') throw new Error(p);
    expect([p.ratio, p.portsPerHost, p.insideHosts, p.outsideAddresses]).toEqual([
      64, 1008, 256, 4,
    ]);
    expect(p.blockOf(70)).toEqual({
      outsideOffset: 1,
      lo: 1024 + 1008 * 6,
      hi: 1024 + 1008 * 7 - 1,
    });
    expect(det44Plan('10.0.0.0/30', '10.0.0.0/24')).toBe('outside-larger');
    expect(det44Plan('10.0.0.0/8', '10.0.0.0/24')).toBe('ratio-too-large');
    expect(det44Plan('nope', '10.0.0.0/24')).toBe('invalid');
  });
  it('every subtree has a schema (pnat included)', () => {
    for (const k of ['det44', 'dslite', 'map', 'cnat', 'pnat'] as const) {
      expect(subtreeSchema(k).properties).toBeDefined();
    }
  });
});

describe('NAT screen: CGNAT and CNAT tabs', () => {
  it('CGNAT: calculator, per-user sessions with the port block, forward lookup', LONG, async () => {
    const api = installFakeApi();
    withNat(api);
    api.on('GET /api/v1/state/nat/det44/sessions', {
      body: {
        user: '10.8.1.5',
        outsideAddress: '10.8.2.200',
        portLo: 6064,
        portHi: 7071,
        page: 1,
        pageSize: 100,
        total: 1,
        items: [
          {
            insidePort: 40000,
            outsidePort: 6064,
            externalAddress: '10.8.2.2',
            externalPort: 80,
            state: 'tcp-established',
            expire: 100,
          },
        ],
      },
    });
    let lookup: unknown;
    api.on('POST /api/v1/actions/nat/det44/lookup', (_r, body) => {
      lookup = body;
      return { body: { inside: '10.8.1.70', outside: '10.8.2.201', portLo: 7072, portHi: 8079 } };
    });
    await signIn();
    render(app('/firewall/nat?tab=cgnat'));
    const plan = await screen.findByTestId('det44-plan', {}, { timeout: 30_000 });
    expect(plan).toHaveTextContent('Ports per host: 1008');
    fireEvent.change(screen.getAllByLabelText('Inside address')[0]!, {
      target: { value: '10.8.1.5' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Show' }));
    expect(
      await screen.findByText('Outside 10.8.2.200, ports 6064–7071 · 1 sessions'),
    ).toBeInTheDocument();
    fireEvent.change(screen.getAllByLabelText('Inside address')[1]!, {
      target: { value: '10.8.1.70' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Inside → outside' }));
    await waitFor(() => expect(lookup).toEqual({ inside: '10.8.1.70' }));
    expect(await screen.findByTestId('det44-lookup-result')).toHaveTextContent(
      '10.8.1.70 → 10.8.2.201 ports 7072–8079',
    );
  });

  it('CNAT: the session table is paged by the server', LONG, async () => {
    const api = installFakeApi();
    withNat(api);
    api.on('GET /api/v1/state/nat/cnat/sessions', {
      body: {
        page: 1,
        pageSize: 100,
        total: 1,
        truncated: false,
        items: [
          {
            dstAddress: '10.8.2.100',
            dstPort: 80,
            srcAddress: '10.8.1.10',
            srcPort: 40000,
            protocol: 'tcp',
            translationIndex: 0,
            flags: 0,
          },
        ],
      },
    });
    await signIn();
    render(app('/firewall/nat?tab=cnat'));
    expect(await screen.findByText('10.8.1.10:40000', {}, { timeout: 30_000 })).toBeInTheDocument();
    const call = api.calls.find((c) => c.path === '/api/v1/state/nat/cnat/sessions');
    expect(call?.search).toContain('pageSize=100');
  });
});
