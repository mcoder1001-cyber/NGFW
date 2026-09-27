import type { AuthMethod } from '@ngfw/schema';

/**
 * F-aaa-login: what one authentication method answered for a login attempt.
 *
 * - `accept` / `reject` — the method KNOWS this identity and decided. The walk stops there: a rogue or
 *   misconfigured directory must never be able to overrule a method that already refused the credential,
 *   and a credential refused locally must not be replayed against every other backend in turn.
 * - `absent` — the method has no such identity (the local user does not exist). The next method is tried.
 * - `unreachable` — no server of that method answered at all. The next method is tried, and the local
 *   fallback (`management.aaa.fallbackLocal`) becomes eligible.
 */
export type MethodAnswer = 'accept' | 'reject' | 'absent' | 'unreachable';

export interface AuthSequence {
  /** The methods to try, in order. */
  steps: readonly AuthMethod[];
  /**
   * Index into `steps` of the local fallback, which runs ONLY when an earlier method was `unreachable`
   * (`-1` when there is none). `fallbackLocal` is "let local users in while the directory is down", so it
   * must not become a second, always-on local login when the directory answered and said no.
   */
  fallbackAt: number;
}

/**
 * The methods a login tries, from `management.aaa.order` plus the optional local fallback.
 *
 * `order` is already unique and non-empty per the schema (`AaaSchema`); duplicates are dropped here anyway so
 * a hand-written or legacy document cannot make the same backend count twice. When `local` is already in the
 * order it is tried at its configured position and no fallback step is added — `fallbackLocal` only ever adds
 * `local` to an order that does not list it.
 */
export function authSequence(order: readonly string[], fallbackLocal: boolean): AuthSequence {
  const steps: AuthMethod[] = [];
  for (const m of order) {
    if (!isAuthMethod(m) || steps.includes(m)) continue;
    steps.push(m);
  }
  if (steps.length === 0) steps.push('local');
  if (fallbackLocal && !steps.includes('local')) {
    steps.push('local');
    return { steps, fallbackAt: steps.length - 1 };
  }
  return { steps, fallbackAt: -1 };
}

const METHODS: readonly string[] = ['local', 'radius', 'tacacs', 'ldap'];

/** An `order` entry this build understands. An unknown one is skipped, never treated as a backend. */
export function isAuthMethod(m: string): m is AuthMethod {
  return METHODS.includes(m);
}

/** Whether step `i` of `seq` may run, given whether an earlier method was `unreachable`. */
export function stepEligible(seq: AuthSequence, i: number, anyUnreachable: boolean): boolean {
  return i !== seq.fallbackAt || anyUnreachable;
}
