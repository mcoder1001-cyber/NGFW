import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { inheritedHints, withUi, X_VRX_UI } from './ui.js';

describe('withUi', () => {
  it('emits title, description and x-vrx-ui into the JSON Schema', () => {
    const s = withUi(z.number().int(), {
      title: 'MTU',
      description: 'bytes',
      widget: 'number',
      order: 3,
    });
    const js = z.toJSONSchema(s);
    expect(js.title).toBe('MTU');
    expect(js.description).toBe('bytes');
    expect(js[X_VRX_UI]).toEqual({ widget: 'number', order: 3 });
    expect(js).not.toHaveProperty('writeOnly');
  });

  it('omits title/description when not given and keeps the schema usable', () => {
    const s = withUi(z.string(), { help: 'h' });
    const js = z.toJSONSchema(s);
    expect(js).not.toHaveProperty('title');
    expect(js).not.toHaveProperty('description');
    expect(js[X_VRX_UI]).toEqual({ help: 'h' });
    expect(s.parse('ok')).toBe('ok');
  });

  it('marks secrets writeOnly and passes itemKey through', () => {
    const secret = withUi(z.string(), { title: 'Hash', widget: 'password', secret: true });
    expect(z.toJSONSchema(secret)).toMatchObject({ writeOnly: true, [X_VRX_UI]: { secret: true } });
    const list = withUi(z.array(z.object({ username: z.string() })), { itemKey: ['username'] });
    expect(z.toJSONSchema(list)[X_VRX_UI]).toEqual({ itemKey: ['username'] });
  });

  it('does not alter the wrapped schema (meta clones it)', () => {
    const base = withUi(z.string().min(1), { title: 'Name' });
    const derived = withUi(base, { title: 'VRF', widget: 'vrf-picker' });
    expect(z.toJSONSchema(base).title).toBe('Name');
    expect(z.toJSONSchema(derived)).toMatchObject({
      title: 'VRF',
      [X_VRX_UI]: { widget: 'vrf-picker' },
    });
    expect(derived.safeParse('').success).toBe(false);
  });
});

describe('withUi inheritance (P02b review H1)', () => {
  const primitive = withUi(z.string(), { title: 'IP', widget: 'ip', help: 'e.g. 10.0.0.1' });

  it('keeps the wrapped primitive’s widget/help when a field adds group/order', () => {
    const field = withUi(primitive, { group: 'General', order: 3 });
    expect(z.toJSONSchema(field)[X_VRX_UI]).toEqual({
      widget: 'ip',
      help: 'e.g. 10.0.0.1',
      group: 'General',
      order: 3,
    });
  });

  it('looks through optional/default wrappers and lets explicit keys win', () => {
    const field = withUi(primitive.optional(), { help: 'gateway', order: 1 });
    expect(inheritedHints(primitive.optional())).toEqual({ widget: 'ip', help: 'e.g. 10.0.0.1' });
    expect(z.toJSONSchema(z.object({ f: field })).properties?.['f']).toMatchObject({
      [X_VRX_UI]: { widget: 'ip', help: 'gateway', order: 1 },
    });
    const withDefault = withUi(primitive.default('10.0.0.1'), { group: 'g' });
    expect(z.toJSONSchema(withDefault, { io: 'input' })[X_VRX_UI]).toEqual({
      widget: 'ip',
      help: 'e.g. 10.0.0.1',
      group: 'g',
    });
  });

  it('inherits secret (and so writeOnly) from a secret primitive', () => {
    const hash = withUi(z.string(), { secret: true, widget: 'password' });
    expect(z.toJSONSchema(withUi(hash.optional(), { order: 4 }))).toMatchObject({
      writeOnly: true,
      [X_VRX_UI]: { secret: true, widget: 'password', order: 4 },
    });
  });

  it('returns {} for a schema without hints', () => {
    expect(inheritedHints(z.string().optional())).toEqual({});
  });
});
