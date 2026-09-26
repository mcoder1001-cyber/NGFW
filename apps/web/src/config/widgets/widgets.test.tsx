import { createFormatters, VrxThemeProvider } from '@ngfw/ui-kit';
import type { ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import '../../i18n';
import { CounterText, IdText, LiveChip, RowStateChip, StatusCell } from './cells';
import { formatCounter, sumCounters } from './format';
import { KeyButton } from './KeyButton';
import { DEV_PREVIEWS } from '../../pages/dev/previews';
import { KitPreview } from './KitPreview';
import { LocalDataGrid } from './LocalDataGrid';
import { matchesFilter, pageRows } from './paging';

function wrap(children: ReactNode, qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return (
    <QueryClientProvider client={qc}>
      <VrxThemeProvider mode="light" lang="en" dir="ltr">
        {children}
      </VrxThemeProvider>
    </QueryClientProvider>
  );
}

const REQ: ServerPageRequest = { page: 0, pageSize: 25, sort: [], filter: [], filterLogic: 'and', quickFilter: [] };

describe('pageRows / matchesFilter', () => {
  const rows = Array.from({ length: 30 }, (_x, i) => ({ id: i, name: `if${i}`, mtu: 1000 + (i % 5) * 100, vrf: i % 3 === 0 ? 'red' : '' }));

  it('pages, sorts numerically and by text, and filters with the grid operators', () => {
    expect(pageRows(rows, REQ)).toMatchObject({ total: 30 });
    expect(pageRows(rows, { ...REQ, page: 1 }).rows.map((r) => r.id)).toEqual([25, 26, 27, 28, 29]);
    expect(pageRows(rows, { ...REQ, sort: [{ field: 'name', dir: 'asc' }] }).rows.slice(0, 3).map((r) => r.name)).toEqual(['if0', 'if1', 'if2']);
    expect(pageRows(rows, { ...REQ, sort: [{ field: 'mtu', dir: 'desc' }, { field: 'id', dir: 'asc' }] }).rows[0]).toMatchObject({ id: 4, mtu: 1400 });
    expect(pageRows(rows, { ...REQ, filter: [{ field: 'mtu', operator: '>=', value: 1300 }] }).total).toBe(12);
    expect(pageRows(rows, { ...REQ, filter: [{ field: 'vrf', operator: 'isEmpty' }] }).total).toBe(20);
    const either = { ...REQ, filterLogic: 'or' as const, filter: [{ field: 'name', operator: 'equals', value: 'IF1' }, { field: 'name', operator: 'equals', value: 'if2' }] };
    expect(pageRows(rows, either).rows.map((r) => r.id)).toEqual([1, 2]);
    expect(pageRows(rows, { ...REQ, quickFilter: ['red', 'if2'] }).rows.map((r) => r.id)).toEqual([21, 24, 27]);
  });

  it('operators', () => {
    const f = (operator: string, value?: unknown) => ({ field: 'x', operator, value });
    expect(matchesFilter('Host-W1', f('contains', 'w1'))).toBe(true);
    expect(matchesFilter('abc', f('doesNotContain', 'b'))).toBe(false);
    expect(matchesFilter('abc', f('startsWith', 'AB'))).toBe(true);
    expect(matchesFilter('abc', f('endsWith', 'bc'))).toBe(true);
    expect(matchesFilter(5, f('=', '5'))).toBe(true);
    expect(matchesFilter(5, f('!=', 5))).toBe(false);
    expect(matchesFilter(5, f('<', 6))).toBe(true);
    expect(matchesFilter('b', f('isAnyOf', ['a', 'B']))).toBe(true);
    expect(matchesFilter('', f('isNotEmpty'))).toBe(false);
  });
});

describe('counters (64-bit decimal strings, D-039)', () => {
  it('formats exactly beyond 2^53, with grouping, Persian digits on request', () => {
    const en = createFormatters({ locale: 'en' });
    expect(formatCounter(en, '18446744073709551615')).toBe('18,446,744,073,709,551,615');
    expect(formatCounter(en, 1234567)).toBe('1,234,567');
    expect(formatCounter(en, '42')).toBe('42');
    expect(formatCounter(en, null)).toBe('');
    expect(formatCounter(en, 'not a number')).toBe('');
    const fa = createFormatters({ locale: 'fa', persianDigits: true });
    expect(formatCounter(fa, '18446744073709551615')).toMatch(/^۱۸.۴۴۶.۷۴۴/);
    expect(sumCounters('18446744073709551615', '1', undefined, 2)).toBe(18446744073709551618n);
  });
});

describe('cells', () => {
  it('status, missing status, counter, row state and live chips', () => {
    render(
      wrap(
        <>
          <StatusCell status="up" />
          <StatusCell status={undefined} />
          <CounterText value="18446744073709551615" />
          <RowStateChip state="removed" />
          <RowStateChip state={undefined} />
          <LiveChip status="reconnecting" />
          <IdText>GigabitEthernet0/8/0</IdText>
        </>,
      ),
    );
    expect(screen.getByRole('status')).toHaveTextContent('Up');
    expect(screen.getByText('not in the data plane')).toBeInTheDocument();
    expect(screen.getByText('18,446,744,073,709,551,615')).toBeInTheDocument();
    expect(screen.getByText('to be removed')).toBeInTheDocument();
    expect(screen.getByText('Live: Reconnecting…')).toBeInTheDocument();
    expect(screen.getByText('GigabitEthernet0/8/0').closest('bdi')).toHaveAttribute('dir', 'ltr');
  });
});

describe('KeyButton (A11Y-1)', () => {
  it('is a real button: named, focusable, activated by Enter and Space, click does not reach the row', async () => {
    const user = userEvent.setup();
    const open = vi.fn();
    const row = vi.fn();
    render(
      wrap(
        <div onClick={row}>
          <KeyButton aria-label="Open host-w1l0" onClick={open}>
            host-w1l0
          </KeyButton>
        </div>,
      ),
    );
    const b = screen.getByRole('button', { name: 'Open host-w1l0' });
    expect(b.tagName).toBe('BUTTON');
    expect(b).toHaveAttribute('type', 'button');
    await user.tab();
    expect(b).toHaveFocus();
    await user.keyboard('{Enter}');
    await user.keyboard(' ');
    expect(open).toHaveBeenCalledTimes(2);
    fireEvent.click(b);
    expect(open).toHaveBeenCalledTimes(3);
    expect(row).not.toHaveBeenCalled();
  });

  it('takes focus when its grid cell does (hasFocus)', () => {
    const { rerender } = render(wrap(<KeyButton onClick={() => undefined} tabIndex={-1} hasFocus={false}>a</KeyButton>));
    expect(screen.getByRole('button')).not.toHaveFocus();
    rerender(wrap(<KeyButton onClick={() => undefined} tabIndex={0} hasFocus>a</KeyButton>));
    expect(screen.getByRole('button')).toHaveFocus();
  });
});

describe('LocalDataGrid', () => {
  it('shows the given rows through the ServerDataGrid protocol and re-pages when the rows change', async () => {
    const cols = [{ field: 'name', headerName: 'Name', flex: 1 }];
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { rerender } = render(wrap(<LocalDataGrid aria-label="t" gridKey={['test', 'grid']} rows={[{ id: 1, name: 'alpha' }]} columns={cols} />, qc));
    const grid = await screen.findByRole('grid', { name: 't' });
    expect(await within(grid).findByText('alpha')).toBeInTheDocument();
    rerender(wrap(<LocalDataGrid aria-label="t" gridKey={['test', 'grid']} rows={[{ id: 2, name: 'beta' }]} columns={cols} />, qc));
    expect(await within(grid).findByText('beta')).toBeInTheDocument();
    expect(within(grid).queryByText('alpha')).toBeNull();
  });
});

describe('dev previews', () => {
  it('the kit preview renders in Persian (RTL) and its key buttons open the drawer', async () => {
    const i18n = (await import('../../i18n')).default;
    await i18n.changeLanguage('fa');
    try {
      render(
        <QueryClientProvider client={new QueryClient()}>
          <VrxThemeProvider mode="light" lang="fa" dir="rtl">
            <KitPreview />
          </VrxThemeProvider>
        </QueryClientProvider>,
      );
      expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('پیش‌نمایش');
      const grid = await screen.findByRole('grid');
      fireEvent.click(await within(grid).findByRole('button', { name: 'باز کردن loop100' }));
      expect(await screen.findByRole('region', { name: 'مورد پیش‌نمایش loop100' })).toBeInTheDocument();
    } finally {
      await i18n.changeLanguage('en');
    }
  });

  it('previews exist in dev/test builds and each route loads a component', async () => {
    expect(DEV_PREVIEWS.map((p) => p.path)).toEqual(['/dev/config-kit', '/dev/secrets']);
    for (const p of DEV_PREVIEWS) {
      expect(p.route.path).toBe(p.path.slice(1));
      const lazy = p.route.lazy as () => Promise<{ Component?: unknown }>;
      expect(typeof (await lazy()).Component).toBe('function');
    }
  });
});
