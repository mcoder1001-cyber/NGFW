import { Inject, Injectable, Logger, type OnModuleDestroy } from '@nestjs/common';
import type { HostNic } from '@ngfw/proto';
import { logicalIfName, type DataplaneConfig, type InterfaceConfig } from '@ngfw/schema';
import { AgentClient } from '../../agent/agent.client.js';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { CommitService } from '../../commit/commit.service.js';
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
  | 'unreachable';

/**
 * F-default-vpp-nics (D-164; owner instruction 2026-10-10, D-249): at every API boot the non-management host NICs the
 * running configuration does not carry yet are registered as dataplane interfaces. The service asks the agent for the
 * host NIC inventory (HostNics), adds `dataplane.managementPci` / `pciWhitelist` / `devices` and one
 * `interfaces.<netdev>` row with a `physical` marker per missing NIC, and persists a system commit (no author) —
 * audited as `system.seed-defaults`. On a fresh appliance that is revision 1; on an existing installation it is one
 * more revision layered over the saved configuration.
 *
 * Idempotent: NICs are matched by PCI address, so existing rows, their logical names, settings, device tuning and host
 * ownership (released NICs) are preserved and an unchanged inventory creates no revision. The commit engine defers the
 * system commit while a candidate has edits or a commit awaits confirmation (the setup wizard included); it is retried
 * every `retryMs`. If the agent is unreachable at boot it is retried on the next agent connect (AgentClient.watchReady)
 * and never blocks the API from starting. Fail-closed (D-192): off unless `NGFW_SEED_DEFAULT_NICS=1`, which only the
 * product unit/firstboot (P10) sets.
 */
@Injectable()
export class SeedService implements OnModuleDestroy {
  private readonly log = new Logger('SeedDefaultNics');
  private inFlight = false;
  private done = false;
  private started = false;
  private stopWatch: (() => void) | undefined;
  private retryTimer: NodeJS.Timeout | undefined;
  /** Outcome of the previous non-final attempt: a repeat is logged at debug and not recorded again (review MINOR 2). */
  private lastOutcome: SeedOutcome | undefined;
  /** Delay before retrying a deferred / refused registration while the agent stays connected. */
  retryMs = 60_000;

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
      this.log.log('default NIC seeding disabled (NGFW_SEED_DEFAULT_NICS is not 1)');
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

  /** Warn on the first occurrence of a non-final outcome (or when it changes); retries repeating it log at debug. */
  private retryLog(outcome: SeedOutcome, message: string): void {
    if (outcome === this.lastOutcome) this.log.debug(message);
    else this.log.warn(message);
  }

  /** One attempt; a deferred / refused / unreachable / empty outcome is retried after `retryMs` (and on agent connect). */
  private async attempt(): Promise<void> {
    if (!this.started) return;
    const outcome = await this.seedIfNeeded().catch((e: unknown) => {
      this.retryLog('deferred', `seed attempt failed: ${(e as Error).message}`);
      this.lastOutcome = 'deferred';
      return 'deferred' as const;
    });
    if (
      ['deferred', 'unreachable', 'no-management', 'no-nics'].includes(outcome) &&
      this.started &&
      !this.done &&
      !this.retryTimer
    ) {
      this.retryTimer = setTimeout(() => {
        this.retryTimer = undefined;
        void this.attempt();
      }, this.retryMs);
      this.retryTimer.unref();
    }
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
          `host NIC inventory unavailable, will retry: ${(e as Error).message}`,
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
            'default NIC seeding deferred: no management NIC could be identified (no default route or control connection); set NGFW_MGMT_IF / NGFW_MGMT_PCI or bring up the management network',
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
      let registered: { name: string; pci: string }[] = [];
      try {
        // `change` runs under the commit lock on the latest running document, so a candidate edit, a pending
        // confirmation or an operator commit that raced ahead is never overwritten (systemCommit defers instead)
        result = await this.commits.systemCommit((doc) => {
          const next = seedDocument(doc, nics);
          if (next) registered = registeredRows(doc, next);
          return next;
        }, 'register missing dataplane NICs from the host inventory (F-default-vpp-nics)');
      } catch (e) {
        this.retryLog('deferred', `seed commit failed, will retry: ${(e as Error).message}`);
        return 'deferred';
      }
      if (result.status === 'deferred') {
        this.retryLog('deferred', `seed deferred (${result.reason}); will retry`);
        return 'deferred';
      }
      if (result.status === 'unchanged') {
        this.done = true;
        this.stop();
        this.log.log('every non-management host NIC is already registered; nothing to add');
        return running.revision === null ? 'unchanged' : 'exists';
      }
      this.done = true;
      this.stop();
      await this.events.record(
        'info',
        'system',
        'system.seed-defaults',
        `registered ${registered.length} missing dataplane NIC(s) from the host inventory`,
        {
          dataplane: registered,
          managementNotes: notes.map(reasonOnly),
        },
      );
      this.log.log(
        `registered ${registered.length} missing dataplane NIC(s) from the host inventory (${registered.map((r) => r.name).join(', ')})`,
      );
      return 'seeded';
    } finally {
      this.inFlight = false;
    }
  }
}

/** review R2R4 #5: persist only the reason class of a management note, never a peer address ("control connection from 1.2.3.4" → "control connection"). */
export function reasonOnly(note: string): string {
  return note.replace(/control connection from [^)\s]+/g, 'control connection');
}

/**
 * Logical name for a NIC already bound to a DPDK driver (no netdev): systemd's predictable form of its PCI address,
 * `enp<bus>s<slot>f<function>` (decimal; `en<domain>p…` outside domain 0). undefined when it would not be a valid
 * logical name (1–15 chars, D-069).
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

/** The interface rows `after` carries a physical marker for that `before` did not (what one registration added). */
function registeredRows(before: Doc, after: Doc): { name: string; pci: string }[] {
  const prev = (before['interfaces'] ?? {}) as Record<string, Partial<InterfaceConfig>>;
  const next = (after['interfaces'] ?? {}) as Record<string, Partial<InterfaceConfig>>;
  return Object.entries(next).flatMap(([name, itf]) =>
    itf.physical && !prev[name]?.physical ? [{ name, pci: itf.physical.pci }] : [],
  );
}

/** Names the data plane creates itself or the startup generator reserves: never adopted as a physical NIC row. */
const RESERVED_NAME = /^(?:local0$|loop\d+$|host-)/;

/**
 * The running document with the missing non-management NICs of `nics` registered, or null when nothing needs adding
 * (→ no commit). Pure: the golden test pins its output for an empty document. Rules:
 * - a NIC is identified by its PCI address (case-insensitive): a PCI that already has a physical row is never added
 *   twice, even after binding removed or renamed its kernel netdev;
 * - management NICs — flagged by the host or saved in `dataplane.managementPci` — are never registered;
 * - an existing `dataplane.devices.<pci>.name` keeps that logical name (its tuning untouched); otherwise the kernel
 *   name (or the PCI-derived name, pciIfName) is used, and an invalid or reserved name is skipped;
 * - an existing row of that name is adopted (its settings kept, the `physical` marker added) unless it is already a
 *   physical NIC or a bond;
 * - other rows, the whitelist and released NICs (owner: host) are untouched; the discovered management PCI addresses
 *   are appended to `dataplane.managementPci` only when something was registered.
 * Needs at least one management NIC and one data NIC (review R1R3 #8, R2R4 #2).
 */
export function seedDocument(doc: Doc, nics: HostNic[]): Doc | null {
  const mgmt = nics.flatMap((n) => (n.isManagement && n.pci ? [n.pci] : []));
  const data = dataNics(nics);
  if (data.length === 0 || mgmt.length === 0) return null;
  const out = structuredClone(doc) as Doc & {
    dataplane?: Partial<DataplaneConfig>;
    interfaces?: Record<string, Partial<InterfaceConfig>>;
  };
  const dp = (out.dataplane ??= {});
  const managementPci = (dp.managementPci ??= []);
  const pciWhitelist = (dp.pciWhitelist ??= []);
  const devices = (dp.devices ??= {});
  const interfaces = (out.interfaces ??= {});
  const protectedPci = new Set([...managementPci, ...mgmt].map((p) => p.toLowerCase()));
  const rows = new Map(
    Object.entries(interfaces).flatMap(([name, itf]) =>
      itf.physical ? [[itf.physical.pci.toLowerCase(), name] as const] : [],
    ),
  );
  const deviceKeys = new Map(Object.keys(devices).map((pci) => [pci.toLowerCase(), pci]));
  const whitelist = new Set(pciWhitelist.map((pci) => pci.toLowerCase()));
  let added = 0;
  for (const n of data) {
    const pci = n.pci.toLowerCase();
    if (protectedPci.has(pci) || rows.has(pci)) continue;
    const deviceKey = deviceKeys.get(pci) ?? n.pci;
    const device = devices[deviceKey];
    const name = device?.name ?? n.name;
    // never reinterpret a reserved or invalid name, nor attach a second NIC to a physical row or a bond
    if (!logicalIfName.safeParse(name).success || RESERVED_NAME.test(name)) continue;
    const existing = interfaces[name];
    if (existing?.physical || existing?.bond) continue;
    devices[deviceKey] = { ...device, name };
    if (!whitelist.has(pci)) {
      pciWhitelist.push(n.pci);
      whitelist.add(pci);
    }
    interfaces[name] = {
      enabled: true,
      ...existing,
      physical: { pci: n.pci, owner: 'dataplane', builtIn: true },
    };
    rows.set(pci, name);
    added++;
  }
  if (added === 0) return null;
  const saved = new Set(managementPci.map((pci) => pci.toLowerCase()));
  for (const pci of mgmt) {
    if (!saved.has(pci.toLowerCase())) {
      managementPci.push(pci);
      saved.add(pci.toLowerCase());
    }
  }
  return out;
}
