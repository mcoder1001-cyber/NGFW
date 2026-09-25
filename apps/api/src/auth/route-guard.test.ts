import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import type { FastifyInstance, RouteOptions } from 'fastify';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createApp } from '../app.js';
import { loadEnv } from '../config.js';
import { TokensService } from './tokens.service.js';

/**
 * P06 acceptance: "no route without an auth guard". Every route Fastify knows (Nest controllers AND the raw WebSocket
 * route) is enumerated through the onRoute hook and called WITHOUT credentials: all must answer 401 problem+json,
 * except the reviewed public allow-list below. A new route therefore cannot ship unguarded, and a new @Public() route
 * fails this test until it is added here on purpose. The app is built offline (no DB/Valkey/agent is contacted: the
 * guard rejects before any handler runs).
 */
const PUBLIC = new Set([
  'GET /api/v1/health',
  'POST /api/v1/auth/login',
  'POST /api/v1/auth/refresh',
  'POST /api/v1/auth/logout',
  // Feature public routes: one line under the feature's anchor (SY1).
  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-aaa
  // wave-BC: F-restconf-yang
]);

/** Mutations a readonly user may call (own credentials only). */
const READONLY_MAY = new Set(['POST /api/v1/auth/password', 'POST /api/v1/users/:name/password']);
/** Routes that need the admin role (@MinRole('admin')). */
const ADMIN_ONLY = new Set([
  'DELETE /api/v1/config/lock',
  'GET /api/v1/audit',
  'POST /api/v1/secrets',
  'DELETE /api/v1/secrets/:kind/:name',
  // Feature admin-only routes: one line under the feature's anchor (SY1).
  // wave-BC: F-aaa
  // wave-BC: F-backup-restore
  'PUT /api/v1/system/license', // F-licensing (unanchored, added by manager at merge)
]);

function concrete(url: string): string {
  return url.replace(/:[A-Za-z_]+/g, '1').replace(/\*$/, 'interfaces/loop0');
}

describe('route guard', () => {
  let app: NestFastifyApplication;
  const routes: { method: string; url: string }[] = [];

  beforeAll(async () => {
    app = await createApp({
      env: loadEnv({}),
      logger: false,
      onRoute: (r: RouteOptions) => {
        for (const m of [r.method].flat())
          if (m !== 'HEAD' && m !== 'OPTIONS') routes.push({ method: m, url: r.url });
      },
    });
    const fastify = app.getHttpAdapter().getInstance() as unknown as FastifyInstance;
    await app.init();
    await fastify.ready();
  });
  afterAll(async () => app.close());

  it('sees every controller and the WebSocket route', () => {
    const keys = routes.map((r) => `${r.method} ${r.url}`);
    expect(keys).toContain('GET /api/v1/stream');
    expect(keys).toContain('PATCH /api/v1/config/*');
    expect(keys).toContain('POST /api/v1/config/commit');
    expect(keys).toContain('POST /api/v1/actions/:action');
    // the OpenAPI UI/JSON registered by createApp() (review L1)
    expect(keys).toContain('GET /api/docs-json');
    expect(keys.some((k) => k.startsWith('GET /api/docs'))).toBe(true);
    expect(routes.length).toBeGreaterThan(30);
  });

  it('answers 401 problem+json on every non-public route without credentials', async () => {
    const unguarded: string[] = [];
    for (const r of routes) {
      const key = `${r.method} ${r.url}`;
      if (PUBLIC.has(key)) continue;
      const res = await app.inject({
        method: r.method as 'GET',
        url: concrete(r.url),
        ...(r.method === 'GET' || r.method === 'DELETE' ? {} : { payload: {} }),
      });
      if (
        res.statusCode !== 401 ||
        !String(res.headers['content-type']).startsWith('application/problem+json')
      ) {
        unguarded.push(`${key} → ${res.statusCode}`);
      }
    }
    expect(unguarded).toEqual([]);
  });

  it('rejects forged and garbage credentials the same way', async () => {
    for (const authorization of [
      'Bearer abc.def.ghi',
      'ApiKey vrxk_nope',
      'Basic YWRtaW46YWRtaW4=',
      'Bearer',
    ]) {
      const res = await app.inject({
        method: 'GET',
        url: '/api/v1/config',
        headers: { authorization },
      });
      expect(res.statusCode).toBe(401);
    }
  });

  it('role matrix: readonly may not mutate, operator may not use admin routes (403 problem+json)', async () => {
    const tokens = app.get(TokensService);
    const ro = await tokens.signAccess({ id: 9001, username: 'ro-matrix', role: 'readonly' });
    const op = await tokens.signAccess({ id: 9002, username: 'op-matrix', role: 'operator' });
    const wrong: string[] = [];
    const expect403 = async (token: string, r: { method: string; url: string }) => {
      const res = await app.inject({
        method: r.method as 'GET',
        url: concrete(r.url),
        headers: { authorization: `Bearer ${token}` },
        ...(r.method === 'GET' || r.method === 'DELETE' ? {} : { payload: {} }),
      });
      if (res.statusCode !== 403) wrong.push(`${r.method} ${r.url} → ${res.statusCode}`);
    };
    for (const r of routes) {
      const key = `${r.method} ${r.url}`;
      if (PUBLIC.has(key)) continue;
      if (r.method !== 'GET' && !READONLY_MAY.has(key)) await expect403(ro, r);
      if (ADMIN_ONLY.has(key)) await expect403(op, r);
    }
    expect(wrong).toEqual([]);
    // every admin-only route in the list really exists
    const keys = new Set(routes.map((r) => `${r.method} ${r.url}`));
    for (const k of ADMIN_ONLY) expect(keys.has(k), k).toBe(true);
  });

  it('serves the API description only with credentials', async () => {
    const token = await app
      .get(TokensService)
      .signAccess({ id: 9001, username: 'ro-matrix', role: 'readonly' });
    expect((await app.inject({ method: 'GET', url: '/api/docs-json' })).statusCode).toBe(401);
    const ok = await app.inject({
      method: 'GET',
      url: '/api/docs-json',
      headers: { authorization: `Bearer ${token}` },
    });
    expect(ok.statusCode).toBe(200);
    expect(ok.json()).toHaveProperty('openapi', '3.1.0');
  });

  it('keeps the public list exactly as reviewed', () => {
    const present = routes.map((r) => `${r.method} ${r.url}`).filter((k) => PUBLIC.has(k));
    expect(new Set(present)).toEqual(PUBLIC);
  });
});
