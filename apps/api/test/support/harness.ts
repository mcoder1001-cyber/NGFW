import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { sql } from 'drizzle-orm';
import { randomBytes } from 'node:crypto';
import { rmSync } from 'node:fs';
import { inject } from 'vitest';
import { createApp } from '../../src/app.js';
import { AuthService } from '../../src/auth/auth.service.js';
import { TokensService } from '../../src/auth/tokens.service.js';
import { hashPassword } from '../../src/auth/password.js';
import { CommitService } from '../../src/commit/commit.service.js';
import { loadEnv, type Env } from '../../src/config.js';
import { DB, runMigrations, type Db } from '../../src/db/db.js';
import { RelayService } from '../../src/telemetry/relay.service.js';
import { FakeAgent } from '../../src/testing/fake-agent.js';

export interface Harness {
  app: NestFastifyApplication;
  fake: FakeAgent;
  env: Env;
  db: Db;
  prefix: string;
  adminPassword: string;
  /** Access token of a user. */
  login(username: string, password: string): Promise<string>;
  /** `app.inject` with a bearer token and JSON body. */
  call(
    token: string | undefined,
    method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE',
    url: string,
    body?: unknown,
    headers?: Record<string, string>,
  ): Promise<{ status: number; body: any; headers: Record<string, unknown>; raw: string }>;
  /** Commit management.users (admin) with argon2id hashes for the given passwords. */
  createUsers(
    token: string,
    users: { username: string; role: string; password: string }[],
  ): Promise<void>;
  close(): Promise<void>;
}

/** A secret for this run only — generated, never written to a file. */
export function runSecret(): string {
  return randomBytes(18).toString('base64url');
}

/**
 * One API instance per test file: fresh schema in the slot database (dropped + migrated), the slot's Valkey db with
 * the e2e key prefix, an in-process fake agent on /run/ngfw-test/<prefix>/fake-agent.sock (the slot's real agent
 * socket stays free for the integration test), a random bootstrap admin password.
 */
export async function startHarness(overrides: Record<string, string> = {}): Promise<Harness> {
  const prefix = inject('prefix');
  const runDir = inject('runDir');
  const socket = `${runDir}/fake-agent.sock`;
  const keyFile = `${runDir}/secret.key`;
  const fake = new FakeAgent({ owner: prefix });
  await fake.start(socket);
  const adminPassword = runSecret();
  const env = loadEnv({
    NGFW_DATABASE_URL: inject('pgDsn'),
    NGFW_VALKEY_DB: String(inject('valkeyDb')),
    NGFW_VALKEY_PREFIX: `ngfw:${prefix}:e2e:${randomBytes(3).toString('hex')}:`,
    NGFW_AGENT_SOCKET: socket,
    NGFW_AGENT_OWNER: prefix,
    NGFW_AGENT_TIMEOUT_MS: '5000',
    NGFW_JWT_SECRET: runSecret() + runSecret(),
    NGFW_SECRET_KEY_FILE: keyFile,
    NGFW_BOOTSTRAP_ADMIN_PASSWORD: adminPassword,
    NGFW_HTTP_PORT: process.env['NGFW_HTTP_PORT'] ?? '3100',
    NGFW_WS_HEARTBEAT_MS: '500',
    ...overrides,
  });
  const app = await createApp({ env, logger: ['error'] });
  const db = app.get<Db>(DB);
  await db.execute(sql`drop schema if exists drizzle cascade`);
  await db.execute(sql`drop schema if exists public cascade`);
  await db.execute(sql`create schema public`);
  await runMigrations(db);
  await app.get(AuthService).seedBootstrapAdmin();
  await app.get(TokensService).loadRevocations();
  await app.init();
  await app.getHttpAdapter().getInstance().ready();
  await app.get(CommitService).resumePending();
  app.get(RelayService).start();

  const call: Harness['call'] = async (token, method, url, body, headers = {}) => {
    const res = await app.inject({
      method,
      url,
      headers: { ...(token ? { authorization: `Bearer ${token}` } : {}), ...headers },
      ...(body === undefined ? {} : { payload: body as object }),
    });
    let parsed: unknown = undefined;
    try {
      parsed = res.body ? JSON.parse(res.body) : undefined;
    } catch {
      parsed = res.body;
    }
    return { status: res.statusCode, body: parsed as any, headers: res.headers, raw: res.body };
  };
  const login = async (username: string, password: string) => {
    const r = await call(undefined, 'POST', '/api/v1/auth/login', { username, password });
    if (r.status !== 200) throw new Error(`login ${username}: ${r.status} ${r.raw}`);
    return r.body.accessToken as string;
  };
  return {
    app,
    fake,
    env,
    db,
    prefix,
    adminPassword,
    login,
    call,
    async createUsers(token, users) {
      const list = [
        { username: env.NGFW_BOOTSTRAP_ADMIN_USER, role: 'admin' },
        ...(await Promise.all(
          users.map(async (u) => ({
            username: u.username,
            role: u.role,
            passwordHash: await hashPassword(u.password),
          })),
        )),
      ];
      const put = await call(token, 'PUT', '/api/v1/config/management/users', list);
      if (put.status !== 200) throw new Error(`PUT users: ${put.status} ${put.raw}`);
      const c = await call(token, 'POST', '/api/v1/config/commit?comment=users');
      if (c.status !== 200) throw new Error(`commit users: ${c.status} ${c.raw}`);
    },
    async close() {
      await app.close();
      await fake.stop();
      rmSync(keyFile, { force: true });
    },
  };
}
