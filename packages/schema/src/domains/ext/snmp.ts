import { z } from 'zod';
import { withUi } from '../../ui.js';
import { objectName } from '../../primitives.js';

/**
 * F-snmp sub-schemas of `services.snmp` (D-086: RF-4's stand-in fields moved into the contract).
 * Keys are wired in `domains/services.ts` (`SnmpSchema` + community / v3-user `view`); the same limits are
 * enforced by the agent's snmpd renderer (`apps/agent/internal/renderers/snmpd/model.go`).
 */

/** Symbolic subtrees the snmpd renderer accepts besides numeric OIDs (renderer allow-list). */
export const SNMP_SYMBOLIC_OIDS = [
  'all',
  'internet',
  'mib-2',
  'system',
  'interfaces',
  'ip',
  'host',
  'ifMIB',
  'enterprises',
  'ucdavis',
  'snmpV2',
] as const;

/** Name of the built-in all-OIDs view the renderer gives every community / user without a view. */
export const SNMP_DEFAULT_VIEW = 'vrx_all';

const oidRe = /^\.?[0-9]{1,10}(?:\.[0-9]{1,10}){0,63}$/;

export const snmpSubtree = withUi(
  z
    .string()
    .refine(
      (s) => oidRe.test(s) || (SNMP_SYMBOLIC_OIDS as readonly string[]).includes(s),
      'expected a numeric OID (.1.3.6.1…) or one of ' + SNMP_SYMBOLIC_OIDS.join(', '),
    ),
  { title: 'OID subtree' },
);

export const snmpViewName = withUi(
  z
    .string()
    .regex(/^[A-Za-z0-9_.-]{1,64}$/, 'expected 1–64 of [A-Za-z0-9_.-]')
    .refine((s) => s !== SNMP_DEFAULT_VIEW, `'${SNMP_DEFAULT_VIEW}' is reserved`),
  { title: 'View' },
);

export const SnmpViewSchema = z.strictObject({
  include: withUi(z.array(snmpSubtree).min(1).max(32), { title: 'Included subtrees' }),
  exclude: withUi(z.array(snmpSubtree).max(32).default([]), { title: 'Excluded subtrees' }),
});

export const SnmpMonitorDiskSchema = z.strictObject({
  path: withUi(
    z
      .string()
      .regex(/^\/[A-Za-z0-9_./-]{0,127}$/, 'expected an absolute path of [A-Za-z0-9_./-]')
      .refine((p) => !p.includes('//') && !/(^|\/)\.\.(\/|$)/.test(p), 'expected a clean path'),
    { title: 'Mount path' },
  ),
  minPercent: withUi(z.number().int().min(1).max(99).default(10), { title: 'Minimum free %' }),
});

const loadThreshold = z.number().int().min(1).max(1000);

export const SnmpMonitorLoadSchema = z.strictObject({
  max1: withUi(loadThreshold, { title: '1-minute load' }),
  max5: withUi(loadThreshold, { title: '5-minute load' }),
  max15: withUi(loadThreshold, { title: '15-minute load' }),
});

export const SnmpMonitorsSchema = z.strictObject({
  disks: withUi(z.array(SnmpMonitorDiskSchema).max(16).default([]), { title: 'Disks' }),
  load: withUi(SnmpMonitorLoadSchema, { title: 'Load average' }).optional(),
});

export const SnmpSubagentSchema = z.strictObject({
  enabled: withUi(z.boolean().default(true), {
    title: 'VRX-MIB subagent',
    widget: 'switch',
    help: 'Serve VPP interface counters and agent health (VRX-MIB) through AgentX',
  }),
});

/** Keys inserted into `SnmpSchema` (services.ts). */
export const snmpViewsField = withUi(z.record(objectName, SnmpViewSchema).default({}), {
  title: 'Views',
  widget: 'record',
  help: `Communities and users without a view see everything ('${SNMP_DEFAULT_VIEW}')`,
});
export const snmpSysServicesField = withUi(z.number().int().min(0).max(127), {
  title: 'sysServices',
}).optional();
export const snmpMonitorsField = withUi(SnmpMonitorsSchema, { title: 'Monitors' }).optional();
export const snmpSubagentField = withUi(SnmpSubagentSchema, {
  title: 'Private MIB (VRX-MIB)',
  help: 'Absent = enabled',
}).optional();
export const snmpViewRefField = withUi(snmpViewName, {
  title: 'View',
  help: 'Name in snmp.views; empty = everything',
}).optional();

export type SnmpView = z.infer<typeof SnmpViewSchema>;
export type SnmpMonitors = z.infer<typeof SnmpMonitorsSchema>;
