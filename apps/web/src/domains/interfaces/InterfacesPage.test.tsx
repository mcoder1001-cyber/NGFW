import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../App';
import i18n from '../../i18n';
import { createTestRouter } from '../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../test-api';
import { pageOf, toRow } from './InterfacesPage';
import type { InterfaceItem } from './model';

/** P08 screen in jsdom against a scripted stand-in of the API (unit level; the real stack runs in test/topology/interfaces). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const live = (name: string, extra: Record<string, unknown> = {}) => ({
  name, vppName: name, swIfIndex: 5, type: 'af-packet', adminUp: true, linkUp: true, mtu: 9000, linkMtu: 9000, mac: '02:fe:00:00:00:01',
  ipv4: ['10.1.1.1/24'], ipv6: [], vrf: 'default', tableId: 0, parent: '', vlanId: 0, innerVlanId: 0, managed: true,
  linkSpeedKbps: '0', rxMode: 'interrupt', description: '', ...extra,
});
const lanCfg = { enabled: true, description: 'lan', ipv4: ['10.1.1.1/24'], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: { '100': { vlanId: 100, enabled: true, ipv4: ['10.1.100.1/24'], ipv6: [], vrf: 'default', dot1ad: false } } };

function withInterfaces(api: FakeApi) {
  api.on('GET /api/v1/state/interfaces', {
    body: {
      items: [
        { name: 'host-w1l0', kind: 'interface', parent: null, state: live('host-w1l0'), config: lanCfg, running: lanCfg, counters: null, hasPendingChange: false },
        { name: 'host-w1l0.100', kind: 'subinterface', parent: 'host-w1l0', state: live('host-w1l0.100', { type: 'sub-interface', linkUp: false, adminUp: false, ipv4: [] }), config: lanCfg.subinterfaces['100'], running: lanCfg.subinterfaces['100'], counters: null, hasPendingChange: true },
        { name: 'host-w1w0', kind: 'interface', parent: null, state: live('host-w1w0', { adminUp: true, linkUp: false, ipv4: ['10.1.2.1/24'] }), config: null, running: null, counters: null, hasPendingChange: false },
      ],
    },
  });
  api.on('GET /api/v1/config/candidate/interfaces', { body: { 'host-w1l0': lanCfg } });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('interface rows', () => {
  it('toRow + pageOf: sub-interfaces, pending flag, sort and quick filter', () => {
    const items = [
      { name: 'host-w1w0', kind: 'interface', parent: null, state: live('host-w1w0', { linkUp: false }), config: null, running: null, counters: { errors: '2', drops: '3' }, hasPendingChange: false },
      { name: 'host-w1l0', kind: 'interface', parent: null, state: live('host-w1l0'), config: lanCfg, running: lanCfg, counters: null, hasPendingChange: true },
    ] as unknown as InterfaceItem[];
    const rows = items.map(toRow);
    expect(rows[0]).toMatchObject({ admin: 'up', link: 'down', errors: 5, addresses: '10.1.1.1/24' });
    const req = { page: 0, pageSize: 25, sort: [{ field: 'name', dir: 'asc' as const }], filter: [], filterLogic: 'and' as const, quickFilter: [] };
    expect(pageOf(rows, req).rows.map((r) => r.name)).toEqual(['host-w1l0', 'host-w1w0']);
    expect(pageOf(rows, { ...req, quickFilter: ['w0'] }).total).toBe(1);
  });

  it('a configured interface VPP does not have yet shows the running configuration, not the Retrieve view (D-105)', () => {
    const row = toRow({
      name: 'host-w1w1', kind: 'interface', parent: null, state: null,
      config: { mtu: 1300, ipv4: ['10.9.9.9/24'], vrf: 'old' }, // what the agent last retrieved
      running: { mtu: 1400, ipv4: ['10.1.3.1/24'], vrf: 'red' }, // what is committed
      counters: null, hasPendingChange: false,
    } as unknown as InterfaceItem);
    expect(row).toMatchObject({ mtu: 1400, addresses: '10.1.3.1/24', vrf: 'red', admin: '', link: '' });
  });
});

describe('interfaces screen', () => {
  it('lists live interfaces with status chips and pending marks; the drawer edits through a merge patch of /interfaces', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withInterfaces(api);
    let patched: unknown;
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patched = body;
      return { body: { pointer: '/interfaces', before: null, after: null } };
    });
    await signIn();
    render(app('/interfaces'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Interfaces' }, { timeout: 15_000 })).toBeInTheDocument();
    const grid = await screen.findByRole('grid');
    expect(await within(grid).findByText('host-w1l0')).toBeInTheDocument();
    // (jsdom has no layout: the virtualised grid renders only its first rows; rows/paging are unit-tested below)
    expect(within(grid).getAllByRole('status').length).toBeGreaterThan(0); // StatusChips

    fireEvent.click(within(grid).getByText('host-w1l0'));
    const drawer = await screen.findByRole('region', { name: 'Interface host-w1l0' });
    expect(within(drawer).getByText('Sub-interfaces (802.1Q)')).toBeInTheDocument();
    expect(await within(drawer).findByText('host-w1l0.100')).toBeInTheDocument();
    const mtu = await within(drawer).findByLabelText(/^MTU/);
    fireEvent.change(mtu, { target: { value: '1400' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    // exactly the MTU: no phantom dhcpClient from the form's defaults
    await waitFor(() => expect(patched).toEqual({ 'host-w1l0': { mtu: 1400 } }));

    // remove the sub-interface
    fireEvent.click(within(drawer).getByRole('button', { name: 'Remove sub-interface 100' }));
    await waitFor(() => expect(patched).toEqual({ 'host-w1l0': { subinterfaces: { '100': null } } }));
  });

  it('saves only the edits against the value the form opened with; a new sub-interface cannot reuse an id (review N4/N5)', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withInterfaces(api);
    const patches: unknown[] = [];
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patches.push(body);
      return { body: { pointer: '/interfaces', before: null, after: null } };
    });
    await signIn();
    render(app('/interfaces'));
    const grid = await screen.findByRole('grid', {}, { timeout: 15_000 });
    fireEvent.click(await within(grid).findByText('host-w1l0'));
    const drawer = await screen.findByRole('region', { name: 'Interface host-w1l0' });
    const mtu = await within(drawer).findByLabelText(/^MTU/);
    // another session changes the description after the form opened
    api.on('GET /api/v1/config/candidate/interfaces', { body: { 'host-w1l0': { ...lanCfg, description: 'set elsewhere' } } });
    fireEvent.change(mtu, { target: { value: '1400' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    // only the MTU: the stale form value 'lan' is not written over the other session's description
    await waitFor(() => expect(patches).toEqual([{ 'host-w1l0': { mtu: 1400 } }]));
    expect(await within(drawer).findByText(/changed in the candidate since you opened the form/)).toBeInTheDocument();

    // "add" with the id of an existing sub-interface is refused before anything is sent
    fireEvent.click(within(drawer).getByRole('button', { name: 'Add sub-interface' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText(/^Sub-interface id/), { target: { value: '100' } });
    expect(await within(dialog).findByText('Sub-interface 100 already exists: edit it from the table')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await new Promise((r) => setTimeout(r, 300));
    expect(patches).toHaveLength(1);
  });

  it('a server error for a new sub-interface lands on its field, mapped with the typed id (review N5)', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withInterfaces(api);
    const msg = 'VLAN 100 is already used by host-w1l0.100';
    const patches: unknown[] = [];
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patches.push(body);
      return {
        status: 400,
        body: {
          type: 'https://vrx.dev/problems/validation', title: 'Validation failed', status: 400, detail: 'The candidate is not valid',
          errors: [{ pointer: '/interfaces/host-w1l0/subinterfaces/200/vlanId', message: msg, rule: 'interfaces.vlan-unique' }],
        },
      };
    });
    await signIn();
    render(app('/interfaces'));
    const grid = await screen.findByRole('grid', {}, { timeout: 15_000 });
    fireEvent.click(await within(grid).findByText('host-w1l0'));
    const drawer = await screen.findByRole('region', { name: 'Interface host-w1l0' });
    await within(drawer).findByText('host-w1l0.100');
    fireEvent.click(within(drawer).getByRole('button', { name: 'Add sub-interface' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText(/^Sub-interface id/), { target: { value: '200' } });
    fireEvent.change(within(dialog).getByLabelText(/^VLAN ID/), { target: { value: '100' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ 'host-w1l0': { subinterfaces: { '200': { vlanId: 100 } } } });
    // the pointer is mapped with the id typed in the dialog ("add" opens with an empty id): the message is the
    // VLAN field's own error, not an unmapped entry in an alert on top
    const vlan = within(dialog).getByLabelText(/^VLAN ID/);
    await waitFor(() => expect(vlan).toHaveAccessibleDescription(msg));
    expect(vlan).toHaveAttribute('aria-invalid', 'true');
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('a refetch of the candidate keeps what the user typed in the open form (review N4)', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withInterfaces(api);
    await signIn();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
    render(<App router={createTestRouter(['/interfaces'], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />);
    const grid = await screen.findByRole('grid', {}, { timeout: 15_000 });
    fireEvent.click(await within(grid).findByText('host-w1l0'));
    const drawer = await screen.findByRole('region', { name: 'Interface host-w1l0' });
    fireEvent.change(await within(drawer).findByLabelText(/^MTU/), { target: { value: '1400' } });
    // another session changes the candidate and the screen refetches it (poll, focus, a save elsewhere)
    api.on('GET /api/v1/config/candidate/interfaces', { body: { 'host-w1l0': { ...lanCfg, description: 'set elsewhere', mtu: 9000 } } });
    const fetches = () => api.calls.filter((c) => c.method === 'GET' && c.path === '/api/v1/config/candidate/interfaces').length;
    const before = fetches();
    await act(async () => {
      await queryClient.refetchQueries();
    });
    expect(fetches()).toBeGreaterThan(before);
    await new Promise((r) => setTimeout(r, 300));
    // the form was not remounted with the refetched value: the typed MTU and the value it opened with stay
    expect((within(drawer).getByLabelText(/^MTU/) as HTMLInputElement).value).toBe('1400');
    expect((within(drawer).getByLabelText(/^Description/) as HTMLInputElement).value).toBe('lan');
  });

  it('renders in Persian', { timeout: 30_000 }, async () => {
    const api = installFakeApi();
    withInterfaces(api);
    await signIn();
    render(app('/interfaces'));
    await screen.findByRole('grid', {}, { timeout: 15_000 });
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByRole('button', { name: 'افزودن اینترفیس' }, { timeout: 15_000 })).toBeInTheDocument();
    expect(screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toContain('اینترفیس‌ها');
  });
});
