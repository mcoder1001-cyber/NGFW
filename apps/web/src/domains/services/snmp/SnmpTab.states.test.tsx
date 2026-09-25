import { QueryClient } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Services → SNMP states (S-web-polish)', () => {
  it('guides the user on empty lists', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/services%2Fsnmp', { body: {} });
    api.on('GET /api/v1/state/snmp', { body: { configured: false, reachable: false } });
    await signIn();
    render(app('/services?tab=snmp'));
    expect(await screen.findByText(/No communities yet/)).toBeInTheDocument();
    expect(screen.getByText(/No SNMPv3 users yet/)).toBeInTheDocument();
    expect(screen.getByText(/No trap receivers/)).toBeInTheDocument();
    expect(screen.queryByText('Not found')).toBeNull();
  });

  it('shows the server problem (title, detail, field pointer) when loading fails', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/services%2Fsnmp', {
      status: 422,
      body: { type: 'https://vrx.dev/problems/validation', title: 'Validation failed', status: 422, detail: 'engine id is not hex', errors: [{ pointer: '/services/snmp/engineId', message: 'bad hex' }] },
    });
    api.on('GET /api/v1/state/snmp', { status: 503, body: { type: 'https://vrx.dev/problems/agent-unavailable', title: 'Agent unavailable', status: 503, detail: 'snmpd is down' } });
    await signIn();
    render(app('/services?tab=snmp'));
    expect(await screen.findByText(/engine id is not hex/)).toBeInTheDocument();
    expect(screen.getByText('/services/snmp/engineId')).toBeInTheDocument();
    expect(await screen.findByText(/snmpd is down/)).toBeInTheDocument();
  });
});
