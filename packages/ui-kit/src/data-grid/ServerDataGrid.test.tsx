import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../test-utils.js';
import { ServerDataGrid, type ServerPageRequest } from './ServerDataGrid.js';

interface Row {
  id: number;
  name: string;
}

const columns = [
  { field: 'id', headerName: 'ID', width: 80 },
  { field: 'name', headerName: 'Name', width: 200 },
];

describe('<ServerDataGrid>', () => {
  it('asks the server for page 0 and renders the returned rows only', async () => {
    const fetchPage = vi.fn(async (req: ServerPageRequest) => ({
      rows: Array.from({ length: req.pageSize }, (_x, i) => ({ id: i, name: `row-${i}` })),
      total: 1_000_000,
    }));
    renderWithProviders(
      <div style={{ height: 400, width: 600 }}>
        <ServerDataGrid<Row> columns={columns} queryKey={['test', 'rows']} fetchPage={fetchPage} initialPageSize={5} disableVirtualization />
      </div>,
    );
    expect(await screen.findByText('row-0')).toBeInTheDocument();
    expect(screen.getByText('row-4')).toBeInTheDocument();
    expect(screen.queryByText('row-5')).toBeNull();
    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(fetchPage.mock.calls[0]![0]).toEqual({ page: 0, pageSize: 5, sort: [], filter: [], filterLogic: 'and', quickFilter: [] });
    expect(screen.getByText(/1,000,000|1.000.000|1 000 000/)).toBeInTheDocument();
  });

  it('shows the translated error state with a retry button', async () => {
    const fetchPage = vi.fn(async () => {
      throw new Error('boom');
    });
    renderWithProviders(
      <div style={{ height: 400, width: 600 }}>
        <ServerDataGrid<Row> columns={columns} queryKey={['test', 'error']} fetchPage={fetchPage} disableVirtualization />
      </div>,
    );
    expect(await screen.findByText('Could not load rows')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());
  });

  it('shows the empty state', async () => {
    const fetchPage = vi.fn(async () => ({ rows: [] as Row[], total: 0 }));
    renderWithProviders(
      <div style={{ height: 400, width: 600 }}>
        <ServerDataGrid<Row> columns={columns} queryKey={['test', 'empty']} fetchPage={fetchPage} disableVirtualization />
      </div>,
    );
    expect(await screen.findByText('No rows')).toBeInTheDocument();
  });

  it('does not flash the loading overlay on background polls, and flags a failed refetch over stale rows (review L5)', async () => {
    let fail = false;
    let calls = 0;
    const fetchPage = vi.fn(async () => {
      calls += 1;
      if (fail) throw new Error('poll failed');
      return { rows: [{ id: 1, name: `poll-${calls}` }], total: 1 };
    });
    renderWithProviders(
      <div style={{ height: 400, width: 600 }}>
        <ServerDataGrid<Row> columns={columns} queryKey={['test', 'poll']} fetchPage={fetchPage} refetchInterval={100} disableVirtualization />
      </div>,
    );
    expect(await screen.findByText('poll-1')).toBeInTheDocument();
    // background refetches replace the row without ever showing the grid's loading overlay
    await screen.findByText(/poll-[2-9]/);
    expect(document.querySelector('.MuiDataGrid-overlay .MuiCircularProgress-root, .MuiDataGrid-loadingOverlay')).toBeNull();
    fail = true;
    expect(await screen.findByText('Refreshing failed; showing the last loaded rows.')).toBeInTheDocument();
    expect(screen.getByText(/poll-\d/)).toBeInTheDocument(); // stale rows still visible
  });
});
