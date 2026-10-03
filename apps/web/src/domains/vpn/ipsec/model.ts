import type { paths } from '@ngfw/api-client';
import type { NgfwStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

/** `GET /api/v1/state/ipsec/tunnels` as generated from the OpenAPI document (never hand-written). */
export type IpsecTunnelsState =
  paths['/api/v1/state/ipsec/tunnels']['get']['responses']['200']['content']['application/json'];
export type IpsecTunnelState = IpsecTunnelsState['tunnels'][number];

/** `GET /api/v1/state/ipsec/sas`. */
export type IpsecSasState =
  paths['/api/v1/state/ipsec/sas']['get']['responses']['200']['content']['application/json'];
export type IpsecIkeSa = IpsecSasState['sas'][number];

function prop(schema: JsonSchema, name: string): JsonSchema {
  const p = (schema.properties as Record<string, JsonSchema> | undefined)?.[name];
  if (p === undefined) throw new Error(`schema property ${name} not found`);
  return p;
}
function recordItem(schema: JsonSchema): JsonSchema {
  const a = schema.additionalProperties;
  if (!a || typeof a !== 'object') throw new Error('record item schema not found');
  return a as JsonSchema;
}

/** `vpn.ipsec.tunnels.<name>` item schema — the one schema (00-CONTEXT rule 5). */
export function tunnelFormSchema(): JsonSchema {
  const schema = structuredClone(
    recordItem(prop(prop(domainSchemas.vpn as JsonSchema, 'ipsec'), 'tunnels')),
  );
  const auth = prop(schema, 'auth');
  for (const union of ['oneOf', 'anyOf'] as const) {
    if (Array.isArray(auth[union])) {
      auth[union] = (auth[union] as JsonSchema[]).filter((branch) => {
        const method = (branch.properties as Record<string, JsonSchema> | undefined)?.method;
        return (
          method?.const === 'psk' || (Array.isArray(method?.enum) && method.enum.includes('psk'))
        );
      });
    }
  }
  return schema;
}

/** `vpn.ipsec.proposals.<name>` item schema. */
export function proposalFormSchema(): JsonSchema {
  return recordItem(prop(prop(domainSchemas.vpn as JsonSchema, 'ipsec'), 'proposals'));
}

/** Status chip colour of a tunnel's live state (unknown / not applied are neutral). */
export function tunnelChip(status: TunnelChipState): NgfwStatus {
  switch (status) {
    case 'up':
      return 'up';
    case 'connecting':
      return 'degraded';
    case 'down':
      return 'down';
    default:
      return 'adminDown';
  }
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Localizes a vpn.ipsec form schema recursively: every property at path p (nested objects such as auth, dpd,
 * rekey, ike, esp, and the branches of the auth union included) takes `field.<p>.title` / `field.<p>.help`, and
 * fieldset groups take `group.<g>`. Help text is only ever the translation — the schema's English help never
 * reaches the form (P11 review R6 M1).
 */
export function localizeIpsecSchema(schema: JsonSchema, t: Translate, prefix = ''): JsonSchema {
  const out: JsonSchema = { ...schema };
  if (schema.properties) {
    const props: Record<string, JsonSchema> = {};
    for (const [name, prop] of Object.entries(schema.properties as Record<string, JsonSchema>)) {
      const p = prefix ? `${prefix}.${name}` : name;
      const hints = (prop['x-ngfw-ui'] ?? {}) as Record<string, unknown>;
      const help = t(`field.${p}.help`, { defaultValue: '' });
      const localized = localizeIpsecSchema(prop, t, p);
      props[name] = {
        ...localized,
        title: t(`field.${p}.title`, { defaultValue: name }),
        'x-ngfw-ui': {
          ...hints,
          help: help === '' ? undefined : help,
          ...(typeof hints.group === 'string'
            ? { group: t(`group.${hints.group}`, { defaultValue: hints.group }) }
            : {}),
        },
      } as JsonSchema;
    }
    out.properties = props;
  }
  for (const key of ['anyOf', 'oneOf'] as const) {
    const branches = schema[key] as JsonSchema[] | undefined;
    if (Array.isArray(branches)) out[key] = branches.map((b) => localizeIpsecSchema(b, t, prefix));
  }
  return out;
}

/** Live status of a configured tunnel for the chip: unknown while state is loading or failed, not applied when the
 * running configuration has no such connection (uncommitted or refused), else charon's status. */
export type TunnelChipState = 'up' | 'connecting' | 'down' | 'unknown' | 'notApplied';

export function tunnelChipState(
  stateLoaded: boolean,
  live: IpsecTunnelState | undefined,
): TunnelChipState {
  if (!stateLoaded) return 'unknown';
  if (!live) return 'notApplied';
  return live.status;
}
