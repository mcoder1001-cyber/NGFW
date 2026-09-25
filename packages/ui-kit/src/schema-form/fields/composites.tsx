import AddIcon from '@mui/icons-material/Add';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import FormHelperText from '@mui/material/FormHelperText';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
import { useSchemaFormContext } from '../context.js';
import { compile, fromFormValue, matchVariantForm, toFormValue } from '../form-value.js';
import {
  defaultValueFor,
  FORM_DEFAULTS,
  getIn,
  groupProperties,
  hintsOf,
  isLtrString,
  itemKeyOf,
  joinPath,
  mergeAllOf,
  propertyTitles,
  recordValueSchema,
  resolveRef,
  sortedProperties,
  titleOf,
  typeOf,
} from '../schema-utils.js';
import { isIdentifierSchema, summarizeValue, tableColumns, type SummaryText, type TableColumn } from '../summary.js';
import type { JsonSchema } from '../types.js';
import { identifier, prose } from './bidi.js';
import { useEnumHints, useFieldError, useHelpText, useSchemaText } from './hooks.js';
import { ChipsInput, MultiSelectInput, PrimitiveInput } from './inputs.js';
import { useController, useFieldArray, useFormContext, useFormState, useWatch } from './rhf.js';
import { SchemaField, type BoundFieldProps } from './SchemaField.js';

const PRIMITIVE = new Set(['string', 'number', 'integer', 'boolean']);
const SMALL = 'small' as const;

const FIELDSET_SX = { border: 1, borderColor: 'divider', borderRadius: 1, p: 2, m: 0, minInlineSize: 0 } as const;

/** Rule-table cells: tighter than MUI's small cells so more columns fit before the table scrolls sideways. */
const CELL = { px: 1 } as const;

/** The rule table's actions column stays visible at the end while the table scrolls sideways. */
const STICKY_END = { position: 'sticky', insetInlineEnd: 0, zIndex: 1, bgcolor: 'background.paper', whiteSpace: 'nowrap' } as const;

/** Per-path texts plus the yes/no words, for row summaries and table cells. */
function useSummaryText(): SummaryText {
  const text = useSchemaText();
  const { t } = useTranslation(UI_KIT_NS);
  return useMemo(() => ({ title: text.title, enumLabels: text.enumLabels, variant: text.variant, yes: t('form.yes'), no: t('form.no') }), [text, t]);
}

// ─── object ────────────────────────────────────────────────────────────────

/** Properties in `x-vrx-ui.order`, grouped by `x-vrx-ui.group` into fieldsets (ungrouped first). */
export function ObjectField({ schema, name, propPath }: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const text = useSchemaText();
  const entries = sortedProperties(schema, ctx.root);
  const groups = groupProperties(entries);
  const titles = propertyTitles(entries);
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
            fallbackTitle={titles[e.key]}
            required={e.required}
          />
        ));
        if (!showTitles) return <Stack key="ungrouped" gap={2}>{fields}</Stack>;
        return (
          <GroupFieldset key={g.name ?? ''} title={g.name === undefined ? t('form.ungrouped') : text.group(propPath, g.name)}>
            {fields}
          </GroupFieldset>
        );
      })}
    </Stack>
  );
}

function GroupFieldset({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box component="fieldset" sx={FIELDSET_SX}>
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {title}
      </Typography>
      <Stack gap={2}>{children}</Stack>
    </Box>
  );
}

// ─── record (additionalProperties keyed by user names) ─────────────────────

export function RecordField({ schema, name, propPath, readOnly }: BoundFieldProps) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const text = useSchemaText();
  const { fields, append, remove } = useFieldArray({ name });
  const valueSchema = mergeAllOf(resolveRef(recordValueSchema(schema), ctx.root), ctx.root);
  const keySchema = schema.propertyNames ? resolveRef(schema.propertyNames, ctx.root) : undefined;
  const keyLabel = text.keyTitle(propPath) ?? keySchema?.title ?? t('form.key');
  const valueLabel = text.itemTitle(propPath) ?? valueSchema.title ?? t('form.value');
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
        onClick={() =>
          append({ key: '', value: toFormValue(valueSchema, defaultValueFor(valueSchema, ctx.root, FORM_DEFAULTS), ctx.root) })
        }
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
  if ((widget === 'chips' || widget === 'tag-picker') && typeOf(items) === 'string') return <ChipsField {...props} items={items} />;
  const itemType = typeOf(items);
  if ((itemType && PRIMITIVE.has(itemType)) || items.enum || items.const !== undefined) {
    return <PrimitiveListField {...props} items={items} />;
  }
  if (widget === 'rule-editor') return <RuleEditorField {...props} items={items} />;
  return <ObjectListField {...props} items={items} />;
}

function MultiSelectField({ schema, name, propPath, label, required, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints, propPath);
  const merged = useMemo(() => ({ ...hintsOf(items), ...hints }), [items, hints]);
  const shown = useEnumHints(merged, propPath);
  const err = fieldState.error?.message;
  return (
    <MultiSelectInput
      schema={schema}
      hints={shown}
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

function ChipsField({ schema, name, propPath, label, required, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints, propPath);
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
      ltr={hints.widget === 'tag-picker' || isLtrString(items, hintsOf(items).widget)}
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
  dense = false,
}: {
  index: number;
  count: number;
  readOnly: boolean;
  onMove: (from: number, to: number) => void;
  onRemove: (index: number) => void;
  /** Small buttons (table rows). */
  dense?: boolean;
}) {
  const { t } = useTranslation(UI_KIT_NS);
  const size = dense ? SMALL : undefined;
  return (
    <Stack direction="row" sx={{ flexShrink: 0 }}>
      <IconButton size={size} aria-label={t('form.moveUp')} disabled={readOnly || index === 0} onClick={() => onMove(index, index - 1)}>
        <ArrowUpwardIcon />
      </IconButton>
      <IconButton
        size={size}
        aria-label={t('form.moveDown')}
        disabled={readOnly || index === count - 1}
        onClick={() => onMove(index, index + 1)}
      >
        <ArrowDownwardIcon />
      </IconButton>
      <IconButton size={size} aria-label={t('form.remove')} disabled={readOnly} onClick={() => onRemove(index)}>
        <DeleteIcon />
      </IconButton>
    </Stack>
  );
}

/** One controller for the whole array; rows are presentational inputs (RHF field arrays need object items). */
function PrimitiveListField({ schema, name, propPath, label, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const { field, fieldState } = useController({ name });
  const help = useHelpText(schema, hints, propPath);
  const arr: unknown[] = Array.isArray(field.value) ? field.value : [];
  const listError = typeof fieldState.error?.message === 'string' ? fieldState.error.message : undefined;
  const itemHints = useEnumHints(hintsOf(items), propPath);
  const set = (next: unknown[]) => field.onChange(next);
  const move = (from: number, to: number) => {
    const next = [...arr];
    const [v] = next.splice(from, 1);
    next.splice(to, 0, v);
    set(next);
  };
  return (
    <Stack component="fieldset" gap={1} sx={FIELDSET_SX}>
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {label}
      </Typography>
      {help && (
        <Typography variant="body2" color="text.secondary">
          {prose(help)}
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
        onClick={() => set([...arr, defaultValueFor(items, ctx.root, FORM_DEFAULTS)])}
        sx={{ alignSelf: 'flex-start' }}
      >
        {t('form.add')}
      </Button>
    </Stack>
  );
}

/**
 * Which rows of a field array are open. A row added through `openNext()` + `append()` opens at once (its id is only
 * known after the append). A row the list had to open (`forced`: it has errors, or no key summary yet) is pinned open
 * as well, so it does not snap shut — taking the focus with it — once the user's typing fixes the error or creates the
 * summary (review M3). Only the user closes an open row; everything else starts closed.
 */
function useRowExpansion(ids: readonly string[], forced: readonly string[]) {
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const pending = useRef(false);
  const lastId = ids[ids.length - 1];
  const pendingId = pending.current ? lastId : undefined;
  useEffect(() => {
    if (pending.current && lastId !== undefined) {
      pending.current = false;
      setExpanded((s) => new Set(s).add(lastId));
    }
  }, [lastId]);
  const forcedKey = forced.join('\u0000');
  useEffect(() => {
    if (forcedKey === '') return;
    const pin = forcedKey.split('\u0000');
    setExpanded((s) => (pin.every((id) => s.has(id)) ? s : new Set([...s, ...pin])));
  }, [forcedKey]);
  return {
    isOpen: (id: string) => expanded.has(id) || id === pendingId || forced.includes(id),
    toggle: (id: string) =>
      setExpanded((s) => {
        const next = new Set(s);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      }),
    openNext: () => {
      pending.current = true;
    },
  };
}

/** The JSON value of one row of an object list (form layout → JSON), `undefined` while it is not there yet. */
function rowJson(items: JsonSchema, rows: unknown, index: number, root: JsonSchema): Record<string, unknown> | undefined {
  const row = Array.isArray(rows) ? (rows[index] as unknown) : undefined;
  if (row === undefined) return undefined;
  const json = fromFormValue(items, row, root);
  return json !== null && typeof json === 'object' && !Array.isArray(json) ? (json as Record<string, unknown>) : undefined;
}

/**
 * Array of objects: react-hook-form field array with a nested object form per row. With `x-vrx-ui.itemKey` every row
 * shows its key summary (`admin`, `10.0.0.0/24 · red`) in its heading and can be collapsed; rows that already have a
 * summary start collapsed, new rows and rows with errors are open.
 */
function ObjectListField({ schema, name, propPath, label, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const text = useSchemaText();
  const stext = useSummaryText();
  const { fields, append, remove, move } = useFieldArray({ name });
  const help = useHelpText(schema, hints, propPath);
  const containerError = useFieldError(name);
  const itemKey = itemKeyOf(hints);
  const keyed = itemKey.length > 0;
  const rows: unknown = useWatch({ name, disabled: !keyed });
  const { errors } = useFormState({ name, disabled: !keyed });
  const itemTitle = text.itemTitle(propPath) ?? titleOf(items);
  const keyIsIdentifier = keyed && itemKey.every((k) => items.properties?.[k] !== undefined && isIdentifierSchema(items.properties[k], ctx.root));
  const summaryOf = (i: number): string => {
    const json = rowJson(items, rows, i, ctx.root);
    if (!json) return '';
    return itemKey
      .map((k) => (items.properties?.[k] ? summarizeValue(items.properties[k], json[k], ctx.root, joinPath(propPath, k), stext) : ''))
      .filter((s) => s !== '')
      .join(' · ');
  };
  const rowState = fields.map((f, i) => {
    const summary = keyed ? summaryOf(i) : '';
    const hasError = keyed && getIn(errors, joinPath(name, i)) !== undefined;
    return { id: f.id, summary, hasError, forced: keyed && (summary === '' || hasError) };
  });
  const rowsOpen = useRowExpansion(
    fields.map((f) => f.id),
    rowState.filter((r) => r.forced).map((r) => r.id),
  );
  return (
    <Stack component="fieldset" gap={1} sx={FIELDSET_SX}>
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {label}
      </Typography>
      {help && (
        <Typography variant="body2" color="text.secondary">
          {prose(help)}
        </Typography>
      )}
      {fields.length === 0 && <Typography color="text.secondary">{t('form.emptyList')}</Typography>}
      {fields.map((f, i) => {
        const title = itemTitle || t('form.itemTitle', { index: i + 1 });
        const { summary, hasError } = rowState[i]!;
        const open = !keyed || rowsOpen.isOpen(f.id);
        return (
          <Paper key={f.id} variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" gap={1} alignItems="flex-start">
              <Box sx={{ flex: 1, minInlineSize: 0 }}>
                <Stack direction="row" alignItems="center" gap={1} sx={open ? { mb: 1 } : undefined}>
                  {keyed && summary !== '' && (
                    <IconButton
                      size="small"
                      aria-expanded={open}
                      aria-label={t(open ? 'form.collapseRow' : 'form.expandRow', { index: i + 1 })}
                      disabled={hasError}
                      onClick={() => rowsOpen.toggle(f.id)}
                    >
                      {open ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                    </IconButton>
                  )}
                  <Typography component="h4" variant="subtitle2">
                    {title}
                    {summary !== '' && (
                      <>
                        {' '}
                        <Box component="span" sx={{ color: 'text.secondary', fontWeight: 'normal' }}>
                          {keyIsIdentifier ? identifier(summary) : prose(summary)}
                        </Box>
                      </>
                    )}
                  </Typography>
                  {hasError && <ErrorOutlineIcon color="error" sx={{ fontSize: '1.25rem' }} titleAccess={t('form.rowHasErrors')} />}
                </Stack>
                {open && (
                  <SchemaField schema={items} name={joinPath(name, i)} propPath={propPath} parentName={joinPath(name, i)} label={title} required bare />
                )}
              </Box>
              <ListControls index={i} count={fields.length} readOnly={readOnly} onMove={move} onRemove={remove} />
            </Stack>
          </Paper>
        );
      })}
      {containerError && <FormHelperText error>{containerError}</FormHelperText>}
      <Button
        startIcon={<AddIcon />}
        disabled={readOnly || (schema.maxItems !== undefined && fields.length >= schema.maxItems)}
        onClick={() => {
          rowsOpen.openNext();
          append(toFormValue(items, defaultValueFor(items, ctx.root, FORM_DEFAULTS) ?? {}, ctx.root) as Record<string, unknown>);
        }}
        sx={{ alignSelf: 'flex-start' }}
      >
        {t('form.add')}
      </Button>
    </Stack>
  );
}

/**
 * `widget: 'rule-editor'`: an ordered list of objects (ACL rules …) as a table — one summary cell per column-worthy
 * member (`tableColumns`: itemKey members first; long text, secrets, nested lists and records left out) — with the
 * full row form opening beneath a row. Rows with errors are marked and forced open; new rows open.
 */
function RuleEditorField({ schema, name, propPath, label, hints, readOnly, items }: BoundFieldProps & { items: JsonSchema }) {
  const ctx = useSchemaFormContext();
  const { t } = useTranslation(UI_KIT_NS);
  const text = useSchemaText();
  const stext = useSummaryText();
  const { fields, append, remove, move } = useFieldArray({ name });
  const rows: unknown = useWatch({ name });
  const { errors } = useFormState({ name });
  const help = useHelpText(schema, hints, propPath);
  const containerError = useFieldError(name);
  const withErrors = fields.filter((_f, i) => getIn(errors, joinPath(name, i)) !== undefined).map((f) => f.id);
  const rowsOpen = useRowExpansion(
    fields.map((f) => f.id),
    withErrors,
  );
  const columns = useMemo(() => tableColumns(items, hints, ctx.root), [items, hints, ctx.root]);
  const itemTitle = text.itemTitle(propPath) ?? titleOf(items);
  const ltrColumns = useMemo(() => new Set(columns.filter((c) => isIdentifierSchema(c.schema, ctx.root)).map((c) => c.key)), [columns, ctx.root]);
  /** Identifier columns (prefixes, names) are LTR; words (enum labels, variants, yes/no) follow the page (review M2). */
  const cell = (c: TableColumn, v: unknown) => {
    const text = summarizeValue(c.schema, v, ctx.root, joinPath(propPath, c.key), stext);
    return ltrColumns.has(c.key) ? identifier(text) : text;
  };
  const headerOf = (c: TableColumn) =>
    c.key === '' ? itemTitle || t('form.value') : ctx.translateLabel(joinPath(propPath, c.key), text.title(joinPath(propPath, c.key), c.title));
  return (
    <Stack component="fieldset" gap={1} sx={FIELDSET_SX}>
      <Typography component="legend" variant="subtitle2" sx={{ px: 1 }}>
        {label}
      </Typography>
      {help && (
        <Typography variant="body2" color="text.secondary">
          {prose(help)}
        </Typography>
      )}
      {fields.length === 0 ? (
        <Typography color="text.secondary">{t('form.emptyList')}</Typography>
      ) : (
        // containerType: the open row's form measures the visible width (`100cqi`), not the (possibly wider) table.
        <TableContainer sx={{ overflowX: 'auto', containerType: 'inline-size' }}>
          <Table size="small" aria-label={label}>
            <TableHead>
              <TableRow>
                <TableCell sx={CELL}>#</TableCell>
                {columns.map((c) => (
                  <TableCell key={c.key} sx={{ ...CELL, verticalAlign: 'bottom' }}>
                    {headerOf(c)}
                  </TableCell>
                ))}
                <TableCell sx={{ ...CELL, ...STICKY_END, textAlign: 'end', verticalAlign: 'bottom' }}>{t('form.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {fields.map((f, i) => {
                const rowName = joinPath(name, i);
                const json = rowJson(items, rows, i, ctx.root);
                const hasError = withErrors.includes(f.id);
                const open = rowsOpen.isOpen(f.id);
                return (
                  <Fragment key={f.id}>
                    <TableRow hover selected={open}>
                      <TableCell sx={CELL}>{i + 1}</TableCell>
                      {columns.map((c) => (
                        <TableCell key={c.key} sx={CELL}>
                          {cell(c, c.key === '' ? json : json?.[c.key])}
                        </TableCell>
                      ))}
                      <TableCell sx={{ ...CELL, ...STICKY_END }}>
                        <Stack direction="row" alignItems="center" justifyContent="flex-end">
                          {hasError && <ErrorOutlineIcon color="error" sx={{ fontSize: '1.25rem' }} titleAccess={t('form.rowHasErrors')} />}
                          <IconButton
                            size="small"
                            aria-expanded={open}
                            aria-label={t(open ? 'form.collapseRow' : 'form.editRow', { index: i + 1 })}
                            disabled={hasError}
                            onClick={() => rowsOpen.toggle(f.id)}
                          >
                            {open ? <ExpandLessIcon /> : <EditIcon />}
                          </IconButton>
                          <ListControls dense index={i} count={fields.length} readOnly={readOnly} onMove={move} onRemove={remove} />
                        </Stack>
                      </TableCell>
                    </TableRow>
                    {open && (
                      <TableRow>
                        <TableCell colSpan={columns.length + 2} sx={{ bgcolor: 'action.hover', p: 0 }}>
                          {/* pinned to the visible part of a sideways-scrolled table, as wide as the container */}
                          <Box sx={{ position: 'sticky', insetInlineStart: 0, inlineSize: '100cqi', boxSizing: 'border-box', p: 2 }}>
                            <SchemaField
                              schema={items}
                              name={rowName}
                              propPath={propPath}
                              parentName={rowName}
                              label={itemTitle || t('form.itemTitle', { index: i + 1 })}
                              required
                              bare
                            />
                          </Box>
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      {containerError && <FormHelperText error>{containerError}</FormHelperText>}
      <Button
        startIcon={<AddIcon />}
        disabled={readOnly || (schema.maxItems !== undefined && fields.length >= schema.maxItems)}
        onClick={() => {
          rowsOpen.openNext();
          append(toFormValue(items, defaultValueFor(items, ctx.root, FORM_DEFAULTS) ?? {}, ctx.root) as Record<string, unknown>);
        }}
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
  const text = useSchemaText();
  const { setValue } = useFormContext();
  const current = useWatch({ name });
  const help = useHelpText(schema, hints, propPath);
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
    if (v) setValue(name, toFormValue(v, defaultValueFor(v, ctx.root, FORM_DEFAULTS), ctx.root), { shouldDirty: true });
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
        helperText={prose(error ?? help)}
        slotProps={{ input: { readOnly } }}
      >
        {variants.map((v, i) => {
          const d = discriminatorOf(v);
          return (
            <MenuItem key={String(i)} value={String(i)}>
              {text.variant(propPath, d ?? String(i), v.title ?? d ?? String(i + 1))}
            </MenuItem>
          );
        })}
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
