import { matchVariant } from './form-value.js';
import {
  hintsOf,
  isLtrString,
  isRecordSchema,
  itemKeyOf,
  joinPath,
  mergeAllOf,
  propertyTitles,
  resolveRef,
  sortedProperties,
  titleOf,
  typeOf,
  variantsOf,
} from './schema-utils.js';
import type { SchemaText } from './text.js';
import type { JsonSchema, UiHints } from './types.js';

/** What a summary needs from the per-path texts plus the yes/no words of the UI language. */
export type SummaryText = Pick<SchemaText, 'title' | 'enumLabels' | 'variant'> & { yes: string; no: string };

/** Arrays and records show this many entries, then `+N`. */
const MAX_LISTED = 3;

function isSecret(schema: JsonSchema): boolean {
  const h = hintsOf(schema);
  return schema.writeOnly === true || h.secret === true || h.widget === 'password' || h.widget === 'hidden';
}

function listed(parts: string[]): string {
  const shown = parts.filter((p) => p !== '');
  if (shown.length <= MAX_LISTED) return shown.join(', ');
  return `${shown.slice(0, MAX_LISTED).join(', ')} +${shown.length - MAX_LISTED}`;
}

/** The first `const` property of an object variant (the discriminator), as `[key, value]`. */
function discriminator(schema: JsonSchema): [string, unknown] | undefined {
  for (const [k, p] of Object.entries(schema.properties ?? {})) if (p.const !== undefined) return [k, p.const];
  return undefined;
}

/**
 * One-line summary of a **JSON** value (not the form layout) for rule-editor cells and `itemKey` row headers:
 * enum values through their per-path labels, booleans as yes/no (inside an object: the member's title when true),
 * unions as `<variant> <members>`, lists and records shortened to three entries. Secrets never appear.
 */
export function summarizeValue(raw: JsonSchema, value: unknown, root: JsonSchema, propPath: string, text: SummaryText): string {
  return summarize(raw, value, root, propPath, text, false);
}

function summarize(raw: JsonSchema, value: unknown, root: JsonSchema, propPath: string, text: SummaryText, member: boolean): string {
  if (value === undefined || value === null) return '';
  const schema = mergeAllOf(resolveRef(raw, root), root);
  if (isSecret(schema)) return '';
  const variants = variantsOf(schema);
  if (variants && typeOf(schema) === undefined) {
    const i = matchVariant(variants, value, root);
    if (i >= 0) return summarizeVariant(mergeAllOf(resolveRef(variants[i]!, root), root), i, value, root, propPath, text);
  }
  if (schema.enum || schema.const !== undefined) return enumLabel(value, propPath, text);
  if (typeof value === 'boolean') {
    if (!member) return value ? text.yes : text.no;
    return value ? text.title(propPath, titleOf(schema, propPath.split('.').pop())) : '';
  }
  if (typeof value === 'number' || typeof value === 'string') return String(value);
  if (Array.isArray(value)) {
    const items = schema.items ?? {};
    return listed(value.map((v) => summarize(items, v, root, propPath, text, false)));
  }
  if (typeof value === 'object') {
    if (isRecordSchema(schema)) return listed(Object.keys(value));
    return objectParts(schema, value as Record<string, unknown>, root, propPath, text, undefined).join(' ');
  }
  return '';
}

function enumLabel(value: unknown, propPath: string, text: SummaryText): string {
  return text.enumLabels(propPath)?.[String(value)] ?? String(value);
}

function objectParts(
  schema: JsonSchema,
  value: Record<string, unknown>,
  root: JsonSchema,
  propPath: string,
  text: SummaryText,
  skip: string | undefined,
): string[] {
  const parts: string[] = [];
  for (const e of sortedProperties(schema, root)) {
    if (e.key === skip) continue;
    const s = summarize(e.schema, value[e.key], root, joinPath(propPath, e.key), text, true);
    if (s !== '') parts.push(s);
  }
  return parts;
}

function summarizeVariant(
  variant: JsonSchema,
  index: number,
  value: unknown,
  root: JsonSchema,
  propPath: string,
  text: SummaryText,
): string {
  if (typeOf(variant) !== 'object' || value === null || typeof value !== 'object' || Array.isArray(value)) {
    return summarize(variant, value, root, propPath, text, false);
  }
  const d = discriminator(variant);
  const key = d ? String(d[1]) : String(index);
  // A variant without discriminator or title has no name worth showing (only its members).
  const label = d || variant.title ? text.variant(propPath, key, variant.title ?? key) : '';
  const parts = objectParts(variant, value as Record<string, unknown>, root, propPath, text, d?.[0]);
  return [label, ...parts].filter((p) => p !== '').join(' ');
}

/** A rule-editor column: a member of the item object (`key`), or the whole item (`key: ''`) when it has no members. */
export interface TableColumn {
  key: string;
  schema: JsonSchema;
  /** Default header (sibling-disambiguated schema title, `propertyTitles`); `''` for the whole-item column. */
  title: string;
}

const NO_COLUMN_WIDGETS: ReadonlySet<string> = new Set(['hidden', 'textarea', 'json', 'password']);

function columnWorthy(raw: JsonSchema, root: JsonSchema): boolean {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  const h = hintsOf(schema);
  if ((h.widget !== undefined && NO_COLUMN_WIDGETS.has(h.widget)) || isSecret(schema)) return false;
  const t = typeOf(schema);
  if (t === 'object' && isRecordSchema(schema)) return false;
  if (t === 'array') {
    const items = mergeAllOf(resolveRef(schema.items ?? {}, root), root);
    if (typeOf(items) === 'object' || variantsOf(items)) return false;
  }
  return true;
}

/**
 * Columns of the rule-editor table for an array whose items are `items`: the `itemKey` members first (in key order),
 * then every other column-worthy member in render order (`x-vrx-ui.order`). Long text (textarea), JSON, secrets,
 * records and lists of objects stay in the row form only.
 */
export function tableColumns(items: JsonSchema, arrayHints: UiHints, root: JsonSchema): TableColumn[] {
  const entries = sortedProperties(items, root);
  if (entries.length === 0) return [{ key: '', schema: items, title: '' }];
  const titles = propertyTitles(entries);
  const key = itemKeyOf(arrayHints);
  const rank = (k: string) => {
    const i = key.indexOf(k);
    return i < 0 ? key.length : i;
  };
  return entries
    .map((e, index) => ({ e, index }))
    .filter(({ e }) => key.includes(e.key) || columnWorthy(e.schema, root))
    .sort((a, b) => rank(a.e.key) - rank(b.e.key) || a.index - b.index)
    .map(({ e }) => ({ key: e.key, schema: e.schema, title: titles[e.key] ?? e.key }));
}

/**
 * Is every value of this schema an identifier (address, prefix, name, range …)? Then its summary is shown LTR even in an
 * RTL page; anything else (enum labels, unions with a variant word, prose) follows the page direction (review M2).
 */
export function isIdentifierSchema(raw: JsonSchema, root: JsonSchema): boolean {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  const widget = hintsOf(schema).widget;
  if (schema.enum || schema.const !== undefined) return false;
  const t = typeOf(schema);
  if (t === 'string') return isLtrString(schema, widget);
  if (t === 'array') return isIdentifierSchema(schema.items ?? {}, root);
  const variants = variantsOf(schema);
  if (variants && t === undefined) {
    if (widget !== undefined && isLtrString({ type: 'string' }, widget)) return true;
    return variants.length > 0 && variants.every((v) => isIdentifierSchema(v, root));
  }
  return false;
}
