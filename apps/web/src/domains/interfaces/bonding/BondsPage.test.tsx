import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import type { InterfaceItem, InterfacesConfig } from '../model';
import { pageOfBonds, toBondRow } from './BondsPage';
import { bondFormSchema, bondStatus, eligibleMembers, memberSchema, memberStatus, nextBondName, type BondItem, type LiveMember } from './model';

/** F-bonding screen in jsdom against a scripted stand-in of the API (unit level; the real stack runs in test/topology/bonding). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const port = (key: number, flags: string[]) => ({ systemPriority: 65535, system: key ? '02:fe:b0:00:00:01' : '00:00:00:00:00:00', key, portPriority: 255, portNumber: 1, state: 0x47, stateFlags: flags });
const lacp = { rxState: 'defaulted', txState: 'transmit', muxState: 'detached', ptxState: 'fast-periodic', actor: port(6000, ['activity', 'timeout', 'aggregation', 'defaulted']), partner: port(0, []) };
const member = (name: string, extra: Partial<LiveMember> = {}): LiveMember => ({
  interface: name, swIfIndex: 7, passive: false, longTimeout: false, weight: 0, isLocalNuma: true, adminUp: true, linkUp: true, lacp, ...extra,
});
const bondCfg = { mode: 'lacp', loadBalance: 'l34', numaOnly: false, members: { tap6000: { passive: false, longTimeout: false }, tap6001: { passive: true, longTimeout: false } } };
const B0: BondItem = {
  name: 'BondEthernet6000',
  state: {
    vppName: 'BondEthernet6000', swIfIndex: 12, id: 6000, mode: 'lacp', loadBalance: 'l34', numaOnly: false, adminUp: true, linkUp: false,
    memberCount: 2, activeMemberCount: 0, members: [member('tap6000'), member('tap6001', { passive: true })],
  },
  running: bondCfg,
  candidate: bondCfg,
  hasPendingChange: false,
};
const B1: BondItem = { name: 'BondEthernet6001', state: null, running: null, candidate: { mode: 'active-backup', members: {} }, hasPendingChange: true };

const liveIf = (name: string, type = 'tap') =>
  ({ name, kind: 'interface', parent: null, state: { name, type }, config: null, running: null, counters: null, hasPendingChange: false }) as unknown as InterfaceItem;

const candidate: InterfacesConfig = {
  BondEthernet6000: { enabled: true, ipv4: ['10.6.10.1/24'], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: {}, bond: bondCfg },
  BondEthernet6001: { enabled: true, ipv4: [], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: {}, bond: { mode: 'active-backup', members: {}, numaOnly: false } },
  tap6000: { enabled: true, ipv4: [], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: {} },
  tap6001: { enabled: true, ipv4: [], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: {} },
  'host-w6l0': { enabled: true, ipv4: ['10.6.1.1/24'], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: {} },
} as unknown as InterfacesConfig;

function withBonds(api: FakeApi) {
  api.on('GET /api/v1/state/interfaces/bonds', { body: { retrievedAt: new Date().toISOString(), live: true, items: [B0, B1] } });
  api.on('GET /api/v1/state/interfaces', {
    body: { items: [liveIf('tap6000'), liveIf('tap6001'), liveIf('tap6002'), liveIf('tap6003'), liveIf('loop6000', 'loopback'), liveIf('host-w6l0', 'af-packet'), liveIf('BondEthernet6000', 'bond')] },
  });
  api.on('GET /api/v1/config/candidate/interfaces', { body: candidate });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('bond model', () => {
  it('rows: live state first, the candidate for a bond VPP does not have yet', () => {
    expect(toBondRow(B0)).toMatchObject({ name: 'BondEthernet6000', mode: 'lacp', loadBalance: 'l34', members: 'tap6000 tap6001', active: '0/2', status: 'degraded' });
    expect(toBondRow(B1)).toMatchObject({ mode: 'active-backup', members: '', active: '', status: '' });
    const req = { page: 0, pageSize: 25, sort: [{ field: 'name', dir: 'desc' as const }], filter: [], filterLogic: 'and' as const, quickFilter: [] };
    expect(pageOfBonds([B0, B1].map(toBondRow), req).rows.map((r) => r.name)).toEqual(['BondEthernet6001', 'BondEthernet6000']);
    expect(pageOfBonds([B0, B1].map(toBondRow), { ...req, quickFilter: ['tap6001'] }).total).toBe(1);
  });

  it('eligible members: NICs without L3, not bonds, loopbacks or members of another bond; own members stay', () => {
    const live = [liveIf('tap6000'), liveIf('tap6001'), liveIf('tap6002'), liveIf('loop6000', 'loopback'), liveIf('host-w6l0', 'af-packet')];
    expect(eligibleMembers('BondEthernet6001', candidate, live)).toEqual(['tap6002']);
    expect(eligibleMembers('BondEthernet6000', candidate, live)).toEqual(['tap6000', 'tap6001', 'tap6002']);
  });

  it('statuses and the next free bond name', () => {
    expect(bondStatus(B0.state)).toBe('degraded');
    expect(bondStatus({ ...B0.state!, activeMemberCount: 1 })).toBe('up');
    expect(bondStatus({ ...B0.state!, adminUp: false })).toBe('adminDown');
    expect(memberStatus(member('a'))).toBe('degraded'); // LACP not collecting/distributing
    expect(memberStatus(member('a', { lacp: null }))).toBe('up');
    expect(memberStatus(member('a', { linkUp: false, lacp: null }))).toBe('down');
    expect(nextBondName(['BondEthernet6000', 'BondEthernet6001', 'tap0'], 6000)).toBe('BondEthernet6002');
    expect(nextBondName([])).toBe('BondEthernet0');
  });

  it('forms come from the one schema: bond fields without members, member options', () => {
    expect(Object.keys(bondFormSchema().properties ?? {}).sort()).toEqual(['id', 'loadBalance', 'mode', 'numaOnly']);
    expect(Object.keys(memberSchema().properties ?? {}).sort()).toEqual(['longTimeout', 'passive', 'weight']);
  });
});

describe('bonds screen', () => {
  it('lists bonds with live member/LACP state; the drawer adds an eligible member through a merge patch', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withBonds(api);
    const patches: unknown[] = [];
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patches.push(body);
      return { body: { pointer: '/interfaces', before: null, after: null } };
    });
    await signIn();
    render(app('/interfaces/bonds'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Bonds' }, { timeout: 15_000 })).toBeInTheDocument();
    const grid = await screen.findByRole('grid');
    expect(await within(grid).findByText('BondEthernet6000')).toBeInTheDocument();
    expect(within(grid).getByText('tap6000: detached')).toBeInTheDocument(); // live LACP state per member
    expect(within(grid).getByText('pending')).toBeInTheDocument(); // BondEthernet6001 is only in the candidate

    fireEvent.click(within(grid).getByText('BondEthernet6000'));
    const drawer = await screen.findByRole('region', { name: 'Bond BondEthernet6000' });
    expect(await within(drawer).findAllByText(/defaulted \(no partner\)/)).toHaveLength(2);
    expect(within(drawer).getByText('0 of 2')).toBeInTheDocument();

    // the picker offers only the eligible NICs: tap6002 and tap6003 (not the loopback, the af_packet NIC with an address,
    // or the bonds)
    fireEvent.mouseDown(within(drawer).getByLabelText('Eligible interface'));
    const list = await screen.findByRole('listbox');
    expect(within(list).getAllByRole('option').map((o) => o.textContent)).toEqual(['tap6002', 'tap6003']);
    fireEvent.click(within(list).getByText('tap6003'));
    fireEvent.click(within(drawer).getByRole('button', { name: 'Add member' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add tap6003 to BondEthernet6000' });
    fireEvent.click(within(dialog).getByLabelText(/^Passive/));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    // tap6003 is not configured yet: it is added enabled, as the member rule requires
    await waitFor(() =>
      expect(patches.at(-1)).toEqual({
        BondEthernet6000: { bond: { members: { tap6003: { passive: true, longTimeout: false } } } },
        tap6003: { enabled: true },
      }),
    );

    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Add tap6003 to BondEthernet6000' })).toBeNull());

    // remove a member
    fireEvent.click(await within(drawer).findByRole('button', { name: 'Remove member tap6001' }));
    await waitFor(() => expect(patches.at(-1)).toEqual({ BondEthernet6000: { bond: { members: { tap6001: null } } } }));

    // edit the bond: only the change is sent
    const lb = within(drawer).getByLabelText(/^Load balance/);
    fireEvent.mouseDown(lb);
    fireEvent.click(within(await screen.findByRole('listbox')).getByText('l23'));
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(patches.at(-1)).toEqual({ BondEthernet6000: { bond: { loadBalance: 'l23' } } }));
  });

  it('adds a bond with the next free id and refuses an id in use', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withBonds(api);
    const patches: unknown[] = [];
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patches.push(body);
      return { body: { pointer: '/interfaces', before: null, after: null } };
    });
    await signIn();
    render(app('/interfaces/bonds'));
    const add = await screen.findByRole('button', { name: 'Add bond' }, { timeout: 15_000 });
    await waitFor(() => expect(add).toBeEnabled()); // once the candidate (taken names) is loaded
    fireEvent.click(add);
    const dialog = await screen.findByRole('dialog', { name: 'Add a bond interface' });
    expect(within(dialog).getByText('Creates BondEthernet0')).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText(/^Bond ID/), { target: { value: '6000' } });
    expect(within(dialog).getByText('BondEthernet6000 is already configured')).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText(/^Bond ID/), { target: { value: '6002' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add bond' }));
    await waitFor(() => expect(patches.at(-1)).toEqual({ BondEthernet6002: { enabled: true, bond: { mode: 'lacp', members: {} } } }));
  });

  it('is in the Interfaces navigation group and renders in Persian (RTL)', { timeout: 60_000 }, async () => {
    const api = installFakeApi('readonly', 'ro');
    withBonds(api);
    await signIn();
    render(app('/interfaces/bonds'));
    await screen.findByRole('grid', {}, { timeout: 15_000 });
    expect(screen.getAllByRole('link', { name: 'Bonds' }).length).toBeGreaterThan(0); // nav entry (Interfaces group)
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByRole('button', { name: 'افزودن باند' }, { timeout: 15_000 })).toBeDisabled(); // readonly role
    expect(screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toContain('باندها');
    expect(screen.getAllByRole('link', { name: 'باندها' }).length).toBeGreaterThan(0);
  });
});
