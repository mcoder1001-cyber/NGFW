import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

/** F-object-model screen in jsdom against a scripted stand-in of the API (the real stack runs in test/topology/object-model). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const running = {
  tags: { prod: { color: '#1e88e5' } },
  addresses: {
    web1: { type: 'host', address: '192.0.2.10', tags: ['prod'] },
    cdn: { type: 'fqdn', fqdn: 'cdn.w3.test', tags: [] },
    lan: { type: 'network', prefix: '10.3.1.0/24', tags: [] },
  },
  addressGroups: { 'web-servers': { members: ['web1'], tags: ['prod'] } },
  services: {},
  serviceGroups: {},
  schedules: {},
  zones: {},
};
// the candidate adds one member: the group is marked pending
const candidate = { ...running, addressGroups: { 'web-servers': { members: ['web1', 'lan'], tags: ['prod'] } } };

function withObjects(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/objects', { body: candidate });
  api.on('GET /api/v1/config/objects', { body: running });
  api.on('GET /api/v1/state/objects/fqdn', {
    body: {
      retrievedAt: '2026-09-24T12:00:00.000Z',
      items: [
        { name: 'cdn', fqdn: 'cdn.w3.test', addresses: ['192.0.2.53', '2001:db8::53'], lastResolved: new Date(Date.now() - 60_000).toISOString(), nextRefresh: null, error: 'lookup cdn.w3.test. on 127.0.0.1:53: timeout', failures: 1 },
      ],
    },
  });
  api.on('GET /api/v1/state/objects/usage', {
    body: {
      name: 'web-servers',
      source: 'candidate',
      definedAs: ['addressGroups'],
      usedBy: [{ pointer: '/acl/lists/web-in/rules/0/destination/name', container: '/acl/lists/web-in/rules/0', domain: 'acl', kind: 'acl-rule-destination' }],
    },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('objects screen', () => {
  it('lists addresses with tag chips and the FQDN resolution column (last-good kept)', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withObjects(api);
    await signIn();
    render(app('/firewall/objects'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Objects' }, { timeout: 15_000 })).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'Addresses' });
    const cdn = (await within(table).findByText('cdn')).closest('tr')!;
    expect(within(cdn).getByText('FQDN')).toBeInTheDocument();
    expect(await within(cdn).findByText('192.0.2.53 2001:db8::53')).toBeInTheDocument();
    expect(within(cdn).getByText('Last good answer kept')).toBeInTheDocument();
    const web1 = within(table).getByText('web1').closest('tr')!;
    expect(within(web1).getByText('prod')).toBeInTheDocument();
    expect(within(web1).queryByText('pending')).toBeNull();
  });

  it('marks a pending group, opens where-used, and saves an edit as a merge patch of /objects', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withObjects(api);
    let patched: unknown;
    api.on('PATCH /api/v1/config/objects', (_r, body) => {
      patched = body;
      return { body: { pointer: '/objects', before: null, after: null } };
    });
    await signIn();
    render(app('/firewall/objects?tab=addressGroups'));
    const table = await screen.findByRole('table', { name: 'Address groups' }, { timeout: 15_000 });
    const row = (await within(table).findByText('web-servers')).closest('tr')!;
    expect(within(row).getByText('pending')).toBeInTheDocument();

    fireEvent.click(within(row).getByRole('button', { name: 'Where is web-servers used' }));
    const drawer = await screen.findByRole('region', { name: 'Where is web-servers used?' });
    expect(await within(drawer).findByText('ACL rule destination')).toBeInTheDocument();
    expect(within(drawer).getByText('/acl/lists/web-in/rules/0/destination/name')).toBeInTheDocument();
    expect(api.calls.find((c) => c.path === '/api/v1/state/objects/usage')?.search).toBe('?name=web-servers&source=candidate');
    fireEvent.click(within(drawer).getByRole('button', { name: 'Close' }));

    fireEvent.click(within(row).getByText('web-servers'));
    const dialog = await screen.findByRole('dialog', { name: 'Edit address group web-servers' });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Description' }), { target: { value: 'DMZ web' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(patched).toEqual({ addressGroups: { 'web-servers': { description: 'DMZ web' } } }));
  });

  it('renders the page right to left in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withObjects(api);
    await signIn();
    render(app('/firewall/objects?tab=tags'));
    await screen.findByRole('table', { name: 'Tags' }, { timeout: 15_000 });
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByRole('heading', { level: 2, name: 'اشیا' }, { timeout: 15_000 })).toBeInTheDocument();
    expect(await screen.findByRole('table', { name: 'برچسب‌ها' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'برچسب‌ها', selected: true })).toBeInTheDocument();
  });
});
