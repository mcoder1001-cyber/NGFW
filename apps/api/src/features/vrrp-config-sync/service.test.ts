import { randomUUID } from 'node:crypto';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import type { CommitService } from '../../commit/commit.service.js';
import type { ConfigRepo, Doc } from '../../datastore/repo.js';
import type { Db } from '../../db/db.js';
import { configRevision } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import type { SecretsService } from '../../secrets/secrets.service.js';
import { ClusterSyncService } from './service.js';
import type * as Transport from './transport.js';
import { signature, type Envelope } from './transport.js';
const send = vi.hoisted(() => vi.fn());
vi.mock('./transport.js', async (importOriginal) => ({
  ...(await importOriginal<typeof Transport>()),
  sendPinned: send,
}));
const KEY = 'NGFW_TEST_PSK_cluster_auth_fixture';
const local = () => ({
  system: { hostname: 'node-b' },
  ha: {
    vrrp: { lan: { enabled: true } },
    cluster: {
      enabled: true,
      nodeName: 'node-b',
      peers: [{ name: 'node-a', address: '192.0.2.1', certificatePin: 'a'.repeat(64) }],
      port: 4370,
      secretRef: 'key/cluster',
      configSync: true,
      syncExclude: [],
    },
  },
  management: { users: [{ username: 'local' }] },
});
describe('cluster delivery lifecycle', () => {
  let previous: string | undefined;
  let svc: ClusterSyncService,
    bus: Bus,
    payload: Doc,
    kind: string,
    pending: unknown,
    repo: ConfigRepo;
  const apply = vi.fn(),
    state = vi.fn(),
    versions = vi.fn();
  beforeEach(() => {
    previous = undefined;
    payload = local();
    kind = 'commit';
    pending = null;
    bus = new Bus();
    send.mockReset();
    send.mockResolvedValue({ revision: 8, role: 'backup' });
    apply.mockReset();
    state.mockReset();
    versions.mockReset();
    versions.mockResolvedValue({ 'key/cluster': 1 });
    state.mockResolvedValue({ routers: [{ name: 'lan', state: 'master', error: '' }] });
    repo = {
      latestRevision: async () => ({ id: 7, kind, payload }),
      pending: async () => pending,
      secretVersions: versions,
    } as unknown as ConfigRepo;
    const commits = { syncStatus: async () => ({ state: 'in-sync' }), clusterCommit: apply };
    apply.mockImplementation(async (change: (doc: Doc) => Promise<Doc>) => {
      payload = await change(payload);
      return { revision: { id: 8 } };
    });
    const db = {
      select: () => ({
        from: (table: unknown) => ({
          where: () =>
            table === configRevision
              ? {
                  orderBy: () => ({ limit: async () => (previous ? [{ comment: previous }] : []) }),
                }
              : Promise.resolve([{ ciphertext: 'test-ciphertext' }]),
        }),
      }),
    };
    svc = new ClusterSyncService(
      repo,
      commits as unknown as CommitService,
      bus,
      { vrrpState: state } as unknown as AgentClient,
      { decrypt: () => KEY } as unknown as SecretsService,
      db as unknown as Db,
    );
  });
  afterEach(() => svc.onApplicationShutdown());
  const envelope = (): Envelope => ({
    origin: 'node-a',
    revision: 4,
    timestamp: Date.now(),
    nonce: randomUUID(),
    document: {
      system: { hostname: 'peer', timezone: 'UTC' },
      management: { users: [{ username: 'peer' }] },
      ha: { vrrp: {} },
    },
  });
  it('refuses plaintext, unknown peers, expired requests and wrong authentication before committing', async () => {
    const e = envelope(),
      mac = signature(JSON.stringify(e), KEY);
    await expect(svc.receive(e, mac, false)).rejects.toMatchObject({ slug: 'unauthorized' });
    await expect(svc.receive({ ...e, origin: 'outsider' }, mac, true)).rejects.toMatchObject({
      slug: 'unauthorized',
    });
    await expect(svc.receive({ ...e, timestamp: 1 }, mac, true)).rejects.toMatchObject({
      slug: 'unauthorized',
    });
    await expect(svc.receive(e, 'f'.repeat(64), true)).rejects.toMatchObject({
      slug: 'unauthorized',
    });
    expect(apply).not.toHaveBeenCalled();
  });
  it('authenticates delivery, preserves local identity and refuses replay', async () => {
    const e = envelope(),
      mac = signature(JSON.stringify(e), KEY);
    await expect(svc.receive(e, mac, true)).resolves.toMatchObject({
      revision: { id: 8 },
      role: 'unknown',
    });
    expect(payload).toMatchObject({
      system: { hostname: 'node-b', timezone: 'UTC' },
      management: { users: [{ username: 'local' }] },
      ha: { cluster: { nodeName: 'node-b' } },
    });
    await expect(svc.receive(e, mac, true)).rejects.toMatchObject({ slug: 'cluster-replay' });
    expect(apply).toHaveBeenCalledTimes(1);
  });
  it('refuses missing secret references by reference without exposing value material', async () => {
    const e = envelope();
    e.document['vpn'] = { ipsec: { tunnels: { lan: { auth: { secretRef: 'psk/missing' } } } } };
    await expect(svc.receive(e, signature(JSON.stringify(e), KEY), true)).rejects.toMatchObject({
      slug: 'cluster-missing-secrets',
    });
    expect(payload).toEqual(local());
  });
  it('ignores pending events and relayed revisions, sends only after confirmed promotion', async () => {
    bus.publish('commit.events', { type: 'pending' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(send).not.toHaveBeenCalled();
    kind = 'cluster-sync';
    bus.publish('commit.events', { type: 'applied' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(send).not.toHaveBeenCalled();
    kind = 'commit';
    pending = { txnId: 'pending' };
    bus.publish('commit.events', { type: 'confirmed' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(send).not.toHaveBeenCalled();
    pending = null;
    bus.publish('commit.events', { type: 'confirmed' });
    await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(1));
    expect(send.mock.calls[0]![6]).not.toHaveProperty('management');
    expect(send.mock.calls[0]![6]).not.toHaveProperty('ha.cluster');
  });
  it('rejects delayed or duplicate source revisions against durable revision provenance', async () => {
    previous = 'cluster sync from node-a revision 6';
    const old = envelope();
    old.revision = 5;
    await expect(svc.receive(old, signature(JSON.stringify(old), KEY), true)).rejects.toMatchObject(
      { slug: 'cluster-stale-source' },
    );
    const duplicate = envelope();
    duplicate.revision = 6;
    await expect(
      svc.receive(duplicate, signature(JSON.stringify(duplicate), KEY), true),
    ).rejects.toMatchObject({ slug: 'cluster-stale-source' });
    expect(payload).toEqual(local());
    const newer = envelope();
    newer.revision = 7;
    await expect(
      svc.receive(newer, signature(JSON.stringify(newer), KEY), true),
    ).resolves.toMatchObject({ revision: { id: 8 } });
  });
  it.each([
    [
      { name: 'lan', state: 'master', error: '' },
      { name: 'wan', state: 'backup', error: '' },
    ],
    [
      { name: 'lan', state: 'master', error: '' },
      { name: 'wan', state: 'unknown', error: '' },
    ],
    [
      { name: 'lan', state: 'master', error: '' },
      { name: 'wan', state: 'master', error: 'unavailable' },
    ],
    [{ name: 'lan', state: 'master', error: '' }],
  ])(
    'refuses automatic writer authority for mixed, unknown, errored or missing enabled roles %#',
    async (...rows) => {
      (payload['ha'] as { vrrp: Record<string, unknown> }).vrrp['wan'] = { enabled: true };
      state.mockResolvedValue({ routers: rows });
      bus.publish('commit.events', { type: 'applied' });
      await new Promise((resolve) => setTimeout(resolve, 10));
      expect(send).not.toHaveBeenCalled();
    },
  );
  it('disabled configured routers do not participate in writer authority', async () => {
    (payload['ha'] as { vrrp: Record<string, unknown> }).vrrp['wan'] = { enabled: false };
    state.mockResolvedValue({
      routers: [
        { name: 'lan', state: 'master', error: '' },
        { name: 'wan', state: 'unknown', error: 'disabled' },
      ],
    });
    bus.publish('commit.events', { type: 'applied' });
    await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(1));
  });
  it('does not auto-push from a backup, and cancels transport on shutdown', async () => {
    state.mockResolvedValue({ routers: [{ state: 'backup', error: '' }] });
    bus.publish('commit.events', { type: 'applied' });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(send).not.toHaveBeenCalled();
    await svc.force();
    const signal = send.mock.calls[0]![7] as AbortSignal;
    expect(signal.aborted).toBe(false);
    svc.onApplicationShutdown();
    expect(signal.aborted).toBe(true);
  });
});
