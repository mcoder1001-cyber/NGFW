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
  /**
   * Write-only secret (password hash, PSK …): the API never returns it (00-CONTEXT rule 10) and the form shows
   * a password widget. Also emitted as JSON Schema `writeOnly: true`.
   */
  secret?: boolean;
  /**
   * For arrays of objects: the member(s) that identify an item (`['username']`, `['vrf', 'prefix']`). Used as
   * the row key in tables and by `diff()` to match items instead of reporting a whole-array replace.
   */
  itemKey?: readonly string[];
}

export interface UiMeta extends UiHints {
  title?: string;
  description?: string;
}

/**
 * Attach `title`, `description` and `x-vrx-ui` hints to a Zod schema. Returns the same schema type, so it can
 * wrap any field: `withUi(z.number().int().min(68).max(9216), { title: 'MTU', widget: 'number', order: 3 })`.
 * Zod 4 `.meta()` clones the schema and stores the data in `z.globalRegistry`; `z.toJSONSchema` copies it into
 * the output — so wrapping a shared primitive again with a different title is safe.
 */
export function withUi<T extends z.ZodType>(
  schema: T,
  { title, description, ...hints }: UiMeta,
): T {
  const meta: z.core.GlobalMeta = {
    ...(title !== undefined ? { title } : {}),
    ...(description !== undefined ? { description } : {}),
    ...(hints.secret ? { writeOnly: true } : {}),
    [X_VRX_UI]: hints,
  };
  return schema.meta(meta);
}
