import { QueryClient } from '@tanstack/react-query';
import { act, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { App } from '../App';
import i18n from '../i18n';
import { createTestRouter } from '../router';
import { installFakeApi, resetSession, signIn } from '../test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  return <App router={createTestRouter([path])} streamUrl={STREAM} queryClient={queryClient} />;
}

describe('reference navigation structure', () => {
  beforeEach(async () => { installFakeApi(); await signIn(); });
  afterEach(async () => {
    await resetSession();
    localStorage.clear();
    await i18n.changeLanguage('en');
  });
  it('opens the existing policy editor through both URLs without losing the selected list', async () => {
    for (const path of ['/firewall/policies', '/firewall/acl']) {
      const router = createTestRouter([`${path}?tab=rules&list=web-in`]);
      const queryClient = new QueryClient({
        defaultOptions: { queries: { enabled: false, retry: false } },
      });
      const rendered = render(<App router={router} streamUrl={STREAM} queryClient={queryClient} />);
      expect(
        await screen.findByRole('heading', { level: 2, name: 'Policies' }),
      ).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: 'Policies', selected: true })).toBeInTheDocument();
      expect(router.state.location.search).toBe('?tab=rules&list=web-in');
      expect(screen.getByRole('link', { name: 'Policies' })).toHaveAttribute(
        'aria-current',
        'page',
      );
      rendered.unmount();
    }
  });

  it('highlights only the object tab and opens the existing routing object editors in Persian', async () => {
    const rendered = render(app('/firewall/objects?tab=zones'));
    expect(await screen.findByRole('tab', { name: 'Zones', selected: true })).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    const current = within(nav)
      .getAllByRole('link')
      .filter((link) => link.getAttribute('aria-current') === 'page');
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveAttribute('href', '/firewall/objects?tab=zones');
    rendered.unmount();
    localStorage.setItem('ngfw.ui.settings', JSON.stringify({ mode: 'light', lang: 'fa' }));
    await i18n.changeLanguage('fa');
    render(app('/routing/objects?tab=route-maps'));
    expect(
      await screen.findByRole('heading', { level: 2, name: 'آبجکت‌های مسیریابی' }),
    ).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute('dir', 'rtl');
    expect(
      screen.getAllByRole('tab').find((tab) => tab.getAttribute('aria-selected') === 'true'),
    ).toHaveAttribute('id', 'routing-tab-route-maps');
  });

  it('expands the current section when only the tab changes to a different section', async () => {
    const router = createTestRouter(['/routing?tab=static']);
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
    render(<App router={router} streamUrl={STREAM} queryClient={queryClient} />);
    await screen.findByRole('heading', { level: 2, name: 'Routing' });
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    expect(within(nav).getByRole('button', { name: 'Tools' })).toHaveAttribute('aria-expanded', 'false');
    await act(async () => { await router.navigate('/routing?tab=ping'); });
    expect(within(nav).getByRole('button', { name: 'Tools' })).toHaveAttribute('aria-expanded', 'true');
    expect(within(nav).getByRole('link', { name: 'Ping' })).toHaveAttribute('aria-current', 'page');
  });

});
