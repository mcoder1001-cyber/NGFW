import type { RootConfig, RootKey } from '../index.js';
import { SemanticRegistry, type SemanticIssue, type ValidatorDefinition } from './registry.js';
import { systemValidators } from './system.js';
import { dataplaneValidators } from './dataplane.js';
import { interfacesValidators } from './interfaces.js';
import { vrfsValidators } from './vrfs.js';
import { routingValidators } from './routing.js';
import { natValidators } from './nat.js';
import { objectsValidators } from './objects.js';
import { aclValidators } from './acl.js';
import { vpnValidators } from './vpn.js';
import { tunnelsValidators } from './tunnels.js';
import { servicesValidators } from './services.js';
import { haValidators } from './ha.js';
import { managementValidators } from './management.js';

export * from './registry.js';

/**
 * All semantic validators, one array per domain (same layout and ownership as `../domains/`): each group adds
 * rules only to its own `semantic/<key>.ts`, so this aggregator never needs editing.
 */
export const SEMANTIC_VALIDATORS: readonly ValidatorDefinition[] = [
  ...systemValidators,
  ...dataplaneValidators,
  ...interfacesValidators,
  ...vrfsValidators,
  ...routingValidators,
  ...natValidators,
  ...objectsValidators,
  ...aclValidators,
  ...vpnValidators,
  ...tunnelsValidators,
  ...servicesValidators,
  ...haValidators,
  ...managementValidators,
];

/** Process-wide registry populated from {@link SEMANTIC_VALIDATORS}. */
export const semanticRegistry = new SemanticRegistry();
for (const v of SEMANTIC_VALIDATORS) semanticRegistry.register(v);

/**
 * Tier (b) validation entry point: run every registered validator against a schema-valid document.
 * Pass `domains` to run only the validators that read those top-level keys. `[]` means no findings.
 */
export function validateSemantics(
  config: RootConfig,
  domains?: readonly RootKey[],
): SemanticIssue[] {
  return semanticRegistry.run(config, domains);
}
