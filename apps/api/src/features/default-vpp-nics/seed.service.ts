import { Inject, Injectable, Logger, type OnModuleDestroy } from '@nestjs/common';
import type { HostNic } from '@ngfw/proto';
import { logicalIfName, type DataplaneConfig, type InterfaceConfig } from '@ngfw/schema';
import { AgentClient } from '../../agent/agent.client.js';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { CommitService } from '../../commit/commit.service.js';
import { ProblemError } from '../../common/problem.js';
import { ENV, type Env } from '../../config.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import type { Doc } from '../../datastore/repo.js';

type SeedOutcome =
  | 'seeded'
  | 'skipped'
  | 'exists'
  | 'no-nics'
  | 'no-management'
  | 'unchanged'
  | 'deferred'
  | 'unreachable'
  | 'failed';

/** Outcomes that are retried while the service is started (on a timer and on every agent connect). */
const RETRIED: ReadonlySet<SeedOutcome> = new Set<SeedOutcome>([
  'deferred',
  'unreachable',
  'no-management',
  'no-nics',
  'failed',
]);
/** Outcomes with no operator in the loop, retried with exponential backoff instead of the fixed `retryMs`. */
const BACKED_OFF: ReadonlySet<SeedOutcome> = new Set<SeedOutcome>(['no-nics', 'failed']);
/** Problem slugs of a system commit that a retry cannot pass without an operator's change: final for this API start. */
const FINAL_PROBLEMS: ReadonlySet<string> = new Set(['validation', 'license-required']);

/**
 * F-default-vpp-nics (HIST-NATIVE-D-164; owner instruction 2026-10-10, D-249): at every API start the non-management
 * host NICs the running configuration does not carry yet are registered as dataplane interfaces. The service asks the
 * agent for the host NIC inventory (HostNics), adds `dataplane.managementPci` / `pciWhitelist` / `devices` and one
 * `interfaces.<netdev>` row with a `physical` marker per missing NIC, and persists a system commit (no author) —
 * audited as `system.seed-defaults`. On a fresh appliance that is revision 1; on an existing installation it is one
 * more revision layered over the saved configuration.
 *
 * Idempotent: NICs are matched by PCI address, so existing rows, their logical names, settings, device tuning and host
 * ownership (released NICs) are preserved and an unchanged inventory creates no revision. The commit engine defers the
 * system commit while a candidate has edits or a commit awaits confirmation (the setup wizard included); it is retried
 * every `retryMs` and on agent reconnect. A commit the validators reject is final for this boot and recorded once as
 * the warning event `system.seed-defaults-failed` (with the pointers); other failures and an empty data inventory are
 * retried with exponential backoff up to `maxRetryMs`. NICs that cannot be registered (reserved/invalid name, name in
 * use, a management NIC that is already engine-owned) are reported in the audit event and as
 * `system.seed-defaults-skipped`. If the agent is unreachable at boot it is retried on the next agent connect
 * (AgentClient.watchReady) and never blocks the API from starting. Fail-closed (D-192): off unless
 * `NGFW_SEED_DEFAULT_NICS=1`, which only the appliance API unit sets.
 */
@Injectable()
export class SeedService implements OnModuleDestroy {
  private readonly log = new Logger('SeedDefaultNics');
  private inFlight = false;
  private done = false;
  private started = false;
  private stopWatch: (() => void) | undefined;
  private retryTimer: NodeJS.Timeout | undefined;
  /** Outcome of the previous non-final attempt: the no-management deferral is recorded once per streak (review MINOR 2). */
  private lastOutcome: SeedOutcome | undefined;
  /** Outcome + reason of the previous non-final attempt: a repeat is logged at debug and not recorded again. */
  private lastKey: string | undefined;
  /** Reason of the last recorded `system.seed-defaults-failed` event: a failure alternating with another outcome is not re-recorded. */
  private lastFailure: string | undefined;
  /** Current backoff delay for the outcomes in BACKED_OFF (0 = none yet). */
  private backoffMs = 0;
  /** Delay before retrying a deferred / refused registration while the agent stays connected. */
  retryMs = 60_000;
  /** Upper bound of the exponential backoff for failed commits and an empty data inventory. */
  maxRetryMs = 3_600_000;

  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
    private readonly commits: CommitService,
    private readonly events: SystemEventsService,
    @Inject(ENV) private readonly env: Env,
  ) {}

  /** Register now, and retry on every agent (re)connect until it succeeds or there is nothing to add. */
  start(): void {
    if (this.started || this.done) return;
    if (!this.env.NGFW_SEED_DEFAULT_NICS) {
      this.log.log('default NIC registration disabled (NGFW_SEED_DEFAULT_NICS is not 1)');
      return;
    }
    this.started = true;
    // subscribe before the first attempt: a synchronous outcome that calls stop() must find the watch to cancel
    this.stopWatch = this.agent.watchReady(() => void this.attempt());
    void this.attempt();
  }

  stop(): void {
    this.started = false;
    this.stopWatch?.();
    this.stopWatch = undefined;
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.retryTimer = undefined;
  }

  onModuleDestroy(): void {
    this.stop();
  }

  /** Warn on the first occurrence of a non-final outcome+reason (or when it changes); repeats log at debug. */
  private retryLog(outcome: SeedOutcome, message: string): boolean {
    const key = `${outcome}:${message}`;
    const first = key !== this.lastKey;
    this.lastKey = key;
    if (first) this.log.warn(message);
    else this.log.debug(message);
    return first;
  }

  /** One attempt; a non-final outcome is retried after `retryMs` (or the backoff) and on agent connect. */
  private async attempt(): Promise<void> {
    if (!this.started) return;
    const outcome = await this.seedIfNeeded().catch((e: unknown) => {
      this.retryLog('deferred', `seed attempt failed: ${reasonOnly((e as Error).message)}`);
      this.lastOutcome = 'deferred';
      return 'deferred' as const;
    });
    // an attempt that overlapped another one (agent READY callback or timer during an attempt) decides nothing
    if (outcome === 'skipped') return;
    if (!BACKED_OFF.has(outcome)) this.backoffMs = 0;
    if (RETRIED.has(outcome) && this.started && !this.done) {
      // the latest outcome decides the next delay: a deferral after a long backoff is retried in `retryMs`, not after
      // the pending long timer
      const delay = BACKED_OFF.has(outcome) ? this.nextBackoff() : this.retryMs;
      if (this.retryTimer) clearTimeout(this.retryTimer);
      this.retryTimer = setTimeout(() => {
        this.retryTimer = undefined;
        void this.attempt();
      }, delay);
      this.retryTimer.unref();
    }
  }

  private nextBackoff(): number {
    this.backoffMs = Math.min(this.backoffMs === 0 ? this.retryMs : this.backoffMs * 2, this.maxRetryMs);
    return this.backoffMs;
  }

  /** Register the missing NICs, whatever else has been committed already. Safe to call repeatedly. */
  async seedIfNeeded(): Promise<SeedOutcome> {
    const outcome = await this.seedOnce();
    if (outcome !== 'skipped') this.lastOutcome = outcome;
    return outcome;
  }

  private async seedOnce(): Promise<SeedOutcome> {
    if (!this.env.NGFW_SEED_DEFAULT_NICS) return 'skipped';
    if (this.done || this.inFlight) return 'skipped';
    this.inFlight = true;
    try {
      const running = await this.ds.getRunning();
      let nics: HostNic[];
      let notes: string[];
      try {
        const resp = await this.agent.hostNics();
        // Management identity = the host's decision (default route, control connection, NGFW_MGMT_IF/PCI) plus the
        // management PCI addresses saved by an earlier boot, so a management NIC whose route is still coming up is
        // never handed to the data plane.
        const saved = savedManagementPci(running.doc);
        nics = resp.nics.map((n) =>
          !n.isManagement && n.pci && saved.has(n.pci.toLowerCase())
            ? { ...n, isManagement: true }
            : n,
        );
        notes = resp.managementNotes;
      } catch (e) {
        this.retryLog(
          'unreachable',
          `host NIC inventory unavailable, will retry: ${reasonOnly((e as Error).message)}`,
        );
        return 'unreachable';
      }
      // review R2R4 #2: without an identified management NIC (no default route, no sshd peer, no --mgmt-if/--mgmt-pci
      // — e.g. first boot before DHCP) every NIC would be handed to the data plane, the management NIC included, as a
      // non-deletable row. Refuse, record a warning and retry on the next agent connect.
      if (!nics.some((n) => n.isManagement && n.pci)) {
        this.retryLog(
          'no-management',
          'no management NIC identified on the host; not seeding (will retry on agent connect)',
        );
        // recorded once per deferral (the first one, or after a different outcome), not on every retry
        if (this.lastOutcome !== 'no-management')
          await this.events.record(
            'warning',
            'system',
            'system.seed-defaults-deferred',
            'default NIC registration deferred (will retry): no management NIC could be identified (no default route or control connection); set NGFW_MGMT_IF / NGFW_MGMT_PCI or bring up the management network',
            { nics: nics.length },
          );
        return 'no-management';
      }
      const data = dataNics(nics);
      if (data.length === 0) {
        // not final: a NIC that is still being probed, or installed later, shows up on a retry or the next boot
        this.retryLog('no-nics', 'no non-management host NICs to register yet; will retry');
        return 'no-nics';
      }
      let result: Awaited<ReturnType<CommitService['systemCommit']>>;
      let plan: RegistrationPlan | undefined;
      try {
        // `change` runs under the commit lock on the latest running document, so a candidate edit, a pending
        // confirmation or an operator commit that raced ahead is never overwritten (systemCommit defers instead)
        result = await this.commits.systemCommit((doc) => {
          plan = planRegistration(doc, nics);
          return plan.next;
        }, 'register missing dataplane NICs from the host inventory (F-default-vpp-nics)');
      } catch (e) {
        return this.failed(e);
      }
      if (result.status === 'deferred') {
        this.retryLog('deferred', `seed deferred (${result.reason}); will retry`);
        return 'deferred';
      }
      const skipped = plan?.skipped ?? [];
      if (result.status === 'unchanged') {
        this.done = true;
        this.stop();
        if (skipped.length > 0) {
          this.log.warn(`no NIC registered; ${describeSkipped(skipped)}`);
          await this.events.record(
            'warning',
            'system',
            'system.seed-defaults-skipped',
            `default NIC registration left ${skipped.length} host NIC(s) unregistered (checked again at the next API start): ${describeSkipped(skipped)}`,
            { skipped },
          );
        } else this.log.log('every non-management host NIC is already registered; nothing to add');
        return running.revision === null ? 'unchanged' : 'exists';
      }
      this.done = true;
      this.stop();
      const added = plan?.added ?? [];
      await this.events.record(
        'info',
        'system',
        'system.seed-defaults',
        `registered ${added.length} missing dataplane NIC(s) from the host inventory`,
        {
          dataplane: added,
          management: plan?.management ?? [],
          skipped,
          managementNotes: notes.map(reasonOnly),
        },
      );
      this.log.log(
        `registered ${added.length} missing dataplane NIC(s) from the host inventory (${added.map((r) => r.name).join(', ')})`,
      );
      if (skipped.length > 0) this.log.warn(describeSkipped(skipped));
      return 'seeded';
    } finally {
      this.inFlight = false;
    }
  }

  /**
   * The system commit threw. A deterministic rejection — the validators (schema, semantic, renderer dry-run → problem
   * `validation`) or a missing licence — is final for this API start: recorded once as a warning event with its
   * pointers; the next start retries and the operator can fix the configuration meanwhile. Anything else (agent or
   * database unavailable, an agent refusal or a failed apply that a later attempt may pass) is retried with backoff and
   * recorded once per distinct reason. The event message says which, as the operator sees only the message.
   */
  private async failed(e: unknown): Promise<SeedOutcome> {
    const problem = e instanceof ProblemError ? e : undefined;
    const final =
      problem !== undefined && problem.getStatus() < 500 && FINAL_PROBLEMS.has(problem.slug);
    const reason = reasonOnly((e as Error).message);
    const issues = (problem?.errors ?? [])
      .slice(0, 20)
      .map((issue) => ({ pointer: issue.pointer, message: reasonOnly(issue.message) }));
    const pointers = issues.map((issue) => issue.pointer);
    const retry = final ? 'not retried before the next API start' : 'will retry';
    if (final) {
      // final before the event is recorded: a failing event store must not turn a final problem into a retry loop
      this.done = true;
      this.stop();
    }
    this.retryLog(
      'failed',
      `default NIC registration commit failed (${retry}): ${reason}${pointers.length > 0 ? ` (${pointers.join(', ')})` : ''}`,
    );
    const key = `${reason}|${pointers.join(',')}`;
    if (key !== this.lastFailure) {
      this.lastFailure = key;
      await this.events.record(
        'warning',
        'system',
        'system.seed-defaults-failed',
        `default NIC registration failed (${retry}): ${reason}`,
        { reason, pointers, issues, final, ...(problem ? { type: problem.slug } : {}) },
      );
    }
    return 'failed';
  }
}

/** review R2R4 #5: persist only the reason class of a management note, never a peer address ("control connection from 1.2.3.4" → "control connection"). */
export function reasonOnly(note: string): string {
  return note.replace(/control connection from [^)\s]+/g, 'control connection');
}

/**
 * A display name for a NIC already bound to a DPDK driver (no netdev), derived from its PCI address in systemd's
 * `enp<bus>s<slot>f<function>` style (decimal; `en<domain>p…` outside domain 0) — not necessarily the name the kernel
 * or VPP gave it, so it is used for inventory counts and reports only, never as a registered logical name (D-249).
 * undefined when it would not be a valid logical name (1–15 chars, D-069).
 */
export function pciIfName(pci: string): string | undefined {
  const m = /^([0-9a-f]{4}):([0-9a-f]{2}):([0-9a-f]{2})\.([0-7])$/i.exec(pci);
  if (!m) return undefined;
  const [dom, bus, dev, fn] = m.slice(1).map((x) => parseInt(x, 16)) as [
    number,
    number,
    number,
    number,
  ];
  const name = `en${dom === 0 ? '' : dom}p${bus}s${dev}f${fn}`;
  return /^[a-z](?:[a-z0-9_-]{0,13}[a-z0-9])?$/.test(name) ? name : undefined;
}

/** The non-management NICs the seed hands to the data plane, each with its logical name (netdev, else from the PCI). */
export function dataNics(nics: HostNic[]): { name: string; pci: string }[] {
  return nics.flatMap((n) => {
    if (n.isManagement || !n.pci) return [];
    const name = n.netdev || pciIfName(n.pci);
    return name ? [{ name, pci: n.pci }] : [];
  });
}

/** `dataplane.managementPci` of a running document, lower-cased (empty when the document has none). */
function savedManagementPci(doc: Doc): Set<string> {
  const dp = doc['dataplane'] as Partial<DataplaneConfig> | undefined;
  return new Set((dp?.managementPci ?? []).map((pci) => pci.toLowerCase()));
}

/**
 * Names the startup generator refuses (apps/agent/internal/renderers/vppstartup/validate.go: reservedNames and
 * reservedStems — a stem alone or followed by a digit, `_` or `-` collides with an interface VPP or a descriptor
 * creates). Never adopted as a physical NIC row, whatever `dataplane.devices.<pci>.name` says.
 */
const RESERVED_NAMES = new Set(['local0', 'default', 'none', 'any', 'all']);
const RESERVED_STEMS = [
  'loop', 'tap', 'tun', 'host', 'memif', 'vxlan', 'gre', 'ipip', 'ipsec', 'wg', 'bond',
  'pg', 'lcp', 'bvi', 'vhost', 'avf', 'rdma', 'af_xdp', 'vmxnet3', 'pppoe', 'gtpu',
  'geneve', 'lisp', 'mpls', 'sr', 'srv6', 'pipe', 'punt', 'span', 'local', 'virtio', 'tapcli',
];
export function isReservedIfName(name: string): boolean {
  if (RESERVED_NAMES.has(name)) return true;
  return RESERVED_STEMS.some((stem) => {
    if (!name.startsWith(stem)) return false;
    const rest = name.slice(stem.length);
    return rest === '' || /^[0-9_-]/.test(rest);
  });
}

export type SkipReason =
  /** the kernel (or saved device) name is not a valid logical name (D-069) */
  | 'invalid-name'
  /** the name is one VPP or a descriptor creates itself (local0, loopN, tapN, bondN, …) */
  | 'reserved-name'
  /** an interface of that name already is a physical NIC, a bond or a PPPoE client */
  | 'name-in-use'
  /** the host flags a NIC as management that the configuration hands to the engine (pciWhitelist/devices) */
  | 'management-engine-owned'
  /** `dataplane.managementPci` already holds the schema maximum of 4 addresses */
  | 'management-limit'
  /** `dataplane.pciWhitelist` already holds the schema maximum of 64 addresses */
  | 'whitelist-limit'
  /** a NIC already bound to a user-space driver has no saved `dataplane.devices.<pci>.name`: VPP's name for it is unknown */
  | 'bound-unnamed'
  /** a kernel NIC whose saved `dataplane.devices.<pci>.name` differs from its kernel name: registered once it is bound */
  | 'name-mismatch';

export interface RegistrationPlan {
  /** The running document with the missing NICs registered, or null when nothing needs adding (→ no commit). */
  next: Doc | null;
  /** Rows this plan adds the physical marker to. */
  added: { name: string; pci: string }[];
  /** The NICs taken as management (host-flagged or saved), for the audit event. */
  management: { name: string; pci: string }[];
  /** Host NICs the plan could not register, with the reason — reported, never silently dropped. */
  skipped: { name: string; pci: string; reason: SkipReason }[];
}

/**
 * Plan the registration of the missing non-management NICs of `nics` over the running document. Pure: the golden test
 * pins `next` for an empty document. Rules:
 * - a NIC is identified by its PCI address (case-insensitive): a PCI that already has a physical row is never added
 *   twice, even after binding removed or renamed its kernel netdev;
 * - management NICs — flagged by the host or saved in `dataplane.managementPci` — are never registered;
 * - the agent tells an unbound NIC apart from a bound one by its kernel name (SplitUnboundPhysical), and VPP names a
 *   bound NIC after `dataplane.devices.<pci>.name`: a kernel NIC is registered under its kernel name — when its saved
 *   device name differs it is skipped (`name-mismatch`) until the start-up configuration binds it under that name —
 *   and a bound NIC under its saved device name — without one it is skipped (`bound-unnamed`); an invalid or reserved
 *   name is skipped;
 * - an existing row of that name is adopted (its settings kept, the `physical` marker added) unless it is already a
 *   physical NIC, a bond or a PPPoE client, or the name is the saved device name / physical row of another PCI
 *   (`dataplane.logical-name-unique` would reject the commit — the operator renames one of them);
 * - at most 64 whitelisted addresses (schema maximum); a 65th data NIC is reported as `whitelist-limit`;
 * - other rows, the whitelist and released NICs (owner: host) are untouched; the discovered management PCI addresses
 *   are appended to `dataplane.managementPci` only when something was registered, never when the configuration hands
 *   that PCI to the engine (`dataplane.management-not-dpdk` would reject the commit — the operator must release the
 *   NIC first; reported as `management-engine-owned`), and never beyond the schema's four addresses.
 * Needs at least one management NIC and one data NIC (review R1R3 #8, R2R4 #2).
 */
export function planRegistration(doc: Doc, nics: HostNic[]): RegistrationPlan {
  const plan: RegistrationPlan = { next: null, added: [], management: [], skipped: [] };
  const mgmt = nics.flatMap((n) =>
    n.isManagement && n.pci ? [{ name: n.netdev || pciIfName(n.pci) || '', pci: n.pci }] : [],
  );
  const data = dataNics(nics);
  if (data.length === 0 || mgmt.length === 0) return plan;
  plan.management = mgmt;
  const out = structuredClone(doc) as Doc & {
    dataplane?: Partial<DataplaneConfig>;
    interfaces?: Record<string, Partial<InterfaceConfig>>;
  };
  const dp = (out.dataplane ??= {});
  const managementPci = (dp.managementPci ??= []);
  const pciWhitelist = (dp.pciWhitelist ??= []);
  const devices = (dp.devices ??= {});
  const interfaces = (out.interfaces ??= {});
  const protectedPci = new Set([...managementPci, ...mgmt.map((m) => m.pci)].map((p) => p.toLowerCase()));
  const rows = new Map(
    Object.entries(interfaces).flatMap(([name, itf]) =>
      itf.physical ? [[itf.physical.pci.toLowerCase(), name] as const] : [],
    ),
  );
  const deviceKeys = new Map(Object.keys(devices).map((pci) => [pci.toLowerCase(), pci]));
  const whitelist = new Set(pciWhitelist.map((pci) => pci.toLowerCase()));
  // logical names already claimed by a PCI (saved device names and physical rows): a NIC never takes another PCI's name
  const claimed = new Map<string, string>();
  for (const [pci, device] of Object.entries(devices))
    if (device.name !== undefined) claimed.set(device.name, pci.toLowerCase());
  for (const [pci, name] of rows) claimed.set(name, pci);
  // A management NIC the configuration hands to the engine is a conflict the operator must resolve (release it):
  // reported whenever a plan is computed (the inventory has a data NIC), whether or not anything is registered, and
  // never written into managementPci.
  const engineOwnedMgmt = mgmt.filter((m) => {
    const pci = m.pci.toLowerCase();
    return whitelist.has(pci) || deviceKeys.has(pci);
  });
  for (const m of engineOwnedMgmt) {
    plan.skipped.push({ name: m.name || (rows.get(m.pci.toLowerCase()) ?? ''), pci: m.pci, reason: 'management-engine-owned' });
  }
  for (const n of nics) {
    if (n.isManagement || !n.pci) continue;
    const pci = n.pci.toLowerCase();
    if (protectedPci.has(pci) || rows.has(pci)) continue;
    const deviceKey = deviceKeys.get(pci) ?? n.pci;
    const device = devices[deviceKey];
    const bound = n.boundToDpdk || !n.netdev;
    if (bound && device?.name === undefined) {
      plan.skipped.push({ name: pciIfName(n.pci) ?? '', pci: n.pci, reason: 'bound-unnamed' });
      continue;
    }
    const name = device?.name ?? n.netdev ?? '';
    if (!bound && device?.name !== undefined && device.name !== n.netdev) {
      plan.skipped.push({ name, pci: n.pci, reason: 'name-mismatch' });
      continue;
    }
    if (!logicalIfName.safeParse(name).success) {
      plan.skipped.push({ name, pci: n.pci, reason: 'invalid-name' });
      continue;
    }
    if (isReservedIfName(name)) {
      plan.skipped.push({ name, pci: n.pci, reason: 'reserved-name' });
      continue;
    }
    const existing = interfaces[name];
    // never attach a second NIC to a physical row, reinterpret a bond or a PPPoE client as a physical port, or take a
    // logical name another PCI's saved device already holds
    const owner = claimed.get(name);
    if (existing?.physical || existing?.bond || existing?.pppoe || (owner !== undefined && owner !== pci)) {
      plan.skipped.push({ name, pci: n.pci, reason: 'name-in-use' });
      continue;
    }
    if (!whitelist.has(pci) && pciWhitelist.length >= 64) {
      plan.skipped.push({ name, pci: n.pci, reason: 'whitelist-limit' });
      continue;
    }
    devices[deviceKey] = { ...device, name };
    if (!whitelist.has(pci)) {
      pciWhitelist.push(n.pci);
      whitelist.add(pci);
    }
    claimed.set(name, pci);
    interfaces[name] = {
      enabled: true,
      ...existing,
      physical: { pci: n.pci, owner: 'dataplane', builtIn: true },
    };
    rows.set(pci, name);
    plan.added.push({ name, pci: n.pci });
  }
  if (plan.added.length === 0) return plan;
  const saved = new Set(managementPci.map((pci) => pci.toLowerCase()));
  const engineOwned = new Set(engineOwnedMgmt.map((m) => m.pci.toLowerCase()));
  for (const m of mgmt) {
    const pci = m.pci.toLowerCase();
    if (saved.has(pci) || engineOwned.has(pci)) continue;
    if (managementPci.length >= 4) {
      plan.skipped.push({ name: m.name, pci: m.pci, reason: 'management-limit' });
      continue;
    }
    managementPci.push(m.pci);
    saved.add(pci);
  }
  plan.next = out;
  return plan;
}

/** The running document with the missing NICs registered, or null when nothing needs adding (→ no commit). */
export function seedDocument(doc: Doc, nics: HostNic[]): Doc | null {
  return planRegistration(doc, nics).next;
}

function describeSkipped(skipped: RegistrationPlan['skipped']): string {
  return skipped.map((s) => `${s.name || s.pci} (${s.pci}): ${s.reason}`).join('; ');
}
