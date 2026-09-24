/**
 * Browser session against the P06 auth API (docs/status/tasks/P06.md §6):
 * - `POST /api/v1/auth/login` → access token (15 min, body) + rotating refresh token (httpOnly SameSite=Strict cookie
 *   scoped to `/api/v1/auth`, invisible to scripts).
 * - `POST /api/v1/auth/refresh` → new access token, the cookie is rotated. Replaying a used refresh token revokes the
 *   whole family, so refreshes are serialised across tabs (Web Locks) and within a tab (single flight), and every
 *   refresh has a deadline so the lock cannot hang (review M3).
 * - The access token lives in memory only (never localStorage): a reload restores the session through the cookie.
 * - Only an explicit refusal (401/400/403) ends a session. Timeouts, network errors, 5xx and 429 mean "device not
 *   answering": the session is kept (or, on a reload, becomes `offline`) and retried — the operator must not land on
 *   the login page in the middle of a confirmed-commit countdown (review M2).
 * - Tabs stay coherent over a BroadcastChannel carrying no token: a sign-out signs every tab out, a sign-in as another
 *   user reloads the other tabs (review M4).
 *
 * Framework-free so the logic is unit-testable; `AuthProvider` exposes it to React.
 */
import { fetchWithTimeout, TIMEOUTS } from '../net';

export type Role = 'admin' | 'operator' | 'readonly';

export interface SessionUser {
  id: number;
  username: string;
  role: Role;
}

/** `offline`: a session may exist but the device does not answer (page reloaded during an outage). */
export type SessionStatus = 'unknown' | 'offline' | 'anonymous' | 'authenticated';

/** Why the user is on the login page (shown there). */
export type EndReason = 'expired' | 'signedOut' | undefined;

export interface SessionState {
  status: SessionStatus;
  user: SessionUser | null;
  endReason: EndReason;
}

export interface LoginFailure {
  /** HTTP status; 0 when the server could not be reached. */
  status: number;
  detail: string | undefined;
}

interface SessionBody {
  accessToken: string;
  tokenType: 'Bearer';
  expiresIn: number;
  user: SessionUser;
}

/** Cross-tab message (never a credential). */
export type AuthSignal = { type: 'logout' } | { type: 'login'; userId: number };

export interface ChannelLike {
  postMessage(msg: AuthSignal): void;
  onmessage: ((ev: { data: unknown }) => void) | null;
  close(): void;
}

export interface SessionOptions {
  /** Cross-tab channel; defaults to `BroadcastChannel('vrx-auth')` when available. `null` disables it. */
  channel?: ChannelLike | null;
  /** What happens when another tab signs in as a different user (default: reload this tab). */
  onForeignLogin?: () => void;
  /** Retry period while offline / unreachable. */
  retryMs?: number;
}

export const AUTH_PREFIX = '/api/v1/auth/';
const REFRESH_LOCK = 'vrx-auth-refresh';
const CHANNEL = 'vrx-auth';

type Fetch = (input: Request) => Promise<Response>;
type RefreshOutcome = 'ok' | 'refused' | 'unavailable';

/** Absolute URL of an auth route (a relative `Request` URL throws outside a browser document). */
function authUrl(name: string): string {
  return new URL(`${AUTH_PREFIX}${name}`, globalThis.location?.href ?? 'http://localhost/').href;
}

/** Fetch resolved at call time, so tests can stub `globalThis.fetch` after this module loaded. */
const defaultFetch: Fetch = (r) => globalThis.fetch(r);

function isSessionBody(v: unknown): v is SessionBody {
  const o = v as Partial<SessionBody> | null;
  return !!o && typeof o.accessToken === 'string' && typeof o.expiresIn === 'number' && !!o.user && typeof o.user.username === 'string';
}

async function problemDetail(res: Response): Promise<string | undefined> {
  try {
    const body = (await res.json()) as { detail?: unknown };
    return typeof body.detail === 'string' ? body.detail : undefined;
  } catch {
    return undefined;
  }
}

/** Serialise a critical section across tabs when the Web Locks API exists (all current browsers), else run it. */
function withCrossTabLock<T>(fn: () => Promise<T>): Promise<T> {
  const locks = (globalThis.navigator as Navigator | undefined)?.locks;
  return locks ? (locks.request(REFRESH_LOCK, fn) as Promise<T>) : fn();
}

function defaultChannel(): ChannelLike | null {
  if (typeof BroadcastChannel === 'undefined') return null;
  const ch = new BroadcastChannel(CHANNEL);
  (ch as unknown as { unref?: () => void }).unref?.(); // never keep a Node test process alive
  return ch as unknown as ChannelLike;
}

/** Only these refresh answers mean "your session is over"; everything else is "the device does not answer". */
const REFUSED = new Set([400, 401, 403]);

export class Session {
  private token: string | null = null;
  private expiresAt = 0;
  private stateValue: SessionState = { status: 'unknown', user: null, endReason: undefined };
  private readonly listeners = new Set<(s: SessionState) => void>();
  private refreshing: Promise<RefreshOutcome> | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private readonly channel: ChannelLike | null;
  private readonly onForeignLogin: () => void;
  private readonly retryMs: number;

  constructor(
    private readonly fetchImpl: Fetch = defaultFetch,
    private readonly now: () => number = () => Date.now(),
    opts: SessionOptions = {},
  ) {
    this.channel = opts.channel === undefined ? defaultChannel() : opts.channel;
    this.onForeignLogin = opts.onForeignLogin ?? (() => globalThis.location?.reload());
    this.retryMs = opts.retryMs ?? 5_000;
    if (this.channel) this.channel.onmessage = (ev) => this.onSignal(ev.data);
  }

  get state(): SessionState {
    return this.stateValue;
  }

  /** Current access token (memory only). */
  get accessToken(): string | null {
    return this.token;
  }

  subscribe(listener: (s: SessionState) => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  /** Page load: try the refresh cookie. No answer → `offline` and keep retrying (never the login page). */
  async restore(): Promise<void> {
    const outcome = await this.rotate();
    if (outcome === 'unavailable' && this.stateValue.status !== 'authenticated') {
      if (this.stateValue.status !== 'offline') this.setState({ status: 'offline', user: null, endReason: undefined });
      this.schedule(this.retryMs, () => void this.restore());
    }
  }

  async login(username: string, password: string): Promise<LoginFailure | null> {
    let res: Response;
    try {
      res = await fetchWithTimeout(
        this.fetchImpl,
        new Request(authUrl('login'), {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ username, password }),
        }),
        TIMEOUTS.auth,
      );
    } catch {
      return { status: 0, detail: undefined };
    }
    if (!res.ok) return { status: res.status, detail: await problemDetail(res) };
    let body: unknown;
    try {
      body = await res.json();
    } catch {
      return { status: 502, detail: undefined };
    }
    if (!isSessionBody(body)) return { status: 502, detail: undefined };
    this.accept(body);
    this.post({ type: 'login', userId: body.user.id });
    return null;
  }

  async logout(): Promise<void> {
    try {
      await fetchWithTimeout(this.fetchImpl, new Request(authUrl('logout'), { method: 'POST', credentials: 'include' }), TIMEOUTS.auth);
    } catch {
      // the cookie is revoked server-side on the next successful call anyway; locally we forget the token
    }
    this.setAnonymous('signedOut');
    this.post({ type: 'logout' });
  }

  /** Rotate the refresh cookie. `false` = no new token (refused → signed out; unavailable → kept, retried). */
  async refresh(): Promise<boolean> {
    return (await this.rotate()) === 'ok';
  }

  /** The API answered 401 to a request that carried our token: refresh once; `true` = retry the request. */
  async handleUnauthorized(tokenUsed: string | null): Promise<boolean> {
    if (this.stateValue.status !== 'authenticated') return false;
    // another request already refreshed while this one was in flight
    if (tokenUsed !== this.token && this.token !== null) return true;
    return this.refresh();
  }

  /** Tear down timers and the channel (tests, hot reload). */
  dispose(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.listeners.clear();
    this.channel?.close();
  }

  /** Milliseconds until the access token expires (0 when there is none). */
  get remainingMs(): number {
    return Math.max(0, this.expiresAt - this.now());
  }

  private rotate(): Promise<RefreshOutcome> {
    this.refreshing ??= withCrossTabLock(async (): Promise<RefreshOutcome> => {
      let res: Response;
      let body: unknown;
      try {
        res = await fetchWithTimeout(this.fetchImpl, new Request(authUrl('refresh'), { method: 'POST', credentials: 'include' }), TIMEOUTS.auth);
        if (res.ok) body = await res.json(); // a 200 that is not JSON (captive portal, proxy page) is "unavailable" too
      } catch {
        return this.unavailable();
      }
      if (REFUSED.has(res.status)) {
        if (this.stateValue.status !== 'anonymous') this.setAnonymous(this.stateValue.status === 'authenticated' ? 'expired' : undefined);
        return 'refused';
      }
      if (!res.ok || !isSessionBody(body)) return this.unavailable();
      this.accept(body);
      return 'ok';
    }).finally(() => {
      this.refreshing = null;
    });
    return this.refreshing;
  }

  /** 5xx / 429 / timeout / network: keep what we have and try again later. */
  private unavailable(): 'unavailable' {
    if (this.stateValue.status === 'authenticated') this.schedule(15_000, () => void this.refresh());
    return 'unavailable';
  }

  private accept(body: SessionBody): void {
    const previous = this.stateValue.user;
    if (previous && previous.id !== body.user.id) {
      // the shared cookie now belongs to someone else (signed in from another tab): never show their data as ours
      this.onForeignLogin();
      return;
    }
    this.token = body.accessToken;
    this.expiresAt = this.now() + body.expiresIn * 1000;
    // refresh at 80 % of the lifetime (the relay closes sockets at expiry; the WS client reconnects with the new token)
    this.schedule(Math.max(5_000, body.expiresIn * 800), () => void this.refresh());
    this.setState({ status: 'authenticated', user: body.user, endReason: undefined });
  }

  private onSignal(data: unknown): void {
    const msg = data as Partial<AuthSignal> | null;
    if (msg?.type === 'logout') {
      if (this.stateValue.status !== 'anonymous') this.setAnonymous('signedOut');
    } else if (msg?.type === 'login' && typeof msg.userId === 'number') {
      const mine = this.stateValue.user?.id;
      if (mine !== undefined && mine !== msg.userId) this.onForeignLogin();
      else if (this.stateValue.status !== 'authenticated') void this.restore(); // pick up the other tab's sign-in
    }
  }

  private post(msg: AuthSignal): void {
    try {
      this.channel?.postMessage(msg);
    } catch {
      // closed channel: nothing to tell
    }
  }

  private schedule(ms: number, fn: () => void): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = null;
      fn();
    }, ms);
  }

  private setAnonymous(reason: EndReason): void {
    this.token = null;
    this.expiresAt = 0;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.setState({ status: 'anonymous', user: null, endReason: reason });
  }

  private setState(s: SessionState): void {
    this.stateValue = s;
    for (const l of this.listeners) l(s);
  }
}

/** The app-wide session. */
export const session = new Session();

/** Sub-protocols for `WS /api/v1/stream` (P06 D-P06-9); `undefined` while signed out keeps the socket closed. */
export function streamProtocols(s: Session = session): string[] | undefined {
  const t = s.accessToken;
  return t ? ['vrx.v1', `bearer.${t}`] : undefined;
}
