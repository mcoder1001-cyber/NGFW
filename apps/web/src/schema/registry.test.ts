import { RootConfig } from '@ngfw/schema';
import { compile, compileError, recordValueSchema, resolveRef } from '@ngfw/ui-kit/schema-form';
import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { domainSchemas, ROOT_KEYS, rootSchema } from './registry';

// Review P07a M4: a schema construct Zod's fromJSONSchema cannot convert makes SchemaForm fail closed. This guards the
// real contract: when P02a/b/c land domain schemas, every one of them must still compile for client-side validation.
describe('generated JSON Schemas compile for SchemaForm validation', () => {
  it('root schema compiles', () => {
    expect(compileError(rootSchema, rootSchema)?.message).toBeUndefined();
  });
  it.each([...ROOT_KEYS])('domain %s compiles', (key) => {
    const schema = domainSchemas[key];
    expect(compileError(schema, schema)?.message).toBeUndefined();
  });
});

/** Presentation metadata is not part of the validation/API contract. */
function withoutPresentation(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(withoutPresentation);
  if (value === null || typeof value !== 'object') return value;
  return Object.fromEntries(
    Object.entries(value)
      .filter(([key]) => !['title', 'description', 'x-ngfw-ui'].includes(key))
      .map(([key, child]) => [key, withoutPresentation(child)]),
  );
}

describe('web schema presentation', () => {
  it('keeps the complete root validation contract while adapting display metadata', () => {
    const contract = z.toJSONSchema(RootConfig, { target: 'draft-2020-12', io: 'input' });
    expect(withoutPresentation(rootSchema)).toEqual(withoutPresentation(contract));
  });

  it('hides automatic pairs in both whole-document and interface forms without dropping their contract', () => {
    const rootInterfaces = resolveRef(rootSchema.properties!.interfaces!, rootSchema);
    for (const [schema, root] of [
      [domainSchemas.interfaces, domainSchemas.interfaces],
      [rootInterfaces, rootSchema],
    ] as const) {
      const item = resolveRef(recordValueSchema(schema), root);
      expect(item.properties?.lcp?.['x-ngfw-ui']?.widget).toBe('hidden');
      expect(item.properties?.lcp).toHaveProperty('properties.hostIfName');
    }
    const parse = compile(domainSchemas.interfaces, domainSchemas.interfaces);
    expect(
      parse.parse({ eth0: { lcp: { hostIfName: 'route0', hostIfType: 'tap' } } }),
    ).toMatchObject({
      eth0: { lcp: { hostIfName: 'route0', hostIfType: 'tap' } },
    });
    expect(parse.safeParse({ eth0: { lcp: { hostIfType: 'invalid' } } }).success).toBe(false);
  });

  it('uses product wording for schema labels without changing configuration identifiers', () => {
    const item = recordValueSchema(domainSchemas.interfaces);
    expect(item.properties?.lcp?.['x-ngfw-ui']?.group).toBe('Routing');
    expect(item.properties?.lcp?.['x-ngfw-ui']?.help).not.toMatch(/FRR|VPP|strongSwan/i);
    expect(domainSchemas.interfaces.properties).toBeUndefined();
  });
});
