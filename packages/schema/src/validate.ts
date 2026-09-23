import type { z } from 'zod';
import { RootConfig } from './index.js';
import { jsonPointer } from './pointer.js';
import { validateSemantics } from './semantic/index.js';
import { sortIssues, type SemanticIssue } from './semantic/registry.js';

/**
 * Tiers (a) + (b) of the three-tier validation (docs/00-MASTER-PROMPT.md §4) as one call for the API and the commit
 * engine. Tier (c), the renderer dry-run, lives in vrx-agent.
 */
export type ValidationResult =
  | { ok: true; config: RootConfig }
  | { ok: false; tier: 'schema' | 'semantic'; issues: SemanticIssue[] };

/** Zod issues → `{ pointer, message }` with RFC 6901 pointers (the `pointer` member of RFC 9457 problem+json). */
export function pointerIssues(error: z.ZodError): SemanticIssue[] {
  return sortIssues(
    error.issues.map((issue) => ({
      pointer: jsonPointer(...issue.path.map(String)),
      message: issue.message,
    })),
  );
}

/**
 * Parse an untrusted document with `RootConfig` (defaults filled, unknown keys rejected), then run every semantic
 * validator. Never throws.
 */
export function validateConfig(document: unknown): ValidationResult {
  const parsed = RootConfig.safeParse(document);
  if (!parsed.success) return { ok: false, tier: 'schema', issues: pointerIssues(parsed.error) };
  const issues = validateSemantics(parsed.data);
  return issues.length === 0 ? { ok: true, config: parsed.data } : { ok: false, tier: 'semantic', issues };
}
