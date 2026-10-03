import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />
  );
}

const modules = [
  { name: 'ngfw-system', namespace: 'urn:ngfw:system', revision: '2026-01-01' },
  { name: 'ngfw-interfaces', namespace: 'urn:ngfw:interfaces', revision: '2026-01-01' },
];

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('RESTCONF / YANG page', () => {
  it('lists the generated modules and downloads one', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/system/yang', { body: { modules } });
    let fetched = '';
    api.on('GET /api/v1/system/yang/ngfw-system', () => {
      fetched = 'ngfw-system';
      return { body: { name: 'ngfw-system', yang: 'module ngfw-system { }' } };
    });
    // jsdom lacks URL.createObjectURL / a real anchor download — stub just those, keep `new URL` working
    const origCreate = URL.createObjectURL;
    const origRevoke = URL.revokeObjectURL;
    URL.createObjectURL = () => 'blob:x';
    URL.revokeObjectURL = () => {};
    const clicked = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    await signIn();
    render(app('/system/restconf'));
    expect(
      await screen.findByRole('heading', { level: 2, name: 'RESTCONF / YANG' }, { timeout: 15_000 }),
    ).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'YANG modules' });
    expect(within(table).getByText('ngfw-system')).toBeInTheDocument();
    expect(within(table).getByText('urn:ngfw:interfaces')).toBeInTheDocument();

    const row = within(table).getByText('ngfw-system').closest('tr')!;
    fireEvent.click(within(row).getByRole('button', { name: 'Download' }));
    await waitFor(() => expect(fetched).toBe('ngfw-system'));
    expect(clicked).toHaveBeenCalled();
    clicked.mockRestore();
    URL.createObjectURL = origCreate;
    URL.revokeObjectURL = origRevoke;
  });

  it('shows the endpoint and renders in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/system/yang', { body: { modules: [] } });
    await signIn();
    render(app('/system/restconf'));
    expect(await screen.findByText('/restconf', {}, { timeout: 15_000 })).toBeInTheDocument();
    expect(await screen.findByText('No modules.')).toBeInTheDocument();
    await i18n.changeLanguage('fa');
    expect(await screen.findByText('ماژولی وجود ندارد.')).toBeInTheDocument();
  });
});
