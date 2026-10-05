import type { RemoteAccessProfile } from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';
export type Profile = RemoteAccessProfile;
export const steps = [
  [
    'enabled',
    'description',
    'localAddr',
    'localId',
    'vrf',
    'underlayVrf',
    'auth',
    'certificate',
    'clientCa',
    'proposal',
  ],
  ['transport', 'accessPolicy', 'outerPolicy'],
  ['pools', 'splitTunnel'],
  ['users', 'radius', 'dpd', 'rekey'],
] as const;
export function stepSchema(index: number): JsonSchema {
  const vpn = domainSchemas.vpn as JsonSchema;
  const remote = (vpn.properties as Record<string, JsonSchema>).remoteAccess!;
  const item = structuredClone(remote.additionalProperties as JsonSchema);
  const fields = steps[index]!;
  item.properties = Object.fromEntries(
    Object.entries(item.properties ?? {}).filter(([key]) => fields.includes(key as never)),
  );
  item.required = (item.required ?? []).filter((key) => fields.includes(key as never));
  return item;
}
export function activationAllowed(
  profile: Partial<Profile>,
  capability: { operational: boolean; supportedAuth: string[] } | undefined,
): boolean {
  if (profile.enabled === false) return true;
  return (
    !!capability?.operational &&
    !!profile.auth &&
    capability.supportedAuth.includes(profile.auth) &&
    !!profile.transport &&
    !!profile.accessPolicy?.ingress.length &&
    !!profile.accessPolicy.egress.length &&
    !!profile.outerPolicy?.ingress.length &&
    !!profile.outerPolicy.egress.length
  );
}

export const sessionColumns = [
  'identity',
  'addresses',
  'uptime',
  'bytesIn',
  'bytesOut',
  'action',
] as const;
export function newProfile(): Partial<Profile> {
  return {
    enabled: false,
    auth: 'eap-mschapv2',
    vrf: 'default',
    underlayVrf: 'default',
    pools: [],
    splitTunnel: [],
    users: [],
  };
}

/** Translate nested canonical fields, including credential references and transit policies. */
export function localizeRaSchema(
  schema: JsonSchema,
  translate: (key: string, fallback: string) => string,
): JsonSchema {
  const result = structuredClone(schema);
  if (result.properties) {
    result.properties = Object.fromEntries(
      Object.entries(result.properties).map(([key, value]) => {
        const field = localizeRaSchema(value as JsonSchema, translate);
        field.title = translate(`field.${key}.title`, field.title ?? key);
        const hints = field['x-ngfw-ui'] as Record<string, unknown> | undefined;
        if (hints) {
          const help = translate(`field.${key}.help`, '');
          const rest = { ...hints };
          delete rest['help'];
          field['x-ngfw-ui'] = {
            ...rest,
            ...(help ? { help } : {}),
            ...(typeof hints.group === 'string'
              ? { group: translate(`field.${hints.group}.title`, hints.group) }
              : {}),
          };
        }
        if (Array.isArray(field.enum)) {
          const enumLabels = Object.fromEntries(
            field.enum
              .filter((value): value is string => typeof value === 'string')
              .map((value) => [value, translate(`option.${value}`, value)]),
          );
          field['x-ngfw-ui'] = {
            ...(field['x-ngfw-ui'] as Record<string, unknown> | undefined),
            enumLabels,
          };
        }
        return [key, field];
      }),
    );
  }
  if (result.items && typeof result.items === 'object' && !Array.isArray(result.items))
    result.items = localizeRaSchema(result.items, translate);
  return result;
}
