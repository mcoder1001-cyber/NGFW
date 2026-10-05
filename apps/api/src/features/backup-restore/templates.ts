import { z } from 'zod';
import { problems } from '../../common/problem.js';
const unsafe = (value: string): boolean =>
  [...value].some((c) => c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127);
const forbidden = new Set(['__proto__', 'constructor', 'prototype']);
/** Exact typed placeholders avoid string interpolation and preserve JSON types. */
export function renderTemplate(
  template: {
    parameters: Record<string, { type: 'string' | 'number' | 'boolean'; required: boolean }>;
    patch: Record<string, unknown>;
  },
  parameters: Record<string, unknown>,
): Record<string, unknown> {
  for (const key of Object.keys(parameters))
    if (!(key in template.parameters))
      throw problems.badRequest('unknown template parameter', [
        { pointer: `/parameters/${key}`, message: 'unknown parameter' },
      ]);
  for (const [name, spec] of Object.entries(template.parameters)) {
    const value = parameters[name];
    if (
      (value === undefined && spec.required) ||
      (value !== undefined && typeof value !== spec.type)
    )
      throw problems.badRequest('invalid template parameter', [
        { pointer: `/parameters/${name}`, message: `expected ${spec.type}` },
      ]);
  }
  const walk = (v: unknown, pointer: string): unknown => {
    if (typeof v === 'string') {
      if (unsafe(v))
        throw problems.badRequest('unsafe template text', [
          { pointer, message: 'control character' },
        ]);
      const match = /^\$\{([a-zA-Z0-9_-]+)\}$/.exec(v);
      if (match) {
        const name = match[1]!;
        if (!Object.hasOwn(template.parameters, name) || parameters[name] === undefined)
          throw problems.badRequest('missing template parameter', [
            { pointer, message: 'unresolved placeholder' },
          ]);
        return walk(parameters[name], pointer);
      }
      return v;
    }
    if (Array.isArray(v)) return v.map((value, index) => walk(value, `${pointer}/${index}`));
    if (v !== null && typeof v === 'object') {
      const out: Record<string, unknown> = {};
      for (const [key, value] of Object.entries(v)) {
        if (forbidden.has(key) || unsafe(key))
          throw problems.badRequest('unsafe template key', [
            {
              pointer: `${pointer}/${key.replace(/~/g, '~0').replace(/\//g, '~1')}`,
              message: 'unsafe member name',
            },
          ]);
        out[key] = walk(value, `${pointer}/${key}`);
      }
      return out;
    }
    return v;
  };
  return z.record(z.string(), z.unknown()).parse(walk(template.patch, '/patch'));
}
