import Autocomplete from '@mui/material/Autocomplete';
import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import FormHelperText from '@mui/material/FormHelperText';
import FormLabel from '@mui/material/FormLabel';
import InputAdornment from '@mui/material/InputAdornment';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import { useTheme } from '@mui/material/styles';
import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
import { prose } from './bidi.js';
import type { PrimitiveInputProps } from './inputs.js';

/**
 * Widgets for string values with structure (`x-vrx-ui.widget` from packages/schema): port-range, ip-range, time,
 * datetime, timezone-picker, color. They edit the **same string** the schema validates (the schema's pattern stays
 * the only rule), are fully controlled, and render their values LTR inside RTL pages (RTL-1).
 */

const LTR = { dir: 'ltr' } as const;

function stringOf(value: unknown): string {
  return typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value);
}

// ─── port-range / ip-range ─────────────────────────────────────────────────

/** `"8000-8080"` → `['8000', '8080']`, `"443"` → `['443', '']` (split at the first `-`). */
export function splitRange(text: string): [string, string] {
  const i = text.indexOf('-');
  return i < 0 ? [text, ''] : [text.slice(0, i), text.slice(i + 1)];
}

/** Inverse of `splitRange`; an empty end means a single value, both empty means absent. */
export function joinRange(start: string, end: string): string | undefined {
  if (start === '' && end === '') return undefined;
  return end === '' ? start : `${start}-${end}`;
}

const RANGE_CHARS = { port: /[^0-9]/g, ip: /[^0-9A-Fa-f.:]/g } as const;

/** Range kinds (named, so JSX never carries a literal). */
export const RANGE_KIND = { port: 'port', ip: 'ip' } as const;

/** Start/end inputs composing `"<start>-<end>"` (or one value). `kind` limits the characters each end accepts. */
export function RangeInput(props: PrimitiveInputProps & { kind: 'port' | 'ip' }) {
  const { kind, value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText } = props;
  const theme = useTheme();
  const { t } = useTranslation(UI_KIT_NS);
  const [start, end] = splitRange(stringOf(value));
  const clean = (text: string) => text.replace(RANGE_CHARS[kind], '');
  /**
   * Typed or pasted into one end. A whole range (`8000-8080`, both parts present) pasted into either end fills both
   * (review L2); a lone `-` typed at either end is dropped and never clears the other end (verify F1).
   */
  const typed = (text: string, other: string, atStart: boolean): string | undefined => {
    if (text.includes('-')) {
      const [a, b] = splitRange(text).map(clean) as [string, string];
      if (a !== '' && b !== '') return joinRange(a, b);
    }
    return atStart ? joinRange(clean(text), other) : joinRange(other, clean(text));
  };
  const input = {
    readOnly,
    sx: { fontFamily: theme.vrx.monoFontFamily },
  };
  const html = {
    ...LTR,
    spellCheck: false,
    ...(kind === 'port' ? { inputMode: 'numeric' as const } : {}),
    'aria-readonly': readOnly || undefined,
  };
  return (
    <FormControl component="fieldset" fullWidth error={error !== undefined} required={required}>
      <FormLabel component="legend" sx={{ mb: 1 }}>
        {label}
      </FormLabel>
      <Stack direction="row" gap={1} alignItems="flex-start">
        <TextField
          sx={{ flex: 1 }}
          label={t('form.rangeStart')}
          value={start}
          inputRef={inputRef}
          onChange={(e) => onChange(typed(e.target.value, end, true))}
          onBlur={onBlur}
          error={error !== undefined}
          slotProps={{ input, htmlInput: html }}
        />
        <TextField
          sx={{ flex: 1 }}
          label={t('form.rangeEnd')}
          value={end}
          placeholder={start}
          onChange={(e) => onChange(typed(e.target.value, start, false))}
          onBlur={onBlur}
          error={error !== undefined}
          slotProps={{ input, htmlInput: html }}
        />
      </Stack>
      {(error ?? helperText) && <FormHelperText>{prose(error ?? helperText)}</FormHelperText>}
    </FormControl>
  );
}

// ─── time (HH:MM) ──────────────────────────────────────────────────────────

export function TimeInput(props: PrimitiveInputProps) {
  const { value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText } = props;
  return (
    <TextField
      fullWidth
      type="time"
      label={label}
      required={required}
      value={stringOf(value)}
      onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
      onBlur={onBlur}
      inputRef={inputRef}
      error={error !== undefined}
      helperText={prose(error ?? helperText)}
      slotProps={{
        inputLabel: { shrink: true },
        input: { readOnly },
        htmlInput: { ...LTR, step: 60, 'aria-readonly': readOnly || undefined },
      }}
    />
  );
}

// ─── datetime (RFC 3339 with offset) ───────────────────────────────────────

const DATE_TIME = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})(?::(\d{2}))?(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

export interface ParsedDateTime {
  /** `YYYY-MM-DDTHH:MM` or `…:SS` when the seconds are not zero — what `<input type="datetime-local">` shows. */
  local: string;
  /** Fractional seconds as stored (`.25`), or `''`; kept as long as the date and time are not edited. */
  frac: string;
  /** `Z` or `±HH:MM`. */
  offset: string;
}

export function parseDateTime(text: string): ParsedDateTime | undefined {
  const m = DATE_TIME.exec(text);
  if (!m) return undefined;
  const seconds = m[2] ?? '00';
  return { local: seconds === '00' ? m[1]! : `${m[1]}:${seconds}`, frac: m[3] ?? '', offset: m[4]! };
}

/** `datetime-local` value + offset → `YYYY-MM-DDTHH:MM:SS[.f]±HH:MM` (seconds are required by `z.iso.datetime`). */
export function joinDateTime(local: string, offset: string, frac = ''): string | undefined {
  if (local === '') return undefined;
  return `${local.length === 16 ? `${local}:00` : local}${frac}${offset}`;
}

/** The browser's UTC offset at `date` as `±HH:MM`. */
export function localOffset(date: Date): string {
  const minutes = -date.getTimezoneOffset();
  const abs = Math.abs(minutes);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${minutes < 0 ? '-' : '+'}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`;
}

/** Offset for a new value: the browser's offset **at the chosen date and time** (daylight saving), or now without one. */
export function defaultOffsetFor(local: string): string {
  const at = local === '' ? new Date() : new Date(local);
  return localOffset(Number.isNaN(at.getTime()) ? new Date() : at);
}

interface DateTimeDraft {
  local: string;
  frac: string;
  /** What the user typed or what was stored; `undefined` = automatic (`defaultOffsetFor` the chosen date). */
  offset: string | undefined;
}

function draftOf(p: ParsedDateTime | undefined): DateTimeDraft {
  return p ? { local: p.local, frac: p.frac, offset: p.offset } : { local: '', frac: '', offset: undefined };
}

/**
 * Date and time with an explicit UTC offset (`2026-09-24T18:00:00+03:30`). The offset is its own input, so a stored value
 * keeps its offset exactly; a new value takes the browser's offset at the chosen date. Both inputs keep what the user is
 * typing: an incomplete offset (`+04:`, or none) is emitted as is, stays in these inputs and fails the schema on
 * blur/submit (review M1). Only a value this widget did not produce and cannot read falls back to plain text.
 */
export function DateTimeInput(props: PrimitiveInputProps) {
  const { value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText } = props;
  const theme = useTheme();
  const { t } = useTranslation(UI_KIT_NS);
  const text = stringOf(value);
  const parsed = parseDateTime(text);
  const emitted = useRef<string | undefined>(undefined);
  const [draft, setDraft] = useState<DateTimeDraft>(() => draftOf(parsed));
  useEffect(() => {
    if (emitted.current !== undefined && text === emitted.current) return; // our own value coming back
    const p = parseDateTime(text);
    if (p || text === '') setDraft((d) => (p ? draftOf(p) : { ...d, local: '', frac: '' })); // loaded, reset or cleared
  }, [text]);
  const mine = emitted.current !== undefined && text === emitted.current;
  const unparsable = text !== '' && parsed === undefined && !mine;
  const emit = (next: DateTimeDraft) => {
    setDraft(next);
    const v = joinDateTime(next.local, next.offset ?? defaultOffsetFor(next.local), next.frac);
    emitted.current = v;
    onChange(v);
  };
  const mono = { fontFamily: theme.vrx.monoFontFamily };
  return (
    <Stack direction="row" gap={1} alignItems="flex-start">
      <TextField
        sx={{ flex: 1 }}
        type={unparsable ? 'text' : 'datetime-local'}
        label={label}
        required={required}
        value={unparsable ? text : draft.local}
        onChange={(e) => {
          if (unparsable) onChange(e.target.value || undefined);
          else emit({ ...draft, local: e.target.value, frac: '' });
        }}
        onBlur={onBlur}
        inputRef={inputRef}
        error={error !== undefined}
        helperText={prose(error ?? helperText)}
        slotProps={{
          inputLabel: { shrink: true },
          input: { readOnly, sx: mono },
          htmlInput: { ...LTR, step: draft.local.length > 16 ? 1 : 60, 'aria-readonly': readOnly || undefined },
        }}
      />
      {!unparsable && (
        <TextField
          sx={{ inlineSize: 128, flexShrink: 0 }}
          label={t('form.utcOffset')}
          value={draft.offset ?? defaultOffsetFor(draft.local)}
          onChange={(e) => emit({ ...draft, offset: e.target.value.replace(/[^0-9:+\-Z]/g, '') })}
          onBlur={onBlur}
          error={error !== undefined}
          slotProps={{
            input: { readOnly, sx: mono },
            htmlInput: { ...LTR, spellCheck: false, maxLength: 6, 'aria-readonly': readOnly || undefined },
          }}
        />
      )}
    </Stack>
  );
}

// ─── suggestions with free text (interface-picker, timezone-picker) ───────

/** Free text with suggestions: the value is whatever is typed; `options` only help (the schema validates). */
export function SuggestInput(props: PrimitiveInputProps & { suggestions: readonly string[]; defaultHelp?: string | undefined }) {
  const { value, onChange, onBlur, label, required, readOnly, error, helperText, suggestions, defaultHelp } = props;
  const theme = useTheme();
  const current = typeof value === 'string' ? value : '';
  return (
    <Autocomplete
      freeSolo
      autoSelect
      options={[...suggestions]}
      value={current}
      readOnly={readOnly}
      onChange={(_e, v) => onChange(typeof v === 'string' && v !== '' ? v : undefined)}
      onInputChange={(_e, text, reason) => {
        if (reason === 'input' || reason === 'clear') onChange(text === '' ? undefined : text);
      }}
      onBlur={onBlur}
      renderInput={(params) => (
        <TextField
          {...params}
          label={label}
          required={required}
          error={error !== undefined}
          helperText={prose(error ?? helperText ?? defaultHelp)}
          slotProps={{
            input: { ...params.InputProps, sx: { fontFamily: theme.vrx.monoFontFamily } },
            htmlInput: { ...params.inputProps, spellCheck: false, ...LTR },
          }}
        />
      )}
    />
  );
}

let zoneCache: readonly string[] | undefined;

/** IANA zones known to this browser, `UTC` first (ICU's list omits it). */
export function timeZoneNames(): readonly string[] {
  if (!zoneCache) {
    const intl = Intl as { supportedValuesOf?: (key: string) => string[] };
    const zones = typeof intl.supportedValuesOf === 'function' ? intl.supportedValuesOf('timeZone') : [];
    zoneCache = ['UTC', ...zones.filter((z) => z !== 'UTC')];
  }
  return zoneCache;
}

export function TimeZoneInput(props: PrimitiveInputProps) {
  return <SuggestInput {...props} suggestions={timeZoneNames()} />;
}

// ─── color (#rrggbb) ───────────────────────────────────────────────────────

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/;

/** Hex text (what is stored) with a native colour swatch as its start adornment. */
export function ColorInput(props: PrimitiveInputProps) {
  const { value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText } = props;
  const theme = useTheme();
  const { t } = useTranslation(UI_KIT_NS);
  const text = stringOf(value);
  return (
    <TextField
      fullWidth
      label={label}
      required={required}
      value={text}
      onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
      onBlur={onBlur}
      inputRef={inputRef}
      error={error !== undefined}
      helperText={prose(error ?? helperText)}
      slotProps={{
        input: {
          readOnly,
          sx: { fontFamily: theme.vrx.monoFontFamily },
          startAdornment: (
            <InputAdornment position="start">
              <Box
                component="input"
                type="color"
                aria-label={t('form.pickColor')}
                value={HEX_COLOR.test(text) ? text.toLowerCase() : '#000000'}
                disabled={readOnly}
                onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
                sx={{ inlineSize: 28, blockSize: 28, p: 0, border: 0, bgcolor: 'transparent', cursor: readOnly ? 'default' : 'pointer' }}
              />
            </InputAdornment>
          ),
        },
        htmlInput: { ...LTR, spellCheck: false, maxLength: 7, 'aria-readonly': readOnly || undefined },
      }}
    />
  );
}
