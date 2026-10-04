import fastifyWebsocket from '@fastify/websocket';
import Fastify from 'fastify';
import { randomBytes } from 'node:crypto';
import { request } from 'node:https';
import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Duplex } from 'node:stream';
import { connect, createServer, type Server, type TLSSocket } from 'node:tls';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { registerStreamRoute } from '../../telemetry/stream.route.js';
import { Bus } from '../../infra/bus.js';
import { MgmtTlsService } from './mgmt-tls.service.js';
import { CERT_POINTER, KEY_POINTER, validateTlsMaterial } from './validator.js';

function hasOpenssl(): boolean {
  try {
    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
    return true;
  } catch {
    return false;
  }
}

const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert

/** Fresh EC P-256 certificates generated per run (no key material is committed to the repository). */
function makePair(dir: string, name: string, cn: string) {
  const key = join(dir, `${name}.key`);
  const crt = join(dir, `${name}.crt`);
  openssl([
    'req',
    '-x509',
    '-newkey',
    'ec',
    '-pkeyopt',
    'ec_paramgen_curve:P-256',
    '-nodes',
    '-keyout',
    key,
    '-out',
    crt,
    '-days',
    '30',
    '-subj',
    `/CN=${cn}`,
    '-addext',
    `subjectAltName=DNS:${cn},IP:192.0.2.1`,
  ]);
  return { cert: readFileSync(crt, 'utf8'), key: readFileSync(key, 'utf8') };
}

describe.skipIf(!hasOpenssl())('management.tls (F-management-ui)', () => {
  let dir: string;
  let a: { cert: string; key: string };
  let b: { cert: string; key: string };
  beforeAll(() => {
    dir = mkdtempSync(join(tmpdir(), 'mgmt-tls-'));
    a = makePair(dir, 'a', 'a.ngfw.test');
    b = makePair(dir, 'b', 'b.ngfw.test');
  });
  afterAll(() => rmSync(dir, { recursive: true, force: true }));

  describe('validateTlsMaterial', () => {
    it('accepts a matching pair and reports public facts only', () => {
      const r = validateTlsMaterial(a.cert, a.key, '1.2');
      expect(r.issues).toEqual([]);
      expect(r.options).toBeDefined();
      expect(r.info?.subject).toBe('CN=a.ngfw.test');
      expect(r.info?.subjectAltNames).toEqual(['DNS:a.ngfw.test', 'IP Address:192.0.2.1']);
      expect(JSON.stringify(r.info)).not.toContain('PRIVATE KEY');
    });

    it('refuses a key that does not match the certificate, at the key pointer, without quoting it', () => {
      const r = validateTlsMaterial(a.cert, b.key, '1.2');
      expect(r.options).toBeUndefined();
      expect(r.issues).toEqual([
        {
          pointer: KEY_POINTER,
          message: 'the private key does not match the certificate',
          rule: 'management.tls.key-mismatch',
        },
      ]);
      expect(JSON.stringify(r)).not.toContain(b.key.split('\n')[1]!);
    });

    it('refuses an expired certificate', () => {
      const r = validateTlsMaterial(a.cert, a.key, '1.2', new Date(Date.now() + 40 * 86_400_000));
      expect(r.issues).toHaveLength(1);
      expect(r.issues[0]).toMatchObject({ pointer: CERT_POINTER, rule: 'management.tls.expired' });
    });

    it('refuses a certificate that is not yet valid', () => {
      const r = validateTlsMaterial(a.cert, a.key, '1.2', new Date(Date.now() - 2 * 86_400_000));
      expect(r.issues[0]).toMatchObject({
        pointer: CERT_POINTER,
        rule: 'management.tls.not-yet-valid',
      });
    });

    it('refuses garbage in either secret', () => {
      expect(validateTlsMaterial('not a pem', a.key, '1.2').issues[0]?.pointer).toBe(CERT_POINTER);
      expect(validateTlsMaterial(a.cert, 'not a key', '1.2').issues[0]?.pointer).toBe(KEY_POINTER);
    });

    it('hot swap to minVersion 1.3 + a new certificate: refuses a TLS 1.2 client and serves a TLS 1.3 one', async () => {
      // the listener starts on 1.2 and is switched to b + 1.3 in place, as MgmtTlsService.reload() does
      const server: Server = createServer(validateTlsMaterial(a.cert, a.key, '1.2').options!);
      server.setSecureContext(validateTlsMaterial(b.cert, b.key, '1.3').options!);
      await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
      const port = (server.address() as { port: number }).port;
      const hello = (maxVersion: 'TLSv1.2' | 'TLSv1.3') =>
        new Promise<string>((resolve) => {
          const s = connect({
            host: '127.0.0.1',
            port,
            servername: 'b.ngfw.test',
            rejectUnauthorized: false,
            maxVersion,
          });
          s.on('secureConnect', () => {
            resolve(`${s.getProtocol() ?? ''} ${s.getPeerX509Certificate()?.subject ?? ''}`);
            s.destroy();
          });
          s.on('error', () => resolve('refused'));
        });
      try {
        expect(await hello('TLSv1.2')).toBe('refused');
        expect(await hello('TLSv1.3')).toBe('TLSv1.3 CN=b.ngfw.test');
      } finally {
        server.close();
      }
    });
  });

  describe('MgmtTlsService', () => {
    function service(doc: unknown, secrets: Record<string, string>) {
      const holder = { doc, revision: 7 };
      const bus = new Bus();
      const ds = {
        getRunning: async () => ({ revision: { id: holder.revision }, doc: holder.doc }),
      };
      const svc = new MgmtTlsService(ds as never, bus, {} as never, {} as never, {} as never);
      svc.secretReader = async (ref) => secrets[ref] ?? null;
      svc.onModuleInit();
      return { svc, holder, bus };
    }
    const tlsDoc = (cert: string, key: string, minVersion = '1.2') => ({
      management: { tls: { certificateRef: cert, privateKeyRef: key, minVersion } },
    });

    it('validate() returns pointer issues for a mismatched pair and none for a good one', async () => {
      const { svc } = service({}, { 'cert/a': a.cert, 'key/a': a.key, 'key/b': b.key });
      expect(await svc.validate(tlsDoc('cert/a', 'key/a'))).toEqual([]);
      const bad = await svc.validate(tlsDoc('cert/a', 'key/b'));
      expect(bad.map((i) => i.pointer)).toEqual([KEY_POINTER]);
      expect(await svc.validate({ management: { tls: { minVersion: '1.2' } } })).toEqual([]);
    });

    it('swaps the secure context on a commit event without a restart; a bad pair keeps the previous one', async () => {
      const secrets = { 'cert/a': a.cert, 'key/a': a.key, 'cert/b': b.cert, 'key/b': b.key };
      const { svc, holder, bus } = service(tlsDoc('cert/a', 'key/a'), secrets);
      const s1 = await svc.state();
      expect(s1.active?.subject).toBe('CN=a.ngfw.test');
      expect(s1.listener).toEqual({ enabled: false, port: null });
      const ctx1 = svc.secureContext();
      expect(ctx1).not.toBeNull();

      holder.doc = tlsDoc('cert/b', 'key/b');
      holder.revision = 8;
      bus.publish('commit.events', { type: 'applied', revision: 8 });
      await new Promise((r) => setTimeout(r, 20));
      const s2 = await svc.state();
      expect(s2.active?.subject).toBe('CN=b.ngfw.test');
      expect(s2.loadedRevision).toBe(8);
      expect(svc.secureContext()).not.toBe(ctx1);

      holder.doc = tlsDoc('cert/a', 'key/b', '1.3');
      holder.revision = 9;
      await svc.reload();
      const s3 = await svc.state();
      expect(s3.active?.subject).toBe('CN=b.ngfw.test');
      expect(s3.error).toContain('does not match');
      expect(s3.loadedRevision).toBe(8);
      expect(s3.minVersion).toBe('1.2');
      expect(JSON.stringify(s3)).not.toContain('PRIVATE KEY');
    });

    it('HTTPS upgrades reuse stream authentication and deliver relay data over the TLS socket', async () => {
      vi.stubEnv('NGFW_HTTPS_PORT', '3201'); // slot 2 test sub-port
      vi.stubEnv('NGFW_HTTP_HOST', '127.0.0.1');
      const fastify = Fastify();
      await fastify.register(fastifyWebsocket);
      const authenticate = vi.fn(async (credential: string | undefined) =>
        credential === 'Bearer test-access' ? { username: 'w2-user', role: 'admin' } : null,
      );
      const attach = vi.fn((socket: { send: (data: string) => void }) =>
        socket.send('w2-stream-delivery'),
      );
      let encrypted = false;
      fastify.addHook('preValidation', async (req) => {
        encrypted = (req.raw.socket as TLSSocket).encrypted;
      });
      await registerStreamRoute(fastify, { authenticate } as never, { attach } as never);
      await fastify.ready();
      const { svc } = service(tlsDoc('cert/a', 'key/a'), {
        'cert/a': a.cert,
        'key/a': a.key,
      });
      // Use the production HTTPS listener, forwarding into the real Fastify stream route/plugin.
      Object.assign(svc, { host: { httpAdapter: { getInstance: () => fastify } } });
      const upgrade = (authorization?: string) =>
        new Promise<{ status: number; body: string; subject?: string }>((resolve, reject) => {
          const req = request({
            hostname: '127.0.0.1',
            port: 3201,
            path: '/api/v1/stream',
            rejectUnauthorized: false,
            agent: false, // rejected upgrades close the socket; do not reuse it for the next attempt
            headers: {
              connection: 'Upgrade',
              upgrade: 'websocket',
              'sec-websocket-version': '13',
              'sec-websocket-key': randomBytes(16).toString('base64'),
              ...(authorization ? { authorization } : {}),
            },
          });
          req.on('error', (error) =>
            reject(new Error(`${authorization ?? 'missing'}: ${error.message}`)),
          );
          req.setTimeout(5_000, () => req.destroy(new Error('upgrade timed out')));
          req.on('response', (res) => {
            let body = '';
            res.on('data', (chunk: Buffer) => {
              body += chunk.toString();
            });
            res.on('end', () => resolve({ status: res.statusCode!, body }));
          });
          req.on('upgrade', (res, socket, head) => {
            const subject = (socket as TLSSocket).getPeerX509Certificate()?.subject;
            const received = (data: Buffer) => {
              socket.destroy();
              resolve({
                status: res.statusCode!,
                body: data.toString(),
                ...(subject === undefined ? {} : { subject }),
              });
            };
            if (head.length > 0) received(head);
            else socket.once('data', received);
          });
          req.end();
        });
      try {
        await svc.onApplicationBootstrap();
        for (const credentials of [undefined, 'Bearer invalid-access']) {
          const refused = await upgrade(credentials);
          expect(refused.status).toBe(401);
          expect(JSON.parse(refused.body)).toMatchObject({ status: 401 });
          expect(attach).not.toHaveBeenCalled();
        }
        const accepted = await upgrade('Bearer test-access');
        expect(accepted.status).toBe(101);
        expect(accepted.subject).toBe('CN=a.ngfw.test');
        expect(accepted.body).toContain('w2-stream-delivery');
        expect(encrypted).toBe(true);
        expect(authenticate.mock.calls.map(([credential]) => credential)).toEqual([
          undefined,
          'Bearer invalid-access',
          'Bearer test-access',
        ]);
        expect(attach).toHaveBeenCalledOnce();
      } finally {
        await svc.onApplicationShutdown();
        await fastify.close();
        vi.unstubAllEnvs();
      }
    });

    it('closes live authenticated HTTPS upgrades on shutdown', async () => {
      vi.stubEnv('NGFW_HTTPS_PORT', '3301'); // slot 3 test sub-port
      vi.stubEnv('NGFW_HTTP_HOST', '127.0.0.1');
      const fastify = Fastify();
      await fastify.register(fastifyWebsocket);
      const authenticate = vi.fn(async (credential: string | undefined) =>
        credential === 'Bearer test-access' ? { username: 'w3-user', role: 'admin' } : null,
      );
      const attach = vi.fn();
      await registerStreamRoute(fastify, { authenticate } as never, { attach } as never);
      await fastify.ready();
      const { svc } = service(tlsDoc('cert/a', 'key/a'), {
        'cert/a': a.cert,
        'key/a': a.key,
      });
      Object.assign(svc, { host: { httpAdapter: { getInstance: () => fastify } } });
      let client: Duplex | undefined;
      let deadline: ReturnType<typeof setTimeout> | undefined;
      let stopping: Promise<void> | undefined;
      let releaseReload: (() => void) | undefined;
      const upgradeRequest = () =>
        request({
          hostname: '127.0.0.1',
          port: 3301,
          path: '/api/v1/stream',
          rejectUnauthorized: false,
          agent: false,
          headers: {
            connection: 'Upgrade',
            upgrade: 'websocket',
            'sec-websocket-version': '13',
            'sec-websocket-key': randomBytes(16).toString('base64'),
            authorization: 'Bearer test-access',
          },
        });
      try {
        await svc.onApplicationBootstrap();
        client = await new Promise<Duplex>((resolve, reject) => {
          const req = upgradeRequest();
          req.on('error', reject);
          req.on('response', (res) => {
            res.resume();
            reject(new Error(`upgrade refused: ${res.statusCode}`));
          });
          req.setTimeout(5_000, () => req.destroy(new Error('upgrade timed out')));
          req.on('upgrade', (res, socket) => {
            req.setTimeout(0);
            socket.setTimeout(0);
            expect(res.statusCode).toBe(101);
            expect((socket as TLSSocket).encrypted).toBe(true);
            resolve(socket);
          });
          req.end();
        });
        expect(attach).toHaveBeenCalledOnce();
        expect(client.destroyed).toBe(false);
        let enteredReload!: () => void;
        const reading = new Promise<void>((resolve) => {
          enteredReload = resolve;
        });
        const blocked = new Promise<void>((resolve) => {
          releaseReload = resolve;
        });
        svc.secretReader = async (ref) => {
          if (ref === 'cert/a') {
            enteredReload();
            await blocked;
          }
          return ref === 'cert/a' ? a.cert : a.key;
        };
        const reloading = svc.reload();
        await reading;
        const closed = new Promise<void>((resolve) => client!.once('close', () => resolve()));
        stopping = svc.onApplicationShutdown();
        // While shutdown waits for an in-flight secret read, a new upgrade must not attach.
        await new Promise<void>((resolve, reject) => {
          const req = upgradeRequest();
          req.on('upgrade', (_, socket) => {
            socket.destroy();
            reject(new Error('new stream accepted during shutdown'));
          });
          req.on('response', (res) => {
            res.resume();
            reject(new Error(`unexpected response during shutdown: ${res.statusCode}`));
          });
          req.on('error', (error: NodeJS.ErrnoException) => {
            if (error.code === 'ECONNRESET') resolve();
            else reject(error);
          });
          req.setTimeout(1_000, () => req.destroy(new Error('shutdown upgrade timed out')));
          req.end();
        });
        expect(attach).toHaveBeenCalledOnce();
        releaseReload!();
        await reloading;
        await Promise.race([
          Promise.all([stopping, closed]),
          new Promise<never>((_, reject) => {
            deadline = setTimeout(
              () => reject(new Error('live HTTPS upgrade survived shutdown')),
              1_000,
            );
          }),
        ]);
        expect(client.destroyed).toBe(true);
        expect((await svc.state()).listener.enabled).toBe(false);
      } finally {
        if (deadline) clearTimeout(deadline);
        client?.destroy();
        releaseReload?.();
        await stopping;
        await svc.onApplicationShutdown();
        await fastify.close();
        vi.unstubAllEnvs();
      }
    });

    it('starts HTTPS when a certificate is configured after an empty bootstrap and reports actual listening state', async () => {
      vi.stubEnv('NGFW_HTTPS_PORT', '3202'); // slot 2 test sub-port
      vi.stubEnv('NGFW_HTTP_HOST', '127.0.0.1');
      const fastify = Fastify();
      fastify.get('/w2-health', async () => ({ ok: true }));
      await fastify.ready();
      const { svc, holder } = service({}, { 'cert/a': a.cert, 'key/a': a.key });
      Object.assign(svc, { host: { httpAdapter: { getInstance: () => fastify } } });
      try {
        await svc.onApplicationBootstrap();
        expect((await svc.state()).listener).toEqual({ enabled: false, port: 3202 });
        holder.doc = tlsDoc('cert/a', 'key/a');
        await svc.reload();
        expect((await svc.state()).listener).toEqual({ enabled: true, port: 3202 });
        const peer = () =>
          new Promise<string | undefined>((resolve, reject) => {
            const req = request(
              {
                hostname: '127.0.0.1',
                port: 3202,
                path: '/w2-health',
                agent: false,
                rejectUnauthorized: false,
              },
              (res) => {
                const subject = (res.socket as TLSSocket).getPeerX509Certificate()?.subject;
                res.resume();
                res.on('end', () => resolve(subject));
              },
            );
            req.on('error', reject);
            req.end();
          });
        expect(await peer()).toBe('CN=a.ngfw.test');
        expect(await svc.validate({})).toMatchObject([
          { pointer: CERT_POINTER, rule: 'management.tls.active-listener' },
        ]);
        holder.doc = {};
        holder.revision = 8;
        await svc.reload();
        expect(await svc.state()).toMatchObject({
          configured: false,
          active: null,
          listener: { enabled: false },
          loadedRevision: 8,
        });
        expect(svc.secureContext()).toBeNull();
        await expect(peer()).rejects.toThrow();
        holder.doc = tlsDoc('cert/a', 'key/a');
        await svc.reload();
        expect(await peer()).toBe('CN=a.ngfw.test');
        await svc.onApplicationShutdown();
        expect((await svc.state()).listener.enabled).toBe(false);
      } finally {
        await svc.onApplicationShutdown();
        await fastify.close();
        vi.unstubAllEnvs();
      }
    });

    it('reports a failed bind as disabled and retries when the configured HTTPS port becomes available', async () => {
      vi.stubEnv('NGFW_HTTPS_PORT', '3203'); // slot 2 test sub-port
      vi.stubEnv('NGFW_HTTP_HOST', '127.0.0.1');
      const blocker = createServer(validateTlsMaterial(a.cert, a.key, '1.2').options!);
      await new Promise<void>((resolve) => blocker.listen(3203, '127.0.0.1', resolve));
      const fastify = Fastify();
      await fastify.ready();
      const { svc, holder } = service({}, { 'cert/a': a.cert, 'key/a': a.key });
      Object.assign(svc, { host: { httpAdapter: { getInstance: () => fastify } } });
      try {
        await svc.onApplicationBootstrap();
        holder.doc = tlsDoc('cert/a', 'key/a');
        await svc.reload();
        expect((await svc.state()).listener.enabled).toBe(false);
        expect((await svc.state()).error).toContain('EADDRINUSE');
        await new Promise<void>((resolve) => blocker.close(() => resolve()));
        await svc.reload();
        expect(await svc.state()).toMatchObject({
          listener: { enabled: true, port: 3203 },
          error: null,
        });
      } finally {
        await svc.onApplicationShutdown();
        await new Promise<void>((resolve) => blocker.close(() => resolve()));
        await fastify.close();
        vi.unstubAllEnvs();
      }
    });

    it('serializes a slow previous reload before applying the newer running certificate', async () => {
      const { svc, holder } = service(tlsDoc('cert/a', 'key/a'), {});
      let release!: () => void;
      let started!: () => void;
      const blocked = new Promise<void>((resolve) => {
        release = resolve;
      });
      const reading = new Promise<void>((resolve) => {
        started = resolve;
      });
      svc.secretReader = async (ref) => {
        if (ref === 'cert/a') {
          started();
          await blocked;
        }
        return (
          (
            { 'cert/a': a.cert, 'key/a': a.key, 'cert/b': b.cert, 'key/b': b.key } as Record<
              string,
              string
            >
          )[ref] ?? null
        );
      };
      const oldReload = svc.reload();
      await reading;
      holder.doc = tlsDoc('cert/b', 'key/b');
      holder.revision = 8;
      const newReload = svc.reload();
      release();
      await Promise.all([oldReload, newReload]);
      expect(await svc.state()).toMatchObject({
        loadedRevision: 8,
        active: { subject: 'CN=b.ngfw.test' },
      });
    });

    it('never returns secret-reader exception text in TLS state', async () => {
      const { svc } = service(tlsDoc('cert/a', 'key/a'), {});
      svc.secretReader = async () => {
        throw new Error('sensitive-decrypt-payload');
      };
      const state = await svc.state();
      expect(state.error).toBe('could not read TLS certificate or key from the secret store');
      expect(await svc.validate(tlsDoc('cert/a', 'key/a'))).toMatchObject([
        { rule: 'management.tls.secret-store' },
      ]);
      expect(JSON.stringify(state)).not.toContain('sensitive-decrypt-payload');
    });

    it('no refs → no certificate, honest state', async () => {
      const { svc } = service({}, {});
      const s = await svc.state();
      expect(s).toMatchObject({ configured: false, active: null, error: null });
      expect(svc.secureContext()).toBeNull();
    });
  });
});
