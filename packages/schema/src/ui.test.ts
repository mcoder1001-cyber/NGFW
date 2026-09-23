import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { withUi, X_VRX_UI } from './ui.js';

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
    expect(z.toJSONSchema(derived)).toMatchObject({ title: 'VRF', [X_VRX_UI]: { widget: 'vrf-picker' } });
    expect(derived.safeParse('').success).toBe(false);
  });
});
