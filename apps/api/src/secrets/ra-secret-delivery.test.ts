import { DesiredState, type DryRunRequest } from '@ngfw/proto';
import { createCipheriv, randomBytes } from 'node:crypto';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Db } from '../db/db.js';
import { AgentClient } from '../agent/agent.client.js';
import { FakeAgent } from '../testing/fake-agent.js';
import { toProblem } from '../common/problem.js';
import { testEnv } from '../testing/fixtures.js';
import { SecretDeliveryService } from './secret-delivery.service.js';

const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((p) => rmSync(p, { recursive: true, force: true })));

function fixture() {
  const dir = mkdtempSync(join(tmpdir(), 'ngfw-ra-delivery-'));
  dirs.push(dir);
  const key = randomBytes(32);
  const file = join(dir, 'master');
  writeFileSync(file, key, { mode: 0o600 });
  const where = vi.fn();
  const service = new SecretDeliveryService(
    { select: () => ({ from: () => ({ where }) }) } as unknown as Db,
    { ...testEnv(), NGFW_SECRET_KEY_FILE: file },
  );
  const row = (ref: string, value: Buffer, version = 1, kind = ref.split('/')[0]) => {
    const iv = randomBytes(12);
    const cipher = createCipheriv('aes-256-gcm', key, iv);
    cipher.setAAD(Buffer.from(ref));
    const encrypted = Buffer.concat([cipher.update(value), cipher.final()]);
    return {
      ref,
      kind,
      version,
      ciphertext: Buffer.concat([iv, cipher.getAuthTag(), encrypted]).toString('base64'),
    };
  };
  return { service, where, row };
}

const local = (enabled?: boolean, passwordRef = 'password/ra-user') => ({
  auth: 'eap-mschapv2',
  ...(enabled === undefined ? {} : { enabled }),
  users: [{ username: 'client', passwordRef }],
});
const radius = (secretRef = 'psk/ra-radius') => ({
  auth: 'eap-radius',
  radius: { servers: [{ address: '192.0.2.9', secretRef }] },
});
const state = (remoteAccess: Record<string, unknown>) =>
  DesiredState.fromJSON({ vpn: { remoteAccess } });

describe('remote-access sealed-channel credential selection', () => {
  it.each([true, undefined])(
    'delivers an active local EAP password (enabled=%s)',
    async (enabled) => {
      const { service, where, row } = fixture();
      const value = randomBytes(24);
      where.mockResolvedValueOnce([row('password/ra-user', value, 3)]);
      const result = await service.resolveVersioned(state({ remote: local(enabled) }));
      expect(result.bundle.values).toEqual({ 'password/ra-user': value });
      expect(result.versions).toEqual({ 'password/ra-user': 3 });
      expect(where).toHaveBeenCalledTimes(1);
    },
  );

  it('delivers the RADIUS shared secret and pins its historical revision', async () => {
    const { service, where, row } = fixture();
    const previous = randomBytes(24);
    where
      .mockResolvedValueOnce([row('psk/ra-radius', randomBytes(24), 2)])
      .mockResolvedValueOnce([row('psk/ra-radius', previous, 1)]);
    const result = await service.resolveVersioned(state({ remote: radius() }), {
      'psk/ra-radius': 1,
    });
    expect(result.bundle.values).toEqual({ 'psk/ra-radius': previous });
    expect(result.versions).toEqual({ 'psk/ra-radius': 1 });
  });

  it('carries selected EAP and RADIUS material through the real private Unix gRPC transport', async () => {
    const { service, where, row } = fixture();
    const password = randomBytes(24);
    const shared = randomBytes(24);
    where
      .mockResolvedValueOnce([row('password/ra-user', password)])
      .mockResolvedValueOnce([row('psk/ra-radius', shared)]);
    const socketDir = mkdtempSync(join(tmpdir(), 'ra-sock-'));
    dirs.push(socketDir);
    const socket = join(socketDir, 'agent.sock');
    const fake = new FakeAgent({ owner: 'w1' });
    const agent = new AgentClient(
      testEnv({ NGFW_AGENT_SOCKET: socket, NGFW_AGENT_OWNER: 'w1' }),
      service,
    );
    try {
      await fake.start(socket);
      await agent.dryRun({
        txnId: 'ra-sealed-channel',
        desiredState: state({ local: local(true), radius: radius() }),
        subsystems: ['vpn'],
      });
      const request = fake.calls.find((c) => c.method === 'DryRun')?.request as DryRunRequest;
      expect(request.owner).toBe('w1');
      expect(request.secretBundle?.values).toEqual({
        'password/ra-user': password,
        'psk/ra-radius': shared,
      });
      expect(request.desiredState?.vpn?.remoteAccess.local?.users[0]?.passwordRef).toBe(
        'password/ra-user',
      );
    } finally {
      agent.close();
      await fake.stop();
    }
  });

  it('deduplicates a password shared by users without selecting admin or inactive secrets', async () => {
    const { service, where, row } = fixture();
    const value = randomBytes(24);
    where.mockResolvedValueOnce([row('password/ra-user', value)]);
    const ds = DesiredState.fromJSON({
      vpn: {
        remoteAccess: {
          remote: {
            ...local(true),
            users: [
              { username: 'client', passwordRef: 'password/ra-user' },
              { username: 'other', passwordRef: 'password/ra-user' },
            ],
            radius: radius('psk/inactive').radius,
          },
          draft: local(false, 'password/draft'),
          tls: { ...local(true, 'password/unused'), auth: 'eap-tls' },
        },
      },
      management: { aaa: { radius: { servers: [{ secretRef: 'psk/admin' }] } } },
    });
    expect((await service.resolve(ds)).values).toEqual({ 'password/ra-user': value });
    expect(where).toHaveBeenCalledTimes(1);
  });

  it('keeps disabled local and RADIUS drafts editable without DB or key access', async () => {
    const { service, where } = fixture();
    expect(
      await service.resolve(
        state({ local: local(false), radius: { ...radius(), enabled: false } }),
      ),
    ).toEqual({ values: {} });
    expect(where).not.toHaveBeenCalled();
  });

  it('rejects aggregate unique references before any credential read, with the first overflow pointer', async () => {
    const { service, where } = fixture();
    const users = (count: number, prefix: string) =>
      Array.from({ length: count }, (_, i) => ({
        username: `${prefix}${i}`,
        passwordRef: `password/${prefix}${i}`,
      }));
    const ds = state({
      first: { ...local(true), users: users(512, 'a') },
      second: { ...local(true), users: users(513, 'b') },
    });
    const problem = await service.resolve(ds).catch(toProblem);
    expect(problem).toMatchObject({
      status: 400,
      errors: [
        {
          pointer: '/vpn/remoteAccess/second/users/512/passwordRef',
          rule: 'secrets.transport-reference-limit',
        },
      ],
    });
    expect(where).not.toHaveBeenCalled();
  });

  it('allows exactly 1024 distinct references through the existing bounded channel', async () => {
    const { service, where, row } = fixture();
    const users = Array.from({ length: 1024 }, (_, i) => ({
      username: `client${i}`,
      passwordRef: `password/client${i}`,
    }));
    for (const ref of users.map((u) => u.passwordRef))
      where.mockResolvedValueOnce([row(ref, randomBytes(16))]);
    const result = await service.resolveVersioned(state({ remote: { ...local(true), users } }));
    expect(Object.keys(result.bundle.values)).toHaveLength(1024);
    expect(Object.keys(result.versions)).toHaveLength(1024);
  });

  it.each([47, 48])(
    'enforces the Go sealed snapshot base64-size bound for %i maximum values',
    async (count) => {
      const { service, where, row } = fixture();
      const users = Array.from({ length: count }, (_, i) => ({
        username: `client${i}`,
        passwordRef: `password/client${i}`,
      }));
      for (const ref of users.map((u) => u.passwordRef))
        where.mockResolvedValueOnce([row(ref, randomBytes(64 * 1024))]);
      if (count === 47) {
        const result = await service.resolve(state({ remote: { ...local(true), users } }));
        const json = JSON.stringify(
          Object.fromEntries(
            Object.entries(result.values).map(([ref, value]) => [ref, value.toString('base64')]),
          ),
        );
        expect(Buffer.byteLength(json)).toBeLessThanOrEqual(4 * 1024 * 1024);
      } else {
        const problem = await service
          .resolve(state({ remote: { ...local(true), users } }))
          .catch(toProblem);
        expect(problem).toMatchObject({
          status: 400,
          errors: [{ rule: 'secrets.transport-bundle-limit' }],
        });
        expect(JSON.stringify(problem)).not.toContain('ciphertext');
      }
    },
  );

  it.each([local(true, 'psk/wrong'), radius('password/wrong')])(
    'refuses a wrong credential kind before decrypting',
    async (profile) => {
      const { service, where } = fixture();
      await expect(service.resolve(state({ remote: profile }))).rejects.toThrow(
        'operational secret kind is invalid',
      );
      expect(where).not.toHaveBeenCalled();
    },
  );

  it('refuses missing material and revision without putting credentials in errors or logs', async () => {
    const { service, where, row } = fixture();
    const value = randomBytes(24);
    const log = vi.spyOn(console, 'log');
    const error = vi.spyOn(console, 'error');
    try {
      where.mockResolvedValueOnce([]);
      await expect(service.resolve(state({ remote: local(true) }))).rejects.toThrow(
        'operational secret is unavailable',
      );
      where.mockResolvedValueOnce([row('password/ra-user', value, 2)]).mockResolvedValueOnce([]);
      await expect(
        service.resolveVersioned(state({ remote: local(true) }), { 'password/ra-user': 1 }),
      ).rejects.toThrow('operational secret version is unavailable');
      expect(log).not.toHaveBeenCalled();
      expect(error).not.toHaveBeenCalled();
    } finally {
      log.mockRestore();
      error.mockRestore();
    }
  });
});

describe('failed operational delivery erases decoded material', () => {
  it.each(['missing', 'kind', 'version', 'tag', 'size', 'database'])(
    'erases earlier decoded values after a later %s failure',
    async (failure) => {
      const { service, where, row } = fixture();
      const first = randomBytes(24);
      const firstRow = row('password/first', first);
      const secondRow = row('password/second', randomBytes(failure === 'size' ? 65537 : 24));
      if (failure === 'kind') secondRow.kind = 'psk';
      if (failure === 'tag') {
        const raw = Buffer.from(secondRow.ciphertext, 'base64');
        raw[12] = raw[12]! ^ 1;
        secondRow.ciphertext = raw.toString('base64');
      }
      where.mockResolvedValueOnce([firstRow]);
      if (failure === 'database') where.mockRejectedValueOnce(new Error('database unavailable'));
      else where.mockResolvedValueOnce(failure === 'missing' ? [] : [secondRow]);
      const allocated: Buffer[] = [];
      const concatenate = Buffer.concat;
      const spy = vi.spyOn(Buffer, 'concat').mockImplementation((values, length) => {
        const value = concatenate(values, length);
        allocated.push(value);
        return value;
      });
      try {
        await expect(
          service.resolveVersioned(
            state({ first: local(true, 'password/first'), second: local(true, 'password/second') }),
            failure === 'version' ? { 'password/second': 0 } : undefined,
          ),
        ).rejects.toThrow();
        expect(allocated.length).toBeGreaterThan(0);
        for (const value of allocated) expect(value.every((byte) => byte === 0)).toBe(true);
      } finally {
        spy.mockRestore();
      }
    },
  );

  it('erases unauthenticated GCM update plaintext when final authentication fails', async () => {
    const { service, where, row } = fixture();
    const value = randomBytes(24);
    const encrypted = row('password/ra-user', value);
    const raw = Buffer.from(encrypted.ciphertext, 'base64');
    raw[12] = raw[12]! ^ 1;
    encrypted.ciphertext = raw.toString('base64');
    where.mockResolvedValueOnce([encrypted]);
    const erased: Buffer[] = [];
    const fill = Buffer.prototype.fill;
    const spy = vi.spyOn(Buffer.prototype, 'fill').mockImplementation(function (
      this: unknown,
      ...args: unknown[]
    ) {
      if (args[0] === 0 && Buffer.isBuffer(this) && this.equals(value)) erased.push(this);
      return Reflect.apply(fill, this, args) as Buffer;
    });
    try {
      await expect(service.resolveVersioned(state({ remote: local(true) }))).rejects.toThrow();
      expect(erased).toHaveLength(1);
      expect(erased[0]!.every((byte) => byte === 0)).toBe(true);
    } finally {
      spy.mockRestore();
    }
  });
});

describe('explicit references take precedence over implicit optional CRLs', () => {
  it.each([undefined, {}])(
    'requires an explicit certificate even when its name matches an optional CRL (versions=%s)',
    async (versions) => {
      const { service, where } = fixture();
      where.mockResolvedValueOnce([]);
      await expect(
        service.resolveVersioned(
          DesiredState.fromJSON({
            vpn: { pki: { cas: { example: { certificateRef: 'cert/example.crl' } } } },
          }),
          versions,
        ),
      ).rejects.toThrow();
      expect(where).toHaveBeenCalledTimes(1);
    },
  );
});
