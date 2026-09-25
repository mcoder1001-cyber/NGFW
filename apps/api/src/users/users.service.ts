import { Inject, Injectable } from '@nestjs/common';
import { count, eq, sql } from 'drizzle-orm';
import type { FastifyRequest } from 'fastify';
import { TokensService } from '../auth/tokens.service.js';
import { hashPassword, verifyPassword } from '../auth/password.js';
import { CommitService } from '../commit/commit.service.js';
import { ENV, type Env } from '../config.js';
import { problems } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import { replaceUserHash } from '../datastore/documents.js';
import type { Doc } from '../datastore/repo.js';
import { DB, type Db } from '../db/db.js';
import { apiKey, appUser, configCandidate, configPending } from '../db/schema.js';
import { releaseKeyLocks } from '../datastore/pg-repo.js';
import { AuthService } from '../auth/auth.service.js';
import { secureTransport, tlsRequired } from '../auth/transport.js';
import { withinCommitLock } from './commit-busy.js';
import { assertPasswordPolicy } from './password-policy.js';

export interface SetPasswordInput {
  password: string;
  current?: string | undefined;
  /** Admin reset only: keep the target's API keys (service users). Default false: they are revoked (D-097). */
  keepApiKeys?: boolean | undefined;
}

export interface SetPasswordResult {
  self: boolean;
  userId: number;
  /** API keys of the target revoked by an admin reset (D-097). */
  apiKeysRevoked: { id: string; name: string }[];
  /** Admin reset with `keepApiKeys: true`: how many keys the target kept (verify V5, audited). */
  apiKeysKept?: number;
  /** A candidate locked by one of the revoked keys was discarded with its lock (review L4, verify V5). */
  discardedCandidate: boolean;
  /** false when Valkey could not be updated after the commit (verify V3): the reset still holds (PostgreSQL gen). */
  revocationPersisted: boolean;
}

/**
 * `POST /api/v1/users/{name}/password` (TD-2 #1, D-097). The hash (argon2id) is computed here and written to app_user
 * only — app_user owns password hashes (D-091/D-097); revisions stay redacted. A hash that a stored candidate, the
 * pending commit or an in-flight (lost-answer) commit staged for that user is replaced too, serialised with commits,
 * so no later promote/confirm/reconcile writes the old one back. The target's other sessions end (credential
 * generation bumped atomically: refresh chains, access tokens, WebSockets); an admin reset also revokes the target's
 * API keys unless `keepApiKeys`. A wrong `current` counts toward the target's login lockout. Rate-limited per caller
 * and per target.
 */
@Injectable()
export class UsersService {
  constructor(
    @Inject(DB) private readonly db: Db,
    @Inject(ENV) private readonly env: Env,
    private readonly tokens: TokensService,
    private readonly auth: AuthService,
    private readonly commits: CommitService,
  ) {}

  async setPassword(
    caller: Principal,
    name: string,
    input: SetPasswordInput,
    req: FastifyRequest,
  ): Promise<SetPasswordResult> {
    // defence in depth (the route pipes apply the same rule): first, so a short password costs no rate-limit hit
    assertPasswordPolicy(input.password, this.env.VRX_DEV_WEAK_PASSWORDS);
    const limit = this.env.VRX_PASSWORD_RATE_PER_MIN;
    if ((await this.tokens.hit(`pwset:${caller.id}`, 60)) > limit) {
      throw problems.tooMany('too many password changes; try again in a minute');
    }
    if (!secureTransport(req)) throw tlsRequired();
    // authorise before the lookup: a non-admin learns nothing about other accounts
    if (name !== caller.username && caller.role !== 'admin') {
      throw problems.forbidden(`role '${caller.role}' may set its own password only`);
    }
    const [u] = await this.db
      .select({
        id: appUser.id,
        passwordHash: appUser.passwordHash,
        lockedUntil: appUser.lockedUntil,
      })
      .from(appUser)
      .where(eq(appUser.username, name));
    if (u === undefined) throw problems.notFound(`user '${name}' does not exist`);
    // review L5: "self" is the account id, not the (reusable) name in the token
    const self = u.id === caller.id;
    if (!self && caller.role !== 'admin') {
      throw problems.forbidden(`role '${caller.role}' may set its own password only`);
    }
    if ((await this.tokens.hit(`pwset:target:${u.id}`, 60)) > limit) {
      throw problems.tooMany('too many password changes for this user; try again in a minute');
    }
    if (self) {
      if (input.current === undefined) {
        throw problems.badRequest('changing your own password needs the current one', [
          { pointer: '/current', message: 'required when the target is the caller' },
        ]);
      }
      // review M2: a locked account takes no guesses; a wrong guess counts like a failed login
      if (u.lockedUntil !== null && u.lockedUntil > new Date()) {
        throw problems.forbidden('the account is locked after too many failed password checks');
      }
      if (!(await verifyPassword(u.passwordHash, input.current))) {
        const locked = await this.auth.registerFailure(u.id);
        throw problems.forbidden(
          locked
            ? 'the current password is wrong; the account is now locked'
            : 'the current password is wrong',
        );
      }
    }
    const hash = await hashPassword(input.password);
    const revokeKeys = !self && input.keepApiKeys !== true;
    // TD-10b (manager addendum, TD-10a review M2): at most 1 s behind a commit, then 409 commit-busy — nothing changed
    const r = await withinCommitLock(
      (f) => this.commits.exclusive(f),
      async () => {
        const out = await this.db.transaction(async (tx) => {
          // verify V1: the credential generation moves in the same transaction as the hash; API-key creation reads this
          // row FOR SHARE, so it waits for this UPDATE and then sees the new generation / the deleted keys
          const [row] = await tx
            .update(appUser)
            .set({
              passwordHash: hash,
              failedLogins: 0,
              lockedUntil: null,
              credentialGen: sql`${appUser.credentialGen} + 1`,
            })
            .where(eq(appUser.id, u.id))
            .returning({ gen: appUser.credentialGen });
          if (row === undefined) throw problems.notFound(`user '${name}' does not exist`);
          // keys before the candidate/pending rows: the same lock order as deleteApiKey (api_key → candidate), so the
          // two cannot deadlock (verify V4)
          let keys: { id: string; name: string }[] = [];
          let discardedCandidate = false;
          let kept: number | undefined;
          if (revokeKeys) {
            // D-097 (review M1): an admin reset answers a suspected compromise — the target's keys go too
            keys = await tx
              .delete(apiKey)
              .where(eq(apiKey.userId, u.id))
              .returning({ id: apiKey.id, name: apiKey.name });
            discardedCandidate = await releaseKeyLocks(
              tx,
              keys.map((k) => k.id),
            );
          } else if (!self) {
            const [n] = await tx.select({ n: count() }).from(apiKey).where(eq(apiKey.userId, u.id));
            kept = n?.n ?? 0;
          }
          const [c] = await tx
            .select({ payload: configCandidate.payload })
            .from(configCandidate)
            .where(eq(configCandidate.id, 1))
            .for('update');
          const cand = c?.payload ? replaceUserHash(c.payload as Doc, name, hash) : null;
          // payload only: the lock's owner and activity time stay as they are
          if (cand !== null)
            await tx
              .update(configCandidate)
              .set({ payload: cand })
              .where(eq(configCandidate.id, 1));
          const [p] = await tx
            .select({ payload: configPending.payload })
            .from(configPending)
            .where(eq(configPending.id, 1))
            .for('update');
          const pend = p ? replaceUserHash(p.payload as Doc, name, hash) : null;
          if (pend !== null)
            await tx.update(configPending).set({ payload: pend }).where(eq(configPending.id, 1));
          return { gen: row.gen, keys, discardedCandidate, kept };
        });
        // review H1 + verify V4: the copy a reconcile would promote after a lost Apply answer — only once the new hash
        // is committed (still inside `exclusive`, so no promote runs in between)
        this.commits.replaceInflightHash(name, hash);
        return out;
      },
    );
    const keep = self && caller.via === 'jwt' ? caller.sid : undefined;
    const revoked = await this.tokens.revokeUser(u.id, r.gen, keep);
    return {
      self,
      userId: u.id,
      apiKeysRevoked: r.keys,
      ...(r.kept !== undefined ? { apiKeysKept: r.kept } : {}),
      discardedCandidate: r.discardedCandidate,
      revocationPersisted: revoked.persisted,
    };
  }
}
