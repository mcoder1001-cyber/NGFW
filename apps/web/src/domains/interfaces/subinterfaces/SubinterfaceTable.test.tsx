import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { VrxThemeProvider } from '@ngfw/ui-kit';
import { afterEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import type { InterfaceItem, SubinterfaceConfig } from '../model';
import { SubinterfaceTable, type SubinterfaceTableProps } from './SubinterfaceTable';

/** F-vlan-qinq: the drawer's sub-interface table in jsdom (the real stack runs in test/topology/vlan-qinq). */
const W = 'host-w5w0';
const sub = (c: Partial<SubinterfaceConfig>) =>
  ({
    enabled: true,
    ipv4: [],
    ipv6: [],
    vrf: 'default',
    dot1ad: false,
    ...c,
  }) as SubinterfaceConfig;
const SUBS: [string, SubinterfaceConfig][] = [
  ['100', sub({ vlanId: 100, ipv4: ['10.5.100.1/24'] })],
  ['200', sub({ vlanId: 200, innerVlanId: 100, dot1ad: true, ipv4: ['10.5.200.1/24'] })],
  ['300', sub({ vlanId: 300, innerVlanId: 30, enabled: false })],
];
const live = (
  name: string,
  vlanId: number,
  innerVlanId: number,
  extra: Record<string, unknown> = {},
) => ({
  name,
  vppName: name,
  swIfIndex: 9,
  type: 'sub-interface',
  adminUp: true,
  linkUp: true,
  mtu: 0,
  linkMtu: 9000,
  mac: '02:fe:00:00:00:05',
  ipv4: [],
  ipv6: [],
  vrf: 'default',
  tableId: 0,
  parent: W,
  vlanId,
  innerVlanId,
  managed: true,
  linkSpeedKbps: '0',
  rxMode: '',
  description: '',
  ...extra,
});
const row = (name: string, state: unknown, config: unknown) =>
  ({
    name,
    kind: 'subinterface',
    parent: W,
    state,
    config,
    running: config,
    counters: null,
    hasPendingChange: false,
  }) as unknown as InterfaceItem;
const ITEMS = [
  row(`${W}.100`, live(`${W}.100`, 100, 0, { linkUp: false }), { vlanId: 100, dot1ad: false }),
  // VPP still has the old inner tag 101 (a pending tag change): the live stack is shown under the configured one
  row(`${W}.200`, live(`${W}.200`, 200, 101), { vlanId: 200, innerVlanId: 101, dot1ad: true }),
];

function renderTable(p: Partial<SubinterfaceTableProps> = {}, dir: 'ltr' | 'rtl' = 'ltr') {
  const props: SubinterfaceTableProps = {
    parent: W,
    subs: SUBS,
    items: ITEMS,
    readOnly: false,
    canAdd: true,
    busy: false,
    error: null,
    onAdd: vi.fn(),
    onEdit: vi.fn(),
    onRemove: vi.fn(),
    ...p,
  };
  render(
    <VrxThemeProvider mode="light" lang={dir === 'rtl' ? 'fa' : 'en'} dir={dir}>
      <SubinterfaceTable {...props} />
    </VrxThemeProvider>,
  );
  return props;
}

const cells = (table: HTMLElement, name: string) =>
  within(within(table).getByText(name).closest('tr')!)
    .getAllByRole('cell')
    .map((c) => c.textContent);

afterEach(async () => {
  await act(async () => {
    await i18n.changeLanguage('en');
  });
});

describe('SubinterfaceTable', () => {
  it('shows the tag stack (Encapsulation, Inner VLAN), live admin/link state and addresses per sub-interface', () => {
    renderTable();
    const table = screen.getByRole('table', { name: `Sub-interfaces of ${W}` });
    expect(
      within(table)
        .getAllByRole('columnheader')
        .map((h) => h.textContent),
    ).toEqual([
      'Sub-interface',
      'Encapsulation',
      'Inner VLAN',
      'Admin',
      'Link',
      'Addresses',
      'Actions',
    ]);
    expect(cells(table, `${W}.100`)).toEqual([
      `${W}.100`,
      'dot1q 100',
      '—',
      'Up',
      'Down',
      '10.5.100.1/24',
      '',
    ]);
    expect(cells(table, `${W}.200`)).toEqual([
      `${W}.200`,
      'dot1ad 200 · dot1q 100in VPP: dot1ad 200 · dot1q 101',
      '100',
      'Up',
      'Up',
      '10.5.200.1/24',
      '',
    ]);
    // configured but not (yet) in VPP
    expect(cells(table, `${W}.300`)).toEqual([
      `${W}.300`,
      'dot1q 300 · dot1q 30',
      '30',
      'not in VPP',
      '—',
      '',
      '',
    ]);
    expect(within(table).getByText('dot1ad 200 · dot1q 100')).toHaveAttribute(
      'title',
      'Tag stack, outer tag first: IEEE 802.1ad service tag (TPID 0x88a8) · IEEE 802.1Q tag (TPID 0x8100)',
    );
    expect(screen.getByText(/match their tag stack exactly \(exact-match\)/)).toBeInTheDocument();
  });

  it('a row opens the editor, the bin removes without opening it, add is a separate action', () => {
    const p = renderTable();
    const table = screen.getByRole('table');
    fireEvent.click(within(table).getByText(`${W}.200`));
    expect(p.onEdit).toHaveBeenCalledWith('200', SUBS[1]![1]);
    fireEvent.click(screen.getByRole('button', { name: 'Remove sub-interface 300' }));
    expect(p.onRemove).toHaveBeenCalledWith('300');
    expect(p.onEdit).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', { name: 'Add sub-interface' }));
    expect(p.onAdd).toHaveBeenCalledTimes(1);
  });

  it('read-only: rows do not open, add and remove are disabled; no parent in the candidate: add is disabled', () => {
    const p = renderTable({ readOnly: true });
    fireEvent.click(within(screen.getByRole('table')).getByText(`${W}.100`));
    expect(p.onEdit).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Add sub-interface' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Remove sub-interface 100' })).toBeDisabled();
    render(
      <VrxThemeProvider mode="light" lang="en" dir="ltr">
        <SubinterfaceTable {...p} readOnly={false} canAdd={false} subs={[]} />
      </VrxThemeProvider>,
    );
    expect(screen.getAllByRole('button', { name: 'Add sub-interface' })[1]).toBeDisabled();
    expect(screen.getByText('No sub-interfaces.')).toBeInTheDocument();
  });

  it('renders in Persian; the tag stack stays left-to-right', async () => {
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    renderTable({}, 'rtl');
    const table = screen.getByRole('table', { name: `زیراینترفیس‌های ${W}` });
    expect(
      within(table)
        .getAllByRole('columnheader')
        .map((h) => h.textContent),
    ).toEqual(['زیراینترفیس', 'کپسوله‌سازی', 'VLAN داخلی', 'مدیریتی', 'لینک', 'آدرس‌ها', 'عملیات']);
    const encap = within(table).getByText('dot1ad 200 · dot1q 100');
    expect(encap).toHaveAttribute('dir', 'ltr');
    expect(within(table).getByText(`${W}.200`)).toHaveAttribute('dir', 'ltr');
    expect(screen.getByRole('button', { name: 'افزودن زیراینترفیس' })).toBeInTheDocument();
  });
});
