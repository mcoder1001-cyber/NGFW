import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../App';
import i18n from '../../i18n';
import { UsersPage } from '../../pages/UsersPage';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../test-api';
import { StatusCell } from '../widgets/cells';
import { CollectionView } from './CollectionView';

/** The kit in jsdom against a scripted stand-in of the API (unit level, like P08's screen tests). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const LONG = { timeout: 60_000 };
const WAIT = { timeout: 15_000 };

function VrfsScreen() {
  return (
    <CollectionView
      spec={{ domain: 'vrfs', ns: 'vrfs' }}
      label="VRFs"
      itemLabel={(k) => `VRF ${k}`}
      keyHeader="Name"
      addLabel="Add VRF"
      columns={[{ field: 'description', headerName: 'Description', flex: 1 }]}
      status={{ header: 'Live', render: (r) => <StatusCell status={r.id === 'red' ? 'up' : undefined} /> }}
      drawerLive={(_row, id) => <div data-testid="live-panel">live of {id}</div>}
    />
  );
}

function UsersScreen() {
  return <CollectionView spec={{ domain: 'management', path: ['users'], ns: 'users' }} label="Users" itemLabel={(k) => `User ${k}`} keyHeader="Username" addLabel="Add user" />;
}

function app(element: React.ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createMemoryRouter([{ path: '/', element }])} streamUrl={STREAM} queryClient={queryClient} />;
}

const red = { id: 10, description: 'red one' };

function withVrfs(api: FakeApi, candidate: Record<string, unknown> = { red, blue: { id: 20 } }) {
  api.on('GET /api/v1/config/candidate/vrfs', { body: candidate });
  api.on('GET /api/v1/config/vrfs', { body: { red } });
}

function recordPatches(api: FakeApi, route: string): unknown[] {
  const patches: unknown[] = [];
  api.on(route, (_r, body) => {
    patches.push(body);
    return { body: { pointer: '/x', before: null, after: null } };
  });
  return patches;
}

async function openDrawer(name: string) {
  const grid = await screen.findByRole('grid', { name: /VRFs|Users/ }, WAIT);
  fireEvent.click(await within(grid).findByRole('button', { name: `Open ${name}` }, WAIT));
  return screen.findByRole('region', { name: new RegExp(`${name}$`) }, WAIT);
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('CollectionView — map collection (vrfs)', () => {
  it('lists the candidate with pending state and the live slot; the key cell is a keyboard button', LONG, async () => {
    const api = installFakeApi();
    withVrfs(api);
    await signIn();
    render(app(<VrfsScreen />));
    const grid = await screen.findByRole('grid', { name: 'VRFs' }, WAIT);
    const key = await within(grid).findByRole('button', { name: 'Open red' }, WAIT);
    expect(within(grid).getByRole('button', { name: 'Open blue' })).toBeInTheDocument();
    expect(within(grid).getByText('new')).toBeInTheDocument(); // blue is not in running
    expect(within(grid).getByText('red one')).toBeInTheDocument(); // plain member column
    expect(within(grid).getAllByRole('status').length).toBeGreaterThan(0); // live-status slot
    key.focus();
    await userEvent.setup().keyboard('{Enter}');
    const drawer = await screen.findByRole('region', { name: 'VRF red' }, WAIT);
    expect(within(drawer).getByTestId('live-panel')).toHaveTextContent('live of red');
    expect(await within(drawer).findByLabelText(/^Description/, {}, WAIT)).toHaveValue('red one');
  });

  it('saves only what changed against the value the form opened with, and warns when it changed elsewhere (N4)', LONG, async () => {
    const api = installFakeApi();
    withVrfs(api);
    const patches = recordPatches(api, 'PATCH /api/v1/config/vrfs');
    await signIn();
    render(app(<VrfsScreen />));
    const drawer = await openDrawer('red');
    const id = await within(drawer).findByLabelText(/^Table ID/, {}, WAIT);
    // another session changes the description after the form opened
    withVrfs(api, { red: { ...red, description: 'set elsewhere' }, blue: { id: 20 } });
    fireEvent.change(id, { target: { value: '11' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(patches).toEqual([{ red: { id: 11 } }]), WAIT);
    expect(await within(drawer).findByText(/changed in the candidate since you opened the form/, {}, WAIT)).toBeInTheDocument();
    expect(within(drawer).getByText('Saved to the candidate. Commit to apply it.')).toBeInTheDocument();
    fireEvent.click(within(drawer).getByRole('button', { name: 'Reload' }));
    await waitFor(() => expect(within(drawer).getByLabelText(/^Description/)).toHaveValue('set elsewhere'), WAIT);
  });

  it('adds an item under a new key checked by the schema and against the fresh candidate (N5)', LONG, async () => {
    const api = installFakeApi();
    withVrfs(api);
    const patches = recordPatches(api, 'PATCH /api/v1/config/vrfs');
    await signIn();
    render(app(<VrfsScreen />));
    await screen.findByRole('button', { name: 'Open red' }, WAIT); // candidate loaded: Add is enabled
    fireEvent.click(screen.getByRole('button', { name: 'Add VRF' }));
    const drawer = await screen.findByRole('region', { name: 'New entry' }, WAIT);
    const name = within(drawer).getByLabelText(/^Name/);
    fireEvent.change(name, { target: { value: '-bad' } });
    expect(within(drawer).getByText('Not a valid name here')).toBeInTheDocument();
    fireEvent.change(name, { target: { value: 'red' } });
    fireEvent.change(within(drawer).getByLabelText(/^Table ID/), { target: { value: '30' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    expect(await within(drawer).findByText('red already exists in the candidate', {}, WAIT)).toBeInTheDocument();
    expect(patches).toEqual([]);
    fireEvent.change(name, { target: { value: 'green' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(patches).toEqual([{ green: { id: 30 } }]), WAIT);
    // the drawer continues on the created item
    expect(await screen.findByRole('region', { name: 'VRF green' }, WAIT)).toBeInTheDocument();
  });

  it('maps server pointers onto the fields and removes after a confirmation', LONG, async () => {
    const api = installFakeApi();
    withVrfs(api);
    api.on('PATCH /api/v1/config/vrfs', {
      status: 400,
      body: { type: 'https://vrx.dev/problems/validation', title: 'Validation failed', status: 400, errors: [{ pointer: '/vrfs/red/id', message: 'table 11 is used by vrf blue' }] },
    });
    await signIn();
    render(app(<VrfsScreen />));
    const drawer = await openDrawer('red');
    fireEvent.change(await within(drawer).findByLabelText(/^Table ID/, {}, WAIT), { target: { value: '11' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    expect(await within(drawer).findByTestId('problem', {}, WAIT)).toHaveTextContent('table 11 is used by vrf blue');
    // the field itself carries the message (pointer made relative to the item)
    const field = within(drawer).getByLabelText(/^Table ID/);
    expect(field).toHaveAttribute('aria-invalid', 'true');

    const patches = recordPatches(api, 'PATCH /api/v1/config/vrfs');
    fireEvent.click(within(drawer).getByRole('button', { name: 'Remove from configuration' }));
    const dialog = await screen.findByRole('dialog', { name: 'Remove red?' }, WAIT);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove from candidate' }));
    await waitFor(() => expect(patches).toEqual([{ red: null }]), WAIT);
    await waitFor(() => expect(screen.queryByRole('region', { name: 'VRF red' })).toBeNull(), WAIT);
  });

  it('a read-only role sees the list; adding is disabled', LONG, async () => {
    const api = installFakeApi('readonly', 'viewer');
    withVrfs(api);
    await signIn();
    render(app(<VrfsScreen />));
    expect(await screen.findByRole('button', { name: 'Open red' }, WAIT)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add VRF' })).toBeDisabled();
  });
});

describe('CollectionView — list collection (management.users, itemKey username)', () => {
  const alice = { username: 'alice', role: 'operator', scope: '*', sshKeys: [], disabled: false, fullName: 'Alice' };
  const bob = { username: 'bob', role: 'readonly', scope: '*', sshKeys: [], disabled: false };

  it('rewrites the array with only the edited members applied to the item as it is now; a rename cannot take an existing key', LONG, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/management', { body: { users: [alice, bob] } });
    api.on('GET /api/v1/config/management', { body: { users: [alice, bob] } });
    const patches = recordPatches(api, 'PATCH /api/v1/config/management');
    await signIn();
    render(app(<UsersScreen />));
    const drawer = await openDrawer('alice');
    const username = await within(drawer).findByLabelText(/^Username/, {}, WAIT);
    // rename onto bob: refused locally, onto the field, nothing sent
    fireEvent.change(username, { target: { value: 'bob' } });
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    expect(await within(drawer).findAllByText('bob already exists in the candidate', {}, WAIT)).not.toHaveLength(0);
    expect(patches).toEqual([]);
    fireEvent.change(username, { target: { value: 'alice' } });
    // another session renames alice's full name meanwhile; we change her role only
    api.on('GET /api/v1/config/candidate/management', { body: { users: [{ ...alice, fullName: 'Alice Elsewhere' }, bob] } });
    fireEvent.mouseDown(within(drawer).getByRole('combobox', { name: /^Role/ }));
    fireEvent.click(await screen.findByRole('option', { name: 'Administrator' }, WAIT));
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(patches).toHaveLength(1), WAIT);
    expect(patches[0]).toEqual({ users: [{ ...alice, role: 'admin', fullName: 'Alice Elsewhere' }, bob] });
  });

  it('does not collide with the key Users and the pending-change bar use for the same domain (review H1)', LONG, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/management', { body: { users: [alice, bob] } });
    api.on('GET /api/v1/config/management', { body: { users: [alice, bob] } });
    await signIn();
    // the app's real default (staleTime: 5_000): a colliding key would be reused as-is, without a refetch
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 5_000 } } });
    const router = createMemoryRouter(
      [
        { path: '/kit', element: <UsersScreen /> },
        { path: '/users', element: <UsersPage /> },
      ],
      { initialEntries: ['/kit'] },
    );
    render(<App router={router} streamUrl={STREAM} queryClient={queryClient} />);
    // the kit has fetched and cached both the candidate and the running management node under its own key
    await screen.findByRole('button', { name: 'Open alice' }, WAIT);
    await act(async () => {
      await router.navigate('/users');
    });
    // before the fix (probe P5) this crashed with "running.map is not a function": Users read the kit's cached node
    // object ({ users: [...] }) from the shared key where it expects a plain ConfigUser[]
    expect(await screen.findByRole('table', { name: 'Users' }, WAIT)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit alice' })).toBeInTheDocument();
  });
});
