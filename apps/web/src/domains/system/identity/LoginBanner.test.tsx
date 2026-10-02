import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import i18n from '../../../i18n';
import { installFakeApi, resetSession } from '../../../test-api';
import { LoginBanner } from './LoginBanner';

afterEach(async () => {
  cleanup();
  await resetSession();
  await i18n.changeLanguage('en');
});
function banner() {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <LoginBanner />
    </QueryClientProvider>,
  );
}

describe('pre-login configured banner', () => {
  it('renders literal markup and line breaks without creating HTML elements', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/auth/banner', {
      body: { banner: '<script>private()</script>\nAuthorised access only' },
    });
    const view = banner();
    const notice = await screen.findByLabelText('Login notice');
    expect(notice.textContent).toContain('<script>private()</script>');
    expect(view.container.querySelector('script')).toBeNull();
  });
  it('supports Persian text and hides empty or unavailable notices', async () => {
    await i18n.changeLanguage('fa');
    const api = installFakeApi();
    api.on('GET /api/v1/auth/banner', { body: { banner: 'ورود فقط برای کاربران مجاز' } });
    banner();
    expect(await screen.findByLabelText('اعلان ورود')).toHaveTextContent(
      'ورود فقط برای کاربران مجاز',
    );
    cleanup();
    api.on('GET /api/v1/auth/banner', { status: 503, body: { detail: 'offline' } });
    const view = banner();
    await waitFor(() =>
      expect(api.calls.filter((c) => c.path === '/api/v1/auth/banner')).toHaveLength(2),
    );
    expect(view.container).toBeEmptyDOMElement();
  });
});
