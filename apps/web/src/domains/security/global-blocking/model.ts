import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

/** `acl.globalBlocking.lists.<name>` as the configuration holds it. */
export interface GbList {
  enabled?: boolean;
  description?: string;
  source?:
    | { kind: 'upload' }
    | {
        kind: 'url';
        url: string;
        refreshSec?: number;
        verifyTls?: boolean;
        caRef?: string;
        authRef?: string;
      };
  allInterfaces?: boolean;
  interfaces?: string[];
  direction?: 'both' | 'inbound' | 'outbound';
  protectHost?: boolean;
  log?: boolean;
  entries?: string[];
}

/** packages/schema objectName. */
export const LIST_NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;

function prop(s: JsonSchema, name: string): JsonSchema {
  const p = s.properties?.[name];
  if (!p || typeof p !== 'object') throw new Error(`schema: property ${name} not found`);
  return p;
}

/** One block list without `entries` (they come from an import or the server URL, never typed in the form). */
export function listSchema(): JsonSchema {
  const lists = prop(prop(domainSchemas.acl, 'globalBlocking'), 'lists')
    .additionalProperties as JsonSchema;
  const properties = { ...(lists.properties ?? {}) };
  delete properties['entries'];
  return { ...lists, properties, required: (lists.required ?? []).filter((r) => r !== 'entries') };
}

/** The form value of a list (its entries are kept aside and written back unchanged). */
export function formValue(l: GbList | undefined): Omit<GbList, 'entries'> | undefined {
  if (!l) return undefined;
  const rest = { ...l };
  delete rest.entries;
  return rest;
}

/** "1h", "2d 3h", "45min" for a refresh interval in seconds. */
export function intervalText(sec: number): string {
  const d = Math.floor(sec / 86_400);
  const h = Math.floor((sec % 86_400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return (
    [d ? `${d}d` : '', h ? `${h}h` : '', m ? `${m}min` : ''].filter(Boolean).join(' ') || `${sec}s`
  );
}
