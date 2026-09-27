import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const CH = 'B'.repeat(43);

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

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('F-aaa-login web', () => {
  it(
    'login page: password → MFA code step → signed in; SSO button when OIDC is offered',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      api.on('POST /api/v1/auth/refresh', { status: 401, body: { detail: 'no refresh token' } });
      api.on('GET /api/v1/auth/methods', { body: { oidc: true } });
      api.on('POST /api/v1/auth/login', {
        body: { mfaRequired: true, challenge: CH, enrolled: true, expiresIn: 300 },
      });
      const session = {
        accessToken: 'h.p.s',
        tokenType: 'Bearer',
        expiresIn: 900,
        user: { id: 1, username: 'admin', role: 'admin' },
      };
      api.on('POST /api/v1/auth/mfa/verify', (_r, body) =>
        (body as { code?: string }).code === '123456'
          ? { body: session }
          : { status: 401, body: { detail: 'invalid code or challenge' } },
      );
      render(app('/login'));
      expect(await screen.findByTestId('sso-button', {}, { timeout: 15_000 })).toHaveAttribute(
        'href',
        '/api/v1/auth/oidc/start',
      );
      fireEvent.change(screen.getByLabelText(/user name|username/i), {
        target: { value: 'admin' },
      });
      fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'pw' } });
      fireEvent.click(screen.getByRole('button', { name: /sign in$/i }));
      const code = await screen.findByLabelText(/authentication code/i);
      fireEvent.change(code, { target: { value: '000000' } });
      fireEvent.click(screen.getByRole('button', { name: 'Verify' }));
      expect(await screen.findByRole('alert')).toHaveTextContent(/not accepted/);
      fireEvent.change(screen.getByLabelText(/authentication code/i), {
        target: { value: '123456' },
      });
      fireEvent.click(screen.getByRole('button', { name: 'Verify' }));
      await waitFor(() => expect(screen.queryByLabelText(/authentication code/i)).toBeNull(), {
        timeout: 15_000,
      });
      const verify = api.calls.filter((c) => c.path === '/api/v1/auth/mfa/verify');
      expect(verify.at(-1)?.body).toEqual({ challenge: CH, code: '123456' });
    },
  );

  it('AAA page: own MFA status and the admin test panel', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/auth/mfa', {
      body: { enrolled: false, recoveryCodesLeft: 0, required: true },
    });
    api.on('POST /api/v1/actions/aaa/test', {
      body: {
        method: 'radius',
        reachable: true,
        authenticated: true,
        groups: ['netadmins'],
        role: 'operator',
        detail: 'authenticated; role operator',
      },
    });
    await signIn();
    render(app('/system/aaa'));
    expect(
      await screen.findByText('Two-factor authentication is off.', {}, { timeout: 15_000 }),
    ).toBeInTheDocument();
    expect(screen.getByText('The MFA policy requires it for your role.')).toBeInTheDocument();
    const panel = screen.getByTestId('aaa-test');
    fireEvent.change(screen.getByLabelText('User name'), { target: { value: 'w1bob' } });
    fireEvent.change(screen.getAllByLabelText('Password').at(-1)!, { target: { value: 'bob-pw' } });
    fireEvent.click(screen.getByRole('button', { name: 'Test' }));
    expect(await screen.findByTestId('aaa-test-result')).toHaveTextContent('operator');
    expect(panel).toBeInTheDocument();
    const call = api.calls.find((c) => c.path === '/api/v1/actions/aaa/test');
    expect(call?.body).toEqual({ method: 'radius', username: 'w1bob', password: 'bob-pw' });
  });
});
