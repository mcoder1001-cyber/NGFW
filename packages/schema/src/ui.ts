import { z } from 'zod';

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
   * the row key in tables and to pair `from`/`to` items when the UI renders a whole-array `replace` from `diff()`
   * (diff itself treats arrays as leaves, D-021). Uniqueness of the key is a semantic rule of the owning domain.
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
 *
 * Hints are **merged** with the ones already carried by `schema` (or by the schema it wraps: `.optional()`,
 * `.default()`, `.nullable()` …): re-wrapping a hinted primitive to set `group`/`order` keeps its `widget` and
 * `help`; a key given here wins. (Zod 4 `.meta()` merges metadata shallowly, so without this the whole `x-vrx-ui`
 * object of the primitive was replaced — P02b review H1.)
 */
export function withUi<T extends z.ZodType>(
  schema: T,
  { title, description, ...hints }: UiMeta,
): T {
  const merged: UiHints = { ...inheritedHints(schema), ...hints };
  const meta: z.core.GlobalMeta = {
    ...(title !== undefined ? { title } : {}),
    ...(description !== undefined ? { description } : {}),
    ...(merged.secret ? { writeOnly: true } : {}),
    [X_VRX_UI]: merged,
  };
  return schema.meta(meta);
}

/** The `x-vrx-ui` hints of `schema`, or of the first schema it wraps that has some; `{}` when none. */
export function inheritedHints(schema: z.ZodType): UiHints {
  let current: unknown = schema;
  while (current instanceof z.ZodType) {
    const hints = z.globalRegistry.get(current)?.[X_VRX_UI] as UiHints | undefined;
    if (hints !== undefined) return hints;
    current = (current.def as { innerType?: unknown }).innerType;
  }
  return {};
}
