import { describe, expect, it } from 'vitest';
import { componentName, generateSchemas } from './generate.js';
import { ROOT_KEYS } from './index.js';
import { X_VRX_UI } from './ui.js';

describe('generateSchemas', () => {
  const g = generateSchemas();

  it('emits a strict draft 2020-12 root schema with exactly the root keys, none required', () => {
    expect(g.root.$schema).toBe('https://json-schema.org/draft/2020-12/schema');
    expect(g.root.additionalProperties).toBe(false);
    expect(Object.keys(g.root.properties ?? {})).toEqual([...ROOT_KEYS]);
    expect(g.root.required ?? []).toEqual([]);
  });

  it('emits one domain schema per root key with a default and UI hints', () => {
    expect(Object.keys(g.domains)).toEqual([...ROOT_KEYS]);
    for (const key of ROOT_KEYS) {
      const s = g.domains[key];
      expect(s.$schema).toBe('https://json-schema.org/draft/2020-12/schema');
      expect(s.default).toEqual({});
      expect(s[X_VRX_UI]).toBeDefined();
      expect(s.title).toBeTruthy();
    }
  });

  it('emits OpenAPI components RootConfig + <Key>Config without $schema', () => {
    const names = Object.keys(g.openapiComponents.schemas);
    expect(names).toEqual(['RootConfig', ...ROOT_KEYS.map(componentName)]);
    expect(componentName('interfaces')).toBe('InterfacesConfig');
    for (const s of Object.values(g.openapiComponents.schemas))
      expect(s).not.toHaveProperty('$schema');
    expect(g.openapiComponents.schemas.SystemConfig).toMatchObject({ type: 'object', default: {} });
  });

  it('is deterministic (gen twice → identical output)', () => {
    expect(JSON.stringify(generateSchemas())).toBe(JSON.stringify(g));
  });
});
