import { Inject, Injectable, Optional } from '@nestjs/common';
import { DesiredState, IssueSeverity, type ObjectResult, type ValidationIssue } from '@ngfw/proto';
import { ROOT_KEYS, validateConfig } from '@ngfw/schema';
import { AgentClient } from '../agent/agent.client.js';
import type { ProblemIssue } from '../common/problem.js';
import { licenseProblem, LicensingService } from '../features/licensing/index.js'; // wave-BC: F-licensing (unanchored)
import { CONFIG_REPO } from '../datastore/datastore.service.js';
import { hydrateHashes, missingSecretIssues, redact, secretRefs } from '../datastore/documents.js';
import type { ConfigRepo, Doc } from '../datastore/repo.js';

export type ValidationTier = 'schema' | 'semantic' | 'agent' | 'license'; // wave-BC: F-licensing ('license', unanchored)

export interface PlanEntry {
  key: string;
  op: string;
  pointer: string;
  subsystem: string;
}

export interface ValidationOutcome {
  ok: boolean;
  tier?: ValidationTier;
  errors: ProblemIssue[];
  warnings: ProblemIssue[];
  /** Parsed, hydrated document (carries password hashes — never returned, never logged). */
  config?: Doc;
  desired?: DesiredState;
  /** Subsystems sent to the agent (implemented by it, HealthResponse.subsystems). */
  subsystems: string[];
  /** Top-level keys this agent build does not implement: committed to running, not applied (yet). */
  notApplied: string[];
  plan: PlanEntry[];
}

function issue(i: ValidationIssue): ProblemIssue {
  const out: ProblemIssue = { pointer: i.pointer, message: i.message };
  if (i.rule) out.rule = i.rule;
  return out;
}

export function planEntry(r: ObjectResult): PlanEntry {
  return { key: r.key, op: opName(r.op), pointer: r.pointer, subsystem: r.subsystem };
}

function opName(op: number): string {
  return ['unspecified', 'create', 'update', 'delete', 'recreate', 'noop'][op] ?? String(op);
}

/**
 * Three-tier validation (00-CONTEXT rule 4, P06 §3): (a) schema → (b) semantic (packages/schema validators + the
 * API's secret-existence rule, D-051) → (c) agent DryRun. Stops at the first failing tier; every error carries an
 * RFC 6901 pointer into the document.
 */
@Injectable()
export class ValidationService {
  constructor(
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    private readonly agent: AgentClient,
    // wave-BC: F-licensing (unanchored) — optional so unit tests that build the service by hand keep working
    @Optional() private readonly licensing?: LicensingService,
  ) {}

  /** The agent's DesiredState for a parsed document: protobuf JSON projection without secret leaves (D-040). */
  static desiredState(config: Doc): DesiredState {
    return DesiredState.fromJSON(redact(config));
  }

  /** Subsystems the agent implements (Health) and the top-level keys it does not. */
  async implemented(): Promise<{ subsystems: string[]; notApplied: string[] }> {
    const health = await this.agent.health();
    const set = new Set(health.subsystems);
    return {
      subsystems: ROOT_KEYS.filter((k) => set.has(k)),
      notApplied: ROOT_KEYS.filter((k) => !set.has(k)),
    };
  }

  async validate(doc: Doc, txnId: string): Promise<ValidationOutcome> {
    const base = {
      warnings: [] as ProblemIssue[],
      subsystems: [] as string[],
      notApplied: [] as string[],
      plan: [],
    };
    const hydrated = hydrateHashes(doc, await this.repo.userHashes());
    const ab = validateConfig(hydrated);
    if (!ab.ok) return { ...base, ok: false, tier: ab.tier, errors: ab.issues };
    const config = ab.config as Doc;

    const refs = secretRefs(config);
    const missing = missingSecretIssues(
      refs,
      await this.repo.existingSecretRefs(refs.map((r) => r.ref)),
    );
    if (missing.length > 0)
      return { ...base, ok: false, tier: 'semantic', errors: missing, config };

    // wave-BC: F-licensing (unanchored): licence stage — 403 problem+json, pointer of the first unlicensed node
    const license = this.licensing ? await this.licensing.check(config) : undefined;
    if (license && license.errors.length > 0) throw licenseProblem(license);

    const health = await this.agent.health();
    const implemented = new Set(health.subsystems);
    const subsystems = ROOT_KEYS.filter((k) => implemented.has(k));
    const notApplied = ROOT_KEYS.filter((k) => !implemented.has(k));
    const desired = ValidationService.desiredState(config);
    const report = await this.agent.dryRun({ txnId, desiredState: desired, subsystems });
    const errors = report.errors
      .filter((i) => i.severity === IssueSeverity.ISSUE_SEVERITY_ERROR)
      .map(issue);
    const warnings = report.errors
      .filter((i) => i.severity !== IssueSeverity.ISSUE_SEVERITY_ERROR)
      .map(issue);
    const outcome = {
      warnings: [...(license?.warnings ?? []), ...warnings], // wave-BC: F-licensing (unanchored)
      subsystems,
      notApplied,
      config,
      desired,
      plan: report.plan.map(planEntry),
    };
    if (!report.ok || errors.length > 0) return { ...outcome, ok: false, tier: 'agent', errors };
    return { ...outcome, ok: true, errors: [] };
  }
}
