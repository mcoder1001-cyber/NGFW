import {
  Inject,
  Injectable,
  Logger,
  type OnModuleDestroy,
  type OnModuleInit,
} from '@nestjs/common';
import type { KeyObject } from 'node:crypto';
import { mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { ProblemError, problems, type ProblemIssue } from '../../common/problem.js';
import { CONFIG_REPO } from '../../datastore/datastore.service.js';
import type { ConfigRepo } from '../../datastore/repo.js';
import { COMMUNITY, entitlementIssues, type Entitlements } from './entitlements.js';
import {
  evaluate,
  GRACE_DAYS,
  LicenseFormatError,
  machineIdHash,
  parseAndVerify,
  publicKeyFrom,
  type HostIdentity,
  type License,
  type LicenseStatus,
} from './format.js';
import {
  LICENSING_OPTIONS,
  PRODUCT_PUBLIC_KEYS,
  type LicensingOptions,
} from './licensing.config.js';

/** What GET /state/license returns: no signature, no binding values, customer name only (D-046 minimisation). */
export interface LicenseState {
  status: LicenseStatus;
  reason?: string;
  licenseId?: string;
  customer?: string;
  issuedAt?: string;
  notBefore?: string;
  expiresAt?: string;
  daysLeft: number;
  graceDays: number;
  bound: { machineId: boolean; serial: boolean };
  /** Entitlements in force right now (community set when none / invalid / expired past grace). */
  entitlements: Entitlements;
}

export interface LicenseCheck {
  errors: ProblemIssue[];
  warnings: ProblemIssue[];
}

function readOptional(path: string): string | undefined {
  try {
    const v = readFileSync(path, 'utf8').trim();
    return v === '' ? undefined : v;
  } catch {
    return undefined;
  }
}

/**
 * Offline licence (F-licensing): verifies the stored `.vrxlic`, computes status and entitlements, and checks a
 * candidate document at commit validation time. Nothing here reaches the agent (D-040).
 */
@Injectable()
export class LicensingService implements OnModuleInit, OnModuleDestroy {
  private readonly log = new Logger('Licensing');
  private keys: KeyObject[] | undefined;
  private host: HostIdentity | undefined;
  private lastStatus: LicenseStatus | undefined;
  private timer: NodeJS.Timeout | undefined;

  constructor(
    @Inject(LICENSING_OPTIONS) private readonly opts: LicensingOptions,
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    private readonly events: SystemEventsService,
  ) {}

  onModuleInit(): void {
    this.timer = setInterval(
      () => void this.state().catch(() => undefined),
      this.opts.checkIntervalMs,
    );
    this.timer.unref();
  }

  onModuleDestroy(): void {
    if (this.timer) clearInterval(this.timer);
  }

  private trustedKeys(): KeyObject[] {
    if (!this.keys) {
      const pems = [...PRODUCT_PUBLIC_KEYS];
      if (this.opts.extraPublicKeyFile) {
        const extra = readOptional(this.opts.extraPublicKeyFile);
        if (extra) pems.push(extra);
      }
      this.keys = pems.map(publicKeyFrom);
    }
    return this.keys;
  }

  private identity(): HostIdentity {
    if (!this.host) {
      const mid = readOptional(this.opts.machineIdFile);
      this.host = {
        machineIdHash: mid === undefined ? undefined : machineIdHash(mid),
        serial: this.opts.serial ?? readOptional(this.opts.dmiSerialFile),
      };
    }
    return this.host;
  }

  private async stored(): Promise<License | 'none' | 'unreadable'> {
    let text: string;
    try {
      text = await readFile(this.opts.file, 'utf8');
    } catch (e) {
      const code = (e as NodeJS.ErrnoException).code;
      if (code === 'ENOENT') return 'none';
      this.log.warn(`stored licence ${this.opts.file} cannot be read (${code ?? 'error'})`);
      return 'unreadable';
    }
    try {
      return parseAndVerify(text, this.trustedKeys());
    } catch (e) {
      // no licence content or signature in the log: only the path and the verification error class
      this.log.warn(
        `stored licence ${this.opts.file} does not verify (${e instanceof LicenseFormatError ? e.code : 'error'}); community entitlements apply`,
      );
      return 'unreadable';
    }
  }

  private describe(lic: License): LicenseState {
    const ev = evaluate(lic, this.opts.now(), this.identity());
    const licensed = ev.status === 'valid' || ev.status === 'grace';
    const out: LicenseState = {
      status: ev.status,
      licenseId: lic.licenseId,
      customer: lic.customer,
      issuedAt: lic.issuedAt,
      notBefore: lic.notBefore,
      expiresAt: lic.expiresAt,
      daysLeft: ev.daysLeft,
      graceDays: GRACE_DAYS,
      bound: {
        machineId: lic.binding.machineIdHash !== undefined,
        serial: lic.binding.serial !== undefined,
      },
      entitlements: licensed
        ? { features: [...lic.entitlements.features], limits: { ...lic.entitlements.limits } }
        : structuredClone(this.opts.community ?? COMMUNITY),
    };
    if (ev.reason !== undefined) out.reason = ev.reason;
    return out;
  }

  /** Current status; records the expiry event in `system_event` on a transition into grace / expired. */
  async state(): Promise<LicenseState> {
    const lic = await this.stored();
    let st: LicenseState;
    if (lic === 'none') {
      st = {
        status: 'community',
        daysLeft: 0,
        graceDays: GRACE_DAYS,
        bound: { machineId: false, serial: false },
        entitlements: structuredClone(this.opts.community ?? COMMUNITY),
      };
    } else if (lic === 'unreadable') {
      st = {
        status: 'invalid',
        reason: 'the stored licence file cannot be verified',
        daysLeft: 0,
        graceDays: GRACE_DAYS,
        bound: { machineId: false, serial: false },
        entitlements: structuredClone(this.opts.community ?? COMMUNITY),
      };
    } else st = this.describe(lic);
    await this.noteTransition(st);
    return st;
  }

  private async noteTransition(st: LicenseState): Promise<void> {
    const prev = this.lastStatus;
    this.lastStatus = st.status;
    if (prev === st.status) return;
    if (st.status === 'grace') {
      await this.events.record(
        'warning',
        'licensing',
        'license.grace',
        `licence ${st.licenseId} expired; grace period ends in ${st.daysLeft} day(s)`,
        { licenseId: st.licenseId, expiresAt: st.expiresAt, daysLeft: st.daysLeft },
      );
    } else if (st.status === 'expired') {
      await this.events.record(
        'error',
        'licensing',
        'license.expired',
        `licence ${st.licenseId} expired and the grace period is over; community entitlements apply to new configuration`,
        { licenseId: st.licenseId, expiresAt: st.expiresAt },
      );
    }
  }

  /** Verify an uploaded licence (400 when malformed, tampered, wrongly signed, bound elsewhere or expired) and store it. */
  async install(text: string): Promise<LicenseState> {
    let lic: License;
    try {
      lic = parseAndVerify(text, this.trustedKeys());
    } catch (e) {
      if (e instanceof LicenseFormatError) throw problems.badRequest(e.message);
      throw e;
    }
    const st = this.describe(lic);
    if (st.status === 'invalid' || st.status === 'expired')
      throw problems.badRequest(
        st.status === 'expired' ? 'licence is expired (past the grace period)' : st.reason!,
      );
    await mkdir(dirname(this.opts.file), { recursive: true, mode: 0o750 });
    const tmp = `${this.opts.file}.tmp-${process.pid}`;
    await writeFile(tmp, text, { mode: 0o640 });
    await rename(tmp, this.opts.file);
    this.log.log(`licence ${lic.licenseId} installed (${st.status})`);
    await this.noteTransition(st);
    return st;
  }

  /**
   * Commit validation stage: errors for new unlicensed use (grandfathered against running), a warning while in
   * grace or when the licence is invalid.
   */
  async check(candidate: Record<string, unknown>): Promise<LicenseCheck> {
    const st = await this.state();
    const running = (await this.repo.latestRevision())?.payload ?? {};
    const errors = entitlementIssues(candidate, running, st.entitlements);
    const warnings: ProblemIssue[] = [];
    if (st.status === 'grace')
      warnings.push({
        pointer: '',
        message: `the licence expired; grace period ends in ${st.daysLeft} day(s)`,
        rule: 'license.grace',
      });
    return { errors, warnings };
  }
}

/** 403 problem+json for a commit using unlicensed features; errors[0].pointer = first offending node. */
export function licenseProblem(check: LicenseCheck): ProblemError {
  return new ProblemError(
    403,
    'license-required',
    'Not licensed',
    'the configuration uses features the licence does not cover',
    check.errors,
    { tier: 'license', warnings: check.warnings },
  );
}
