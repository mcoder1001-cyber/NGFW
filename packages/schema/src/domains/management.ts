import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `management` — Management.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   users[] { username, role (admin|operator|readonly), passwordHash }, aaa { ... }, tls { ... }, syslog[]
 *
 * Guardrail (vdom.md #3): role assignments are `{ role, scope: "*" }` — `scope` becomes a tenant name later.
 * Secrets rule (00-CONTEXT #10): `passwordHash` is write-only — never returned by GET, never logged.
 * Semantic validator to add: at least one admin user.
 *
 * TODO(P02a): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/management.ts`. Only P02a edits this file.
 */
export const ManagementSchema = withUi(z.looseObject({}), {
  title: 'Management',
  description: 'Local users, AAA, TLS and syslog.',
  order: 130,
});

export type ManagementConfig = z.infer<typeof ManagementSchema>;
