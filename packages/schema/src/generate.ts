import { z } from 'zod';
import { RootConfig, ROOT_KEYS, type RootKey } from './index.js';

export type JsonSchema = z.core.JSONSchema.BaseSchema;

/** Everything `pnpm gen` writes to `dist/`, as data — so tests can assert on it without touching the filesystem. */
export interface GeneratedSchemas {
  /** Root document, JSON Schema draft 2020-12 (import/export validation, UI). */
  root: JsonSchema;
  /** One JSON Schema per top-level key, same dialect (UI form renderer, vdom.md guardrail #4). */
  domains: Record<RootKey, JsonSchema>;
  /** OpenAPI 3.1 `components.schemas`: `RootConfig` + `<Key>Config` per domain. */
  openapiComponents: { schemas: Record<string, JsonSchema> };
}

/**
 * `io: 'input'` describes what the API accepts: optional-with-default keys stay optional and carry `default`.
 * OpenAPI 3.1 schema objects are JSON Schema 2020-12 (D-006), so the same output is reused minus `$schema`.
 */
const OPTIONS = { target: 'draft-2020-12', io: 'input' } as const;

/** `interfaces` → `InterfacesConfig` — the OpenAPI component (and TS type) name of a domain. */
export function componentName(key: RootKey): string {
  return key.charAt(0).toUpperCase() + key.slice(1) + 'Config';
}

function withoutDialect(schema: JsonSchema): JsonSchema {
  const copy = { ...schema };
  delete copy.$schema;
  return copy;
}

export function generateSchemas(): GeneratedSchemas {
  const root = z.toJSONSchema(RootConfig, OPTIONS);
  const domains = {} as Record<RootKey, JsonSchema>;
  const schemas: Record<string, JsonSchema> = { RootConfig: withoutDialect(root) };
  for (const key of ROOT_KEYS) {
    const schema = z.toJSONSchema(RootConfig.shape[key], OPTIONS);
    domains[key] = schema;
    schemas[componentName(key)] = withoutDialect(schema);
  }
  return { root, domains, openapiComponents: { schemas } };
}
