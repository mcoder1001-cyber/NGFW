import { sql } from 'drizzle-orm';
import type { Harness } from './harness.js';

/**
 * TD-10b e2e helpers: a request as it arrives through the local trusted proxy (the product nginx, tools/app's vite
 * with xfwd) for the browser at `client` — socket peer 127.0.0.1, X-Forwarded-For/-Proto set as the proxy sets them.
 */
export interface ViaOpts {
  client: string;
  /** how the browser reached the proxy (default https: the product nginx) */
  proto?: 'http' | 'https';
  token?: string;
  cookie?: string;
  headers?: Record<string, string>;
}

export async function via(
  h: Harness,
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE',
  url: string,
  body: unknown,
  o: ViaOpts,
): Promise<{ status: number; body: any; headers: Record<string, unknown>; raw: string }> {
  const res = await h.app.inject({
    method,
    url,
    remoteAddress: '127.0.0.1',
    headers: {
      'x-forwarded-for': o.client,
      'x-forwarded-proto': o.proto ?? 'https',
      ...(o.token ? { authorization: `Bearer ${o.token}` } : {}),
      ...(o.cookie ? { cookie: o.cookie } : {}),
      ...o.headers,
    },
    ...(body === undefined ? {} : { payload: body as object }),
  });
  let parsed: unknown;
  try {
    parsed = res.body ? JSON.parse(res.body) : undefined;
  } catch {
    parsed = res.body;
  }
  return { status: res.statusCode, body: parsed as any, headers: res.headers, raw: res.body };
}

/** `name=value` of a Set-Cookie header of the response. */
export function cookieOf(headers: Record<string, unknown>, name: string): string | undefined {
  const raw = [headers['set-cookie']].flat().find((c) => String(c).startsWith(`${name}=`));
  return raw === undefined ? undefined : String(raw).split(';')[0];
}

/** The whole Set-Cookie header (attributes included, lower-cased) of cookie `name`. */
export function cookieAttrs(headers: Record<string, unknown>, name: string): string | undefined {
  const raw = [headers['set-cookie']].flat().find((c) => String(c).startsWith(`${name}=`));
  return raw === undefined ? undefined : String(raw).toLowerCase();
}

/** `after->>'reason'` of the audit rows of `action` for `username` (null = user-less rows), oldest first. */
export async function auditReasons(h: Harness, action: string, username: string | null) {
  const rows = await h.db.execute(
    username === null
      ? sql`select after->>'reason' as reason from audit_log where action = ${action} and username is null order by id`
      : sql`select after->>'reason' as reason from audit_log where action = ${action} and username = ${username} order by id`,
  );
  return rows.rows.map((r) => r['reason'] as string | null);
}

/** Poll until `fn` is truthy (audit rows written without awaiting), at most `ms`. */
export async function eventually<T>(fn: () => Promise<T>, ms = 3000): Promise<T> {
  const end = Date.now() + ms;
  for (;;) {
    const v = await fn();
    if (v || Date.now() > end) return v;
    await new Promise((r) => setTimeout(r, 50));
  }
}
