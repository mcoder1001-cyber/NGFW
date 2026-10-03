import http from 'node:http';
import https from 'node:https';

/**
 * F-global-blocking: downloads a block-list file from its server URL. https verifies the certificate unless the list
 * says otherwise (a private CA comes from the secret store); the body is capped (`maxBytes`) and the whole exchange is
 * bounded by `timeoutMs`; `If-None-Match` / `If-Modified-Since` from the last good download make an unchanged file a
 * cheap 304. Redirects are not followed (the URL the admin configured is the one that is trusted). Any failure is a
 * FetchError with a short reason — the caller keeps its last good list.
 */

export const MAX_FETCH_BYTES = 20 * 1024 * 1024;
export const FETCH_TIMEOUT_MS = 30_000;

export class FetchError extends Error {}

export interface FetchOptions {
  verifyTls: boolean;
  /** PEM of a private CA (from the `cert/<name>` secret). */
  ca?: string;
  /** `Authorization` header value (Bearer token or Basic user:pass). */
  authorization?: string;
  etag?: string;
  lastModified?: string;
  timeoutMs?: number;
  maxBytes?: number;
}

export type FetchResult =
  | { status: 'ok'; text: string; bytes: number; etag?: string; lastModified?: string }
  | { status: 'not-modified' };

/** `Authorization` from a secret: a `token/` ref is a Bearer token, a `password/` ref holds `user:pass` (Basic). */
export function authorizationHeader(ref: string, value: string): string {
  return ref.startsWith('token/')
    ? `Bearer ${value.trim()}`
    : `Basic ${Buffer.from(value.trim(), 'utf8').toString('base64')}`;
}

export function fetchList(url: string, o: FetchOptions): Promise<FetchResult> {
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return Promise.reject(new FetchError('invalid URL'));
  }
  if (u.protocol !== 'https:' && u.protocol !== 'http:') {
    return Promise.reject(new FetchError(`unsupported scheme ${u.protocol}`));
  }
  const maxBytes = o.maxBytes ?? MAX_FETCH_BYTES;
  const timeoutMs = o.timeoutMs ?? FETCH_TIMEOUT_MS;
  const headers: Record<string, string> = {
    accept: 'text/plain, */*;q=0.5',
    'user-agent': 'ngfw-global-blocking',
  };
  if (o.authorization) headers['authorization'] = o.authorization;
  if (o.etag) headers['if-none-match'] = o.etag;
  if (o.lastModified) headers['if-modified-since'] = o.lastModified;
  return new Promise((resolve, reject) => {
    let settled = false;
    const fail = (why: string) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      req.destroy();
      reject(new FetchError(why));
    };
    const opts: https.RequestOptions = { method: 'GET', headers };
    if (u.protocol === 'https:') {
      opts.rejectUnauthorized = o.verifyTls;
      if (o.ca) opts.ca = o.ca;
    }
    const req = (u.protocol === 'https:' ? https : http).request(u, opts, (res) => {
      const code = res.statusCode ?? 0;
      if (code === 304) {
        res.resume();
        settled = true;
        clearTimeout(timer);
        resolve({ status: 'not-modified' });
        return;
      }
      if (code < 200 || code > 299) {
        res.resume();
        fail(`HTTP ${code}${code >= 300 && code < 400 ? ' (redirects are not followed)' : ''}`);
        return;
      }
      const declared = Number(res.headers['content-length'] ?? NaN);
      if (Number.isFinite(declared) && declared > maxBytes) {
        fail(`the file is ${declared} bytes, more than ${maxBytes}`);
        return;
      }
      const chunks: Buffer[] = [];
      let n = 0;
      res.on('data', (c: Buffer) => {
        n += c.length;
        if (n > maxBytes) {
          fail(`the file is more than ${maxBytes} bytes`);
          return;
        }
        chunks.push(c);
      });
      res.on('error', (e) => fail(e.message));
      res.on('end', () => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        const etag = headerOf(res.headers['etag']);
        const lastModified = headerOf(res.headers['last-modified']);
        resolve({
          status: 'ok',
          text: Buffer.concat(chunks).toString('utf8'),
          bytes: n,
          ...(etag ? { etag } : {}),
          ...(lastModified ? { lastModified } : {}),
        });
      });
    });
    const timer = setTimeout(() => fail(`no complete answer within ${timeoutMs} ms`), timeoutMs);
    req.on('error', (e) => fail(e.message));
    req.end();
  });
}

function headerOf(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}
