import {
  Inject,
  Injectable,
  Logger,
  type OnModuleDestroy,
  type OnModuleInit,
} from '@nestjs/common';
import { createHash } from 'node:crypto';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { ENV, type Env } from '../../config.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { VALKEY, type Valkey } from '../../infra/valkey.js';
import { expiryStates, type ExpiringRule } from './expiry.js';

/**
 * F-rule-expiry warnings: every NGFW_RULE_EXPIRY_CHECK_SEC the running configuration is scanned; a rule expiring within
 * NGFW_RULE_EXPIRY_WARN_DAYS gets one `RULE_EXPIRING` system event (warning) and an expired one one `RULE_EXPIRED`
 * (info: the agent has already removed it from the data plane). Each event is raised once per rule and expiresAt
 * (a Valkey SET NX marker; extending the rule arms it again) — alarms/notifications (F-notifications) pick them up.
 */
@Injectable()
export class RuleExpiryService implements OnModuleInit, OnModuleDestroy {
  private readonly log = new Logger('RuleExpiry');
  private timer: NodeJS.Timeout | undefined;

  constructor(
    @Inject(ENV) private readonly env: Env,
    private readonly ds: DatastoreService,
    private readonly events: SystemEventsService,
    @Inject(VALKEY) private readonly kv: Valkey,
  ) {}

  onModuleInit(): void {
    this.timer = setInterval(
      () => void this.scan().catch((e: unknown) => this.log.warn(`rule expiry scan: ${String(e)}`)),
      this.env.NGFW_RULE_EXPIRY_CHECK_SEC * 1000,
    );
    this.timer.unref();
  }

  onModuleDestroy(): void {
    if (this.timer) clearInterval(this.timer);
  }

  /** One scan; returns the events raised (tests). */
  async scan(now = new Date()): Promise<{ code: string; id: string }[]> {
    const { doc } = await this.ds.getRunning();
    const { expiring, expired } = expiryStates(doc, now, this.env.NGFW_RULE_EXPIRY_WARN_DAYS);
    const raised: { code: string; id: string }[] = [];
    const once = async (
      code: string,
      r: ExpiringRule,
      severity: 'warning' | 'info',
      message: string,
    ) => {
      const marker = createHash('sha256')
        .update(`${code}|${r.id}|${r.expiresAt}`)
        .digest('hex')
        .slice(0, 32);
      // kept past the expiry plus the warning window: the same rule and date never raise twice
      const ttl =
        Math.max(60, Math.ceil((Date.parse(r.expiresAt) - now.getTime()) / 1000)) +
        (this.env.NGFW_RULE_EXPIRY_WARN_DAYS + 30) * 86_400;
      if ((await this.kv.set(`ruleexp:${marker}`, '1', 'EX', ttl, 'NX')) !== 'OK') return;
      await this.events.record(severity, 'rules', code, message, {
        rule: r.id,
        pointer: r.pointer,
        expiresAt: r.expiresAt,
        ...(r.owner ? { owner: r.owner } : {}),
        ...(r.ticket ? { ticket: r.ticket } : {}),
      });
      raised.push({ code, id: r.id });
    };
    const who = (r: ExpiringRule) =>
      [r.owner ? `owner ${r.owner}` : '', r.ticket ? `ticket ${r.ticket}` : '']
        .filter(Boolean)
        .join(', ');
    for (const r of expiring) {
      await once(
        'RULE_EXPIRING',
        r,
        'warning',
        `${r.pointer} expires at ${r.expiresAt}${who(r) ? ` (${who(r)})` : ''}; extend it or let it expire`,
      );
    }
    for (const r of expired) {
      await once(
        'RULE_EXPIRED',
        r,
        'info',
        `${r.pointer} expired at ${r.expiresAt}${who(r) ? ` (${who(r)})` : ''}: removed from the data plane, kept in the configuration`,
      );
    }
    return raised;
  }
}
