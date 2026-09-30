import { QueryClient } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('automatic routing interfaces', () => {
  it.each(['en', 'fa'])('redirects the retired pairs tab to neighbours in %s', async (language) => {
    installFakeApi('admin');
    await signIn();
    await i18n.changeLanguage(language);
    render(
      <App
        router={createTestRouter(['/routing/bgp?tab=pairs'], { devRoutes: false })}
        streamUrl="ws://127.0.0.1:1/api/v1/stream"
        queryClient={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      />,
    );
    const tab = await screen.findByRole('tab', { name: i18n.t('bgp:tab.neighbors') });
    expect(tab).toHaveAttribute('aria-selected', 'true');
    expect(screen.getAllByRole('tab')).toHaveLength(3);
    expect(document.body.textContent).not.toMatch(/Linux|linux-cp|لینوکس|FRR|VPP|strongSwan/i);
  });
});
