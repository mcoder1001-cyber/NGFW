import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

/** F-host-acl-nftables screen in jsdom against a scripted stand-in of the API (the real stack runs in test/topology). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

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

const rule = (sequence: number, action: string, extra: Record<string, unknown> = {}) => ({
  sequence,
  action,
  enabled: true,
  ipVersion: 'any',
  source: { kind: 'any' },
  destination: { kind: 'any' },
  service: { kind: 'any' },
  log: false,
  ...extra,
});

const running = {
  lists: {},
  macip: {},
  attachments: [],
  macipAttachments: [],
  host: {
    'mgmt-in': {
      description: 'management plane',
      tags: [],
      rules: [
        rule(10, 'accept', {
          source: { kind: 'prefix', prefix: '10.9.0.0/24' },
          service: {
            kind: 'inline',
            spec: { protocol: 'tcp', destinationPorts: ['22'], sourcePorts: [] },
          },
        }),
        rule(20, 'drop'),
      ],
    },
  },
  hostAttachments: [{ list: 'mgmt-in', chain: 'input', priority: 0, enabled: true }],
};
// the candidate adds a description to the list: it is marked pending
const candidate = {
  ...running,
  host: { 'mgmt-in': { ...running.host['mgmt-in'], description: 'management plane (edited)' } },
};

const state = {
  retrievedAt: '2026-09-24T12:00:00.000Z',
  table: 'vrx_w9',
  mode: 'netns',
  present: true,
  inSync: true,
  sets: [{ name: 'a4_admins', type: 'ipv4_addr', object: 'admins', elements: ['10.9.0.0/24'] }],
  chains: [
    {
      name: 'in_mgmt-in',
      hook: 'input',
      priority: 0,
      policy: 'accept',
      list: 'mgmt-in',
      rules: [
        {
          kind: 'established',
          list: '',
          sequence: 0,
          pointer: '',
          text: 'ct state established,related accept',
          verdict: 'accept',
          comment: 'vrx:pre:ct',
          packets: '41',
          bytes: '3000',
        },
        {
          kind: 'anti-lockout',
          list: '',
          sequence: 0,
          pointer: '',
          text: 'tcp dport { 22, 443 } accept',
          verdict: 'accept',
          comment: 'vrx:anti-lockout',
          packets: '3',
          bytes: '180',
        },
        {
          kind: 'rule',
          list: 'mgmt-in',
          sequence: 20,
          pointer: '/acl/host/mgmt-in/rules/1',
          text: 'drop',
          verdict: 'drop',
          comment: 'vrx:mgmt-in:20/0:ab',
          packets: '1234567',
          bytes: '74074020',
        },
      ],
    },
  ],
  rules: [
    {
      list: 'mgmt-in',
      sequence: 20,
      pointer: '/acl/host/mgmt-in/rules/1',
      packets: '1234567',
      bytes: '74074020',
      nftRules: 1,
    },
  ],
};

function withHostAcl(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/acl', { body: candidate });
  api.on('GET /api/v1/config/acl', { body: running });
  api.on('GET /api/v1/state/host-acl', { body: state });
  api.on('GET /api/v1/config/candidate/objects', { body: {} });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('host ACL screen', () => {
  it(
    'lists host rules with live counters, the pending mark and the anti-lockout banner',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withHostAcl(api);
      await signIn();
      render(app('/firewall/host-acl'));
      expect(
        await screen.findByRole('heading', { level: 2, name: 'Host ACL' }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      expect(
        await screen.findByText(
          /management TCP 22, 443 from any source on any interface is always accepted/,
        ),
      ).toBeInTheDocument();
      const table = await screen.findByRole('table', { name: 'Rules of mgmt-in' });
      const drop = within(table).getByText('20').closest('tr')!;
      expect(within(drop).getByText('1,234,567')).toBeInTheDocument();
      expect(within(drop).getByText('74,074,020')).toBeInTheDocument();
      const accept = within(table).getByText('10').closest('tr')!;
      expect(within(accept).getByText('10.9.0.0/24')).toBeInTheDocument();
      expect(within(accept).getByText('tcp/22')).toBeInTheDocument();
      expect(screen.getByText('pending')).toBeInTheDocument();
    },
  );

  it(
    'edits a rule and saves the rules array of its list as a merge patch of /acl',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withHostAcl(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/acl', (_r, body) => {
        patched = body;
        return { body: { pointer: '/acl', before: null, after: null } };
      });
      await signIn();
      render(app('/firewall/host-acl'));
      const table = await screen.findByRole(
        'table',
        { name: 'Rules of mgmt-in' },
        { timeout: 15_000 },
      );
      fireEvent.click(within(table).getByText('20'));
      const dialog = await screen.findByRole('dialog', { name: 'Edit rule 20 of mgmt-in' });
      fireEvent.change(within(dialog).getByRole('textbox', { name: 'Description' }), {
        target: { value: 'default deny' },
      });
      fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(patched).toBeDefined());
      const rules = (
        patched as { host: Record<string, { rules: { sequence: number; description?: string }[] }> }
      ).host['mgmt-in']!.rules;
      expect(rules.map((r) => [r.sequence, r.description])).toEqual([
        [10, undefined],
        [20, 'default deny'],
      ]);
    },
  );

  it(
    'shows the rendered table and renders right to left in Persian',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withHostAcl(api);
      await signIn();
      render(app('/firewall/host-acl?tab=rendered'));
      const chain = await screen.findByRole(
        'table',
        { name: 'Rules of chain in_mgmt-in' },
        { timeout: 15_000 },
      );
      expect(within(chain).getByText('ct state established,related accept')).toBeInTheDocument();
      expect(within(chain).getByText('anti-lockout')).toBeInTheDocument();
      expect(screen.getByRole('table', { name: 'Sets' })).toBeInTheDocument();
      await act(async () => {
        await i18n.changeLanguage('fa');
      });
      expect(
        await screen.findByRole('heading', { level: 2, name: 'ACL میزبان' }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole('tab', { name: 'جدول پیاده‌شده', selected: true }),
      ).toBeInTheDocument();
      expect(
        await screen.findByRole('table', { name: 'قاعده‌های زنجیرهٔ in_mgmt-in' }),
      ).toBeInTheDocument();
    },
  );
});
