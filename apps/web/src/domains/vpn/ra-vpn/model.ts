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
  if (!profile.enabled) return true;
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
