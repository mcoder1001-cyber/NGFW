import type { RootConfig, RootKey } from '../index.js';

/**
 * One finding from a semantic (cross-field) validator — tier (b) of the three-tier validation in
 * docs/00-MASTER-PROMPT.md §4. `pointer` is an RFC 6901 pointer into the config document (build it with
 * `jsonPointer()` from `../pointer.js`) and becomes `pointer` in the RFC 9457 problem+json body.
 */
export interface SemanticIssue {
  pointer: string;
  message: string;
}

export type SemanticValidator = (config: RootConfig) => SemanticIssue[];

export interface ValidatorDefinition {
  /** Unique, stable name prefixed with the owning domain, e.g. `interfaces.vrf-exists`. */
  name: string;
  /** Top-level keys this validator reads — lets the commit engine skip validators whose inputs are unchanged. */
  domains: readonly RootKey[];
  validate: SemanticValidator;
}

/** Deterministic ordering for problem+json output: by pointer, then message. */
export function sortIssues(issues: readonly SemanticIssue[]): SemanticIssue[] {
  return [...issues].sort(
    (x, y) => x.pointer.localeCompare(y.pointer) || x.message.localeCompare(y.message),
  );
}

/** Registry of named validators. Validators are pure functions of the whole (schema-valid) document. */
export class SemanticRegistry {
  private readonly validators = new Map<string, ValidatorDefinition>();

  register(definition: ValidatorDefinition): this {
    if (this.validators.has(definition.name)) {
      throw new Error(`semantic validator '${definition.name}' is already registered`);
    }
    this.validators.set(definition.name, definition);
    return this;
  }

  has(name: string): boolean {
    return this.validators.has(name);
  }

  /** Registered validators in registration order. */
  list(): readonly ValidatorDefinition[] {
    return [...this.validators.values()];
  }

  /** Run every validator (or only those touching `domains`) and return the sorted issues; `[]` means valid. */
  run(config: RootConfig, domains?: readonly RootKey[]): SemanticIssue[] {
    const issues: SemanticIssue[] = [];
    for (const v of this.validators.values()) {
      if (domains !== undefined && !v.domains.some((d) => domains.includes(d))) continue;
      issues.push(...v.validate(config));
    }
    return sortIssues(issues);
  }
}
