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
  });

  it('omits title/description when not given and keeps the schema usable', () => {
    const s = withUi(z.string(), { help: 'h' });
    const js = z.toJSONSchema(s);
    expect(js).not.toHaveProperty('title');
    expect(js).not.toHaveProperty('description');
    expect(js[X_VRX_UI]).toEqual({ help: 'h' });
    expect(s.parse('ok')).toBe('ok');
  });
});
