import { createPublicKey, createPrivateKey } from 'node:crypto';
import { EventKind } from '@ngfw/proto';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { setFakeWireguardPeer } from '../../src/features/wireguard/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-wireguard API on the host PostgreSQL with the fake agent: the key-pair action (private key stored as a secret,
 * never returned), vpn.wireguard through the generic config routes (duplicate public key on two interfaces → 400
 * problem+json with the pointer of the second peer), `GET /state/vpn/wireguard` joined with the running names, RBAC.
 */
describe('F-wireguard e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let ro: string;
  let pubA = '';
  let pubB = '';
  const PEER_PUB = 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=';

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  it('key pair: stores key/<name>, returns the reference and the public key only (admin)', async () => {
    const r = await h.call(admin, 'POST', '/api/v1/actions/vpn/wireguard/keypair', {
      name: 'w1-site-a',
    });
    expect(r.status).toBe(200);
    expect(Object.keys(r.body).sort()).toEqual(['publicKey', 'ref', 'version']);
    expect(r.body.ref).toBe('key/w1-site-a');
    expect(r.body.publicKey).toMatch(/^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$/);
    pubA = r.body.publicKey;
    const b = await h.call(admin, 'POST', '/api/v1/actions/vpn/wireguard/keypair', {
      name: 'w1-site-b',
    });
    pubB = b.body.publicKey;
    expect(pubB).not.toBe(pubA);
    // the stored value is the private key of that public key (read through the service, never through the API)
    const { SecretsService } = await import('../../src/secrets/secrets.service.js');
    const secrets = h.app.get(SecretsService);
    const { secret } = await import('../../src/db/schema.js');
    const { eq } = await import('drizzle-orm');
    const [row] = await h.db.select().from(secret).where(eq(secret.ref, 'key/w1-site-a'));
    const priv = secrets.decrypt(row!.ciphertext, 'key/w1-site-a');
    const pk = createPrivateKey({
      key: {
        kty: 'OKP',
        crv: 'X25519',
        d: Buffer.from(priv, 'base64').toString('base64url'),
        x: Buffer.from(pubA, 'base64').toString('base64url'),
      },
      format: 'jwk',
    });
    const x = createPublicKey(pk).export({ format: 'jwk' }).x!;
    expect(Buffer.from(x, 'base64url').toString('base64')).toBe(pubA);
    expect(r.raw).not.toContain(priv);
    // listed as a reference only; a second key pair under the same name is refused
    const list = await h.call(ro, 'GET', '/api/v1/secrets');
    expect(JSON.stringify(list.body)).not.toContain(priv);
    expect((list.body as { ref: string }[]).map((s) => s.ref)).toContain('key/w1-site-a');
    expect(
      (await h.call(admin, 'POST', '/api/v1/actions/vpn/wireguard/keypair', { name: 'w1-site-a' }))
        .status,
    ).toBe(409);
    // RBAC and input validation
    expect(
      (await h.call(op, 'POST', '/api/v1/actions/vpn/wireguard/keypair', { name: 'x' })).status,
    ).toBe(403);
    expect(
      (await h.call(ro, 'POST', '/api/v1/actions/vpn/wireguard/keypair', { name: 'x' })).status,
    ).toBe(403);
    const bad = await h.call(admin, 'POST', '/api/v1/actions/vpn/wireguard/keypair', {
      name: '../x',
    });
    expect(bad.status).toBe(400);
    expect(bad.headers['content-type']).toMatch(/^application\/problem\+json/);
  });

  it('an operator may not set secret references (admin only, D-051)', async () => {
    const r = await h.call(
      op,
      'PATCH',
      '/api/v1/config/vpn/wireguard/interfaces',
      { x: { instance: 1009, listenAddress: '10.1.51.1', privateKeyRef: 'key/w1-site-a' } },
      MP,
    );
    expect(r.status).toBe(403);
  });

  it('duplicate public key on two interfaces → 400 problem+json pointing at the second peer', async () => {
    const wg = {
      a: {
        instance: 1001,
        listenAddress: '10.1.51.1',
        listenPort: 20110,
        privateKeyRef: 'key/w1-site-a',
        peers: { p1: { publicKey: PEER_PUB, allowedIps: ['10.1.53.0/24'] } },
      },
      b: {
        instance: 1002,
        listenAddress: '10.1.51.1',
        listenPort: 20111,
        privateKeyRef: 'key/w1-site-b',
        peers: { dup: { publicKey: PEER_PUB, allowedIps: ['10.1.54.0/24'] } },
      },
    };
    expect(
      (await h.call(admin, 'PATCH', '/api/v1/config/vpn/wireguard/interfaces', wg, MP)).status,
    ).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/vpn/wireguard/interfaces/b/peers/dup/publicKey',
        message: expect.stringMatching(/already used by peer 'p1' of WireGuard interface 'a'/),
      }),
    );
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('commit → state joined with the running names; live status from the agent', async () => {
    const wg = {
      'site-a': {
        instance: 1001,
        description: 'HQ',
        listenAddress: '10.1.51.1',
        listenPort: 20110,
        privateKeyRef: 'key/w1-site-a',
        address: ['10.1.52.1/24'],
        peers: {
          branch: {
            publicKey: pubB,
            endpoint: { address: '10.1.51.2', port: 20111 },
            allowedIps: ['10.1.54.0/24'],
            persistentKeepaliveSec: 25,
          },
        },
      },
    };
    expect(
      (await h.call(admin, 'PATCH', '/api/v1/config/vpn/wireguard/interfaces', wg, MP)).status,
    ).toBe(200);
    // vpn.local-address-configured: the listen address lives on an interface of the underlay VRF
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config/interfaces/loop101',
          { ipv4: ['10.1.51.1/24'] },
          MP,
        )
      ).status,
    ).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=wg');
    expect(c.body.errors ?? []).toEqual([]);
    expect(c.status).toBe(200);
    setFakeWireguardPeer(h.fake, pubB, {
      established: true,
      dead: false,
      at: new Date('2026-09-25T10:00:00Z'),
    });
    h.fake.emit(EventKind.EVENT_KIND_WIREGUARD_PEER_CHANGED, {
      interface: 'wg1001',
      attributes: { public_key: pubB, established: 'true', dead: 'false' },
    });
    const st = await h.call(ro, 'GET', '/api/v1/state/vpn/wireguard');
    expect(st.status).toBe(200);
    expect(st.body).toMatchObject({
      eventsActive: true,
      interfaces: [
        {
          name: 'site-a',
          vppName: 'wg1001',
          instance: 1001,
          listenAddress: '10.1.51.1',
          listenPort: 20110,
          peers: [
            {
              name: 'branch',
              publicKey: pubB,
              status: 'established',
              established: true,
              dead: false,
              endpoint: '10.1.51.2',
              endpointPort: 20111,
              lastHandshake: '2026-09-25T10:00:00.000Z',
              persistentKeepaliveSec: 25,
              allowedIps: ['10.1.54.0/24'],
            },
          ],
        },
      ],
    });
    expect(JSON.stringify(st.body)).not.toMatch(/privateKey|presharedKey"/);
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/vpn/wireguard?interface=wg9')).body.interfaces,
    ).toEqual([]);
    expect((await h.call(ro, 'GET', '/api/v1/state/vpn/wireguard?interface=eth0')).status).toBe(
      400,
    );
    const call = h.fake.calls.filter((x) => x.method === 'WireguardState').at(-1);
    expect(call?.request).toMatchObject({ owner: h.fake.owner });
  });

  it('rollback to the revision before the WireGuard commit removes it (agent and state)', async () => {
    const revs = await h.call(ro, 'GET', '/api/v1/config/revisions?limit=10');
    const items = revs.body.items as { id: number; comment?: string | null }[];
    const wgRev = items.find((r) => r.comment === 'wg');
    expect(wgRev).toBeDefined();
    const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${wgRev!.id - 1}`);
    expect(rb.status).toBe(200);
    expect(rb.body.revision).toMatchObject({ kind: 'rollback' });
    const wg = ((h.fake.current['vpn'] as Record<string, any> | undefined)?.['wireguard']?.[
      'interfaces'
    ] ?? {}) as object;
    expect(Object.keys(wg)).toEqual([]);
    expect((await h.call(ro, 'GET', '/api/v1/state/vpn/wireguard')).body.interfaces).toEqual([]);
  });
});
