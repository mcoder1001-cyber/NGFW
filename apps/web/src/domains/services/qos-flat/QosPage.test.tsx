import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import type { PolicerItem, QosCfg } from './model';

/** F-qos-flat screen in jsdom against a scripted stand-in of the API (the real stack: test/topology/qos-flat). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const qos: QosCfg = {
  policers: {
    'w1-gold': {
      type: '2r3c-rfc2698',
      rateUnit: 'kbps',
      cir: 20_000,
      eir: 40_000,
      cb: 25_000,
      eb: 50_000,
      conformAction: { action: 'transmit' },
      exceedAction: { action: 'mark-and-transmit', dscp: 10 },
      violateAction: { action: 'drop' },
    },
  },
  shapers: { 'w1-uplink': { rateKbps: 50_000 } },
  maps: { 'w1-remark': { id: 1001, rows: { ip: [{ from: 46, to: 34 }] } } },
  interfaces: {
    loop1001: { policer: { input: 'w1-gold' }, shaper: 'w1-uplink', record: 'vlan' },
    loop1002: { mark: { map: 'w1-remark', output: 'ip' } },
  },
};

const zero = { packets: '0', bytes: '0' };
const items: PolicerItem[] = [
  {
    name: 'w1-gold',
    kind: 'policer',
    vppName: 'w1-gold',
    configured: true,
    present: true,
    index: 3,
    type: '2r3c-rfc2698',
    rateUnit: 'kbps',
    cir: 20_000,
    eir: 40_000,
    cb: 25_000,
    eb: 50_000,
    bucket: { current: 10, limit: 10, extendedCurrent: 20, extendedLimit: 20 },
    conform: { packets: '1234', bytes: '98765' },
    exceed: { packets: '56', bytes: '4000' },
    violate: { packets: '7', bytes: '500' },
    attachments: [{ interface: 'loop1001', direction: 'input' }],
  },
  {
    name: 'w1-uplink',
    kind: 'shaper',
    vppName: 'shaper:w1-uplink',
    configured: true,
    present: true,
    index: 4,
    type: '1r2c',
    rateUnit: 'kbps',
    cir: 50_000,
    eir: 0,
    cb: 62_500,
    eb: 0,
    bucket: { current: 1, limit: 1, extendedCurrent: 0, extendedLimit: 0 },
    conform: { packets: '99', bytes: '9900' },
    exceed: { packets: '3', bytes: '300' },
    violate: zero,
    attachments: [{ interface: 'loop1001', direction: 'output' }],
  },
];

function withQos(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/services', { body: { qos } });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: { loop1001: { enabled: true }, loop1002: { enabled: true, subinterfaces: { '10': { vlanId: 10 } } } },
  });
  api.on('GET /api/v1/state/services/qos/policers', {
    body: { retrievedAt: '2026-09-25T10:00:00.000Z', countersError: null, items },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

async function openQos() {
  render(app('/services?tab=qos'));
  expect(await screen.findByRole('tab', { name: 'QoS' }, { timeout: 15_000 })).toBeInTheDocument();
  return screen.findByRole('tablist', { name: 'QoS sections' });
}

describe('qos screen', () => {
  it(
    'lists policers with live counters and status, rate limits with the drop caveat, maps and attachments',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withQos(api);
      await signIn();
      const sections = await openQos();
      expect(within(sections).getAllByRole('tab').map((t) => t.textContent)).toEqual([
        'Policers',
        'Rate limits (egress)',
        'Marking maps',
        'Interface attachments',
      ]);
      const policers = await screen.findByRole('table', { name: 'Policers' });
      expect(await within(policers).findByText('w1-gold')).toBeInTheDocument();
      expect(await within(policers).findByText('Applied')).toBeInTheDocument();
      expect(within(policers).getByText('1,234')).toBeInTheDocument();
      expect(within(policers).getByText('20000 / 40000 kbit/s')).toBeInTheDocument();

      fireEvent.click(within(sections).getByRole('tab', { name: 'Rate limits (egress)' }));
      expect(await screen.findByText(/traffic above the rate is DROPPED, not delayed/)).toBeInTheDocument();
      const shapers = await screen.findByRole('table', { name: 'Rate limits (egress)' });
      expect(await within(shapers).findByText('auto (62,500 B: ≈ 10 ms, ≥ 3000 B)')).toBeInTheDocument();
      expect(within(shapers).getByText('99')).toBeInTheDocument();

      fireEvent.click(within(sections).getByRole('tab', { name: 'Marking maps' }));
      const maps = await screen.findByRole('table', { name: 'Marking maps' });
      expect(within(maps).getByText('w1-remark')).toBeInTheDocument();
      expect(within(maps).getByText('1001')).toBeInTheDocument();
      expect(within(maps).getByText('IP DSCP: 1')).toBeInTheDocument();

      fireEvent.click(within(sections).getByRole('tab', { name: 'Interface attachments' }));
      const att = await screen.findByRole('table', { name: 'Interface attachments' });
      expect(within(att).getByText('rate limit w1-uplink')).toBeInTheDocument();
      expect(within(att).getByText('w1-remark → IP DSCP')).toBeInTheDocument();
      expect(screen.getByText(/write-only, D-063/)).toBeInTheDocument();

      // D-132: the state is read on demand by Refresh, never polled faster than 30 s
      const reads = () => api.calls.filter((c) => c.path === '/api/v1/state/services/qos/policers').length;
      const before = reads();
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
      await waitFor(() => expect(reads()).toBeGreaterThan(before));
    },
  );

  it('resets a policer through the action route (the shaper by its VPP name)', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withQos(api);
    const resets: string[] = [];
    api.on('POST /api/v1/actions/qos/policers/w1-gold/reset', () => {
      resets.push('w1-gold');
      return { body: { name: 'w1-gold', index: 3, resetAt: '2026-09-25T10:00:01.000Z' } };
    });
    api.on('POST /api/v1/actions/qos/policers/shaper%3Aw1-uplink/reset', () => {
      resets.push('shaper:w1-uplink');
      return { body: { name: 'shaper:w1-uplink', index: 4, resetAt: '2026-09-25T10:00:02.000Z' } };
    });
    await signIn();
    const sections = await openQos();
    const policers = await screen.findByRole('table', { name: 'Policers' });
    fireEvent.click(await within(policers).findByRole('button', { name: 'Reset w1-gold' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/Counters are kept/)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Reset' }));
    expect(await within(dialog).findByText('Token buckets of w1-gold refilled.')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    fireEvent.click(within(sections).getByRole('tab', { name: 'Rate limits (egress)' }));
    const shapers = await screen.findByRole('table', { name: 'Rate limits (egress)' });
    fireEvent.click(await within(shapers).findByRole('button', { name: 'Reset w1-uplink' }));
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Reset' }));
    await waitFor(() => expect(resets).toEqual(['w1-gold', 'shaper:w1-uplink']));
  });

  it(
    'adds a policer through a merge patch of /config/services; problems map onto the form',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withQos(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patched = body;
        return { body: { pointer: '/services', before: null, after: null } };
      });
      await signIn();
      await openQos();
      fireEvent.click(await screen.findByRole('button', { name: 'Add policer' }));
      const dialog = await screen.findByRole('dialog');
      fireEvent.change(within(dialog).getByLabelText('Name'), { target: { value: 'w1-silver' } });
      // the rate/burst helper fills CIR and CB (10 ms of 8000 kbit/s = 10000 bytes)
      fireEvent.change(within(dialog).getByLabelText('Rate'), { target: { value: '8000' } });
      expect(within(dialog).getByText('Burst ≈ 10000 bytes')).toBeInTheDocument();
      fireEvent.click(within(dialog).getByRole('button', { name: 'Use rate and burst' }));
      await waitFor(() =>
        expect(within(dialog).getByLabelText(/^Committed information rate \(CIR\)/)).toHaveValue(8000),
      );
      fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() =>
        expect(patched).toMatchObject({
          qos: { policers: { 'w1-silver': { cir: 8000, cb: 10_000, rateUnit: 'kbps', type: '1r2c' } } },
        }),
      );
    },
  );

  it('edits a marking map in the grid: one {from, to} per filled cell, sorted', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withQos(api);
    let patched: unknown;
    api.on('PATCH /api/v1/config/services', (_r, body) => {
      patched = body;
      return { body: { pointer: '/services', before: null, after: null } };
    });
    await signIn();
    const sections = await openQos();
    fireEvent.click(within(sections).getByRole('tab', { name: 'Marking maps' }));
    const maps = await screen.findByRole('table', { name: 'Marking maps' });
    fireEvent.click(within(maps).getByRole('button', { name: 'Edit w1-remark' }));
    const dialog = await screen.findByRole('dialog');
    const grid = within(dialog).getByRole('grid', { name: 'Translation grid for IP DSCP' });
    expect(within(grid).getAllByRole('gridcell')).toHaveLength(64);
    expect(within(grid).getByLabelText('Output for IP DSCP 46')).toHaveValue('34');
    fireEvent.change(within(grid).getByLabelText('Output for IP DSCP 10'), { target: { value: '18' } });
    fireEvent.change(within(grid).getByLabelText('Output for IP DSCP 46'), { target: { value: '' } });
    // PCP grid: 8 cells
    fireEvent.click(within(dialog).getByRole('tab', { name: /802\.1p PCP/ }));
    const pcp = within(dialog).getByRole('grid', { name: 'Translation grid for 802.1p PCP' });
    expect(within(pcp).getAllByRole('gridcell')).toHaveLength(8);
    fireEvent.change(within(pcp).getByLabelText('Output for 802.1p PCP 5'), { target: { value: '300' } });
    expect(within(dialog).getByText('Some cells are not a number between 0 and 255.')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Save to candidate' })).toBeDisabled();
    fireEvent.change(within(pcp).getByLabelText('Output for 802.1p PCP 5'), { target: { value: '46' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() =>
      expect(patched).toEqual({
        qos: { maps: { 'w1-remark': { rows: { ip: [{ from: 10, to: 18 }], vlan: [{ from: 5, to: 46 }] } } } },
      }),
    );
  });

  it('attaches QoS to an interface; server problems show next to the field', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withQos(api);
    let patched: unknown;
    api.on('PATCH /api/v1/config/services', (_r, body) => {
      patched = body;
      return {
        status: 400,
        body: {
          type: 'https://vrx.dev/problems/validation',
          title: 'Invalid configuration',
          status: 400,
          errors: [
            {
              pointer: '/services/qos/interfaces/loop1002.10/store/source',
              message: 'VPP 26.06 stores a QoS value for the ip source only',
            },
          ],
        },
      };
    });
    await signIn();
    const sections = await openQos();
    fireEvent.click(within(sections).getByRole('tab', { name: 'Interface attachments' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Attach to an interface' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Interface' }));
    fireEvent.click(await screen.findByRole('option', { name: 'loop1002.10' }));
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Ingress policer' }));
    fireEvent.click(await screen.findByRole('option', { name: 'w1-gold' }));
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Store a fixed QoS value' }));
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Source slot' }));
    fireEvent.click(await screen.findByRole('option', { name: '802.1p PCP' }));
    fireEvent.change(within(dialog).getByLabelText('Value'), { target: { value: '5' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() =>
      expect(patched).toEqual({
        qos: { interfaces: { 'loop1002.10': { policer: { input: 'w1-gold' }, store: { source: 'vlan', value: 5 } } } },
      }),
    );
    expect(await within(dialog).findByText('VPP 26.06 stores a QoS value for the ip source only')).toBeInTheDocument();
  });
});
