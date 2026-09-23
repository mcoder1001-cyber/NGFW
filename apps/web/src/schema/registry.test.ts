import { compileError } from '@ngfw/ui-kit/schema-form';
import { describe, expect, it } from 'vitest';
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
