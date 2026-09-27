import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { authorizationHeader, FetchError, fetchList } from './fetch.js';

// F-global-blocking fetcher against a local HTTP server: success with ETag, 304 on the conditional request, 404,
// redirect not followed, timeout, oversize (declared and streamed), Authorization header.

let server: http.Server;
let base: string;
const seen: http.IncomingHttpHeaders[] = [];

beforeAll(async () => {
  server = http.createServer((req, res) => {
    seen.push(req.headers);
    switch (req.url) {
      case '/list.txt':
        if (req.headers['if-none-match'] === '"v1"') {
          res.writeHead(304).end();
          return;
        }
        res
          .writeHead(200, { etag: '"v1"', 'last-modified': 'Sun, 27 Sep 2026 00:00:00 GMT' })
          .end('192.0.2.7\n# c\n10.0.0.0/8\n');
        return;
      case '/missing':
        res.writeHead(404).end('no');
        return;
      case '/moved':
        res.writeHead(302, { location: '/list.txt' }).end();
        return;
      case '/slow':
        setTimeout(() => res.writeHead(200).end('192.0.2.1\n'), 2_000);
        return;
      case '/big-declared':
        res.writeHead(200, { 'content-length': String(2048) }).end('x'.repeat(2048));
        return;
      case '/big-chunked':
        res.writeHead(200);
        for (let i = 0; i < 8; i++) res.write('y'.repeat(512));
        res.end();
        return;
      default:
        res.writeHead(500).end();
    }
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});

afterAll(() => {
  server.closeAllConnections();
  server.close();
});

describe('fetchList', () => {
  it('downloads with ETag/Last-Modified and answers 304 to the conditional request', async () => {
    const r = await fetchList(`${base}/list.txt`, { verifyTls: true, authorization: 'Bearer t0k' });
    expect(r).toMatchObject({ status: 'ok', text: '192.0.2.7\n# c\n10.0.0.0/8\n', etag: '"v1"' });
    expect(seen.at(-1)?.['authorization']).toBe('Bearer t0k');
    const again = await fetchList(`${base}/list.txt`, { verifyTls: true, etag: '"v1"' });
    expect(again).toEqual({ status: 'not-modified' });
  });

  it('refuses HTTP errors, redirects, slow servers and oversize files', async () => {
    await expect(fetchList(`${base}/missing`, { verifyTls: true })).rejects.toThrow('HTTP 404');
    await expect(fetchList(`${base}/moved`, { verifyTls: true })).rejects.toThrow(
      'redirects are not followed',
    );
    await expect(fetchList(`${base}/slow`, { verifyTls: true, timeoutMs: 200 })).rejects.toThrow(
      'within 200 ms',
    );
    await expect(
      fetchList(`${base}/big-declared`, { verifyTls: true, maxBytes: 1024 }),
    ).rejects.toThrow('2048 bytes');
    await expect(
      fetchList(`${base}/big-chunked`, { verifyTls: true, maxBytes: 1024 }),
    ).rejects.toBeInstanceOf(FetchError);
    await expect(fetchList('ftp://example.org/x', { verifyTls: true })).rejects.toThrow(
      'unsupported scheme',
    );
    await expect(fetchList('http://127.0.0.1:1/x', { verifyTls: true })).rejects.toBeInstanceOf(
      FetchError,
    );
  });

  it('builds the Authorization header from the secret kind', () => {
    expect(authorizationHeader('token/feed', ' abc \n')).toBe('Bearer abc');
    expect(authorizationHeader('password/feed', 'u:p')).toBe(
      `Basic ${Buffer.from('u:p').toString('base64')}`,
    );
  });
});
