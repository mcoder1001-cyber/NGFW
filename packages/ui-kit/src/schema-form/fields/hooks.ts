import { useMemo } from 'react';
import { useFormState } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { productEngineLabels, UI_KIT_NS, useProductWording } from '../../i18n/index.js';
import { useSchemaFormContext } from '../context.js';
import { getIn } from '../schema-utils.js';
import { createSchemaText, type SchemaText } from '../text.js';
import type { JsonSchema, UiHints } from '../types.js';

interface ErrorLike {
  message?: unknown;
  root?: { message?: unknown };
}

/** Message for a field, including the `root` slot react-hook-form uses for field-array containers. */
export function useFieldError(name: string): string | undefined {
  const wording = useProductWording();
  const { errors } = useFormState({ name });
  const e = getIn(errors, name) as ErrorLike | undefined;
  const msg = e?.message ?? e?.root?.message;
  return typeof msg === 'string' ? wording(msg) : undefined;
}

/** Does any error sit at or below `name` (a collapsed row, a hidden section)? */
export function useHasErrorsBelow(name: string): boolean {
  const { errors } = useFormState({ name });
  return getIn(errors, name) !== undefined;
}

/** Per-path texts (I18N-1) for the current language and `<SchemaForm i18nPrefix>`. */
export function useSchemaText(): SchemaText {
  const { i18nPrefix } = useSchemaFormContext();
  const { i18n } = useTranslation(UI_KIT_NS);
  const lang = i18n.language;
  // `lang` is a dependency on purpose: the texts change when the language does.
  return useMemo(() => createSchemaText(i18n, i18nPrefix), [i18n, i18nPrefix, lang]);
}

/**
 * Per-path help (`<prefix>.<propPath>.help`), else `x-vrx-ui.help` (an i18n key when it exists in the app's
 * resources, else literal), else the description.
 */
export function useHelpText(schema: JsonSchema, hints: UiHints, propPath = ''): string | undefined {
  const text = useSchemaText();
  return text.help(propPath, hints.help, schema.description);
}

/** Per-path enum labels merged over the literal `x-vrx-ui.enumLabels` (same object when nothing is translated). */
export function useEnumHints(hints: UiHints, propPath: string, schema: JsonSchema): UiHints {
  const text = useSchemaText();
  const { i18n } = useTranslation(UI_KIT_NS);
  const labels = text.enumLabels(propPath);
  // A generic editor can render an enum as its root node (propPath === ''). Infer labels from its actual
  // allowed values too; never treat free-text values or record keys as implementation choices.
  const names = productEngineLabels(i18n.language);
  const defaults = Object.fromEntries(
    (schema.enum ?? (schema.const === undefined ? [] : [schema.const])).flatMap((value) =>
      typeof value === 'string' && names[value] !== undefined ? [[value, names[value]]] : [],
    ),
  );
  const translated =
    Object.keys(defaults).length > 0 || labels !== undefined
      ? { ...defaults, ...(hints.enumLabels ?? {}), ...labels }
      : undefined;
  return useMemo(
    () => (translated ? { ...hints, enumLabels: translated } : hints),
    // the labels object is rebuilt per render: compare it by content
    [hints, JSON.stringify(translated ?? null)],
  );
}
