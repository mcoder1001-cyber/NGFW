import AddIcon from '@mui/icons-material/Add';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import DeleteIcon from '@mui/icons-material/Delete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import FormHelperText from '@mui/material/FormHelperText';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
import { useSchemaFormContext } from '../context.js';
import { compile, fromFormValue, matchVariantForm, toFormValue } from '../form-value.js';
import {
  defaultValueFor,
  groupProperties,
  hintsOf,
  joinPath,
  mergeAllOf,
  recordValueSchema,
  resolveRef,
  sortedProperties,
  titleOf,
  typeOf,
} from '../schema-utils.js';
import type { JsonSchema } from '../types.js';
import { useFieldError, useHelpText, useTranslatedText } from './hooks.js';
import { ChipsInput, MultiSelectInput, PrimitiveInput } from './inputs.js';
import { useController, useFieldArray, useFormContext, useWatch } from './rhf.js';
import { SchemaField, type BoundFieldProps } from './SchemaField.js';

const PRIMITIVE = new Set(['string', 'number', 'integer', 'boolean']);

// ─── object ────────────────────────────────────────────────────────────────

/** Properties in `x-vrx-ui.order`, grouped by `x-vrx-ui.group` into fieldsets (ungrouped first). */
export function ObjectField({ schema, name, propPath }: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const entries = sortedProperties(schema, ctx.root);
  const groups = groupProperties(entries);
  if (entries.length === 0) {
    return <Typography color="text.secondary">{t('form.emptyObject')}</Typography>;
  }
  const showTitles = groups.length > 1 || groups[0]?.name !== undefined;
  return (
    <Stack gap={2}>
      {groups.map((g) => {
        const fields = g.entries.map((e) => (
          <SchemaField
            key={e.key}
            schema={e.schema}
            name={joinPath(name, e.key)}
            propPath={joinPath(propPath, e.key)}
            parentName={name}
            required={e.required}
          />
        ));
        if (!showTitles) return <Stack key="ungrouped" gap={2}>{fields}</Stack>;
        return (
          <GroupFieldset key={g.name ?? ''} title={g.name ?? t('form.ungrouped')}>
            {fields}
          </GroupFieldset>
        );
      })}
    </Stack>
  );
}

function GroupFieldset({ title, children }: { title: string; children: React.ReactNode }) {
  const text = useTranslatedText(title);
  return (
    <Box
      component="fieldset"
      sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, m: 0, minInlineSize: 0 }}
    >
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {text}
      </Typography>
      <Stack gap={2}>{children}</Stack>
    </Box>
  );
}

// ─── record (additionalProperties keyed by user names) ─────────────────────

export function RecordField({ schema, name, propPath, readOnly }: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const { fields, append, remove } = useFieldArray({ name });
  const valueSchema = mergeAllOf(resolveRef(recordValueSchema(schema), ctx.root), ctx.root);
  const keySchema = schema.propertyNames ? resolveRef(schema.propertyNames, ctx.root) : undefined;
  const keyLabel = keySchema?.title ?? t('form.key');
  const valueLabel = valueSchema.title ?? t('form.value');
  const containerError = useFieldError(name);
  const valueIsObject = typeOf(valueSchema) === 'object';
  return (
    <Stack gap={1}>
      {fields.length === 0 && <Typography color="text.secondary">{t('form.emptyList')}</Typography>}
      {fields.map((f, i) => (
        <Paper key={f.id} variant="outlined" sx={{ p: 2 }}>
          <Stack direction={{ xs: 'column', md: 'row' }} gap={2} alignItems="flex-start">
            <Box sx={{ inlineSize: { xs: '100%', md: 260 }, flexShrink: 0 }}>
              <RecordKeyField name={joinPath(name, `${i}.key`)} label={keyLabel} schema={keySchema} readOnly={readOnly} />
            </Box>
            <Box sx={{ flex: 1, minInlineSize: 0, inlineSize: '100%' }}>
              <SchemaField
                schema={valueSchema}
                name={joinPath(name, `${i}.value`)}
                propPath={propPath}
                parentName={joinPath(name, `${i}.value`)}
                label={valueLabel}
                required
                bare={valueIsObject}
              />
            </Box>
            <IconButton aria-label={t('form.remove')} onClick={() => remove(i)} disabled={readOnly}>
              <DeleteIcon />
            </IconButton>
          </Stack>
        </Paper>
      ))}
      {containerError && <FormHelperText error>{containerError}</FormHelperText>}
      <Button
        startIcon={<AddIcon />}
        onClick={() => append({ key: '', value: toFormValue(valueSchema, defaultValueFor(valueSchema, ctx.root), ctx.root) })}
        disabled={readOnly}
        sx={{ alignSelf: 'flex-start' }}
      >
        {t('form.add')}
      </Button>
    </Stack>
  );
}

function RecordKeyField({
  name,
  label,
  schema,
  readOnly,
}: {
  name: string;
  label: string;
  schema: JsonSchema | undefined;
  readOnly: boolean;
}) {
  const { field, fieldState } = useController({ name });
  const s: JsonSchema = { type: 'string', ...(schema ?? {}) };
  const hints = { ...hintsOf(s), widget: hintsOf(s).widget ?? 'mono' };
  return (
    <PrimitiveInput
      schema={s}
      hints={hints}
      widget={hints.widget}
      value={field.value}
      onChange={(v) => field.onChange(v ?? '')}
      onBlur={field.onBlur}
      inputRef={field.ref}
      label={label}
      required
      readOnly={readOnly}
      error={typeof fieldState.error?.message === 'string' ? fieldState.error.message : undefined}
    />
  );
}

// ─── array ─────────────────────────────────────────────────────────────────

export function ArrayField(props: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const items = mergeAllOf(resolveRef(props.schema.items ?? {}, ctx.root), ctx.root);
  const widget = props.hints.widget;
  if (items.enum && (widget === 'multiselect' || props.schema.uniqueItems)) {
    return <MultiSelectField {...props} items={items} />;
  }
  if (widget === 'chips' && typeOf(items) === 'string') return <ChipsField {...props} />;
  const itemType = typeOf(items);
  if ((itemType && PRIMITIVE.has(itemType)) || items.enum || items.const !== undefined) {
    return <PrimitiveListField {...props} items={items} />;
  }
  return <ObjectListField {...props} items={items} />;
}

function MultiSelectField({ schema, name, label, required, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints);
  const err = fieldState.error?.message;
  return (
    <MultiSelectInput
      schema={schema}
      hints={{ ...hintsOf(items), ...hints }}
      widget={hints.widget}
      choices={items.enum ?? []}
      value={field.value}
      onChange={field.onChange}
      onBlur={field.onBlur}
      label={label}
      required={required}
      readOnly={readOnly}
      error={typeof err === 'string' ? err : undefined}
      helperText={help}
    />
  );
}

function ChipsField({ schema, name, label, required, hints, readOnly }: BoundFieldProps) {
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints);
  const err = fieldState.error?.message;
  return (
    <ChipsInput
      schema={schema}
      hints={hints}
      widget={hints.widget}
      value={field.value}
      onChange={field.onChange}
      onBlur={field.onBlur}
      label={label}
      required={required}
      readOnly={readOnly}
      error={typeof err === 'string' ? err : undefined}
      helperText={help}
    />
  );
}

interface ListErrors {
  message?: unknown;
  [index: string]: unknown;
}

function itemErrorOf(errors: unknown, index: number): string | undefined {
  const e = (errors as ListErrors | undefined)?.[String(index)] as { message?: unknown } | undefined;
  return typeof e?.message === 'string' ? e.message : undefined;
}

function ListControls({
  index,
  count,
  readOnly,
  onMove,
  onRemove,
}: {
  index: number;
  count: number;
  readOnly: boolean;
  onMove: (from: number, to: number) => void;
  onRemove: (index: number) => void;
}) {
  const { t } = useTranslation(UI_KIT_NS);
  return (
    <Stack direction="row" sx={{ flexShrink: 0 }}>
      <IconButton aria-label={t('form.moveUp')} disabled={readOnly || index === 0} onClick={() => onMove(index, index - 1)}>
        <ArrowUpwardIcon />
      </IconButton>
      <IconButton
        aria-label={t('form.moveDown')}
        disabled={readOnly || index === count - 1}
        onClick={() => onMove(index, index + 1)}
      >
        <ArrowDownwardIcon />
      </IconButton>
      <IconButton aria-label={t('form.remove')} disabled={readOnly} onClick={() => onRemove(index)}>
        <DeleteIcon />
      </IconButton>
    </Stack>
  );
}

/** One controller for the whole array; rows are presentational inputs (RHF field arrays need object items). */
function PrimitiveListField({ schema, name, label, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints);
  const arr: unknown[] = Array.isArray(field.value) ? field.value : [];
  const listError = typeof fieldState.error?.message === 'string' ? fieldState.error.message : undefined;
  const itemHints = hintsOf(items);
  const set = (next: unknown[]) => field.onChange(next);
  const move = (from: number, to: number) => {
    const next = [...arr];
    const [v] = next.splice(from, 1);
    next.splice(to, 0, v);
    set(next);
  };
  return (
    <Stack component="fieldset" gap={1} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, m: 0, minInlineSize: 0 }}>
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {label}
      </Typography>
      {help && (
        <Typography variant="body2" color="text.secondary">
          {help}
        </Typography>
      )}
      {arr.length === 0 && <Typography color="text.secondary">{t('form.emptyList')}</Typography>}
      {arr.map((v, i) => (
        <Stack key={i} direction="row" gap={1} alignItems="flex-start">
          <Box sx={{ flex: 1, minInlineSize: 0 }}>
            <PrimitiveInput
              schema={items}
              hints={itemHints}
              widget={itemHints.widget}
              value={v}
              onChange={(nv) => {
                const next = [...arr];
                next[i] = nv;
                set(next);
              }}
              onBlur={field.onBlur}
              label={t('form.itemTitle', { index: i + 1 })}
              required
              readOnly={readOnly}
              error={itemErrorOf(fieldState.error, i)}
              options={itemHints.widget === 'interface-picker' ? ctx.interfaceOptions : undefined}
            />
          </Box>
          <ListControls index={i} count={arr.length} readOnly={readOnly} onMove={move} onRemove={(idx) => set(arr.filter((_x, j) => j !== idx))} />
        </Stack>
      ))}
      {listError && <FormHelperText error>{listError}</FormHelperText>}
      <Button
        startIcon={<AddIcon />}
        disabled={readOnly || (schema.maxItems !== undefined && arr.length >= schema.maxItems)}
        onClick={() => set([...arr, defaultValueFor(items, ctx.root)])}
        sx={{ alignSelf: 'flex-start' }}
      >
        {t('form.add')}
      </Button>
    </Stack>
  );
}

/** Array of objects: react-hook-form field array with a nested object form per row. */
function ObjectListField({ schema, name, propPath, label, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const { fields, append, remove, move } = useFieldArray({ name });
  const help = useHelpText(schema, hints);
  const containerError = useFieldError(name);
  return (
    <Stack component="fieldset" gap={1} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, m: 0, minInlineSize: 0 }}>
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {label}
      </Typography>
      {help && (
        <Typography variant="body2" color="text.secondary">
          {help}
        </Typography>
      )}
      {fields.length === 0 && <Typography color="text.secondary">{t('form.emptyList')}</Typography>}
      {fields.map((f, i) => (
        <Paper key={f.id} variant="outlined" sx={{ p: 2 }}>
          <Stack direction="row" gap={1} alignItems="flex-start">
            <Box sx={{ flex: 1, minInlineSize: 0 }}>
              <Typography component="h4" variant="subtitle2" gutterBottom>
                {titleOf(items) || t('form.itemTitle', { index: i + 1 })}
              </Typography>
              <SchemaField
                schema={items}
                name={joinPath(name, i)}
                propPath={propPath}
                parentName={joinPath(name, i)}
                label={titleOf(items) || t('form.itemTitle', { index: i + 1 })}
                required
                bare
              />
            </Box>
            <ListControls index={i} count={fields.length} readOnly={readOnly} onMove={move} onRemove={remove} />
          </Stack>
        </Paper>
      ))}
      {containerError && <FormHelperText error>{containerError}</FormHelperText>}
      <Button
        startIcon={<AddIcon />}
        disabled={readOnly || (schema.maxItems !== undefined && fields.length >= schema.maxItems)}
        onClick={() => append(toFormValue(items, defaultValueFor(items, ctx.root) ?? {}, ctx.root) as Record<string, unknown>)}
        sx={{ alignSelf: 'flex-start' }}
      >
        {t('form.add')}
      </Button>
    </Stack>
  );
}

// ─── oneOf / anyOf ─────────────────────────────────────────────────────────

function discriminatorOf(schema: JsonSchema): string | undefined {
  for (const p of Object.values(schema.properties ?? {})) {
    if (p.const !== undefined) return String(p.const);
  }
  return undefined;
}

/** Variant picker + the selected variant's fields. The picker follows the value when it changes underneath. */
export function VariantField({
  schema,
  name,
  propPath,
  parentName,
  label,
  required,
  hints,
  readOnly,
  variants,
  bare,
}: BoundFieldProps & { variants: JsonSchema[]; bare: boolean }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const { setValue } = useFormContext();
  const current = useWatch({ name });
  const help = useHelpText(schema, hints);
  const error = useFieldError(name);
  const matched = useMemo(() => matchVariantForm(variants, current, ctx.root), [variants, current, ctx.root]);
  const [selected, setSelected] = useState(matched >= 0 ? matched : current === undefined ? -1 : 0);
  useEffect(() => {
    if (matched < 0 || matched === selected) return;
    const chosen = variants[selected];
    const stillValid = chosen ? compile(chosen, ctx.root).safeParse(fromFormValue(chosen, current, ctx.root)).success : false;
    if (!stillValid) setSelected(matched);
  }, [matched, selected, variants, current, ctx.root]);

  const pick = (i: number) => {
    setSelected(i);
    const v = variants[i];
    if (v) setValue(name, toFormValue(v, defaultValueFor(v, ctx.root), ctx.root), { shouldDirty: true });
  };
  const chosen = selected >= 0 ? variants[selected] : undefined;
  const body = (
    <Stack gap={2}>
      <TextField
        select
        fullWidth
        label={t('form.variant')}
        required={required}
        value={selected < 0 ? '' : String(selected)}
        onChange={(e) => pick(Number(e.target.value))}
        error={error !== undefined}
        helperText={error ?? help}
        slotProps={{ input: { readOnly } }}
      >
        {variants.map((v, i) => (
          <MenuItem key={String(i)} value={String(i)}>
            {v.title ?? discriminatorOf(v) ?? String(i + 1)}
          </MenuItem>
        ))}
      </TextField>
      {chosen && (
        <SchemaField schema={chosen} name={name} propPath={propPath} parentName={parentName} label={label} required={required} bare />
      )}
    </Stack>
  );
  if (bare) return body;
  return (
    <Paper variant="outlined" component="section" sx={{ p: 2 }}>
      <Typography component="h3" variant="subtitle2" gutterBottom>
        {label}
      </Typography>
      {body}
    </Paper>
  );
}
