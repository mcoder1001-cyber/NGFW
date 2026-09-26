import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

/** F-acl screen in jsdom against a scripted stand-in of the API (unit level; the real stack runs in test/topology/acl). */
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

const REASON = 'acl.stats-enable belongs to the globals owner on this host (D-071)';

const listsState = {
  countersAvailable: false,
  countersReason: REASON,
  agentError: null,
  retrievedAt: new Date(Date.now() - 5_000).toISOString(),
  lists: [
    {
      name: 'web-in',
      description: 'web servers',
      tags: [],
      rules: 3,
      pending: null,
      attachments: [
        {
          target: { kind: 'interface', interface: 'host-w3l0' },
          direction: 'in',
          sequence: 10,
          enabled: true,
        },
      ],
      live: { aclIndex: 7, vppRules: 4, mappingKnown: true, configRules: 3, packets: 0, bytes: 0 },
    },
    {
      name: 'draft',
      description: null,
      tags: [],
      rules: 0,
      pending: 'added',
      attachments: [],
      live: null,
    },
  ],
  macip: [
    {
      name: 'lan-macs',
      description: null,
      rules: 2,
      pending: null,
      interfaces: ['host-w3l0'],
      live: { aclIndex: 9, vppRules: 2 },
    },
  ],
};

const rule = (sequence: number, extra: Record<string, unknown> = {}) => ({
  sequence,
  enabled: true,
  action: 'permit',
  ipVersion: 'any',
  source: { kind: 'any' },
  destination: { kind: 'prefix', prefix: '10.3.1.0/24' },
  service: { kind: 'inline', spec: { protocol: 'tcp', destinationPorts: ['22'], sourcePorts: [] } },
  log: false,
  ...extra,
});
const items = [
  {
    index: 0,
    sequence: 10,
    rule: rule(10, { description: 'ssh from anywhere' }),
    pending: null,
    live: { status: 'applied', vppRules: 2, packets: 1234, bytes: 1536 },
  },
  {
    index: 1,
    sequence: 20,
    rule: rule(20, { action: 'deny' }),
    pending: 'changed',
    live: { status: 'applied', vppRules: 1, packets: 0, bytes: 0 },
  },
  {
    index: 2,
    sequence: 21,
    rule: rule(21, { enabled: false }),
    pending: null,
    live: { status: 'disabled', vppRules: 0, packets: 0, bytes: 0 },
  },
];

function withAcl(api: FakeApi, { counters = true }: { counters?: boolean } = {}) {
  api.on('GET /api/v1/state/acl/lists', { body: listsState });
  api.on('GET /api/v1/config/candidate/objects', {
    body: {
      tags: {},
      addresses: {},
      addressGroups: {},
      services: {},
      serviceGroups: {},
      schedules: {},
      zones: {},
    },
  });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: { 'host-w3l0': { enabled: true, subinterfaces: {} } },
  });
  api.on('GET /api/v1/state/acl/lists/web-in/rules', (req) => {
    const q = new URL(req.url).searchParams;
    const page = Number(q.get('page') ?? '1');
    const pageSize = Number(q.get('pageSize') ?? '100');
    const filter = q.get('filter') ?? '';
    const matching = items.filter((i) => !filter || JSON.stringify(i.rule).includes(filter));
    return {
      body: {
        list: 'web-in',
        source: q.get('source') ?? 'candidate',
        page,
        pageSize,
        total: matching.length,
        size: items.length,
        applied: true,
        mappingKnown: true,
        countersAvailable: counters,
        countersReason: counters ? '' : REASON,
        agentError: null,
        items: matching.slice((page - 1) * pageSize, page * pageSize),
      },
    };
  });
}

const ruleCalls = (api: FakeApi) =>
  api.calls.filter((c) => c.method === 'GET' && c.path === '/api/v1/state/acl/lists/web-in/rules');

async function openEditor(api: FakeApi) {
  await signIn();
  render(app('/firewall/acl?tab=rules&list=web-in'));
  const grid = await screen.findByRole('grid', { name: 'Rules of web-in' }, { timeout: 15_000 });
  await within(grid).findAllByText('10.3.1.0/24', {}, { timeout: 15_000 });
  await waitFor(() => expect(ruleCalls(api).length).toBeGreaterThan(0));
  return grid;
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('ACL screen', () => {
  it(
    'lists the access lists with attachments, live VPP status and the counters caveat; ADL is a link',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api);
      await signIn();
      render(app('/firewall/acl'));
      expect(
        await screen.findByRole('heading', { level: 2, name: 'Access lists' }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      const table = await screen.findByRole('table', { name: 'Access lists' });
      const web = (await within(table).findByText('web-in')).closest('tr')!;
      expect(within(web).getByText('ACL 7')).toBeInTheDocument();
      expect(within(web).getByText('4 VPP rules')).toBeInTheDocument();
      expect(within(web).getByText(/host-w3l0.* · in/)).toBeInTheDocument();
      const draft = within(table).getByText('draft').closest('tr')!;
      expect(within(draft).getByText('new')).toBeInTheDocument();
      expect(within(draft).getByText('Not committed')).toBeInTheDocument();
      expect(screen.getByText(REASON)).toBeInTheDocument();
      expect(screen.getByRole('link', { name: 'Open ADL / Auto-SDL' })).toHaveAttribute(
        'href',
        '/firewall/adl',
      );
      // D-132: no timer below 30 s, and an explicit Refresh
      const before = api.calls.filter((c) => c.path === '/api/v1/state/acl/lists').length;
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
      await waitFor(() =>
        expect(
          api.calls.filter((c) => c.path === '/api/v1/state/acl/lists').length,
        ).toBeGreaterThan(before),
      );
    },
  );

  it(
    'rule editor: server paging with pageSize, quick filter → filter, counters "—" when unavailable',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api, { counters: false });
      const grid = await openEditor(api);
      expect(ruleCalls(api)[0]!.search).toBe('?page=1&pageSize=100&source=candidate');
      expect(within(grid).getAllByText('—').length).toBeGreaterThan(0);

      fireEvent.change(screen.getByRole('searchbox', { name: 'Search rules' }), {
        target: { value: 'ssh from' },
      });
      const query = (c: { search: string }) => Object.fromEntries(new URLSearchParams(c.search));
      await waitFor(
        () =>
          expect(ruleCalls(api).map(query)).toContainEqual({
            page: '1',
            pageSize: '100',
            source: 'candidate',
            filter: 'ssh from',
          }),
        { timeout: 5_000 },
      );
      expect(await screen.findByText('1 of 3 rules match')).toBeInTheDocument();

      fireEvent.click(screen.getByLabelText('Only rules with hits'));
      await waitFor(() =>
        expect(ruleCalls(api).some((c) => c.search.includes('hitsOnly=true'))).toBe(true),
      );
    },
  );

  it(
    'bulk disable posts the selected sequences; a single delete goes through the bulk action too',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api);
      const bodies: unknown[] = [];
      api.on('POST /api/v1/actions/acl/lists/web-in/rules/bulk', (_r, body) => {
        bodies.push(body);
        return { body: { list: 'web-in', op: 'disable', changed: 1, total: 3, before: 3 } };
      });
      const grid = await openEditor(api);
      const boxes = await within(grid).findAllByRole('checkbox', { name: 'Select row' });
      fireEvent.click(boxes[1]!);
      const bar = await screen.findByRole('toolbar', { name: 'Bulk actions' });
      expect(within(bar).getByText('1 rule selected')).toBeInTheDocument();
      fireEvent.click(within(bar).getByRole('button', { name: 'Disable' }));
      await waitFor(() => expect(bodies).toEqual([{ op: 'disable', sequences: [20] }]));
      expect(await screen.findByText('1 rule changed in the candidate.')).toBeInTheDocument();

      fireEvent.click(await within(grid).findByRole('button', { name: 'Delete rule 10' }));
      const confirm = await screen.findByRole('dialog', { name: 'Delete rule 10?' });
      fireEvent.click(within(confirm).getByRole('button', { name: 'Delete' }));
      await waitFor(() =>
        expect(bodies).toEqual([
          { op: 'disable', sequences: [20] },
          { op: 'delete', sequences: [10] },
        ]),
      );
    },
  );

  it(
    'edit saves the rule with PUT to its document position, after checking the candidate still has it there',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api);
      api.on('GET /api/v1/config/candidate/acl/lists/web-in/rules/1', { body: items[1]!.rule });
      let put: { path: string; body: unknown } | null = null;
      api.on('PUT /api/v1/config/acl/lists/web-in/rules/1', (_r, body) => {
        put = { path: '/acl/lists/web-in/rules/1', body };
        return { body: { pointer: '/acl/lists/web-in/rules/1', before: null, after: null } };
      });
      const grid = await openEditor(api);
      const row = (await within(grid).findAllByRole('row')).find(
        (r) => r.getAttribute('data-id') === '1',
      )!;
      fireEvent.doubleClick(within(row).getAllByRole('gridcell')[2]!);
      const dialog = await screen.findByRole('dialog', { name: 'Rule 20' }, { timeout: 15_000 });
      fireEvent.change(within(dialog).getByRole('textbox', { name: 'Description' }), {
        target: { value: 'block ssh' },
      });
      fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(put).not.toBeNull(), { timeout: 15_000 });
      expect(put!.body).toMatchObject({
        sequence: 20,
        action: 'deny',
        description: 'block ssh',
        destination: { kind: 'prefix', prefix: '10.3.1.0/24' },
      });
    },
  );

  it(
    'dragging a rule onto another: PATCH its sequence into a free gap, else move to the target sequence',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api);
      const patches: { path: string; body: unknown }[] = [];
      api.on('PATCH /api/v1/config/acl/lists/web-in/rules/2', (_r, body) => {
        patches.push({ path: '/acl/lists/web-in/rules/2', body });
        return { body: { pointer: '/acl/lists/web-in/rules/2', before: null, after: null } };
      });
      const bulks: unknown[] = [];
      api.on('POST /api/v1/actions/acl/lists/web-in/rules/bulk', (_r, body) => {
        bulks.push(body);
        return { body: { list: 'web-in', op: 'move', changed: 1, total: 3, before: 3 } };
      });
      const grid = await openEditor(api);
      const rowOf = async (id: string) =>
        (await within(grid).findAllByRole('row')).find((r) => r.getAttribute('data-id') === id)!;
      const dataTransfer = { setData: vi.fn(), effectAllowed: '', dropEffect: '' };

      // rule 21 onto rule 10 (first of the list): the gap 0…10 has room → sequence 5
      fireEvent.dragStart(within(await rowOf('2')).getByRole('button', { name: /Drag rule 21/ }), {
        dataTransfer,
      });
      const first = within(await rowOf('0')).getAllByRole('gridcell')[3]!;
      fireEvent.dragOver(first, { dataTransfer });
      fireEvent.drop(first, { dataTransfer });
      await waitFor(() =>
        expect(patches).toEqual([{ path: '/acl/lists/web-in/rules/2', body: { sequence: 5 } }]),
      );

      // rule 10 onto rule 21 (after 20: no free sequence) → move 10 to 21
      fireEvent.dragStart(within(await rowOf('0')).getByRole('button', { name: /Drag rule 10/ }), {
        dataTransfer,
      });
      const last = within(await rowOf('2')).getAllByRole('gridcell')[3]!;
      fireEvent.drop(last, { dataTransfer });
      await waitFor(() => expect(bulks).toEqual([{ op: 'move', sequences: [10], to: 21 }]));
    },
  );

  it(
    'CSV import: a dry run shows the result, then Import posts the same file with dryRun=false',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api);
      // the scripted API parses JSON bodies only: CSV posts are answered here
      const imports: { search: string; body: string; type: string | null }[] = [];
      const inner = globalThis.fetch;
      vi.stubGlobal('fetch', async (req: Request) => {
        const url = new URL(req.url);
        if (url.pathname !== '/api/v1/actions/acl/import') return inner(req);
        const body = await req.text();
        imports.push({ search: url.search, body, type: req.headers.get('content-type') });
        const dryRun = url.searchParams.get('dryRun') === 'true';
        const result = {
          list: 'web-in',
          mode: 'append',
          dryRun,
          rows: 2,
          valid: 2,
          errorCount: 0,
          errors: [],
          warnings: [],
          existingRules: 3,
          preview: [rule(30), rule(40, { action: 'deny' })],
          imported: dryRun ? 0 : 2,
          total: 5,
        };
        return new Response(JSON.stringify(result), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        });
      });
      await openEditor(api);
      fireEvent.click(screen.getByRole('button', { name: 'Import CSV' }));
      const dialog = await screen.findByRole('dialog', { name: 'Import rules into web-in' });
      const csv = 'sequence,action,destination\n30,permit,10.3.1.0/24\n40,deny,any\n';
      fireEvent.change(dialog.querySelector('input[type=file]')!, {
        target: { files: [new File([csv], 'rules.csv', { type: 'text/csv' })] },
      });
      expect(await within(dialog).findByText('rules.csv')).toBeInTheDocument();
      fireEvent.click(within(dialog).getByRole('radio', { name: 'Append to the list' }));
      fireEvent.click(within(dialog).getByRole('button', { name: 'Check (dry run)' }));
      const result = await within(dialog).findByRole('region', { name: 'Dry-run result' });
      expect(within(result).getByText(/2 rows: 2 valid, 0 with errors\./)).toBeInTheDocument();
      expect(within(result).getByRole('table', { name: 'Preview' })).toBeInTheDocument();
      expect(imports[0]).toEqual({
        search: '?list=web-in&mode=append&dryRun=true',
        body: csv,
        type: 'text/csv',
      });

      fireEvent.click(within(dialog).getByRole('button', { name: 'Import' }));
      expect(
        await within(dialog).findByText(
          'Imported 2 rules; the list now has 5 rules in the candidate.',
        ),
      ).toBeInTheDocument();
      expect(imports[1]!.search).toBe('?list=web-in&mode=append&dryRun=false');
    },
  );

  it(
    'attachments: VPP chains with other owners’ ACLs marked foreign; removing an attachment patches the whole array',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withAcl(api);
      const attached = [
        {
          list: 'web-in',
          target: { kind: 'interface', interface: 'host-w3l0' },
          direction: 'in',
          sequence: 10,
          enabled: true,
        },
      ];
      api.on('GET /api/v1/config/candidate/acl/attachments', { body: attached });
      api.on('GET /api/v1/config/acl/attachments', { body: attached });
      api.on('GET /api/v1/state/acl/attachments', {
        body: {
          agentError: null,
          retrievedAt: new Date().toISOString(),
          interfaces: [
            {
              interface: 'host-w3l0',
              swIfIndex: 3,
              input: [
                { aclIndex: 1, name: null, tag: 'rpf:adl-guard', foreign: true },
                { aclIndex: 7, name: 'web-in', tag: 'w3:web-in', foreign: false },
              ],
              output: [],
              macip: null,
              expected: { input: ['web-in'], output: [], macip: null },
              inSync: true,
            },
          ],
        },
      });
      let patched: unknown;
      api.on('PATCH /api/v1/config/acl', (_r, body) => {
        patched = body;
        return { body: { pointer: '/acl', before: null, after: null } };
      });
      await signIn();
      render(app('/firewall/acl?tab=attachments'));
      const live = await screen.findByRole(
        'table',
        { name: 'ACLs per interface in VPP' },
        { timeout: 15_000 },
      );
      const row = (await within(live).findByText('host-w3l0')).closest('tr')!;
      expect(within(row).getByText('rpf:adl-guard')).toBeInTheDocument();
      expect(within(row).getByText('other owner')).toBeInTheDocument();
      expect(within(row).getByText('web-in')).toBeInTheDocument();
      expect(within(row).getByText('in sync')).toBeInTheDocument();
      // no timer on this tab: only the explicit refresh walks VPP again
      expect(api.calls.filter((c) => c.path === '/api/v1/state/acl/attachments')).toHaveLength(1);

      const configured = screen.getByRole('table', { name: 'Configured attachments' });
      fireEvent.click(
        await within(configured).findByRole('button', {
          name: 'Remove attachment of web-in to host-w3l0',
        }),
      );
      const confirm = await screen.findByRole('dialog', { name: 'Remove attachment?' });
      fireEvent.click(within(confirm).getByRole('button', { name: 'Delete' }));
      await waitFor(() => expect(patched).toEqual({ attachments: [] }));
    },
  );

  it(
    'renders right to left in Persian, with Persian digits when the user asks for them',
    { timeout: 60_000 },
    async () => {
      localStorage.setItem(
        'vrx.ui.settings',
        JSON.stringify({ mode: 'light', lang: 'fa', persianDigits: true, dense: true }),
      );
      const api = installFakeApi();
      withAcl(api);
      await signIn();
      render(app('/firewall/acl'));
      await act(async () => {
        await i18n.changeLanguage('fa');
      });
      expect(
        await screen.findByRole(
          'heading',
          { level: 2, name: 'فهرست‌های دسترسی' },
          { timeout: 15_000 },
        ),
      ).toBeInTheDocument();
      expect(document.documentElement).toHaveAttribute('dir', 'rtl');
      expect(screen.getByRole('tab', { name: 'فهرست‌ها', selected: true })).toBeInTheDocument();
      const table = await screen.findByRole('table', { name: 'فهرست‌های دسترسی' });
      const web = (await within(table).findByText('web-in')).closest('tr')!;
      expect(within(web).getByText('ACL شمارهٔ ۷')).toBeInTheDocument();
      expect(within(web).getByText('۳')).toBeInTheDocument();
      expect(within(web).getByText('۴ قانون VPP')).toBeInTheDocument();
    },
  );
});
