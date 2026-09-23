import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import { useController, useWatch, type ReactNode } from './rhf.js';
import { useSchemaFormContext } from '../context.js';
import { formPathFor } from '../form-value.js';
import { ROOT_FIELD } from '../resolver.js';
import {
  dependencyMet,
  dependsOnOf,
  hintsOf,
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
import { useFieldError, useHelpText } from './hooks.js';
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
  required = false,
  bare = false,
}: SchemaFieldProps & { hints: UiHints }) {
  const ctx = useSchemaFormContext();
  if (hints.widget === 'hidden') return null;
  const readOnly = ctx.readOnly || schema.readOnly === true;
  const lastKey = propPath.split('.').pop() ?? '';
  const label = givenLabel ?? ctx.translateLabel(propPath, titleOf(schema, lastKey));
  const bound: BoundFieldProps = { schema, name, propPath, parentName, label, required, hints, readOnly };

  const Custom = hints.widget ? ctx.widgets[hints.widget] : undefined;
  if (Custom) return <Custom {...bound} />;

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
    const help = hints.help ?? schema.description;
    const inner = isRecordSchema(schema) ? <RecordField {...bound} /> : <ObjectField {...bound} />;
    if (bare) return inner;
    return (
      <Paper variant="outlined" component="section" sx={{ p: 2 }}>
        <Typography component="h3" variant="subtitle2" gutterBottom={!help}>
          {label}
        </Typography>
        {help && (
          <Typography variant="body2" color="text.secondary" gutterBottom>
            {help}
          </Typography>
        )}
        {inner}
      </Paper>
    );
  }
  return <PrimitiveField {...bound} />;
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
export function PrimitiveField({ schema, name, label, required, hints, readOnly }: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints);
  const err = fieldState.error?.message;
  return (
    <PrimitiveInput
      schema={schema}
      hints={hints}
      widget={hints.widget}
      value={field.value}
      onChange={field.onChange}
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
