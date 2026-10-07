import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { NgfwThemeProvider } from '@ngfw/ui-kit';
import '../../../i18n';
import { BackupRestorePage } from './BackupRestorePage';
import { UpgradePage } from './UpgradePage';

const mocks = vi.hoisted(() => ({
  role: 'admin',
  json: vi.fn(),
  download: vi.fn(),
  request: vi.fn(),
}));
vi.mock('../../../auth/AuthProvider', () => ({ usePermissions: () => ({ role: mocks.role }) }));
vi.mock('./transport', () => ({
  json: mocks.json,
  download: mocks.download,
  request: mocks.request,
  archiveBase64: async () => 'AA==',
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mocks.role = 'admin';
});
function mount(page: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <NgfwThemeProvider mode="light" lang="en" dir="ltr">
      <QueryClientProvider client={client}>{page}</QueryClientProvider>
    </NgfwThemeProvider>,
  );
}
describe('backup and upgrade workflows', () => {
  it('does not fetch privileged endpoints for an operator', () => {
    mocks.role = 'operator';
    mount(<BackupRestorePage />);
    expect(
      screen.getByText('Only administrators can manage backups and upgrades.'),
    ).toBeInTheDocument();
    expect(mocks.json).not.toHaveBeenCalled();
  });
  it('restores to candidate, clears the passphrase and shows diff without committing', async () => {
    mocks.json.mockImplementation(async (path: string) => {
      if (path === '/config-templates') return { items: {} };
      if (path === '/state/backup') return { runs: [] };
      if (path === '/actions/restore')
        return { staged: true, diff: { baseRevision: 1, changes: [] } };
      return { enabled: false, schedule: '0 2 * * *', retention: 7, revisions: 100 };
    });
    mount(<BackupRestorePage />);
    fireEvent.change(screen.getByLabelText('Archive passphrase'), {
      target: { value: 'test-only-phrase' },
    });
    fireEvent.change(screen.getByLabelText('Backup archive'), {
      target: { files: [new File(['backup'], 'test.ngfwbackup')] },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Upload and preview restore' }));
    await waitFor(() =>
      expect(mocks.json).toHaveBeenCalledWith('/actions/restore', {
        archive: 'AA==',
        passphrase: 'test-only-phrase',
      }),
    );
    expect(screen.getByLabelText('Archive passphrase')).toHaveValue('');
    await screen.findByText(
      'Review these changes, then use the pending changes bar to commit or discard.',
    );
    expect(mocks.json.mock.calls.some(([path]) => String(path).includes('/config/commit'))).toBe(
      false,
    );
  });
  it('reads candidate schedule but replaces the management node through the config PUT route', async () => {
    mocks.json.mockImplementation(async (path: string) => {
      if (path === '/config-templates') return { items: {} };
      if (path === '/state/backup') return { runs: [] };
      return {
        enabled: false,
        schedule: '0 2 * * *',
        retention: 7,
        revisions: 100,
        target: { type: 'local', path: '/data/backups' },
      };
    });
    mount(<BackupRestorePage />);
    await waitFor(() =>
      expect(mocks.json).toHaveBeenCalledWith('/config/candidate/management/backup'),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Stage schedule' }));
    await waitFor(() =>
      expect(mocks.json).toHaveBeenCalledWith(
        '/config/management/backup',
        expect.objectContaining({ enabled: false, retention: 7 }),
        'PUT',
      ),
    );
    expect(
      mocks.json.mock.calls.some(
        ([path, , method]) => path === '/config/candidate/management/backup' && method === 'PUT',
      ),
    ).toBe(false);
  });
  it('enables confirm only after the trial slot is actually active', async () => {
    mocks.json.mockResolvedValue({
      lines: [
        JSON.stringify({
          active_slot: 'A',
          default_slot: 'A',
          versions: { A: '1.0.0', B: '1.1.0' },
          pending_slot: 'B',
          staged_slot: 'B',
          confirmed: false,
        }),
      ],
      done: { exitCode: 0, summary: '', stats: {} },
    });
    mount(<UpgradePage />);
    await screen.findByText('Trial boot awaits confirmation');
    expect(screen.getByRole('button', { name: 'Confirm current slot' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Roll back' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(mocks.json).toHaveBeenCalledTimes(1);
  });
});
