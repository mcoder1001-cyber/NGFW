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
/**
 * Logical interface name of a DPDK NIC (D-069: the name VPP gives the interface and the `interface/<name>` key):
 * 1–15 lower-case letters, digits, `_`, `-`; starts with a letter, does not end with `_`/`-` (15 = Linux IFNAMSIZ−1,
 * linux-cp mirrors the name). VPP-reserved names and VPP-created stems are rejected by the generator (F-startup-gen).
 */
export const logicalIfName = withUi(
  z
    .string()
    .regex(
      /^[a-z](?:[a-z0-9_-]{0,13}[a-z0-9])?$/,
      'expected 1–15 characters [a-z0-9_-], starting with a letter, not ending with _ or -',
    ),
  { title: 'Logical name', help: 'e.g. lan, wan, dmz' },
);

/** VPP plugin file name as installed in the plugin directory, e.g. `linux_cp_plugin.so`. */
export const vppPluginFile = z
  .string()
  .max(64)
  .regex(/^[a-z0-9][a-z0-9_-]*_plugin\.so$/, 'expected a plugin file name like linux_cp_plugin.so');

/** `dataplane.plugins` (D-084): a wrapper so "absent" (keep the current file's switches) and "present, empty"
 * (no plugin switches at all) stay distinguishable in the proto as well. */
export const DataplanePluginSetSchema = z.strictObject({
  switches: withUi(z.record(vppPluginFile, z.boolean()).default({}), {
    title: 'Plugin switches',
    help: 'plugin file → true (enable) / false (disable)',
    order: 1,
  }),
});

/** One DPDK device (`dataplane.devices.<pci>`). */
export const DataplaneDeviceSchema = z.strictObject({
  name: withUi(logicalIfName.optional(), {
    title: 'Logical name',
    help: 'dpdk { dev <pci> { name <logical> } } — the interface name used everywhere else in the configuration',
    order: 1,
  }),
  rxQueues: withUi(z.number().int().min(1).max(256).optional(), { title: 'RX queues', order: 2 }),
  txQueues: withUi(z.number().int().min(1).max(256).optional(), { title: 'TX queues', order: 3 }),
  rxDesc: withUi(z.number().int().min(64).max(16384).optional(), {
    title: 'RX descriptors',
    help: 'power of two',
    order: 4,
  }),
  txDesc: withUi(z.number().int().min(64).max(16384).optional(), {
    title: 'TX descriptors',
    help: 'power of two',
    order: 5,
  }),
});

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
    managementPci: withUi(z.array(pciAddress).max(4).default([]), {
      title: 'Management NICs',
      help: 'PCI addresses of the management NIC(s): always blacklisted, never handed to DPDK (the generator also protects the NIC it detects on the host)',
      group: 'dpdk',
      order: 8,
    }),
    devices: withUi(z.record(pciAddress, DataplaneDeviceSchema).default({}), {
      title: 'DPDK devices',
      help: 'NICs handed to DPDK keyed by PCI address, each with its logical interface name (dpdk { dev <pci> { name lan } })',
      group: 'dpdk',
      order: 9,
    }),
    buffersPerNuma: withUi(z.number().int().min(1024).max(4194304).optional(), {
      title: 'Buffers per NUMA node',
      help: 'buffers { buffers-per-numa N }; must fit in the hugepage reservation',
      group: 'memory',
      order: 10,
    }),
    plugins: withUi(DataplanePluginSetSchema.optional(), {
      title: 'Plugins',
      help: 'plugins { plugin <file> { enable|disable } }. Present = authoritative: exactly these switches are rendered (true = enable, false = disable). Absent = the switches of the current start-up configuration are kept (D-084)',
      group: 'plugins',
      order: 11,
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
