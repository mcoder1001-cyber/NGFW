import { describe, expect, it } from 'vitest';
import { presentationPointer } from './presentation-pointer.js';
import type { JsonSchema } from './types.js';

describe('schema diagnostic locations', () => {
  const schema: JsonSchema = {
    type: 'object',
    properties: {
      vppCache: {
        type: 'object',
        title: 'DNS cache',
        properties: { enabled: { type: 'boolean' } },
      },
      peers: {
        type: 'object',
        additionalProperties: { type: 'object', properties: { enabled: { type: 'boolean' } } },
      },
    },
  };
  it('labels known implementation fields while retaining user-chosen record keys', () => {
    expect(presentationPointer(schema, '/vppCache/enabled', 'en')).toBe('/DNS cache/enabled');
    expect(presentationPointer(schema, '/peers/VPP/enabled', 'en')).toBe('/peers/VPP/enabled');
    expect(schema.properties).toHaveProperty('vppCache');
  });
});
