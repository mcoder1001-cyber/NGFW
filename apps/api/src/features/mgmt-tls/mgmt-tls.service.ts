import {
  Inject,
  Injectable,
  Logger,
  type OnApplicationBootstrap,
  type OnApplicationShutdown,
  type OnModuleInit,
} from '@nestjs/common';
import { HttpAdapterHost, ModuleRef } from '@nestjs/core';
import { eq } from 'drizzle-orm';
import type { FastifyInstance } from 'fastify';
import { createServer, type Server } from 'node:https';
import type { SecureContextOptions } from 'node:tls';
import type { ProblemIssue } from '../../common/problem.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { secret } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import {
  CERT_POINTER,
  KEY_POINTER,
  validateTlsMaterial,
  type CertInfo,
  type MinVersion,
} from './validator.js';

/** The `management.tls` subtree as the parsed document holds it. */
export interface TlsConfig {
  certificateRef?: string;
  privateKeyRef?: string;
  minVersion?: MinVersion;
}

/** Reads a secret's current plaintext by `<kind>/<name>`; null = not in the store. */
export type SecretReader = (ref: string) => Promise<string | null>;

export interface MgmtTlsState {
  configured: boolean;
  certificateRef: string | null;
  minVersion: MinVersion;
  active: (CertInfo & { daysLeft: number }) | null;
  listener: { enabled: boolean; port: number | null };
  loadedRevision: number | null;
  loadedAt: string | null;
  error: string | null;
}

function tlsOf(doc: unknown): TlsConfig {
  const m = (doc as { management?: { tls?: TlsConfig } } | null)?.management;
  return m?.tls ?? {};
}

/** Optional HTTPS listener port (`VRX_HTTPS_PORT`); unset = the API keeps plain HTTP only. */
export function httpsPortFromEnv(env: NodeJS.ProcessEnv = process.env): number | null {
  const raw = env['VRX_HTTPS_PORT'];
  if (raw === undefined || raw === '') return null;
  const n = Number(raw);
  return Number.isInteger(n) && n >= 1 && n <= 65535 ? n : null;
}

/**
 * F-management-ui: applies `management.tls` to the API. Validation (called by the commit's semantic tier) resolves the
 * certificate/key refs and refuses a bad pair before commit; after every commit / confirm / revert the running
 * document is re-read and the secure context is swapped in place, so new handshakes use the new certificate without a
 * restart. The key is only ever held inside the SecureContext; it is never logged or returned.
 */
@Injectable()
export class MgmtTlsService implements OnModuleInit, OnApplicationBootstrap, OnApplicationShutdown {
  private readonly log = new Logger('mgmt-tls');
  private context: SecureContextOptions | null = null;
  private current: Omit<MgmtTlsState, 'listener'> | null = null;
  private server: Server | null = null;
  private reloadQueue: Promise<void> = Promise.resolve();
  private bootstrapped = false;
  private shuttingDown = false;
  private unsubscribe: (() => void) | null = null;
  readonly httpsPort = httpsPortFromEnv();
  /** Clock (tests move it). */
  now: () => Date = () => new Date();

  constructor(
    private readonly ds: DatastoreService,
    private readonly bus: Bus,
    private readonly moduleRef: ModuleRef,
    @Inject(DB) private readonly db: Db,
    private readonly host: HttpAdapterHost,
  ) {}

  /** Test seam: replaces the secret store lookup. */
  secretReader: SecretReader | null = null;

  private async readSecret(ref: string): Promise<string | null> {
    if (this.secretReader) return this.secretReader(ref);
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref))
      .limit(1);
    if (!row) return null;
    // resolved lazily (module and provider): SecretsService → CommitService → ValidationService → this service is a cycle
    const { SecretsService } = await import('../../secrets/secrets.service.js');
    return this.moduleRef.get(SecretsService, { strict: false }).decrypt(row.ciphertext, ref);
  }

  onModuleInit(): void {
    this.unsubscribe = this.bus.onPublish((m) => {
      if (m.topic !== 'commit.events') return;
      const type = (m.data as { type?: string } | null)?.type;
      if (type === 'applied' || type === 'confirmed' || type === 'reverted') void this.reload();
    });
  }

  async onApplicationBootstrap(): Promise<void> {
    if (this.httpsPort === null) return;
    await this.reload();
    this.bootstrapped = true;
    await this.ensureListener();
  }

  private async ensureListener(): Promise<void> {
    if (!this.bootstrapped || this.shuttingDown || this.httpsPort === null || this.server) return;
    const fastify = this.host.httpAdapter.getInstance() as unknown as FastifyInstance;
    // without a configured certificate the listener stays down (no self-signed fallback is generated here)
    if (this.context === null) {
      this.log.warn(
        'VRX_HTTPS_PORT is set but management.tls has no usable certificate: HTTPS listener not started',
      );
      return;
    }
    this.server = createServer(this.context, (req, res) => fastify.routing(req, res));
    // Reuse Fastify's existing upgrade handler: it runs stream authentication and route hooks.
    // Preserve the TLS socket and upgrade head; never create a second websocket/auth stack.
    this.server.on('upgrade', (req, socket, head) => {
      if (!fastify.server.emit('upgrade', req, socket, head)) socket.destroy();
    });
    const hostName = process.env['VRX_HTTP_HOST'] ?? '127.0.0.1';
    const server = this.server;
    await new Promise<void>((resolve, reject) => {
      const onError = (error: Error) => {
        if (this.server === server) this.server = null;
        reject(error);
      };
      server.once('error', onError);
      server.listen(this.httpsPort!, hostName, () => {
        server.off('error', onError);
        this.log.log(
          `HTTPS listener on ${hostName}:${this.httpsPort} (certificate from management.tls)`,
        );
        resolve();
      });
    });
  }

  async onApplicationShutdown(): Promise<void> {
    this.shuttingDown = true;
    this.unsubscribe?.();
    await this.reloadQueue;
    await new Promise<void>((r) => (this.server ? this.server.close(() => r()) : r()));
  }

  /** The secure-context options new handshakes use; null = no usable certificate configured. */
  secureContext(): SecureContextOptions | null {
    return this.context;
  }

  /** Semantic-tier check of a parsed document's `management.tls` (errors carry pointers). */
  async validate(doc: unknown): Promise<ProblemIssue[]> {
    const tls = tlsOf(doc);
    if (!tls.certificateRef || !tls.privateKeyRef) return [];
    return (await this.check(tls)).issues;
  }

  private async check(tls: TlsConfig) {
    const [cert, key] = await Promise.all([
      this.readSecret(tls.certificateRef!),
      this.readSecret(tls.privateKeyRef!),
    ]);
    if (cert === null)
      return {
        issues: [
          {
            pointer: CERT_POINTER,
            message: `secret ${tls.certificateRef} not found`,
            rule: 'management.tls.certificate-pem',
          },
        ],
      };
    if (key === null)
      return {
        issues: [
          {
            pointer: KEY_POINTER,
            message: `secret ${tls.privateKeyRef} not found`,
            rule: 'management.tls.key-pem',
          },
        ],
      };
    return validateTlsMaterial(cert, key, tls.minVersion ?? '1.2', this.now());
  }

  /** Re-reads running and swaps the secure context. A bad pair keeps the previous context and records the error. */
  reload(): Promise<void> {
    // One running read/secret resolution/apply at a time: slow older reads cannot overwrite newer commits.
    this.reloadQueue = this.reloadQueue.then(() => this.reloadRunning());
    return this.reloadQueue;
  }

  private async reloadRunning(): Promise<void> {
    try {
      const running = await this.ds.getRunning();
      const tls = tlsOf(running.doc);
      const base = {
        certificateRef: tls.certificateRef ?? null,
        minVersion: tls.minVersion ?? '1.2',
        loadedRevision: running.revision?.id ?? null,
        loadedAt: this.now().toISOString(),
      } as const;
      if (!tls.certificateRef || !tls.privateKeyRef) {
        // the running listener keeps its last certificate until the API restarts (it cannot serve without one)
        this.context = null;
        this.current = { ...base, configured: false, active: null, error: null };
        return;
      }
      const res = await this.check(tls);
      if (res.issues.length > 0 || !('options' in res) || !res.options || !res.info) {
        const error = res.issues.map((i) => i.message).join('; ') || 'no TLS context';
        this.current = { ...base, configured: true, active: this.current?.active ?? null, error };
        this.log.warn(`management.tls of revision ${base.loadedRevision} not applied: ${error}`);
        return;
      }
      this.context = res.options;
      // hot reload: new handshakes use the new certificate and protocol floor, open connections are kept
      this.server?.setSecureContext(res.options);
      this.current = { ...base, configured: true, active: this.withDays(res.info), error: null };
      await this.ensureListener();
      this.log.log(
        `management.tls applied: ${res.info.subject} (sha256 ${res.info.fingerprintSha256})`,
      );
    } catch (e) {
      const error = `could not load management.tls: ${(e as Error).message}`;
      this.current = { ...(this.current ?? emptyState()), error };
      this.log.warn(error);
    }
  }

  private withDays(info: CertInfo) {
    const daysLeft = Math.floor(
      (new Date(info.notAfter).getTime() - this.now().getTime()) / 86_400_000,
    );
    return { ...info, daysLeft };
  }

  async state(): Promise<MgmtTlsState> {
    if (this.current === null) await this.reload();
    const cur = this.current ?? emptyState();
    return {
      ...cur,
      active: cur.active ? this.withDays(cur.active) : null,
      listener: { enabled: this.server?.listening ?? false, port: this.httpsPort },
    };
  }
}

function emptyState(): Omit<MgmtTlsState, 'listener'> {
  return {
    configured: false,
    certificateRef: null,
    minVersion: '1.2',
    active: null,
    loadedRevision: null,
    loadedAt: null,
    error: null,
  };
}
