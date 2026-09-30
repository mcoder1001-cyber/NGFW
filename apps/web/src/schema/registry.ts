import { RootConfig, ROOT_KEYS, type RootKey } from '@ngfw/schema';
import { productWording } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { z } from 'zod';

/**
 * The one schema, consumed by the UI as JSON Schema (00-CONTEXT rule 5). Generated in the browser from the
 * same Zod source `pnpm gen` uses (identical options to packages/schema/src/generate.ts, whose
 * `generateSchemas()` is not exported by the package yet — see P07a-questions.md), so navigation
 * (vdom.md guardrail 4) and forms can never drift from the contract.
 */
const OPTIONS = { target: 'draft-2020-12', io: 'input' } as const;

function schemaWording(text: string): string {
  return productWording(
    text
      .replace(/frr-linuxcp/gi, 'Routing')
      .replace(/kernel-vpp/gi, 'IPsec integration')
      .replace(/vpp-ikev2/gi, 'Native IKEv2'),
  );
}

/** Presentation only: automatic pairs stay in the contract and submitted values, without manual controls. */
function presentationSchema(schema: JsonSchema): JsonSchema {
  const out = { ...schema };
  for (const key of ['title', 'description'] as const) {
    if (schema[key] !== undefined) out[key] = schemaWording(schema[key]);
  }
  if (schema['x-vrx-ui']) {
    const hints = { ...schema['x-vrx-ui'] };
    for (const key of ['help', 'group', 'placeholder'] as const) {
      if (typeof hints[key] === 'string') hints[key] = schemaWording(hints[key]);
    }
    out['x-vrx-ui'] = hints;
  }
  for (const key of ['properties', '$defs'] as const) {
    if (schema[key]) {
      out[key] = Object.fromEntries(
        Object.entries(schema[key]).map(([name, child]) => {
          const presented = presentationSchema(child);
          if (key === 'properties' && name === 'lcp') {
            presented['x-vrx-ui'] = { ...presented['x-vrx-ui'], widget: 'hidden' };
          }
          return [name, presented];
        }),
      );
    }
  }
  for (const key of ['items', 'propertyNames', 'additionalProperties'] as const) {
    const child = schema[key];
    if (typeof child === 'object') out[key] = presentationSchema(child);
  }
  for (const key of ['oneOf', 'anyOf', 'allOf', 'prefixItems'] as const) {
    if (schema[key]) out[key] = schema[key].map(presentationSchema);
  }
  return out;
}

export const rootSchema = presentationSchema(z.toJSONSchema(RootConfig, OPTIONS) as JsonSchema);
export const domainSchemas = Object.fromEntries(
  ROOT_KEYS.map((key) => [
    key,
    presentationSchema(z.toJSONSchema(RootConfig.shape[key], OPTIONS) as JsonSchema),
  ]),
) as Record<RootKey, JsonSchema>;
export { ROOT_KEYS };
export type { RootKey };

export interface DomainInfo {
  key: RootKey;
  /** Title from the schema (English source of truth; `nav:domains.<key>` may translate it). */
  title: string;
  description: string | undefined;
  /** `x-vrx-ui.order` on the domain schema — navigation order. */
  order: number;
  schema: JsonSchema;
}

function orderOf(schema: JsonSchema): number {
  const hints = schema['x-vrx-ui'];
  const order = hints && typeof hints === 'object' ? (hints as { order?: unknown }).order : undefined;
  return typeof order === 'number' ? order : Number.MAX_SAFE_INTEGER;
}

/** Every top-level configuration domain, in navigation order. */
export const domains: readonly DomainInfo[] = ROOT_KEYS.map((key) => {
  const schema = domainSchemas[key];
  return { key, title: schema.title ?? key, description: schema.description, order: orderOf(schema), schema };
}).sort((a, b) => a.order - b.order);

export function domainByKey(key: string): DomainInfo | undefined {
  return domains.find((d) => d.key === key);
}
