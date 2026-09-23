import { Inject, Injectable, Logger } from '@nestjs/common';
import { count, desc } from 'drizzle-orm';
import { DB, type Db } from '../db/db.js';
import { auditLog } from '../db/schema.js';

export interface AuditEntry {
  userId: number | null;
  username: string | null;
  sourceIp: string | null;
  action: string;
  resource: string | null;
  /** Must already be redacted (redactSecrets) — the audit log never stores secrets. */
  before?: unknown;
  after?: unknown;
  result: 'success' | 'failure';
  status: number | null;
}

/** Writes and lists `audit_log` (P06 §7: user, ip, route, before/after diff, result for every mutation). */
@Injectable()
export class AuditService {
  private readonly log = new Logger('Audit');

  constructor(@Inject(DB) private readonly db: Db) {}

  async write(e: AuditEntry): Promise<void> {
    try {
      await this.db.insert(auditLog).values({
        userId: e.userId,
        username: e.username,
        sourceIp: e.sourceIp,
        action: e.action,
        resource: e.resource,
        before: e.before ?? null,
        after: e.after ?? null,
        result: e.result,
        status: e.status,
      });
    } catch (err) {
      // never fail (or leak into) the response because of the audit write; the operator sees it in the log
      this.log.error(`audit write failed for ${e.action}: ${(err as Error).message}`);
    }
  }

  async list(limit: number, offset: number) {
    const items = await this.db
      .select()
      .from(auditLog)
      .orderBy(desc(auditLog.id))
      .limit(limit)
      .offset(offset);
    const [c] = await this.db.select({ n: count() }).from(auditLog);
    return { items, total: c?.n ?? 0 };
  }
}
