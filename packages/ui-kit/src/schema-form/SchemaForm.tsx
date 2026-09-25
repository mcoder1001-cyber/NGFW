import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import type { SxProps, Theme } from '@mui/material/styles';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { FormProvider, useForm, type FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../i18n/index.js';
import { SchemaFormContextProvider, type SchemaFormContextValue, type WidgetComponent } from './context.js';
import { SchemaField } from './fields/SchemaField.js';
import { compileError, pointerToFormPath, toFormValue, withDefaults } from './form-value.js';
import { createSchemaResolver, ROOT_FIELD } from './resolver.js';
import { FORM_DEFAULTS, getIn } from './schema-utils.js';
import type { JsonSchema, ProblemDetails, ProblemFieldError, Translate } from './types.js';

export interface SchemaFormProps {
  /** JSON Schema 2020-12 (root of the document being edited; `$defs` are resolved against it). */
  schema: JsonSchema;
  /** Initial JSON value; defaults from the schema when omitted. Changing its *content* resets the form (identity alone does not). */
  value?: unknown;
  /** Receives the *validated, parsed* JSON value (defaults applied, records as objects). */
  onSubmit: (value: unknown) => void | Promise<void>;
  /** RFC 9457 problem from the server; `pointer`s are mapped onto fields, the rest is listed on top. */
  problem?: ProblemDetails | null | undefined;
  readOnly?: boolean | undefined;
  submitLabel?: string | undefined;
  resetLabel?: string | undefined;
  /** Hide Save/Reset (submit through `id` + an external `<Button form={id} type="submit">`). */
  hideActions?: boolean | undefined;
  id?: string | undefined;
  /** Custom widgets by `x-vrx-ui.widget` name. */
  widgets?: Readonly<Record<string, WidgetComponent>> | undefined;
  /** Choices for `interface-picker`. */
  interfaceOptions?: readonly string[] | undefined;
  /** Label override hook (property path → label); receives the per-path translation or the schema title as fallback. */
  translateLabel?: ((propPath: string, fallback: string) => string) | undefined;
  /**
   * Namespaced i18n key prefix for per-path titles, help, placeholders, enum/variant/group labels (I18N-1), e.g.
   * `'users:field'` → `users:field.role.title`, `users:field.role.enum.admin`. Key layout: `SchemaText` (text.ts).
   * Missing keys fall back to the schema's own English texts.
   */
  i18nPrefix?: string | undefined;
  /** When to validate; defaults to `onBlur` (submit always validates). */
  mode?: 'onBlur' | 'onChange' | 'onSubmit' | undefined;
  /** Extra action buttons rendered next to Save. */
  children?: ReactNode;
  sx?: SxProps<Theme> | undefined;
}

interface Unmapped {
  pointer: string;
  detail: string;
}

function problemEntries(problem: ProblemDetails): ProblemFieldError[] {
  const out: ProblemFieldError[] = [];
  if (typeof problem.pointer === 'string') {
    out.push({ pointer: problem.pointer, ...(problem.detail !== undefined ? { detail: problem.detail } : {}) });
  }
  for (const e of problem.errors ?? []) if (typeof e.pointer === 'string') out.push(e);
  return out;
}

/**
 * Renders MUI fields from a JSON Schema + `x-vrx-ui` hints, validates with Zod derived from the same
 * schema, and maps RFC 9457 `pointer` errors back onto fields. See docs/05-ui-spec.md "Schema-driven forms".
 */
export function SchemaForm({
  schema,
  value,
  onSubmit,
  problem,
  readOnly = false,
  submitLabel,
  resetLabel,
  hideActions = false,
  id,
  widgets,
  interfaceOptions,
  translateLabel,
  i18nPrefix,
  mode = 'onBlur',
  children,
  sx,
}: SchemaFormProps) {
  const { t } = useTranslation(UI_KIT_NS);
  const translate = useMemo<Translate>(() => (key, options) => String(t(key, options ?? {})), [t]);
  // Reset only when the value's *content* changes (review P07a L8): an inline `value={{…}}` literal gets a new identity on
  // every parent render and must not wipe the user's edits.
  const valueKey = useMemo(() => JSON.stringify(value ?? null), [value]);
  const stableValue = useRef<{ key: string; value: unknown }>({ key: valueKey, value });
  if (stableValue.current.key !== valueKey) stableValue.current = { key: valueKey, value };
  const currentValue = stableValue.current.value;
  // Optional objects stay absent until switched on (presence, P08-questions Q2); everything else gets its defaults.
  const initial = useMemo<FieldValues>(
    () => ({ [ROOT_FIELD]: toFormValue(schema, withDefaults(schema, currentValue, schema, FORM_DEFAULTS), schema) }),
    [schema, currentValue],
  );
  const resolver = useMemo(() => createSchemaResolver(schema, translate), [schema, translate]);
  /** Schema Zod cannot compile → validation unavailable: shown up front and submitting is refused (review M4). */
  const compileFailed = useMemo(() => compileError(schema, schema) !== undefined, [schema]);
  const methods = useForm<FieldValues>({ defaultValues: initial, resolver, mode, reValidateMode: 'onChange' });
  const { reset, setError, getValues, handleSubmit, formState } = methods;

  useEffect(() => {
    reset(initial);
  }, [initial, reset]);

  const [unmapped, setUnmapped] = useState<Unmapped[]>([]);
  useEffect(() => {
    if (!problem) {
      setUnmapped([]);
      return;
    }
    const rest: Unmapped[] = [];
    const formRoot = getValues(ROOT_FIELD);
    for (const e of problemEntries(problem)) {
      const detail = e.detail ?? e.title ?? problem.title ?? '';
      const path = pointerToFormPath(schema, formRoot, e.pointer, schema);
      // A field inside an absent container (an optional object switched off, a missing list item) is not rendered:
      // its error would be invisible, so it is listed with the unmapped ones instead.
      const parent = path === null ? '' : path.split('.').slice(0, -1).join('.');
      if (path === null || (parent !== '' && getIn(formRoot, parent) === undefined)) rest.push({ pointer: e.pointer, detail });
      else setError(`${ROOT_FIELD}.${path}`, { type: 'server', message: detail });
    }
    setUnmapped(rest);
  }, [problem, schema, getValues, setError]);

  const ctx = useMemo<SchemaFormContextValue>(
    () => ({
      root: schema,
      readOnly,
      widgets: widgets ?? {},
      interfaceOptions: interfaceOptions ?? [],
      translateLabel: translateLabel ?? ((_p, fallback) => fallback),
      i18nPrefix,
    }),
    [schema, readOnly, widgets, interfaceOptions, translateLabel, i18nPrefix],
  );

  const submit = handleSubmit(async (values) => {
    await onSubmit(values[ROOT_FIELD]);
  });
  const rootError = getIn(formState.errors, 'root.schema') as { message?: unknown } | undefined;
  const rootMessage = typeof rootError?.message === 'string' ? rootError.message : undefined;
  const showProblem = problem && (unmapped.length > 0 || problemEntries(problem).length === 0);

  return (
    <FormProvider {...methods}>
      <SchemaFormContextProvider value={ctx}>
        <Box
          component="form"
          id={id}
          noValidate
          onSubmit={submit}
          sx={[{ display: 'flex', flexDirection: 'column', gap: 2 }, ...(Array.isArray(sx) ? sx : sx ? [sx] : [])]}
        >
          {showProblem && (
            <Alert severity="error" role="alert">
              <AlertTitle>{t('form.serverError')}</AlertTitle>
              {problem.detail ?? problem.title}
              {unmapped.length > 0 && (
                <Box component="ul" sx={{ m: 0, ps: 2 }}>
                  {unmapped.map((u) => (
                    <li key={u.pointer}>{t('form.unmappedError', { pointer: u.pointer, detail: u.detail })}</li>
                  ))}
                </Box>
              )}
            </Alert>
          )}
          {compileFailed && (
            <Alert severity="error" role="alert">
              {t('form.validationUnavailable')}
            </Alert>
          )}
          {rootMessage && !compileFailed && (
            <Alert severity="error" role="alert" sx={{ whiteSpace: 'pre-line' }}>
              <AlertTitle>{t('form.schemaError')}</AlertTitle>
              {rootMessage}
            </Alert>
          )}
          <SchemaField schema={schema} name={ROOT_FIELD} propPath="" parentName={ROOT_FIELD} required bare />
          {!hideActions && (
            <Stack direction="row" gap={1} justifyContent="flex-end" flexWrap="wrap">
              {children}
              <Button type="button" variant="text" onClick={() => reset(initial)} disabled={!formState.isDirty || formState.isSubmitting}>
                {resetLabel ?? t('form.reset')}
              </Button>
              <Button type="submit" variant="contained" disabled={readOnly || compileFailed || formState.isSubmitting}>
                {submitLabel ?? t('form.submit')}
              </Button>
            </Stack>
          )}
        </Box>
      </SchemaFormContextProvider>
    </FormProvider>
  );
}
