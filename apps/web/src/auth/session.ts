/**
 * Browser session against the P06 auth API (docs/status/tasks/P06.md §6):
 * - `POST /api/v1/auth/login` → access token (15 min, body) + rotating refresh token (httpOnly SameSite=Strict cookie
 *   scoped to `/api/v1/auth`, invisible to scripts).
 * - `POST /api/v1/auth/refresh` → new access token, the cookie is rotated. Replaying a used refresh token revokes the
 *   whole family, so refreshes are serialised across tabs (Web Locks) and within a tab (single flight).
 * - The access token lives in memory only (never localStorage): a reload restores the session through the cookie.
 *
 * Framework-free so the logic is unit-testable; `AuthProvider` exposes it to React.
 */

export type Role = 'admin' | 'operator' | 'readonly';

export interface SessionUser {
  id: number;
  username: string;
  role: Role;
}

export type SessionStatus = 'unknown' | 'anonymous' | 'authenticated';

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

export const AUTH_PREFIX = '/api/v1/auth/';
const REFRESH_LOCK = 'vrx-auth-refresh';

type Fetch = (input: Request) => Promise<Response>;

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

export class Session {
  private token: string | null = null;
  private expiresAt = 0;
  private stateValue: SessionState = { status: 'unknown', user: null, endReason: undefined };
  private readonly listeners = new Set<(s: SessionState) => void>();
  private refreshing: Promise<boolean> | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private readonly fetchImpl: Fetch = defaultFetch,
    private readonly now: () => number = () => Date.now(),
  ) {}

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

  /** Page load: try the refresh cookie once. */
  async restore(): Promise<void> {
    const ok = await this.refresh();
    if (!ok && this.stateValue.status === 'unknown') this.setAnonymous(undefined);
  }

  async login(username: string, password: string): Promise<LoginFailure | null> {
    let res: Response;
    try {
      res = await this.fetchImpl(
        new Request(`${AUTH_PREFIX}login`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ username, password }),
        }),
      );
    } catch {
      return { status: 0, detail: undefined };
    }
    if (!res.ok) return { status: res.status, detail: await problemDetail(res) };
    const body: unknown = await res.json();
    if (!isSessionBody(body)) return { status: 502, detail: undefined };
    this.accept(body);
    return null;
  }

  async logout(): Promise<void> {
    try {
      await this.fetchImpl(new Request(`${AUTH_PREFIX}logout`, { method: 'POST', credentials: 'include' }));
    } catch {
      // the cookie is revoked server-side on the next successful call anyway; locally we forget the token
    }
    this.setAnonymous('signedOut');
  }

  /**
   * Rotate the refresh cookie; single flight within the tab, Web Lock across tabs. `false` = the session is over
   * (the caller shows the login page), except for network errors, which keep the current state (reconnecting).
   */
  refresh(): Promise<boolean> {
    this.refreshing ??= withCrossTabLock(async () => {
      let res: Response;
      try {
        res = await this.fetchImpl(new Request(`${AUTH_PREFIX}refresh`, { method: 'POST', credentials: 'include' }));
      } catch {
        // server unreachable: keep the session (the token may still be valid when it comes back), retry later
        if (this.stateValue.status === 'authenticated') this.schedule(15_000);
        return false;
      }
      if (!res.ok) {
        if (this.stateValue.status !== 'anonymous') this.setAnonymous(this.stateValue.status === 'authenticated' ? 'expired' : undefined);
        return false;
      }
      const body: unknown = await res.json();
      if (!isSessionBody(body)) return false;
      this.accept(body);
      return true;
    }).finally(() => {
      this.refreshing = null;
    });
    return this.refreshing;
  }

  /** The API answered 401 to a request that carried our token: refresh once; `true` = retry the request. */
  async handleUnauthorized(tokenUsed: string | null): Promise<boolean> {
    if (this.stateValue.status !== 'authenticated') return false;
    // another request already refreshed while this one was in flight
    if (tokenUsed !== this.token && this.token !== null) return true;
    return this.refresh();
  }

  /** Tear down timers (tests, hot reload). */
  dispose(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.listeners.clear();
  }

  private accept(body: SessionBody): void {
    this.token = body.accessToken;
    this.expiresAt = this.now() + body.expiresIn * 1000;
    // refresh at 80 % of the lifetime (the relay closes sockets at expiry; the WS client reconnects with the new token)
    this.schedule(Math.max(5_000, body.expiresIn * 800));
    this.setState({ status: 'authenticated', user: body.user, endReason: undefined });
  }

  private schedule(ms: number): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.refresh();
    }, ms);
  }

  private setAnonymous(reason: EndReason): void {
    this.token = null;
    this.expiresAt = 0;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.setState({ status: 'anonymous', user: null, endReason: reason });
  }

  /** Milliseconds until the access token expires (0 when there is none). */
  get remainingMs(): number {
    return Math.max(0, this.expiresAt - this.now());
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
