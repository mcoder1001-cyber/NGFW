import { Inject, Injectable, Logger } from '@nestjs/common';
import type { HostNic } from '@ngfw/proto';
import { deepEqual } from '@ngfw/schema';
import { AgentClient } from '../../agent/agent.client.js';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { CommitService } from '../../commit/commit.service.js';
import { ENV, type Env } from '../../config.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { emptyDocument } from '../../datastore/documents.js';
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
 * F-default-vpp-nics (D-164): on first boot (no revision, empty candidate) every non-management host NIC becomes a
 * dataplane interface owned by VPP. The service asks the agent for the host NIC inventory (HostNics), seeds
 * `dataplane.managementPci` / `pciWhitelist` / `devices` and one `interfaces.<netdev>` with a `physical` marker per
 * non-management NIC, and commits revision 1 (system commit, no author) — auditing `system.seed-defaults`.
 *
 * Idempotent: it never re-seeds once any revision exists or the document already carries a physical interface. If the
 * agent is unreachable at boot it is retried on the next agent connect (AgentClient.watchReady) and never blocks the
 * API from starting. Fail-closed (D-192): off unless `NGFW_SEED_DEFAULT_NICS=1`, which only the product unit/firstboot (P10) sets.
 */
@Injectable()
export class SeedService {
  private readonly log = new Logger('SeedDefaultNics');
  private inFlight = false;
  private done = false;
  private stopWatch: (() => void) | undefined;
  private retryTimer: NodeJS.Timeout | undefined;
  /** Outcome of the previous non-final attempt: a repeat is logged at debug and not recorded again (review MINOR 2). */
  private lastOutcome: SeedOutcome | undefined;
  /** Delay before retrying a deferred / refused seed while the agent stays connected. */
  retryMs = 60_000;

  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
    private readonly commits: CommitService,
    private readonly events: SystemEventsService,
    @Inject(ENV) private readonly env: Env,
  ) {}

  /** Try to seed now, and retry on every agent (re)connect until it succeeds or is no longer needed. */
  start(): void {
    if (!this.env.NGFW_SEED_DEFAULT_NICS) {
      this.log.log('default NIC seeding disabled (NGFW_SEED_DEFAULT_NICS is not 1)');
      return;
    }
    void this.attempt();
    this.stopWatch = this.agent.watchReady(() => void this.attempt());
  }

  stop(): void {
    this.stopWatch?.();
    this.stopWatch = undefined;
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.retryTimer = undefined;
  }

  /** Warn on the first occurrence of a non-final outcome (or when it changes); retries repeating it log at debug. */
  private retryLog(outcome: SeedOutcome, message: string): void {
    if (outcome === this.lastOutcome) this.log.debug(message);
    else this.log.warn(message);
  }

  /** One seed attempt; a deferred / refused / unreachable outcome is retried after `retryMs` (and on agent connect). */
  private async attempt(): Promise<void> {
    const outcome = await this.seedIfNeeded().catch((e: unknown) => {
      this.retryLog('deferred', `seed attempt failed: ${(e as Error).message}`);
      this.lastOutcome = 'deferred';
      return 'deferred' as const;
    });
    if (
      ['deferred', 'unreachable', 'no-management'].includes(outcome) &&
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

  /** Seed once if a revision does not exist yet. Safe to call repeatedly. */
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
      if (running.revision !== null) {
        this.done = true;
        this.stop();
        return 'exists';
      }
      let nics: HostNic[];
      let notes: string[];
      try {
        const resp = await this.agent.hostNics();
        nics = resp.nics;
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
        this.log.log('no non-management host NICs to seed; leaving the document empty');
        this.done = true;
        this.stop();
        return 'no-nics';
      }
      let result: Awaited<ReturnType<CommitService['systemCommit']>>;
      try {
        result = await this.commits.systemCommit(
          (doc) => seedDocument(doc, nics),
          'seed default dataplane NICs from the host inventory (F-default-vpp-nics)',
        );
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
        return 'unchanged';
      }
      this.done = true;
      this.stop();
      await this.events.record(
        'info',
        'system',
        'system.seed-defaults',
        `seeded ${data.length} dataplane NIC(s) and ${nics.length - data.length} management NIC(s) as the default configuration`,
        {
          dataplane: data.map((n) => ({ name: n.name, pci: n.pci })),
          managementNotes: notes.map(reasonOnly),
        },
      );
      this.log.log(`seeded ${data.length} dataplane NIC(s) as revision 1`);
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

/**
 * The default document seeded from a host inventory, or null (→ no commit). Only an untouched document is seeded
 * (review R1R3 #8: equal to emptyDocument(), so an operator commit that raced ahead is never layered over), and only
 * with at least one management NIC and one data NIC. Pure: the golden test pins its output.
 */
export function seedDocument(doc: Doc, nics: HostNic[]): Doc | null {
  if (!deepEqual(doc, emptyDocument())) return null;
  const mgmt = nics.flatMap((n) => (n.isManagement && n.pci ? [n.pci] : []));
  const data = dataNics(nics);
  if (data.length === 0 || mgmt.length === 0) return null;
  const out = structuredClone(doc) as unknown as {
    dataplane: {
      managementPci: string[];
      pciWhitelist: string[];
      devices: Record<string, { name: string }>;
    };
    interfaces: Record<string, unknown>;
  };
  out.dataplane.managementPci = mgmt;
  out.dataplane.pciWhitelist = data.map((n) => n.pci);
  for (const n of data) {
    out.dataplane.devices[n.pci] = { name: n.name };
    out.interfaces[n.name] = {
      enabled: true,
      physical: { pci: n.pci, owner: 'dataplane', builtIn: true },
    };
  }
  return out as unknown as Doc;
}
