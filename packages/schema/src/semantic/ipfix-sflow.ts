import { ipFamily } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators of F-ipfix-sflow (`services.ipfix`), the rules the VPP flowprobe/sflow/ipfix plugins impose
 * and the schema cannot express (docs/agent/descriptors/{ipfix,flowprobe,sflow}.md):
 * - flowprobe records leave the box only through IPFIX exporter 0, which takes an IPv4 collector: monitored
 *   interfaces need an enabled exporter with an IPv4 collector (the agent makes the first such exporter, by name,
 *   exporter 0);
 * (VPP enables one flowprobe variant per interface; the schema default `ip4` + `ip6` is realised by the agent as
 *  ip4 with a warning, not rejected here — the shipped examples use that default.)
 * - VPP rounds the sFlow header size to a multiple of 32 silently, so any other value would drift forever;
 * - VPP identifies additional exporters by collector address alone: enabled exporters need distinct collector
 *   addresses.
 */

const P = (...segments: (string | number)[]): string =>
  jsonPointer('services', 'ipfix', ...segments);

const flowprobeNeedsExporter: ValidatorDefinition = {
  name: 'services.ipfix-sflow-flowprobe-exporter',
  domains: ['services'],
  validate(config) {
    const ipfix = config.services.ipfix;
    if (ipfix.flowprobe.interfaces.length === 0) return [];
    const ok = Object.values(ipfix.exporters).some(
      (e) => e.enabled !== false && ipFamily(e.collector.address) === 4,
    );
    return ok
      ? []
      : [
          {
            pointer: P('flowprobe', 'interfaces'),
            message:
              'flowprobe records are sent through IPFIX exporter 0 only: enable an exporter with an IPv4 collector',
          },
        ];
  },
};

const sflowHeaderBytes: ValidatorDefinition = {
  name: 'services.ipfix-sflow-header-bytes',
  domains: ['services'],
  validate(config) {
    const s = config.services.ipfix.sflow;
    if (s === undefined || s.headerBytes % 32 === 0) return [];
    return [
      {
        pointer: P('sflow', 'headerBytes'),
        message: `sampled header bytes must be a multiple of 32 (VPP rounds ${s.headerBytes} silently)`,
      },
    ];
  },
};

const exporterCollectorUnique: ValidatorDefinition = {
  name: 'services.ipfix-sflow-collector-unique',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    for (const name of Object.keys(config.services.ipfix.exporters).sort()) {
      const e = config.services.ipfix.exporters[name]!;
      if (e.enabled === false) continue;
      const a = e.collector.address;
      const other = seen.get(a);
      if (other !== undefined) {
        issues.push({
          pointer: P('exporters', name, 'collector', 'address'),
          message: `exporters ${other} and ${name} use the same collector ${a} (VPP keys exporters by collector address)`,
        });
      } else seen.set(a, name);
    }
    return issues;
  },
};

export const ipfixSflowValidators: readonly ValidatorDefinition[] = [
  flowprobeNeedsExporter,
  sflowHeaderBytes,
  exporterCollectorUnique,
];
