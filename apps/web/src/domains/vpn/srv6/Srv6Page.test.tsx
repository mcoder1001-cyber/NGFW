import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import {
  canonLocalSid,
  canonPolicy,
  canonSteering,
  effectiveEncapSource,
  globalsFormSchema,
  localSidFormSchema,
  localSidRows,
  move,
  policyFormSchema,
  policyRows,
  sidListProblems,
  sortSteering,
  srv6Of,
  steeringRows,
  type Srv6State,
  type Srv6SteeringConfig,
} from './model';
import { SRV6_STATE_POLL_MS, srv6StateQuery } from './queries';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

const routing = {
  static: [],
  srv6: {
    encapSource: 'fd00:4::1',
    localSids: {
      'fd00:4:ff::1': { behavior: 'end', psp: true, vrf: 'default' },
      'fd00:4:ff::a': { behavior: 'end.dt4', psp: false, vrf: 'default', lookupVrf: 'cust-a' },
    },
    policies: {
      'fd00:4:bb::1': {
        type: 'default',
        encap: true,
        vrf: 'default',
        sidLists: [{ sids: ['fd00:4:ee::1', 'fd00:4:ff::a'], weight: 1 }],
      },
    },
    steering: [{ type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::1' }],
  },
};

const state: Srv6State = {
  retrievedAt: '2026-09-25T10:00:00.000Z',
  localSids: [
    {
      sid: 'fd00:4:ff::a',
      behavior: 'end.dt4',
      psp: false,
      vrf: 'default',
      table: 0,
      interface: null,
      nextHop: null,
      lookupVrf: 'cust-a',
      lookupTable: 4001,
      goodPackets: 1234,
      goodBytes: 98765,
      badPackets: 2,
      badBytes: 200,
      configured: true,
    },
  ],
  policies: [
    {
      bsid: 'fd00:4:bb::1',
      type: 'default',
      encap: true,
      vrf: 'default',
      table: 0,
      encapSource: 'fd00:4::1',
      sidLists: [{ sids: ['fd00:4:ee::1', 'fd00:4:ff::a'], weight: 1 }],
      configured: true,
    },
  ],
  steering: [
    {
      type: 'l3',
      trafficType: 'ipv4',
      prefix: '10.4.100.0/24',
      vrf: 'cust-a',
      table: 4001,
      interface: null,
      bsid: 'fd00:4:bb::1',
      configured: true,
    },
  ],
};

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('SRv6 model', () => {
  it('extracts the one schema for the forms', () => {
    expect(Object.keys(localSidFormSchema().properties ?? {})).toEqual([
      'behavior',
      'psp',
      'vrf',
      'interface',
      'nextHop',
      'lookupVrf',
    ]);
    const policy = policyFormSchema();
    expect(Object.keys(policy.properties ?? {})).toEqual(['type', 'encap', 'vrf', 'encapSource']);
    expect(policy.required ?? []).not.toContain('sidLists');
    expect(Object.keys(globalsFormSchema().properties ?? {})).toEqual([
      'encapSource',
      'encapHopLimit',
    ]);
  });

  it('canonicalises addresses before saving (routing.srv6-canonical)', () => {
    expect(
      canonLocalSid({
        behavior: 'end.x',
        psp: false,
        vrf: 'default',
        interface: 'loop1',
        nextHop: 'FD00:4:1:0::2',
      }),
    ).toMatchObject({
      nextHop: 'fd00:4:1::2',
    });
    expect(
      canonPolicy({
        type: 'default',
        encap: true,
        vrf: 'default',
        encapSource: 'FD00:4::1',
        sidLists: [{ sids: ['FD00:4:EE:0:0::1'], weight: 1 }],
      }),
    ).toMatchObject({ encapSource: 'fd00:4::1', sidLists: [{ sids: ['fd00:4:ee::1'] }] });
    expect(
      canonSteering({
        type: 'l3',
        prefix: 'FD00:4:100::/48',
        vrf: 'default',
        bsid: 'FD00:4:BB::1',
      }),
    ).toEqual({
      type: 'l3',
      prefix: 'fd00:4:100::/48',
      vrf: 'default',
      bsid: 'fd00:4:bb::1',
    });
  });

  it('saves steering in Retrieve order: L3 by VRF then prefix (code-unit order), then L2 by interface', () => {
    const list: Srv6SteeringConfig[] = [
      { type: 'l2', interface: 'host-w4l1', bsid: 'fd00:4:bb::2' },
      { type: 'l3', prefix: 'fd00:4:100::/48', vrf: 'default', bsid: 'fd00:4:bb::3' },
      { type: 'l2', interface: 'host-w4l0', bsid: 'fd00:4:bb::2' },
      { type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::1' },
      { type: 'l3', prefix: '10.4.0.0/16', vrf: 'default', bsid: 'fd00:4:bb::1' },
      { type: 'l3', prefix: '10.4.1.0/24', vrf: 'Z', bsid: 'fd00:4:bb::1' },
    ];
    expect(
      sortSteering(list).map((s) => (s.type === 'l3' ? `${s.vrf} ${s.prefix}` : s.interface)),
    ).toEqual([
      'Z 10.4.1.0/24',
      'cust-a 10.4.100.0/24',
      'default 10.4.0.0/16',
      'default fd00:4:100::/48',
      'host-w4l0',
      'host-w4l1',
    ]);
  });

  it('checks segment lists (≤ 16 IPv6 SIDs, weight 1–65535) and reorders SIDs', () => {
    expect(sidListProblems([])).toEqual([{ list: -1, problem: 'noList' }]);
    expect(
      sidListProblems([
        { sids: ['fd00:4:ee::1'], weight: 1 },
        { sids: [], weight: 1 },
        { sids: Array.from({ length: 17 }, (_, i) => `fd00::${i + 1}`), weight: 1 },
        { sids: ['10.0.0.1'], weight: 1 },
        { sids: ['fd00::1'], weight: 0 },
      ]),
    ).toEqual([
      { list: 1, problem: 'empty' },
      { list: 2, problem: 'tooMany' },
      { list: 3, problem: 'badSid' },
      { list: 4, problem: 'badWeight' },
    ]);
    expect(move(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b']);
    expect(move(['a', 'b', 'c'], 0, -1)).toEqual(['a', 'b', 'c']);
  });

  it('joins configuration and live state into rows (installed / missing / not configured)', () => {
    const cfg = srv6Of(routing);
    const extra: Srv6State = {
      ...state,
      policies: [
        ...state.policies,
        { ...state.policies[0]!, bsid: 'fd00:4:bb::9', configured: false },
      ],
    };
    expect(localSidRows(cfg, state).map((r) => [r.sid, r.status])).toEqual([
      ['fd00:4:ff::1', 'missing'],
      ['fd00:4:ff::a', 'installed'],
    ]);
    expect(policyRows(cfg, extra).map((r) => [r.bsid, r.status])).toEqual([
      ['fd00:4:bb::1', 'installed'],
      ['fd00:4:bb::9', 'unmanaged'],
    ]);
    expect(steeringRows(cfg, state).map((r) => r.status)).toEqual(['installed']);
    expect(effectiveEncapSource(cfg.policies['fd00:4:bb::1']!, cfg)).toBe('fd00:4::1');
    expect(srv6Of({})).toEqual({ localSids: {}, policies: {}, steering: [] });
  });

  it('D-132: the state is not refetched on focus or remount within 30 s', () => {
    expect(SRV6_STATE_POLL_MS).toBeGreaterThanOrEqual(30_000);
    expect(srv6StateQuery.staleTime).toBeGreaterThanOrEqual(30_000);
    expect(srv6StateQuery.refetchInterval).toBeGreaterThanOrEqual(30_000);
    expect(srv6StateQuery.refetchOnWindowFocus).toBe(false);
  });
});

function renderTab() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  render(
    <App
      router={createTestRouter(['/vpn?tab=srv6'], { devRoutes: false })}
      streamUrl={STREAM}
      queryClient={queryClient}
    />,
  );
}

describe('SRv6 tab', () => {
  it('lists local SIDs with counters, policies with segment lists, steering; the proxy note', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/srv6', { body: state });
    api.on('GET /api/v1/config/candidate/routing', { body: routing });
    await signIn();
    renderTab();
    const row = await screen.findByTestId('srv6-sid-fd00:4:ff::a', {}, { timeout: 15_000 });
    expect(within(row).getByText('1,234 pkts / 98,765 B')).toBeInTheDocument();
    expect(within(row).getByText('2 pkts / 200 B')).toBeInTheDocument();
    expect(within(row).getByText('Installed')).toBeInTheDocument();
    expect(
      within(screen.getByTestId('srv6-sid-fd00:4:ff::1')).getByText('Not in the data plane'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('srv6-proxy-note')).toHaveTextContent(/End\.AD/);
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Policies' }));
    const policy = await screen.findByTestId('srv6-policy-fd00:4:bb::1');
    expect(within(policy).getByText('fd00:4:ee::1 → fd00:4:ff::a (w 1)')).toBeInTheDocument();
    expect(within(policy).getByText('fd00:4::1')).toBeInTheDocument();
    expect(screen.getByTestId('srv6-globals')).toHaveTextContent(/globals owner/);
    // a policy with steering cannot be deleted alone
    expect(within(policy).getByRole('button', { name: 'Delete' })).toBeDisabled();

    fireEvent.click(screen.getByRole('tab', { name: 'Steering' }));
    const steer = await screen.findByTestId('srv6-steer-l3|cust-a|10.4.100.0/24');
    expect(within(steer).getByText('IPv4')).toBeInTheDocument();
  });

  it('SID-list editor: add, reorder and save a policy (canonical, through PATCH /config/routing)', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/srv6', { body: state });
    api.on('GET /api/v1/config/candidate/routing', { body: routing });
    api.on('PATCH /api/v1/config/routing', { body: {} });
    await signIn();
    renderTab();
    fireEvent.click(await screen.findByRole('tab', { name: 'Policies' }, { timeout: 15_000 }));
    fireEvent.click(await screen.findByRole('button', { name: 'Add policy' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Binding SID (IPv6 address)'), {
      target: { value: 'FD00:4:BB::2' },
    });
    const editor = within(dialog).getByTestId('srv6-sid-list-0');
    fireEvent.change(within(editor).getByLabelText('Segment 1 of list 1'), {
      target: { value: 'fd00:4:ee::2' },
    });
    fireEvent.click(within(editor).getByRole('button', { name: 'Add segment (at most 16)' }));
    fireEvent.change(within(editor).getByLabelText('Segment 2 of list 1'), {
      target: { value: 'FD00:4:EE::1' },
    });
    fireEvent.click(within(editor).getAllByRole('button', { name: 'Move up' })[1]!);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH')).toBe(true));
    const body = api.calls.find((c) => c.method === 'PATCH')!.body as {
      srv6: {
        policies: Record<
          string,
          { sidLists: { sids: string[]; weight: number }[]; encap: boolean }
        >;
      };
    };
    expect(Object.keys(body.srv6.policies)).toEqual(['fd00:4:bb::2']);
    expect(body.srv6.policies['fd00:4:bb::2']).toMatchObject({
      encap: true,
      sidLists: [{ sids: ['fd00:4:ee::1', 'fd00:4:ee::2'], weight: 1 }],
    });
  });

  it('adds a steering entry and saves the whole list in Retrieve order', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/srv6', { body: state });
    api.on('GET /api/v1/config/candidate/routing', { body: routing });
    api.on('GET /api/v1/config/candidate/vrfs', { body: { 'cust-a': { id: 4001 } } });
    api.on('GET /api/v1/config/candidate/interfaces', { body: {} });
    api.on('PATCH /api/v1/config/routing', { body: {} });
    await signIn();
    renderTab();
    fireEvent.click(await screen.findByRole('tab', { name: 'Steering' }, { timeout: 15_000 }));
    fireEvent.click(await screen.findByRole('button', { name: 'Add steering' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Prefix'), { target: { value: '10.4.0.0/16' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH')).toBe(true));
    const body = api.calls.find((c) => c.method === 'PATCH')!.body as {
      srv6: { steering: unknown[] };
    };
    expect(body.srv6.steering).toEqual([
      { type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::1' },
      { type: 'l3', prefix: '10.4.0.0/16', vrf: 'default', bsid: 'fd00:4:bb::1' },
    ]);
  });

  it('renders in Persian (RTL) with the translated sub-tabs', { timeout: 30_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/srv6', { body: state });
    api.on('GET /api/v1/config/candidate/routing', { body: routing });
    await signIn();
    renderTab();
    await screen.findByTestId('srv6-sid-fd00:4:ff::a', {}, { timeout: 15_000 });
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(
      await screen.findByRole('tab', { name: 'SIDهای محلی' }, { timeout: 15_000 }),
    ).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'هدایت ترافیک' })).toBeInTheDocument();
  });
});
