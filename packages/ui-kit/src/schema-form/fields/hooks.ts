import { useFormState } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
import { getIn } from '../schema-utils.js';
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

/** `x-vrx-ui.help` (an i18n key when it exists in the app's resources, else literal) or the description. */
export function useHelpText(schema: JsonSchema, hints: UiHints): string | undefined {
  const { i18n } = useTranslation(UI_KIT_NS);
  const help = hints.help;
  if (help) return i18n.exists(help) ? String(i18n.t(help)) : help;
  return schema.description;
}

/** Group names in schemas may be i18n keys too. */
export function useTranslatedText(text: string): string {
  const { i18n } = useTranslation(UI_KIT_NS);
  return i18n.exists(text) ? String(i18n.t(text)) : text;
}
