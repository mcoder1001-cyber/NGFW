import { toNestErrors } from '@hookform/resolvers';
import type { FieldError, FieldValues, Resolver } from 'react-hook-form';
import { compile, compileError, findRecordKeyIssues, formPathFor, fromFormValue } from './form-value.js';
import { issueMessage } from './messages.js';
import type { JsonPath, JsonSchema, Translate } from './types.js';

/** react-hook-form stores the whole document under this key so the root may be a record, array or union. */
export const ROOT_FIELD = '$';

/** RHF root-level error slot for issues that cannot be attached to a field (e.g. unknown keys). */
export const ROOT_ERROR = 'root.schema';

/**
 * Resolver: form layout → JSON → Zod (from the same JSON Schema) → translated errors mapped back onto
 * fields. Successful validation hands the *parsed* JSON (defaults applied) to `onSubmit`.
 */
export function createSchemaResolver(schema: JsonSchema, t: Translate): Resolver<FieldValues> {
  const zodSchema = compile(schema, schema);
  const broken = compileError(schema, schema);
  return async (formValues, _context, options) => {
    if (broken) {
      // Fail closed (review M4): without client-side validation nothing is submitted.
      return { values: {}, errors: toNestErrors({ [ROOT_ERROR]: { type: 'compile', message: t('form.validationUnavailable') } }, options) };
    }
    const formRoot = formValues[ROOT_FIELD];
    // Fields hidden by `dependsOn` are dropped here: neither validated nor submitted (review M3).
    const value = fromFormValue(schema, formRoot, schema, formRoot);
    const flat = new Map<string, FieldError>();
    const prefixed = (p: string) => (p === '' ? ROOT_FIELD : `${ROOT_FIELD}.${p}`);

    for (const k of findRecordKeyIssues(schema, formRoot, schema)) {
      flat.set(prefixed(k.path), {
        type: k.type,
        message: t(k.type === 'duplicate' ? 'form.duplicateKey' : 'validation.required'),
      });
    }

    const result = await zodSchema.safeParseAsync(value, { error: (issue) => issueMessage(issue, t) });
    const unmapped: string[] = [];
    if (!result.success) {
      for (const issue of result.error.issues) {
        const path: JsonPath = issue.path.map((p) => (typeof p === 'symbol' ? String(p) : p));
        const formPath = path.length === 0 ? null : formPathFor(schema, formRoot, path, schema);
        if (formPath === null) {
          unmapped.push(path.length ? `/${path.join('/')}: ${issue.message}` : issue.message);
          continue;
        }
        const key = prefixed(formPath);
        if (!flat.has(key)) flat.set(key, { type: issue.code, message: issue.message });
      }
    }
    if (unmapped.length > 0) flat.set(ROOT_ERROR, { type: 'schema', message: unmapped.join('\n') });

    if (flat.size === 0) {
      return { values: { [ROOT_FIELD]: result.success ? result.data : value }, errors: {} };
    }
    // Parents before children so a container error is not overwritten by an item error object.
    const ordered = [...flat.entries()].sort((a, b) => a[0].split('.').length - b[0].split('.').length);
    return { values: {}, errors: toNestErrors(Object.fromEntries(ordered), options) };
  };
}
