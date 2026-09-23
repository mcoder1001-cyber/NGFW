import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `objects` — Objects.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   addresses { ... }, addressGroups { ... }, services { ... }, schedules { ... } — each a record keyed by name
 *
 * Guardrail (vdom.md #2): names are unique within their own kind (the record key), never globally;
 * API paths stay `/config/objects/<kind>/<name>`.
 *
 * TODO(P02b): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/objects.ts`. Only P02b edits this file.
 */
export const ObjectsSchema = withUi(z.looseObject({}), {
  title: 'Objects',
  description:
    'Reusable address, address-group, service and schedule objects referenced by ACL and NAT.',
  order: 70,
});

export type ObjectsConfig = z.infer<typeof ObjectsSchema>;
