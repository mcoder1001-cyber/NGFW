import { z } from 'zod';
import {
  descriptionText,
  ipAddress,
  objectName,
  portNumber,
  secretRefOf,
} from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-dashboard-prom-alarms: `management.prometheus` (the agent's external Prometheus listener) and `management.alarms`
 * (threshold rules and their notification targets, evaluated in the API). The loopback `/metrics` endpoint P05 built
 * is always on; `management.prometheus` only opens an extra listener on a chosen address with an allow-list.
 * Webhook credentials are `token/<name>` secret references, never inline.
 */

const GROUP = 'observability';

/** The metrics a rule may test — the families the exporter and the API both know (kept in sync with promexport). */
export const ALARM_METRICS = [
  'interface_rx_bps',
  'interface_tx_bps',
  'interface_rx_drops',
  'interface_tx_drops',
  'interface_link_down',
  'worker_cpu_percent',
  'buffer_used_percent',
  'node_error_rate',
] as const;
export const AlarmMetric = z.enum(ALARM_METRICS);
export type AlarmMetric = z.infer<typeof AlarmMetric>;

export const AlarmOp = z.enum(['gt', 'ge', 'lt', 'le', 'eq']);
export type AlarmOp = z.infer<typeof AlarmOp>;

export const AlarmSeverity = z.enum(['info', 'warning', 'critical']);
export type AlarmSeverity = z.infer<typeof AlarmSeverity>;

export const AlarmRuleSchema = z.strictObject({
  metric: withUi(AlarmMetric, { title: 'Metric', widget: 'select', order: 1 }),
  op: withUi(AlarmOp.default('gt'), {
    title: 'Comparison',
    widget: 'select',
    help: 'gt > · ge ≥ · lt < · le ≤ · eq =',
    order: 2,
  }),
  threshold: withUi(z.number(), {
    title: 'Threshold',
    help: 'compared against the metric (bps, drops/s, percent, or 1/0 for link_down)',
    order: 3,
  }),
  forSec: withUi(z.number().int().min(0).max(86_400).default(0), {
    title: 'For (seconds)',
    help: 'the condition must hold this long before the alarm is raised (hysteresis); 0 = immediately',
    order: 4,
  }),
  severity: withUi(AlarmSeverity.default('warning'), {
    title: 'Severity',
    widget: 'select',
    order: 5,
  }),
  interface: withUi(z.string().max(63).optional(), {
    title: 'Interface',
    widget: 'interface-picker',
    help: 'limit an interface metric to this interface; empty = every interface',
    order: 6,
  }),
  description: withUi(descriptionText.optional(), { title: 'Description', order: 7 }),
  enabled: withUi(z.boolean().default(true), { title: 'Enabled', order: 8 }),
  targets: withUi(z.array(objectName).max(16).default([]), {
    title: 'Notify',
    help: 'names of management.alarms.targets to notify on raise and clear; empty = record only',
    order: 9,
  }),
});
export type AlarmRule = z.infer<typeof AlarmRuleSchema>;

const webhookTarget = z.strictObject({
  kind: z.literal('webhook'),
  url: withUi(z.string().url().max(2048), {
    title: 'Webhook URL',
    help: 'https endpoint the alarm JSON is POSTed to',
    order: 2,
  }),
  secretRef: withUi(secretRefOf('token').optional(), {
    title: 'Bearer token',
    help: 'token/<name> sent as Authorization: Bearer',
    order: 3,
  }),
});

const emailTarget = z.strictObject({
  kind: z.literal('email'),
  address: withUi(z.email().max(255), { title: 'Email address', order: 2 }),
});

export const AlarmTargetSchema = withUi(
  z.discriminatedUnion('kind', [webhookTarget, emailTarget]),
  { title: 'Target' },
);
export type AlarmTarget = z.infer<typeof AlarmTargetSchema>;

export const ManagementAlarmsSchema = z.strictObject({
  rules: withUi(z.record(objectName, AlarmRuleSchema).default({}), {
    title: 'Alarm rules',
    order: 1,
  }),
  targets: withUi(z.record(objectName, AlarmTargetSchema).default({}), {
    title: 'Notification targets',
    order: 2,
  }),
});
export type ManagementAlarms = z.infer<typeof ManagementAlarmsSchema>;

export const ManagementPrometheusSchema = z.strictObject({
  enabled: withUi(z.boolean().default(false), {
    title: 'Enabled',
    help: 'open an external Prometheus listener (the loopback /metrics endpoint is always on)',
    order: 1,
  }),
  listen: withUi(ipAddress.default('0.0.0.0'), {
    title: 'Listen address',
    help: 'address the external /metrics listener binds to',
    order: 2,
  }),
  port: withUi(portNumber.default(9101), { title: 'Port', order: 3 }),
  allow: withUi(z.array(z.string().max(49)).max(64).default([]), {
    title: 'Allow list',
    help: 'CIDRs allowed to scrape (empty = allow any source that reaches the listener)',
    order: 4,
  }),
});
export type ManagementPrometheus = z.infer<typeof ManagementPrometheusSchema>;

/** `management.prometheus` field. */
export const managementPrometheusField = withUi(ManagementPrometheusSchema.prefault({}), {
  title: 'Prometheus',
  group: GROUP,
  order: 5,
});

/** `management.alarms` field. */
export const managementAlarmsField = withUi(ManagementAlarmsSchema.prefault({}), {
  title: 'Alarms',
  group: GROUP,
  order: 6,
});
