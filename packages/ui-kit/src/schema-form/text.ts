import type { i18n as I18n } from 'i18next';
import { productEngineLabels, productWording } from '../i18n/product-wording.js';

/**
 * Per-path texts of a schema-driven form (I18N-1). The schema's English `title`/`description`/`x-ngfw-ui` stay the
 * source of truth; when `<SchemaForm i18nPrefix>` is set, these keys (in the app's own namespaces) win:
 *
 * ```
 * <prefix>.<propPath>.title         label of the property
 * <prefix>.<propPath>.help          help text
 * <prefix>.<propPath>.placeholder   placeholder
 * <prefix>.<propPath>.enum          { "<value>": "label" }      enum selects, arrays of enums, table cells, summaries
 * <prefix>.<propPath>.variant       { "<discriminator>": "…" }  oneOf/anyOf picker (discriminator const, else index)
 * <prefix>.<propPath>.group         { "<group>": "label" }      `x-ngfw-ui.group` of this object's properties
 * <prefix>.group                    { "<group>": "label" }      root object's groups, and fallback for every object
 * <prefix>.<propPath>.itemTitle     title of one array item / record value
 * <prefix>.<propPath>.keyTitle      label of a record's key
 * ```
 *
 * `propPath` is the dotted property path without array indexes or record keys (`subinterfaces.vlanId`,
 * `rules.action`); the prefix carries the namespace (`users:field` → `users` namespace, `field.…` keys). Maps are read
 * with `returnObjects`, so enum values and group names may contain dots or spaces.
 */
export interface SchemaText {
  title(propPath: string, fallback: string): string;
  /** Per-path help, else `x-ngfw-ui.help` (itself an i18n key when it exists), else the schema description. */
  help(propPath: string, hintHelp: string | undefined, description: string | undefined): string | undefined;
  placeholder(propPath: string, fallback: string | undefined): string | undefined;
  enumLabels(propPath: string): Readonly<Record<string, string>> | undefined;
  group(objectPath: string, name: string): string;
  variant(propPath: string, key: string, fallback: string): string;
  itemTitle(propPath: string): string | undefined;
  keyTitle(propPath: string): string | undefined;
}

type Lookup = (key: string) => unknown;

function stringMap(v: unknown): Readonly<Record<string, string>> | undefined {
  if (v === null || typeof v !== 'object' || Array.isArray(v)) return undefined;
  const out: Record<string, string> = {};
  for (const [k, s] of Object.entries(v as Record<string, unknown>)) if (typeof s === 'string') out[k] = s;
  return out;
}

export function createSchemaText(i18n: I18n, prefix: string | undefined): SchemaText {
  const wording = (text: string) => productWording(text, i18n.language);
  const optionalWording = (text: string | undefined) =>
    text === undefined ? undefined : wording(text);
  const lookup: Lookup = (key) =>
    i18n.exists(key) ? (i18n.t(key, { returnObjects: true }) as unknown) : undefined;
  const keyOf = (path: string, suffix: string) =>
    `${prefix}.${path === '' ? '' : `${path}.`}${suffix}`;
  const str = (path: string, suffix: string): string | undefined => {
    if (prefix === undefined) return undefined;
    const v = lookup(keyOf(path, suffix));
    return typeof v === 'string' && v !== '' ? v : undefined;
  };
  const map = (path: string, suffix: string) => (prefix === undefined ? undefined : stringMap(lookup(keyOf(path, suffix))));
  /** Historical behaviour: a hint text that is itself an existing i18n key is translated. */
  const asKey = (text: string) => {
    const v = lookup(text);
    return typeof v === 'string' ? v : text;
  };
  return {
    title: (p, fallback) => wording((p === '' ? undefined : str(p, 'title')) ?? fallback),
    help: (p, hintHelp, description) =>
      optionalWording(str(p, 'help') ?? (hintHelp ? asKey(hintHelp) : description)),
    placeholder: (p, fallback) => optionalWording(str(p, 'placeholder') ?? fallback),
    enumLabels: (p) =>
      p.split('.').at(-1) === 'engine'
        ? { ...productEngineLabels(i18n.language), ...map(p, 'enum') }
        : map(p, 'enum'),
    group: (objectPath, name) =>
      wording(
        map(objectPath, 'group')?.[name] ??
          map('', 'group')?.[name] ??
          (name === 'frr-linuxcp'
            ? i18n.language.startsWith('fa')
              ? 'مسیریابی'
              : 'Routing'
            : asKey(name)),
      ),
    variant: (p, key, fallback) =>
      wording(map(p, 'variant')?.[key] ?? productEngineLabels(i18n.language)[key] ?? fallback),
    itemTitle: (p) => optionalWording(str(p, 'itemTitle')),
    keyTitle: (p) => optionalWording(str(p, 'keyTitle')),
  };
}
