import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import en from '../../../locales/en/loopback-bvi-gso-lldp-span.json';
import fa from '../../../locales/fa/loopback-bvi-gso-lldp-span.json';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { ageText, mirrorPatch, sessionsOf, withSession, type NeighborsPage } from './model';

/** F-loopback-bvi-gso-lldp-span screens in jsdom against a scripted stand-in of the API (real stack: test/topology). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

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

const ifaceDefaults = {
  enabled: true,
  ipv4: [],
  ipv6: [],
  vrf: 'default',
  promiscuous: false,
  subinterfaces: {},
};
const candidateIfs = {
  loop7101: {
    ...ifaceDefaults,
    gso: true,
    mirror: [
      { destination: 'loop7102', direction: 'both', level: 'device' },
      { destination: 'gre7', direction: 'rx', level: 'l2' },
    ],
  },
  loop7102: { ...ifaceDefaults },
};
const neighbors: NeighborsPage = {
  page: 1,
  pageSize: 25,
  total: 2,
  items: [
    {
      interface: 'loop7101',
      swIfIndex: 3,
      heard: true,
      chassisId: '02:00:00:00:71:01',
      chassisIdSubtype: 'mac-address',
      portId: 'Ethernet7',
      portIdSubtype: 'interface-name',
      ttl: 120,
      lastHeardSecAgo: 12,
      lastSentSecAgo: 3,
      configured: true,
      portDescription: 'w7 bvi',
    },
    {
      interface: 'loop7102',
      swIfIndex: 4,
      heard: false,
      chassisId: '',
      chassisIdSubtype: '',
      portId: '',
      portIdSubtype: '',
      ttl: 0,
      lastHeardSecAgo: 0,
      lastSentSecAgo: 3,
      configured: false,
      portDescription: null,
    },
  ],
};

function withFeature(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/interfaces', { body: candidateIfs });
  api.on('GET /api/v1/config/candidate/services', {
    body: {
      lldp: {
        enabled: true,
        txHold: 4,
        txIntervalSec: 30,
        interfaces: [{ interface: 'loop7101', portDescription: 'w7 bvi' }],
      },
      nsim: {
        delayMs: 20,
        bandwidthMbps: 100,
        packetSize: 1500,
        dropFraction: 0,
        outputInterfaces: ['loop7102'],
      },
    },
  });
  api.on('GET /api/v1/state/lldp/neighbors', { body: neighbors });
  api.on('GET /api/v1/state/interfaces', {
    body: {
      items: [
        {
          name: 'loop7101',
          kind: 'interface',
          parent: null,
          state: null,
          config: { mirror: [{ destination: 'loop7102', direction: 'both', level: 'device' }] },
          running: { mirror: candidateIfs.loop7101.mirror },
          counters: null,
          hasPendingChange: false,
        },
      ],
    },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('loopback-bvi-gso-lldp-span model', () => {
  it('sessions with live status, patches, ages; en and fa have the same keys', () => {
    const rows = sessionsOf(candidateIfs as unknown as Parameters<typeof sessionsOf>[0], [
      {
        name: 'loop7101',
        kind: 'interface',
        parent: null,
        state: null,
        config: { mirror: [{ destination: 'loop7102', level: 'device' }] },
        running: { mirror: [{ destination: 'loop7102', direction: 'both', level: 'device' }] },
        counters: null,
        hasPendingChange: true,
      },
    ]);
    expect(rows.map((r) => [r.destination, r.active, r.pending])).toEqual([
      ['loop7102', true, false],
      ['gre7', false, true],
    ]);
    const s = [{ destination: 'a', direction: 'both' as const, level: 'device' as const }];
    expect(withSession(s, -1, { destination: 'b', direction: 'rx', level: 'l2' })).toHaveLength(2);
    expect(withSession(s, 0, null)).toEqual([]);
    expect(mirrorPatch('GigabitEthernet0/0/0', [])).toEqual({
      'GigabitEthernet0/0/0': { mirror: null },
    });
    const t = (k: string, o?: Record<string, unknown>) => `${k}:${String(o?.['n'])}`;
    expect(ageText(0, t)).toBe('');
    expect(ageText(12.4, t)).toBe('age.sec:12');
    expect(ageText(600, t)).toBe('age.min:10');
    const keys = (o: object, p = ''): string[] =>
      Object.entries(o).flatMap(([k, v]) =>
        typeof v === 'object' && v !== null ? keys(v, `${p}${k}.`) : [`${p}${k}`],
      );
    expect(keys(fa).sort()).toEqual(keys(en).sort());
  });
});

describe('LLDP, mirroring and nsim screens', () => {
  it(
    'LLDP: settings form and the live neighbour table; nav entries in the interfaces group',
    { timeout: 30_000 },
    async () => {
      const api = installFakeApi('operator');
      withFeature(api);
      await signIn();
      render(app('/interfaces/lldp'));
      expect(await screen.findByRole('heading', { name: 'LLDP', level: 2 })).toBeTruthy();
      expect(
        (await screen.findByRole('link', { name: 'Port mirroring' })).getAttribute('href'),
      ).toBe('/interfaces/mirroring');
      expect(
        (await screen.findByRole('link', { name: 'Delay simulator (lab)' })).getAttribute('href'),
      ).toBe('/tools/nsim');
      const grid = await screen.findByRole('grid', { name: 'Neighbours' });
      await waitFor(() => expect(within(grid).getByText('02:00:00:00:71:01')).toBeTruthy());
      expect(within(grid).getByText('neighbour heard')).toBeTruthy();
      expect(within(grid).getByText('nothing heard')).toBeTruthy();
      expect(within(grid).getByText('not in the configuration')).toBeTruthy();
      expect(within(grid).getByText('12 s ago')).toBeTruthy();
      // D-132: no fast polling of the walk; a Refresh button asks again
      const before = api.calls.filter((c) => c.path === '/api/v1/state/lldp/neighbors').length;
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
      await waitFor(() =>
        expect(
          api.calls.filter((c) => c.path === '/api/v1/state/lldp/neighbors').length,
        ).toBeGreaterThan(before),
      );
      expect(
        api.calls.some(
          (c) => c.path === '/api/v1/state/lldp/neighbors' && c.search.includes('pageSize=25'),
        ),
      ).toBe(true);
    },
  );

  it(
    'mirroring: lists the sessions with their status; removing one patches the source list',
    { timeout: 30_000 },
    async () => {
      const api = installFakeApi('operator');
      withFeature(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
        patched = body;
        return { body: {} };
      });
      await signIn();
      render(app('/interfaces/mirroring'));
      const table = await screen.findByRole('table', { name: 'Port mirroring' });
      await waitFor(() => expect(within(table).getByText('gre7')).toBeTruthy());
      expect(within(table).getByText('active')).toBeTruthy();
      expect(within(table).getByText('not in VPP')).toBeTruthy();
      fireEvent.click(within(table).getByRole('button', { name: 'Remove loop7101 → gre7' }));
      await waitFor(() =>
        expect(patched).toEqual({
          loop7101: { mirror: [{ destination: 'loop7102', direction: 'both', level: 'device' }] },
        }),
      );
    },
  );

  it(
    'nsim: under Tools, marked as a lab tool; renders in Persian',
    { timeout: 30_000 },
    async () => {
      const api = installFakeApi('readonly');
      withFeature(api);
      await signIn();
      render(app('/tools/nsim'));
      expect(
        await screen.findByRole('heading', { name: 'Network delay simulator', level: 2 }),
      ).toBeTruthy();
      expect(screen.getByText('lab tool')).toBeTruthy();
      expect(await screen.findByText('The simulator is configured.')).toBeTruthy();
      await act(async () => {
        await i18n.changeLanguage('fa');
      });
      expect(
        await screen.findByRole('heading', { name: 'شبیه‌ساز تأخیر شبکه', level: 2 }),
      ).toBeTruthy();
      expect(screen.getByText('ابزار آزمایشگاهی')).toBeTruthy();
    },
  );
});

// Review M6: the two SchemaForms are submitted and the PATCH bodies asserted.
describe('form submits', () => {
  it(
    'LLDP: saving an edit with empty management fields patches only the change',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi('operator');
      withFeature(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patched = body;
        return { body: {} };
      });
      await signIn();
      render(app('/interfaces/lldp'));
      const name = await screen.findByLabelText(/^System name/, {}, { timeout: 20_000 });
      fireEvent.change(name, { target: { value: 'vrx-lab' } });
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await waitFor(() => expect(patched).toEqual({ lldp: { systemName: 'vrx-lab' } }), {
        timeout: 20_000,
      });
    },
  );

  it(
    'nsim: a model without a cross-connect is saved without one',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi('operator');
      withFeature(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patched = body;
        return { body: {} };
      });
      await signIn();
      render(app('/tools/nsim'));
      const delay = await screen.findByLabelText(/^Delay \(ms\)/, {}, { timeout: 20_000 });
      fireEvent.change(delay, { target: { value: '35' } });
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await waitFor(() => expect(patched).toEqual({ nsim: { delayMs: 35 } }), { timeout: 20_000 });
    },
  );

  it(
    'nsim: switching the cross-connect off removes it (merge patch null)',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi('operator');
      withFeature(api);
      api.on('GET /api/v1/config/candidate/services', {
        body: {
          nsim: {
            delayMs: 20,
            bandwidthMbps: 100,
            packetSize: 1500,
            dropFraction: 0,
            crossConnect: { a: 'loop7101', b: 'loop7102' },
            outputInterfaces: [],
          },
        },
      });
      let patched: unknown;
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patched = body;
        return { body: {} };
      });
      await signIn();
      render(app('/tools/nsim'));
      const toggle = await screen.findByLabelText(
        /Cross-connect two interfaces/,
        {},
        { timeout: 20_000 },
      );
      expect((toggle as HTMLInputElement).checked).toBe(true);
      fireEvent.click(toggle);
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await waitFor(() => expect(patched).toEqual({ nsim: { crossConnect: null } }), {
        timeout: 20_000,
      });
    },
  );
});
