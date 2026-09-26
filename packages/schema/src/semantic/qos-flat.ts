import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-qos-flat: semantic rules of `services.qos` that the data plane adds on top of `services.qos-references` and
 * `services.qos-consistency` (semantic/services.ts, P02c), which already check that attachments name existing
 * interfaces, policers, shapers and maps, that map ids are unique and that marked values fit the output header.
 *
 * - `services.qos-flat-store-source`: VPP 26.06 implements `qos store` for the `ip` source only
 *   (`qos_store_enable_disable` answers UNIMPLEMENTED for ext / vlan / mpls, docs/agent/descriptors/qos.md), so any
 *   other source is refused here with a pointer instead of failing the commit at apply time.
 *
 * The other two F-qos-flat rules are structural and already enforced by the schema tier (`domains/services.ts`,
 * QosInterfaceSchema), which runs before this tier and never lets such a document through: `mark` requires `map` (a
 * required key) and `shaper` excludes `policer.output` (both are one egress policer on the interface). They are not
 * repeated here (a semantic rule can never see a document the schema rejected); semantic/qos-flat.test.ts pins both.
 */

const P = (...segments: (string | number)[]): string => jsonPointer('services', 'qos', ...segments);

/** The only `qos store` source VPP 26.06 implements. */
export const QOS_STORE_SOURCES_VPP = ['ip'] as const;

const storeSource: ValidatorDefinition = {
  name: 'services.qos-flat-store-source',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    for (const [ifName, a] of Object.entries(config.services.qos.interfaces)) {
      const source = a.store?.source;
      if (source === undefined || (QOS_STORE_SOURCES_VPP as readonly string[]).includes(source)) {
        continue;
      }
      issues.push({
        pointer: P('interfaces', ifName, 'store', 'source'),
        message: `VPP 26.06 stores a QoS value for the ip source only (qos store ${source} is not implemented); use record for ${source}`,
      });
    }
    return issues;
  },
};

export const qosFlatValidators: readonly ValidatorDefinition[] = [storeSource];
