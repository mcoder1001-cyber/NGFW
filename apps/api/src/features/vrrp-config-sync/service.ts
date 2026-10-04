import { Inject, Injectable, type OnApplicationShutdown } from '@nestjs/common';
import type { VrrpStateResponse } from '@ngfw/proto';
import type { HaCluster } from '@ngfw/schema';
import { and, desc, eq, sql } from 'drizzle-orm';
import { AgentClient } from '../../agent/agent.client.js';
import { CommitService } from '../../commit/commit.service.js';
import { problems } from '../../common/problem.js';
import { CONFIG_REPO } from '../../datastore/datastore.service.js';
import { redact, secretRefs } from '../../datastore/documents.js';
import type { ConfigRepo, Doc } from '../../datastore/repo.js';
import { DB, type Db } from '../../db/db.js';
import { configRevision, secret } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import { exportDocument, mergeDocument } from './document.js';
import { sendPinned, validSignature, type Envelope } from './transport.js';

interface PeerStatus {
  role: string;
  name: string;
  address: string;
  revision: number | null;
  sourceRevision: number | null;
  error: string;
  syncedAt: string | null;
}
const clusterOf = (doc: Doc) => (doc['ha'] as { cluster?: HaCluster } | undefined)?.cluster;
function observedRole(doc: Doc, view: VrrpStateResponse): 'master' | 'backup' | 'unknown' {
  const config =
    (doc['ha'] as { vrrp?: Record<string, { enabled?: boolean }> } | undefined)?.vrrp ?? {};
  const names = Object.entries(config)
    .filter(([, v]) => v.enabled !== false)
    .map(([name]) => name);
  if (names.length === 0) return 'unknown';
  const rows = names.map((name) => view.routers.find((row) => row.name === name));
  if (rows.some((row) => !row || row.error)) return 'unknown';
  if (rows.every((row) => row!.state === 'master')) return 'master';
  if (rows.every((row) => row!.state === 'backup')) return 'backup';
  return 'unknown';
}

@Injectable()
export class ClusterSyncService implements OnApplicationShutdown {
  private readonly peers = new Map<string, PeerStatus>();
  private readonly nonces = new Map<string, number>();
  private readonly stop: () => void;
  private readonly abort = new AbortController();
  private active: Promise<unknown> | undefined;
  private queued = false;
  constructor(
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    private readonly commits: CommitService,
    private readonly bus: Bus,
    private readonly agent: AgentClient,
    private readonly secrets: SecretsService,
    @Inject(DB) private readonly db: Db,
  ) {
    this.stop = bus.onPublish((m) => {
      const event = m.data as { type?: string; state?: string };
      if (
        m.topic !== 'commit.events' ||
        !(
          ['applied', 'confirmed'].includes(event.type ?? '') ||
          (event.type === 'sync' && event.state === 'in-sync')
        )
      )
        return;
      this.queued = true;
      if (!this.active) this.startAutomatic();
    });
  }
  onApplicationShutdown(): void {
    this.stop();
    this.queued = false;
    this.abort.abort();
  }
  private startAutomatic(): void {
    this.queued = false;
    this.active = this.sync(false)
      .catch(() => undefined)
      .finally(() => {
        this.active = undefined;
        if (this.queued && !this.abort.signal.aborted) this.startAutomatic();
      });
  }
  private async key(ref: string): Promise<string> {
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref));
    if (!row) throw problems.unavailable('cluster authentication key is unavailable');
    const value = this.secrets.decrypt(row.ciphertext, ref);
    if (Buffer.byteLength(value) < 32)
      throw problems.unavailable('cluster authentication key requires at least 32 bytes');
    return value;
  }
  async state() {
    const running = await this.repo.latestRevision(),
      c = running && clusterOf(running.payload);
    return {
      nodeName: c?.nodeName ?? '',
      enabled: c?.enabled ?? false,
      revision: running?.id ?? null,
      members: (c?.peers ?? []).map((p) => ({
        ...this.peers.get(p.name),
        role: this.peers.get(p.name)?.role ?? 'unknown',
        name: p.name,
        address: p.address,
        revision: this.peers.get(p.name)?.revision ?? null,
        sourceRevision: this.peers.get(p.name)?.sourceRevision ?? null,
        error: this.peers.get(p.name)?.error ?? '',
        syncedAt: this.peers.get(p.name)?.syncedAt ?? null,
        lag:
          running?.id && this.peers.get(p.name)?.sourceRevision != null
            ? running.id - this.peers.get(p.name)!.sourceRevision!
            : null,
      })),
    };
  }
  async force() {
    if (this.active) throw problems.conflict('cluster-sync-busy', 'a cluster sync is in progress');
    this.active = this.sync(true);
    try {
      return await this.active;
    } finally {
      this.active = undefined;
      if (this.queued && !this.abort.signal.aborted) this.startAutomatic();
    }
  }
  private async sync(force: boolean) {
    const running = await this.repo.latestRevision();
    const c = running && clusterOf(running.payload);
    if (!running || !c?.enabled || !c.configSync) {
      if (force) throw problems.conflict('cluster-disabled', 'configuration sync is disabled');
      return this.state();
    }
    if (running.kind === 'cluster-sync' && !force) return this.state();
    if (
      (await this.repo.pending()) !== null ||
      (await this.commits.syncStatus()).state !== 'in-sync'
    )
      return this.state();
    if (!force) {
      const view = await this.agent.vrrpState();
      if (observedRole(running.payload, view) !== 'master') return this.state();
    }
    const key = await this.key(c.secretRef),
      document = exportDocument(redact(running.payload), c.syncExclude ?? []);
    for (const peer of c.peers) {
      if (this.abort.signal.aborted) break;
      const row: PeerStatus = {
        name: peer.name,
        role: 'unknown',
        address: peer.address,
        revision: null,
        sourceRevision: null,
        error: '',
        syncedAt: null,
        ...(this.peers.get(peer.name)?.address === peer.address ? this.peers.get(peer.name) : {}),
      };
      row.error = '';
      row.role = 'unknown';
      try {
        const delivered = await sendPinned(
          peer.address,
          c.port,
          peer.certificatePin ?? '',
          key,
          c.nodeName,
          running.id,
          document,
          this.abort.signal,
        );
        row.revision = delivered.revision;
        row.role = delivered.role;
        row.sourceRevision = running.id;
        row.syncedAt = new Date().toISOString();
      } catch {
        row.error =
          'peer sync failed; check TLS certificate pin, peer key, missing secret references and candidate conflicts';
      }
      this.peers.set(peer.name, row);
    }
    return this.state();
  }
  async receive(envelope: Envelope, mac: string, encrypted: boolean) {
    if (!encrypted || !mac)
      throw problems.unauthorized('HTTPS and cluster authentication required');
    const running = await this.repo.latestRevision(),
      c = running && clusterOf(running.payload);
    if (!c?.enabled || !c.configSync || !c.peers.some((p) => p.name === envelope.origin))
      throw problems.unauthorized('unknown cluster peer');
    if (
      Math.abs(Date.now() - envelope.timestamp) > 30000 ||
      !validSignature(JSON.stringify(envelope), await this.key(c.secretRef), mac)
    )
      throw problems.unauthorized('invalid cluster authentication');
    for (const [id, expiry] of this.nonces) if (expiry < Date.now()) this.nonces.delete(id);
    if (this.nonces.has(envelope.nonce))
      throw problems.conflict('cluster-replay', 'cluster message was already received');
    if (this.nonces.size >= 1024) throw problems.tooMany('cluster replay cache is full');
    this.nonces.set(envelope.nonce, Date.now() + 60000);
    const result = await this.commits.clusterCommit(async (local) => {
      const current = clusterOf(local);
      // Recheck membership after acquiring the same lock used by configuration commits.
      if (
        !current?.enabled ||
        !current.configSync ||
        current.secretRef !== c.secretRef ||
        !current.peers.some((p) => p.name === envelope.origin)
      )
        throw problems.conflict('cluster-changed', 'cluster membership changed during delivery');
      // The source stamp is committed atomically with the document. Query under the same
      // cross-process commit lock: stale delivery/replay remains refused after API restart.
      const prefix = `cluster sync from ${envelope.origin} revision `;
      const [previous] = await this.db
        .select({ comment: configRevision.comment })
        .from(configRevision)
        .where(
          and(
            eq(configRevision.kind, 'cluster-sync'),
            sql`left(${configRevision.comment}, ${prefix.length}) = ${prefix}`,
          ),
        )
        .orderBy(desc(configRevision.id))
        .limit(1);
      if (previous) {
        const stamp = previous.comment.slice(prefix.length),
          floor = Number(stamp);
        if (!/^\d+$/.test(stamp) || !Number.isSafeInteger(floor) || envelope.revision <= floor)
          throw problems.conflict(
            'cluster-stale-source',
            'source revision is not newer than the last accepted revision from this peer',
          );
      }
      const next = mergeDocument(redact(envelope.document), local, current.syncExclude ?? []);
      const refs = [...new Set(secretRefs(next).map((r) => r.ref))];
      const existing = await this.repo.secretVersions(refs),
        missing = refs.filter((ref) => existing[ref] === undefined);
      if (missing.length)
        throw problems.conflict(
          'cluster-missing-secrets',
          `peer is missing secret references: ${missing.join(', ')}`,
        );
      return next;
    }, `${envelope.origin} revision ${envelope.revision}`);
    let role = 'unknown';
    try {
      const view = await this.agent.vrrpState();
      role = observedRole((await this.repo.latestRevision())?.payload ?? {}, view);
    } catch {
      /* Config was committed; state observation remains explicitly unknown. */
    }
    return { ...result, role };
  }
}
