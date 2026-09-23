import { z } from 'zod';
import { withUi } from '../ui.js';

/**
 * `dataplane` — Dataplane.
 *
 * Target shape (docs/04-api-datamodel.md, prompts/P02-schema-package.md §2):
 *   workers, rxQueues, hugepagesGb, pciWhitelist[], mainCore, corelist
 *
 * Notes: rendered into the VPP startup configuration by vrx-agent — the schema only carries desired values.
 *
 * TODO(P02a): replace this passthrough placeholder with the full model and add the domain's semantic
 * validators in `../semantic/dataplane.ts`. Only P02a edits this file.
 */
export const DataplaneSchema = withUi(z.looseObject({}), {
  title: 'Dataplane',
  description:
    'VPP data-plane tuning: worker threads, RX queues, hugepages, PCI whitelist and core placement.',
  order: 20,
});

export type DataplaneConfig = z.infer<typeof DataplaneSchema>;
