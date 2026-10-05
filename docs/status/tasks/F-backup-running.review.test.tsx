import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { NgfwThemeProvider } from '@ngfw/ui-kit';
import i18n from '../../../apps/web/src/i18n';
import { BackupRestorePage } from '../../../apps/web/src/domains/system/backup-restore/BackupRestorePage';

vi.mock('../../../apps/web/src/auth/AuthProvider', () => ({ usePermissions: () => ({ role: 'admin' }) }));
vi.mock('../../../apps/web/src/domains/system/backup-restore/transport', () => ({
 json: async (path: string) => path === '/state/backup'
  ? { runs: ['running', 'success', 'failure'].map(result => ({ at: '2026-10-05T17:00:00Z', result, filename: result + '-evidence' })) }
  : path === '/config-templates' ? {items:{}} : {enabled:false,schedule:'0 2 * * *',retention:7,revisions:100},
 download: vi.fn(), archiveBase64: vi.fn(),
}));
afterEach(async () => {cleanup(); await i18n.changeLanguage('en');});
for (const [lang, label] of [['en','In progress'],['fa','در حال اجرا']] as const) {
 it(`renders running as localized non-error status in ${lang}`, async () => {
  await i18n.changeLanguage(lang);
  const client = new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<NgfwThemeProvider mode="light" lang={lang} dir={lang==='fa'?'rtl':'ltr'}><QueryClientProvider client={client}><BackupRestorePage /></QueryClientProvider></NgfwThemeProvider>);
  const running=await screen.findByText(/running-evidence/);
  const success=await screen.findByText(/success-evidence/);
  const failure=await screen.findByText(/failure-evidence/);
  expect(running.textContent).toContain(label);
  expect(running.textContent).not.toContain(lang==='fa'?'ناموفق':'Failure');
  expect(failure.textContent).toContain(lang==='fa'?'ناموفق':'Failure');
  expect(getComputedStyle(running).color).toBe(getComputedStyle(success).color);
  expect(getComputedStyle(running).color).not.toBe(getComputedStyle(failure).color);
  client.clear();
 });
}
