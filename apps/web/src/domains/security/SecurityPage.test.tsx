import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { ApiError } from '../../api-problem';
import { App } from '../../App';
import i18n from '../../i18n';
import { buildNav, BUILT_DOMAINS } from '../../nav/nav';
import { createTestRouter } from '../../router';
import { domains } from '../../schema/registry';
import { installFakeApi, resetSession, signIn } from '../../test-api';
import { problemUnder } from './policy/PolicyTab';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

/** A complete `security` candidate as the server stores it (defaults filled). */
const RULE = { source: 'webLogin', enabled: true, threshold: 5, windowSec: 60, blockSec: 900, escalate: true, maxBlockSec: 86_400 };
const CANDIDATE = {
  autoBlock: { enabled: true, rules: [RULE], allowlist: ['10.0.0.0/8'], maxEntries: 10_000, description: 'mgmt nets stay open' },
};

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

async function openPolicy(api: ReturnType<typeof installFakeApi>, candidate: unknown = CANDIDATE) {
  api.on('GET /api/v1/config/candidate/security', () => ({ body: candidate }));
  await signIn();
  render(app('/firewall/security'));
  expect(await screen.findByRole('heading', { level: 2, name: 'Security' }, { timeout: 15_000 })).toBeInTheDocument();
  return screen.findByRole('button', { name: 'Save to candidate' }, { timeout: 15_000 });
}

describe('Firewall → Security (F-security-page)', () => {
  it('is a built domain: the nav entry is available and the Auto-block entry links to its Blocked sources tab', () => {
    expect(BUILT_DOMAINS.has('security')).toBe(true);
    const items = buildNav(domains, { devRoutes: false }).flatMap((g) => g.items);
    expect(items.find((i) => i.id === 'security')).toMatchObject({ path: '/firewall/security', available: true });
    expect(items.find((i) => i.id === 'auto-block')).toMatchObject({ path: '/firewall/security?tab=blocked', available: true });
  });

  it('renders the page with its two tabs, the policy tab first', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    await openPolicy(api);
    const tabs = within(screen.getByRole('tablist', { name: 'Security sections' })).getAllByRole('tab');
    expect(tabs.map((t) => t.textContent)).toEqual(['Auto-block policy', 'Blocked sources']);
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByText('Not yet available')).toBeNull();
    expect(screen.getByRole('link', { name: 'Open in the advanced editor' })).toHaveAttribute('href', '/config/security/autoBlock');
  });

  it('policy: loads the candidate slice and saves a change as a merge patch of security.autoBlock', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    let patched: unknown;
    api.on('PATCH /api/v1/config/security', (_req, body) => {
      patched = body;
      return { body: { pointer: '/security', before: null, after: null } };
    });
    const save = await openPolicy(api);
    const cap = screen.getByLabelText('Maximum blocked entries', { exact: false });
    expect(cap).toHaveValue(10_000);
    expect(screen.getByDisplayValue('10.0.0.0/8')).toBeInTheDocument();
    expect(screen.getByDisplayValue('mgmt nets stay open')).toBeInTheDocument();
    fireEvent.change(cap, { target: { value: '5000' } });
    fireEvent.click(save);
    await waitFor(() => expect(patched).toBeDefined());
    expect(patched).toEqual({ autoBlock: { ...CANDIDATE.autoBlock, maxEntries: 5000 } });
    expect(await screen.findByTestId('security-policy-saved')).toHaveTextContent('Saved to the candidate');
  });

  it('policy: an empty security candidate seeds the form from the schema defaults, which the save sends (R1)', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    let patched: unknown;
    api.on('PATCH /api/v1/config/security', (_req, body) => {
      patched = body;
      return { body: { pointer: '/security', before: null, after: null } };
    });
    const save = await openPolicy(api, {});
    expect(screen.getByLabelText('Maximum blocked entries', { exact: false })).toHaveValue(10_000);
    fireEvent.click(save);
    await waitFor(() => expect(patched).toBeDefined());
    // no description key at all: it was never there, so nothing is sent for it (R2: never wipe a concurrent edit)
    expect(patched).toEqual({ autoBlock: { enabled: false, rules: [], allowlist: [], maxEntries: 10_000 } });
  });

  it('policy: a description the form cleared is sent as null only when the candidate had one (R1/R2)', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    let patched: unknown;
    api.on('PATCH /api/v1/config/security', (_req, body) => {
      patched = body;
      return { body: { pointer: '/security', before: null, after: null } };
    });
    const save = await openPolicy(api);
    fireEvent.change(screen.getByDisplayValue('mgmt nets stay open'), { target: { value: '' } });
    fireEvent.click(save);
    await waitFor(() => expect(patched).toBeDefined());
    const { description, ...rest } = CANDIDATE.autoBlock;
    void description;
    expect(patched).toEqual({ autoBlock: { ...rest, description: null } });
  });

  it('policy: a validation problem from the API lands on its field and is listed as the server sent it', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    api.on('PATCH /api/v1/config/security', () => ({
      status: 400,
      body: {
        type: 'https://ngfw.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        errors: [{ pointer: '/security/autoBlock/maxEntries', message: 'the host supports at most 20000 entries' }],
      },
    }));
    const save = await openPolicy(api);
    fireEvent.click(save);
    expect(await screen.findByTestId('problem')).toHaveTextContent('Validation failed');
    await waitFor(() =>
      expect(screen.getByLabelText('Maximum blocked entries', { exact: false })).toHaveAttribute('aria-invalid', 'true'),
    );
    expect(screen.getAllByText('the host supports at most 20000 entries').length).toBeGreaterThan(0);
    expect(screen.queryByTestId('security-policy-saved')).toBeNull();
  });

  it('policy: a read-only role sees the form but cannot save', { timeout: 60_000 }, async () => {
    const api = installFakeApi('readonly');
    const save = await openPolicy(api);
    expect(save).toBeDisabled();
    expect(screen.getByText('Your role cannot change the configuration.')).toBeInTheDocument();
    expect(api.calls.some((c) => c.method === 'PATCH')).toBe(false);
  });

  it('policy: Persian labels follow the schema paths', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    await openPolicy(api);
    await i18n.changeLanguage('fa');
    expect(await screen.findByRole('heading', { level: 2, name: 'امنیت' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'سیاست مسدودسازی خودکار' })).toBeInTheDocument();
    expect(screen.getByLabelText('بیشینهٔ ورودی‌های مسدود', { exact: false })).toHaveValue(10_000);
    expect(screen.getAllByText('آشکارسازها').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'ذخیره در نامزد' })).toBeInTheDocument();
  });

  it('/firewall/auto-block still works: it redirects to the Blocked sources tab', { timeout: 60_000 }, async () => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/config/candidate/security', () => ({ body: CANDIDATE }));
    api.on('GET /api/v1/state/auto-block', { body: { items: [] } });
    await signIn();
    const router = createTestRouter(['/firewall/auto-block?tab=policy&source=203.0.113.7#row-1'], { devRoutes: false });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
    render(<App router={router} streamUrl={STREAM} queryClient={queryClient} />);
    expect(await screen.findByRole('heading', { level: 2, name: 'Security' }, { timeout: 15_000 })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Blocked sources' })).toHaveAttribute('aria-selected', 'true');
    // the old address's query and hash survive (R1); only tab is forced
    expect(router.state.location).toMatchObject({ pathname: '/firewall/security', search: '?tab=blocked&source=203.0.113.7', hash: '#row-1' });
    expect(await screen.findByText('No sources are blocked right now.')).toBeInTheDocument();
    // exactly one nav entry is current: the Auto-block tab link, not the Security page itself
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    const current = nav.querySelectorAll('[aria-current="page"]');
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveTextContent('Auto-block');
  });

  it('maps server pointers under /security/autoBlock onto the form and leaves others alone', () => {
    expect(problemUnder(new Error('x'))).toBeNull();
    const problem = problemUnder(
      new ApiError(400, {
        title: 'Validation failed',
        status: 400,
        pointer: '/security/autoBlock',
        errors: [
          { pointer: '/security/autoBlock/rules/0/maxBlockSec', message: 'the maximum block must be at least the first-offence block' },
          { pointer: '/acl/globalBlocking', message: 'elsewhere' },
        ],
      }, false),
    );
    expect(problem).toMatchObject({
      pointer: '/',
      errors: [
        { pointer: '/rules/0/maxBlockSec', detail: 'the maximum block must be at least the first-offence block' },
        { pointer: '/acl/globalBlocking', detail: 'elsewhere' },
      ],
    });
  });
});
