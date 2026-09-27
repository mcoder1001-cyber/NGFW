import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import type { MfaStep } from '../../../auth/session';

type Ok<P extends keyof paths, M extends 'get' | 'post'> = paths[P][M] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type MfaStatus = Ok<'/api/v1/auth/mfa', 'get'>;
export type AaaTestResult = Ok<'/api/v1/actions/aaa/test', 'post'>;
export type Enrolment = Ok<'/api/v1/auth/mfa/setup', 'post'>;

export const aaaKeys = { mfa: ['auth', 'mfa'] as const, methods: ['auth', 'methods'] as const };

export function useMfaStatus() {
  return useQuery({
    queryKey: aaaKeys.mfa,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/auth/mfa', { signal }))).data as MfaStatus,
  });
}

/** Public: does the login page offer single sign-on? (no session yet, so plain fetch, no auth middleware). */
export function useLoginMethods() {
  return useQuery({
    queryKey: aaaKeys.methods,
    retry: false,
    queryFn: async ({ signal }) => {
      const res = await fetch(
        new Request(new URL('/api/v1/auth/methods', globalThis.location.href), {
          signal,
          credentials: 'same-origin',
        }),
      );
      if (!res.ok) return { oidc: false };
      const b = (await res.json()) as { oidc?: unknown };
      return { oidc: b.oidc === true };
    },
  });
}

export function useMfaSetup() {
  return useMutation({
    mutationFn: async (v: { current: string; token: string }) =>
      (await call(api.POST('/api/v1/auth/mfa/setup', { body: v }))).data as Enrolment,
  });
}

/** D-159: an admin issues (or revokes) a user's one-time MFA enrolment token; the token is shown once. */
export function useEnrolmentToken() {
  const issue = useMutation({
    mutationFn: async (name: string) =>
      (
        await call(
          api.POST('/api/v1/auth/mfa/users/{name}/enrolment-token', { params: { path: { name } } }),
        )
      ).data as { token: string; expiresIn: number },
  });
  const revoke = useMutation({
    mutationFn: async (name: string) =>
      call(
        api.DELETE('/api/v1/auth/mfa/users/{name}/enrolment-token', { params: { path: { name } } }),
      ),
  });
  return { issue, revoke };
}

export function useMfaActivate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (code: string) =>
      (await call(api.POST('/api/v1/auth/mfa/activate', { body: { code } }))).data as {
        recoveryCodes: string[];
      },
    onSuccess: () => qc.invalidateQueries({ queryKey: aaaKeys.mfa }),
  });
}

export function useAaaTest() {
  return useMutation({
    mutationFn: async (b: {
      method: 'radius' | 'ldap' | 'tacacs';
      username: string;
      password: string;
    }) => (await call(api.POST('/api/v1/actions/aaa/test', { body: b }))).data as AaaTestResult,
  });
}

export function useMfaReset() {
  return useMutation({
    mutationFn: async (name: string) =>
      call(api.DELETE('/api/v1/auth/mfa/users/{name}', { params: { path: { name } } })),
  });
}

/** D-102: an admin sets a user's password through the users route, never by staging a hash in the config. */
export function useSetPassword() {
  return useMutation({
    mutationFn: async (v: { name: string; password: string; keepApiKeys: boolean }) =>
      call(
        api.POST('/api/v1/users/{name}/password', {
          params: { path: { name: v.name } },
          body: { password: v.password, ...(v.keepApiKeys ? { keepApiKeys: true } : {}) },
        }),
      ),
  });
}

/**
 * The OIDC callback hands over in the URL fragment (never sent to a server): `#mfa=<challenge>&enrolled=0|1` when the
 * MFA policy applies, `#error=<slug>` on failure. Read once, then removed from the address bar and history entry.
 */
export function readSsoHandover(loc: { hash: string } = globalThis.location): {
  step?: MfaStep;
  error?: string;
} {
  const h = new URLSearchParams(loc.hash.replace(/^#/, ''));
  const out: { step?: MfaStep; error?: string } = {};
  const challenge = h.get('mfa');
  if (challenge && /^[A-Za-z0-9_-]{43}$/.test(challenge))
    out.step = { mfa: true, challenge, enrolled: h.get('enrolled') === '1' };
  const error = h.get('error');
  if (error && /^[a-z-]{1,40}$/.test(error)) out.error = error;
  if (
    (out.step || out.error) &&
    typeof history !== 'undefined' &&
    typeof location !== 'undefined'
  ) {
    history.replaceState(history.state, '', `${location.pathname}${location.search}`);
  }
  return out;
}
