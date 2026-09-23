import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `acl` — ACL.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   lists { ... } (record keyed by name), attachments[] { list, interface, direction, vrf }
 *
 * Guardrails (vdom.md #1, #2): attachments carry `vrf`; list names are unique within `acl` only.
 *
 * TODO(P02b): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/acl.ts`. Only P02b edits this file.
 */
export const AclSchema = withUi(z.looseObject({}), {
  title: 'ACL',
  description: 'Access-control lists and their attachments to interfaces.',
  order: 80,
});

export type AclConfig = z.infer<typeof AclSchema>;
