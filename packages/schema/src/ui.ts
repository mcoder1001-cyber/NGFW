import type { z } from 'zod';

/** Name of the JSON Schema extension keyword carrying form-renderer hints. */
export const X_VRX_UI = 'x-vrx-ui' as const;

/**
 * Hints consumed by the UI form renderer (packages/ui-kit SchemaForm). They are emitted verbatim into the
 * generated JSON Schema / OpenAPI components under `x-vrx-ui`, so the UI never needs a hand-written form.
 */
export interface UiHints {
  /** Widget override, e.g. 'select', 'textarea', 'password', 'cidr', 'interface-picker'. */
  widget?: string;
  /** Visual grouping (section / tab) inside a domain form. */
  group?: string;
  /** Sort order within the group — or, on a domain schema, the navigation order (vdom.md guardrail #4). */
  order?: number;
  /** Help text or i18n key shown next to the field. */
  help?: string;
}

export interface UiMeta extends UiHints {
  title?: string;
  description?: string;
}

/**
 * Attach `title`, `description` and `x-vrx-ui` hints to a Zod schema. Returns the same schema type, so it can
 * wrap any field: `withUi(z.number().int().min(68).max(9216), { title: 'MTU', widget: 'number', order: 3 })`.
 * Zod 4 `.meta()` stores the data in `z.globalRegistry`; `z.toJSONSchema` copies it into the output.
 */
export function withUi<T extends z.ZodType>(
  schema: T,
  { title, description, ...hints }: UiMeta,
): T {
  const meta: z.core.GlobalMeta = {
    ...(title !== undefined ? { title } : {}),
    ...(description !== undefined ? { description } : {}),
    [X_VRX_UI]: hints,
  };
  return schema.meta(meta);
}
