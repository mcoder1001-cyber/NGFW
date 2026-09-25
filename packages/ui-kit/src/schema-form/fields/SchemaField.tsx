import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
import { useController, useFormContext, useWatch, type ReactNode } from './rhf.js';
import { useSchemaFormContext } from '../context.js';
import { formPathFor, toFormValue } from '../form-value.js';
import { ROOT_FIELD } from '../resolver.js';
import {
  acceptsNull,
  defaultValueFor,
  dependencyMet,
  dependsOnOf,
  FORM_DEFAULTS,
  hintsOf,
  isOptionalObject,
  isRecordSchema,
  joinPath,
  mergeAllOf,
  parsePointer,
  resolveRef,
  titleOf,
  typeOf,
  variantsOf,
} from '../schema-utils.js';
import type { JsonSchema, UiDependsOn, UiHints } from '../types.js';
import { ArrayField, ObjectField, RecordField, VariantField } from './composites.js';
import { prose } from './bidi.js';
import { useEnumHints, useFieldError, useHelpText, useSchemaText } from './hooks.js';
import { PrimitiveInput } from './inputs.js';

export interface SchemaFieldProps {
  schema: JsonSchema;
  /** react-hook-form path of this value. */
  name: string;
  /** Property path without indexes/record keys, used for label translation. */
  propPath: string;
  /** RHF path of the enclosing object — `dependsOn` siblings resolve against it. */
  parentName: string;
  label?: string | undefined;
  /** Default label when no per-path translation exists (the sibling-disambiguated schema title; see `propertyTitles`). */
  fallbackTitle?: string | undefined;
  required?: boolean | undefined;
  /** Render objects/records without their own titled panel (used by unions and record values). */
  bare?: boolean | undefined;
}

export interface BoundFieldProps {
  schema: JsonSchema;
  name: string;
  propPath: string;
  parentName: string;
  label: string;
  required: boolean;
  hints: UiHints;
  readOnly: boolean;
}

const PRIMITIVE = new Set(['string', 'number', 'integer', 'boolean']);

/** Renders any schema node: resolves `$ref`/`allOf`, applies `dependsOn`, dispatches on type and widget. */
export function SchemaField(props: SchemaFieldProps) {
  const ctx = useSchemaFormContext();
  const schema = mergeAllOf(resolveRef(props.schema, ctx.root), ctx.root);
  const hints = hintsOf(schema);
  const dep = dependsOnOf(hints);
  const body = <SchemaFieldBody {...props} schema={schema} hints={hints} />;
  if (!dep) return body;
  return (
    <DependsOnGate dep={dep} parentName={props.parentName}>
      {body}
    </DependsOnGate>
  );
}

function SchemaFieldBody({
  schema,
  hints,
  name,
  propPath,
  parentName,
  label: givenLabel,
  fallbackTitle,
  required = false,
  bare = false,
}: SchemaFieldProps & { hints: UiHints }) {
  const ctx = useSchemaFormContext();
  const text = useSchemaText();
  if (hints.widget === 'hidden') return null;
  const readOnly = ctx.readOnly || schema.readOnly === true;
  const lastKey = propPath.split('.').pop() ?? '';
  const label = givenLabel ?? ctx.translateLabel(propPath, text.title(propPath, fallbackTitle ?? titleOf(schema, lastKey)));
  const bound: BoundFieldProps = { schema, name, propPath, parentName, label, required, hints, readOnly };

  const Custom = hints.widget ? ctx.widgets[hints.widget] : undefined;
  if (Custom) return <Custom {...bound} />;
  // Opaque blobs: edit any node as JSON text regardless of its type.
  if (hints.widget === 'json') return <PrimitiveField {...bound} />;

  const variants = variantsOf(schema);
  if (variants && typeOf(schema) === undefined) {
    const resolved = variants.map((v) => mergeAllOf(resolveRef(v, ctx.root), ctx.root));
    const merged = mergePrimitiveUnion(schema, resolved, hints);
    if (merged) return <PrimitiveField {...bound} schema={merged} hints={hintsOf(merged)} />;
    return <VariantField {...bound} variants={resolved} bare={bare} />;
  }

  const t = typeOf(schema);
  if (t === 'array') return <ArrayField {...bound} />;
  if (t === 'object') {
    const inner = isRecordSchema(schema) ? <RecordField {...bound} /> : <ObjectField {...bound} />;
    if (bare) return inner;
    return (
      <ObjectSection {...bound} optional={isOptionalObject(schema, required, ctx.root)}>
        {inner}
      </ObjectSection>
    );
  }
  return <PrimitiveField {...bound} />;
}

/**
 * Titled panel of an object member. An optional object (`isOptionalObject`) gets a presence switch: absent until the
 * user switches it on (then filled with its defaults), removed again when switched off — never invented by the form
 * (P08-questions Q2). Its fields are neither rendered, validated nor submitted while it is absent.
 */
function ObjectSection({ schema, name, propPath, label, hints, readOnly, optional, children }: BoundFieldProps & { optional: boolean; children: ReactNode }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const help = useHelpText(schema, hints, propPath);
  const { setValue, clearErrors } = useFormContext();
  const value: unknown = useWatch({ name, disabled: !optional });
  const present = !optional || value !== undefined;
  const toggle = (on: boolean) => {
    if (on) {
      setValue(name, toFormValue(schema, defaultValueFor(schema, ctx.root, FORM_DEFAULTS), ctx.root), { shouldDirty: true });
    } else {
      clearErrors(name);
      setValue(name, undefined, { shouldDirty: true });
    }
  };
  if (!optional) {
    return (
      <Paper variant="outlined" component="section" sx={{ p: 2 }}>
        <Typography component="h3" variant="subtitle2" gutterBottom={!help}>
          {label}
        </Typography>
        {help && (
          <Typography variant="body2" color="text.secondary" gutterBottom>
            {prose(help)}
          </Typography>
        )}
        {children}
      </Paper>
    );
  }
  // Same structure (the heading stays a direct child of the section), with the switch in a second grid column.
  const full = { gridColumn: '1 / -1' } as const;
  return (
    <Paper
      variant="outlined"
      component="section"
      sx={{ p: 2, display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) auto', alignItems: 'center', columnGap: 1 }}
    >
      <Typography component="h3" variant="subtitle2">
        {label}
      </Typography>
      <Switch
        checked={present}
        onChange={(e) => toggle(e.target.checked)}
        disabled={readOnly}
        slotProps={{ input: { 'aria-label': t('form.presence', { label }) } }}
      />
      {help && (
        <Typography variant="body2" color="text.secondary" gutterBottom sx={full}>
          {prose(help)}
        </Typography>
      )}
      <Stack sx={full}>
        {present ? (
          children
        ) : (
          <Typography variant="body2" color="text.secondary">
            {t('form.notConfigured')}
          </Typography>
        )}
      </Stack>
    </Paper>
  );
}

/** A union of same-typed primitives (e.g. ipv4 | ipv6) renders as one input; a union of consts as a select. */
function mergePrimitiveUnion(schema: JsonSchema, variants: JsonSchema[], hints: UiHints): JsonSchema | null {
  if (variants.length === 0) return null;
  const base = { ...(schema.title ? { title: schema.title } : {}), ...(schema.description ? { description: schema.description } : {}) };
  if (variants.every((v) => v.const !== undefined)) {
    const first = variants[0]!;
    return { ...base, ...(first.type ? { type: first.type } : {}), enum: variants.map((v) => v.const), 'x-vrx-ui': { ...hintsOf(first), ...hints } };
  }
  const kinds = variants.map((v) => (v.enum || v.const !== undefined ? 'enum' : typeOf(v) === 'integer' ? 'number' : typeOf(v)));
  const kind = kinds[0];
  if (!kind || kind === 'enum' || !PRIMITIVE.has(kind) || kinds.some((k) => k !== kind)) return null;
  const first = variants[0]!;
  return { ...first, ...base, 'x-vrx-ui': { ...hintsOf(first), ...hints } };
}

/** One registered field bound to a presentational input. */
export function PrimitiveField({ schema, name, propPath, label, required, hints, readOnly }: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const text = useSchemaText();
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints, propPath);
  const enumHints = useEnumHints(hints, propPath);
  const placeholder = text.placeholder(propPath, typeof hints.placeholder === 'string' ? hints.placeholder : undefined);
  const shownHints = placeholder === undefined || placeholder === hints.placeholder ? enumHints : { ...enumHints, placeholder };
  const err = fieldState.error?.message;
  // react-hook-form shows a field's mount-time value again once its value becomes `undefined` (useController → useWatch
  // falls back to the default), so a cleared input would snap back to its old text and typing would append to it.
  // "Cleared" is therefore stored as `null`; fromFormValue turns it into "absent" unless the schema allows null.
  const nullable = acceptsNull(schema, ctx.root);
  return (
    <PrimitiveInput
      schema={schema}
      hints={shownHints}
      widget={hints.widget}
      value={field.value === null && !nullable ? undefined : field.value}
      onChange={(v) => field.onChange(v === undefined ? null : v)}
      onBlur={field.onBlur}
      inputRef={field.ref}
      label={label}
      required={required}
      readOnly={readOnly}
      error={typeof err === 'string' ? err : undefined}
      helperText={help}
      options={hints.widget === 'interface-picker' ? ctx.interfaceOptions : undefined}
    />
  );
}

function DependsOnGate({ dep, parentName, children }: { dep: UiDependsOn; parentName: string; children: ReactNode }) {
  const ctx = useSchemaFormContext();
  const absolute = dep.field.startsWith('/');
  const rootValue = useWatch({ name: ROOT_FIELD, disabled: !absolute });
  let path: string | null;
  if (absolute) {
    const p = formPathFor(ctx.root, rootValue, parsePointer(dep.field), ctx.root);
    path = p === null ? null : joinPath(ROOT_FIELD, p);
  } else {
    path = joinPath(parentName, dep.field);
  }
  const actual = useWatch({ name: path ?? ROOT_FIELD, disabled: path === null });
  const value = path === null ? undefined : actual;
  return dependencyMet(dep, value) ? <>{children}</> : null;
}

/** Container-level error (array/record/union), for composites that do not own a controller. */
export function useContainerError(name: string): string | undefined {
  return useFieldError(name);
}
