import { Inject, Injectable, Logger } from '@nestjs/common';
import { count, desc } from 'drizzle-orm';
import { DB, type Db } from '../db/db.js';
import { systemEvent } from '../db/schema.js';

export type Severity = 'info' | 'warning' | 'error';

/** `system_event`: commits, confirm reverts, agent degradation — the device's own history (not per-user). */
@Injectable()
export class SystemEventsService {
  private readonly log = new Logger('SystemEvents');

  constructor(@Inject(DB) private readonly db: Db) {}

  async record(
    severity: Severity,
    subsystem: string,
    code: string,
    message: string,
    data?: unknown,
  ): Promise<void> {
    try {
      await this.db
        .insert(systemEvent)
        .values({ severity, subsystem, code, message, data: data ?? null });
    } catch (err) {
      // TD-10b: the driver's cause — drizzle's own message repeats the query with its parameters
      const cause = ((err as { cause?: unknown } | null)?.cause ?? err) as { message?: unknown };
      this.log.error(`system_event write failed (${code}): ${String(cause?.message ?? cause)}`);
    }
  }

  async list(limit: number, offset: number) {
    const items = await this.db
      .select()
      .from(systemEvent)
      .orderBy(desc(systemEvent.id))
      .limit(limit)
      .offset(offset);
    const [c] = await this.db.select({ n: count() }).from(systemEvent);
    return { items, total: c?.n ?? 0 };
  }
}
