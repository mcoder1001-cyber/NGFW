import { useMemo } from 'react';
import { useFormState } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
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
  const { errors } = useFormState({ name });
  const e = getIn(errors, name) as ErrorLike | undefined;
  const msg = e?.message ?? e?.root?.message;
  return typeof msg === 'string' ? msg : undefined;
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
export function useEnumHints(hints: UiHints, propPath: string): UiHints {
  const text = useSchemaText();
  const labels = text.enumLabels(propPath);
  return useMemo(
    () => (labels ? { ...hints, enumLabels: { ...(hints.enumLabels ?? {}), ...labels } } : hints),
    // the labels object is rebuilt per render: compare it by content
    [hints, JSON.stringify(labels ?? null)],
  );
}
