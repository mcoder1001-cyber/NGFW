import { z } from 'zod';
import { cpuCore, pciAddress } from '../primitives.js';
import { withUi } from '../ui.js';

/**
 * `dataplane` — VPP start-up tuning (docs/04-api-datamodel.md). vrx-agent renders these into the `cpu {}`,
 * `dpdk {}` and hugepage settings of the VPP start-up configuration; the schema only carries desired values and
 * every field is optional (absent = keep the platform default). Changing them needs a data-plane restart, which the
 * commit engine reports as a "restart required" warning — nothing here is applied live.
 *
 * `workers` and `corelist` describe the same thing two ways (`cpu { workers N }` vs `cpu { corelist-workers a,b-c }`):
 * when both are given they must agree (`dataplane.workers-match-corelist`), `mainCore` must not be a worker core
 * (`dataplane.main-core-not-worker`) and PCI addresses must be unique (`dataplane.pci-unique`).
 */
export const DataplaneSchema = withUi(
  z.strictObject({
    workers: withUi(z.number().int().min(0).max(255).optional(), {
      title: 'Worker threads',
      help: 'number of VPP worker threads; 0 = run the graph on the main thread only',
      group: 'cpu',
      order: 1,
    }),
    corelist: withUi(z.array(cpuCore).max(256).optional(), {
      title: 'Worker cores',
      help: 'CPU ids pinned to worker threads (corelist-workers); when set, `workers` must equal its length',
      group: 'cpu',
      order: 2,
    }),
    mainCore: withUi(cpuCore.optional(), {
      title: 'Main core',
      help: 'CPU id of the VPP main thread (must not be a worker core)',
      group: 'cpu',
      order: 3,
    }),
    rxQueues: withUi(z.number().int().min(1).max(256).optional(), {
      title: 'RX queues per NIC',
      help: 'dpdk { dev default { num-rx-queues N } }',
      group: 'dpdk',
      order: 4,
    }),
    txQueues: withUi(z.number().int().min(1).max(256).optional(), {
      title: 'TX queues per NIC',
      help: 'dpdk { dev default { num-tx-queues N } }',
      group: 'dpdk',
      order: 5,
    }),
    hugepagesGb: withUi(z.number().int().min(1).max(1024).optional(), {
      title: 'Hugepages (GB)',
      help: 'total 2 MB hugepage memory reserved for the data plane',
      group: 'memory',
      order: 6,
    }),
    pciWhitelist: withUi(z.array(pciAddress).max(64).default([]), {
      title: 'PCI whitelist',
      help: 'NICs handed to DPDK (dpdk { dev 0000:0b:00.0 }); empty = no DPDK devices',
      group: 'dpdk',
      order: 7,
    }),
  }),
  {
    title: 'Dataplane',
    description:
      'VPP data-plane tuning: worker threads, RX queues, hugepages, PCI whitelist and core placement.',
    order: 20,
  },
);

export type DataplaneConfig = z.infer<typeof DataplaneSchema>;
