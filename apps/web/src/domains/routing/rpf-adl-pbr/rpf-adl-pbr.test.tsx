import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import {
  adlText,
  pathText,
  policyViews,
  schemas,
  securityPatch,
  statusColour,
  urpfText,
  withChoices,
  type PbrState,
} from './model';

/** F-rpf-adl-pbr screens in jsdom against a scripted stand-in of the API (unit level; the real stack is in the status report). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const tt = (k: string, o?: Record<string, unknown>) => (o ? `${k}(${JSON.stringify(o)})` : k);

function app(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App
      router={createTestRouter([path], { devRoutes: false })}
      streamUrl={STREAM}
      queryClient={queryClient}
    />
  );
}

const policy = {
  acl: 'lan-b',
  priority: 10,
  paths: [{ address: '10.1.2.254', interface: 'loop102', vrf: 'default', weight: 1 }],
};
const state: PbrState = {
  retrievedAt: '2026-09-24T12:00:00.000Z',
  pendingChange: false,
  policies: [
    {
      name: 'via-l2',
      acl: 'lan-b',
      priority: 10,
      paths: policy.paths,
      status: 'in-sync',
      attachments: 1,
    },
  ],
  attachments: {
    page: 1,
    pageSize: 1000,
    total: 1,
    items: [{ policy: 'via-l2', interface: 'loop101', family: 'ipv4', status: 'in-sync' }],
  },
  counters: { available: false, reason: 'x' },
};

function withConfig(api: FakeApi) {
  const routing = {
    static: [],
    pbr: {
      policies: { 'via-l2': policy },
      attachments: [{ policy: 'via-l2', interface: 'loop101', family: 'ipv4' }],
    },
  };
  const interfaces = {
    loop101: {
      enabled: true,
      ipv4: ['10.1.1.1/24'],
      urpf: { ipv4: 'strict', direction: 'rx' },
      adl: { ipv4: true, ipv6: false, allowVrf: 'allow', defaultAllow: true },
    },
    loop102: { enabled: true, ipv4: ['10.1.2.1/24'] },
  };
  api.on('GET /api/v1/config/candidate/routing', { body: routing });
  api.on('GET /api/v1/config/routing', { body: routing });
  api.on('GET /api/v1/config/candidate/acl', { body: { lists: { 'lan-b': { rules: [] } } } });
  api.on('GET /api/v1/config/candidate/vrfs', { body: { allow: { id: 1002 } } });
  api.on('GET /api/v1/config/candidate/interfaces', { body: interfaces });
  api.on('GET /api/v1/config/interfaces', { body: interfaces });
  api.on('GET /api/v1/config/candidate/services', {
    body: { autoSdl: { enabled: true, threshold: 5, removeTimeoutSec: 300 } },
  });
  api.on('GET /api/v1/state/pbr', { body: state });
  api.on('PATCH /api/v1/config/routing', { body: { pointer: '/routing' } });
  api.on('PATCH /api/v1/config/interfaces', { body: { pointer: '/interfaces' } });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('rpf-adl-pbr model', () => {
  it('withChoices sets enums on nested properties and drops the free-text constraints', () => {
    const s = withChoices(schemas.policy(), {
      acl: ['a', 'b'],
      'paths.items.vrf': ['default', 'red'],
    }) as {
      properties: {
        acl: { enum: string[]; pattern?: string };
        paths: { items: { properties: { vrf: { enum: string[] } } } };
      };
    };
    expect(s.properties.acl.enum).toEqual(['a', 'b']);
    expect(s.properties.acl.pattern).toBeUndefined();
    expect(s.properties.paths.items.properties.vrf.enum).toEqual(['default', 'red']);
    // the source schema is untouched
    expect(
      (schemas.policy() as { properties: { acl: { enum?: unknown } } }).properties.acl.enum,
    ).toBeUndefined();
  });

  it('texts: paths, uRPF, ADL, status colours', () => {
    expect(pathText({ address: '10.0.0.1', interface: 'loop1', weight: 2 }, tt)).toBe(
      '10.0.0.1 path.via({"interface":"loop1"}) ×2',
    );
    expect(pathText({ vrf: 'red' }, tt)).toBe('path.lookup({"vrf":"red"})');
    expect(pathText({ address: '10.0.0.1', vrf: 'red' }, tt)).toBe(
      '10.0.0.1 path.inVrf({"vrf":"red"})',
    );
    expect(urpfText(undefined, tt)).toBe('off');
    expect(urpfText({ ipv4: 'strict', direction: 'tx' }, tt)).toBe(
      'IPv4 mode.strict (direction.tx)',
    );
    expect(adlText({ ipv4: false, ipv6: false, defaultAllow: true }, tt)).toBe('off');
    expect(adlText({ ipv4: true, ipv6: true, allowVrf: 'allow', defaultAllow: true }, tt)).toBe(
      'adl.summary({"families":"IPv4 · IPv6","vrf":"allow"})',
    );
    expect(
      ['in-sync', 'drift', 'missing', 'unmanaged'].map((s) => statusColour(s as 'in-sync')),
    ).toEqual(['up', 'degraded', 'down', 'degraded']);
  });

  it('securityPatch: an object that checks nothing is removed (absent = off)', () => {
    expect(
      securityPatch({
        urpf: { direction: 'rx' },
        adl: { ipv4: false, ipv6: false, defaultAllow: true },
      }),
    ).toEqual({ urpf: null, adl: null });
    expect(
      securityPatch({
        urpf: { ipv6: 'loose', direction: 'rx' },
        adl: { ipv4: true, ipv6: false, allowVrf: 'a', defaultAllow: true },
      }),
    ).toEqual({
      urpf: { ipv6: 'loose', direction: 'rx' },
      adl: { ipv4: true, ipv6: false, allowVrf: 'a', defaultAllow: true },
    });
  });

  it('policyViews: candidate vs running marks new/changed/removed, live rows are joined by name', () => {
    const views = policyViews({ policies: { a: policy, c: { ...policy, priority: 20 } } }, state, {
      policies: { b: policy, c: policy },
    });
    expect(views.map((v) => [v.name, v.pending, v.live?.status])).toEqual([
      ['a', 'new', undefined],
      ['b', 'removed', undefined],
      ['c', 'changed', undefined],
      ['via-l2', undefined, 'in-sync'],
    ]);
  });
});

describe('Policy routing screen', () => {
  it('lists policies with live status and attachments; adding a policy patches the candidate', async () => {
    const api = installFakeApi('operator', 'op1');
    withConfig(api);
    await signIn();
    render(app('/routing/pbr'));
    const row = await screen.findByTestId('policy-via-l2');
    expect(within(row).getByText('lan-b')).toBeTruthy();
    expect(within(row).getByText('in sync')).toBeTruthy();
    expect(within(row).getByText('10.1.2.254 via loop102')).toBeTruthy();
    expect(
      screen.getByText('ACL hit counters per policy are not available in this release.'),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Add policy' }));
    fireEvent.change(await screen.findByTestId('policy-name'), { target: { value: 'bad#name' } });
    expect(screen.getByText('letters, digits, _, . and - only (no #), at most 63')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByTestId('policy-name')).toBeNull());
    fireEvent.click(
      await screen.findByRole(
        'button',
        { name: 'Detach via-l2 from loop101' },
        { timeout: 10_000 },
      ),
    );
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === 'PATCH' && c.path === '/api/v1/config/routing'),
      ).toBe(true),
    );
    const patch = api.calls.find((c) => c.method === 'PATCH')!.body as {
      pbr: { attachments: unknown[] };
    };
    expect(patch.pbr.attachments).toEqual([]);
  }, 60_000);

  it('renders in Persian (RTL namespace strings)', async () => {
    const api = installFakeApi('readonly', 'ro');
    withConfig(api);
    await signIn();
    render(app('/routing/pbr'));
    await screen.findByTestId('policy-via-l2', {}, { timeout: 15_000 });
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(
      await screen.findByRole(
        'heading',
        { level: 2, name: 'مسیریابی مبتنی بر سیاست' },
        { timeout: 15_000 },
      ),
    ).toBeTruthy();
    expect(await screen.findAllByText('هماهنگ', {}, { timeout: 10_000 })).toHaveLength(2); // policy + attachment
    expect(screen.getByRole('button', { name: 'افزودن سیاست' })).toBeDisabled(); // readonly role
  }, 60_000);
});

describe('ADL / Auto-SDL screen', () => {
  it('shows uRPF/ADL per interface and the Auto-SDL form', async () => {
    const api = installFakeApi('operator', 'op1');
    withConfig(api);
    await signIn();
    render(app('/firewall/adl'));
    const row = await screen.findByTestId('sec-loop101');
    expect(within(row).getByText('IPv4 strict (rx)')).toBeTruthy();
    expect(within(row).getByText('IPv4 → allow')).toBeTruthy();
    expect(within(await screen.findByTestId('sec-loop102')).getAllByText('off')).toHaveLength(2);
    expect(await screen.findByText('Auto-SDL', { selector: 'h3' })).toBeTruthy();
  }, 60_000);

  it('the interface drawer strings of the security group come from this namespace', () => {
    expect(i18n.t('interfaces:group.rpf-adl-pbr')).toBe('Security');
    expect(i18n.t('interfaces:field.urpf.title')).toBe('Unicast RPF');
    expect(i18n.t('interfaces:group.rpf-adl-pbr', { lng: 'fa' })).toBe('امنیت');
  });
});
