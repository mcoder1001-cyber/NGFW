import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `system` — System.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   hostname (RFC 1123), timezone (IANA), banner, ntp { ... }, dns { ... }
 *
 * Notes: use the shared primitives (`../primitives.js` hostname) rather than re-declaring them.
 *
 * TODO(P02a): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/system.ts`. Only P02a edits this file.
 */
export const SystemSchema = withUi(z.looseObject({}), {
  title: 'System',
  description: 'Hostname, timezone, login banner, NTP and DNS client settings.',
  order: 10,
});

export type SystemConfig = z.infer<typeof SystemSchema>;
