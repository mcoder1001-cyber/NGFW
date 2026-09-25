import VisibilityIcon from '@mui/icons-material/Visibility';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff';
import Autocomplete from '@mui/material/Autocomplete';
import Checkbox from '@mui/material/Checkbox';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import FormHelperText from '@mui/material/FormHelperText';
import FormLabel from '@mui/material/FormLabel';
import IconButton from '@mui/material/IconButton';
import InputAdornment from '@mui/material/InputAdornment';
import MenuItem from '@mui/material/MenuItem';
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';
import Slider from '@mui/material/Slider';
import Switch from '@mui/material/Switch';
import TextField from '@mui/material/TextField';
import { useTheme } from '@mui/material/styles';
import { useEffect, useId, useState, type Ref } from 'react';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../../i18n/index.js';
import { IDENTIFIER_WIDGETS, isLtrString, typeOf } from '../schema-utils.js';
import type { JsonSchema, UiHints } from '../types.js';
import { prose } from './bidi.js';
import { ColorInput, DateTimeInput, RANGE_KIND, RangeInput, SuggestInput, TimeInput, TimeZoneInput } from './widgets.js';

/**
 * Presentational, fully controlled inputs. `PrimitiveInput` picks the control from the schema type and the
 * `x-vrx-ui.widget` hint; the react-hook-form binding lives in `SchemaField.tsx` (and array rows reuse these
 * directly so a primitive list needs only one registered field).
 */
export interface PrimitiveInputProps {
  schema: JsonSchema;
  hints: UiHints;
  widget: string | undefined;
  value: unknown;
  onChange: (value: unknown) => void;
  onBlur?: (() => void) | undefined;
  inputRef?: Ref<HTMLInputElement> | undefined;
  label: string;
  required: boolean;
  readOnly: boolean;
  error?: string | undefined;
  helperText?: string | undefined;
  /** Choices for `interface-picker` (free text stays allowed). */
  options?: readonly string[] | undefined;
}

/**
 * Technical values (IP/CIDR/MAC/interface names/JSON, identifier widgets, identifier formats, ASCII-only patterns) stay
 * LTR in RTL locales so bidi never reorders them (review L7, RTL-1); free text follows the page direction.
 */
const LTR = { dir: 'ltr' } as const;
const NUMERIC = { inputMode: { integer: 'numeric', decimal: 'decimal' }, anyStep: 'any' } as const;

export function inferStringWidget(schema: JsonSchema): string {
  switch (schema.format) {
    case 'ipv4':
    case 'ipv6':
      return 'ip';
    case 'cidrv4':
    case 'cidrv6':
      return 'cidr';
    case 'password':
      return 'password';
    default:
      break;
  }
  if (schema.writeOnly) return 'password';
  // Long prose gets a textarea; a long identifier (hostname: 253, ASCII-only pattern) stays a one-line LTR input.
  if ((schema.maxLength ?? 0) > 200 && !isLtrString(schema, undefined)) return 'textarea';
  return 'text';
}

function enumLabel(v: unknown, hints: UiHints): string {
  const labels = hints.enumLabels;
  if (labels && typeof labels === 'object') {
    const l = (labels as Record<string, unknown>)[String(v)];
    if (typeof l === 'string') return l;
  }
  return v === null ? '—' : String(v);
}

export function PrimitiveInput(props: PrimitiveInputProps) {
  const { schema, widget } = props;
  if (schema.const !== undefined) return <ConstInput {...props} />;
  if (schema.enum) return widget === 'radio' ? <RadioInput {...props} /> : <SelectInput {...props} />;
  const t = typeOf(schema);
  if (t === 'boolean') return <BooleanInput {...props} />;
  if (t === 'number' || t === 'integer') {
    if (widget === 'slider' && schema.minimum !== undefined && schema.maximum !== undefined) {
      return <SliderInput {...props} />;
    }
    return <NumberInput {...props} />;
  }
  if (t === 'string') {
    const w = widget ?? inferStringWidget(schema);
    switch (w) {
      case 'interface-picker':
        return <InterfacePickerInput {...props} />;
      case 'port-range':
        return <RangeInput {...props} kind={RANGE_KIND.port} />;
      case 'ip-range':
        return <RangeInput {...props} kind={RANGE_KIND.ip} />;
      case 'time':
        return <TimeInput {...props} />;
      case 'datetime':
        return <DateTimeInput {...props} />;
      case 'timezone':
      case 'timezone-picker':
        return <TimeZoneInput {...props} />;
      case 'color':
        return <ColorInput {...props} />;
      default:
        return <TextInput {...props} widget={w} />;
    }
  }
  return <JsonInput {...props} />;
}

function TextInput(props: PrimitiveInputProps) {
  const { widget, value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText, hints, schema } =
    props;
  const theme = useTheme();
  const { t } = useTranslation(UI_KIT_NS);
  const [show, setShow] = useState(false);
  const isPassword = widget === 'password';
  const mono = widget !== undefined && IDENTIFIER_WIDGETS.has(widget);
  const ltr = mono || isLtrString(schema, widget);
  const placeholder = typeof hints.placeholder === 'string' ? hints.placeholder : undefined;
  return (
    <TextField
      fullWidth
      label={label}
      required={required}
      value={typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value)}
      onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
      onBlur={onBlur}
      inputRef={inputRef}
      error={error !== undefined}
      helperText={prose(error ?? helperText)}
      multiline={widget === 'textarea'}
      minRows={widget === 'textarea' ? 3 : undefined}
      type={isPassword && !show ? 'password' : 'text'}
      autoComplete={isPassword ? 'new-password' : 'off'}
      slotProps={{
        input: {
          readOnly,
          ...(mono ? { sx: { fontFamily: theme.vrx.monoFontFamily } } : {}),
          ...(isPassword
            ? {
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label={t(show ? 'form.hidePassword' : 'form.showPassword')}
                      onClick={() => setShow((s) => !s)}
                      edge="end"
                    >
                      {show ? <VisibilityOffIcon /> : <VisibilityIcon />}
                    </IconButton>
                  </InputAdornment>
                ),
              }
            : {}),
        },
        htmlInput: {
          ...(placeholder ? { placeholder } : {}),
          ...(schema.maxLength !== undefined ? { maxLength: schema.maxLength } : {}),
          ...(ltr && !isPassword ? { spellCheck: false } : {}),
          ...(ltr ? LTR : {}),
          'aria-readonly': readOnly || undefined,
        },
      }}
    />
  );
}

function InterfacePickerInput(props: PrimitiveInputProps) {
  const { t } = useTranslation(UI_KIT_NS);
  return <SuggestInput {...props} suggestions={props.options ?? []} defaultHelp={t('form.interfacePickerHelp')} />;
}

function toNumberValue(raw: string): unknown {
  if (raw.trim() === '') return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : raw;
}

function NumberInput(props: PrimitiveInputProps) {
  const { schema, value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText, hints } = props;
  const integer = typeOf(schema) === 'integer';
  const placeholder = typeof hints.placeholder === 'string' ? hints.placeholder : undefined;
  return (
    <TextField
      fullWidth
      label={label}
      required={required}
      type="number"
      value={value === undefined || value === null ? '' : String(value)}
      onChange={(e) => onChange(toNumberValue(e.target.value))}
      onBlur={onBlur}
      inputRef={inputRef}
      error={error !== undefined}
      helperText={prose(error ?? helperText)}
      slotProps={{
        input: { readOnly },
        htmlInput: {
          inputMode: integer ? NUMERIC.inputMode.integer : NUMERIC.inputMode.decimal,
          step: schema.multipleOf ?? (integer ? 1 : NUMERIC.anyStep),
          ...(schema.minimum !== undefined ? { min: schema.minimum } : {}),
          ...(schema.maximum !== undefined ? { max: schema.maximum } : {}),
          ...(placeholder ? { placeholder } : {}),
          'aria-readonly': readOnly || undefined,
        },
      }}
    />
  );
}

function SliderInput(props: PrimitiveInputProps) {
  const { schema, value, onChange, onBlur, label, required, readOnly, error, helperText } = props;
  const id = useId();
  const min = schema.minimum ?? 0;
  const max = schema.maximum ?? 100;
  const num = typeof value === 'number' ? value : min;
  return (
    <FormControl fullWidth error={error !== undefined} required={required} disabled={readOnly}>
      <FormLabel id={id}>{label}</FormLabel>
      <Slider
        aria-labelledby={id}
        value={num}
        min={min}
        max={max}
        step={schema.multipleOf ?? (typeOf(schema) === 'integer' ? 1 : undefined)}
        marks={[
          { value: min, label: String(min) },
          { value: max, label: String(max) },
        ]}
        valueLabelDisplay="auto"
        onChange={(_e, v) => onChange(Array.isArray(v) ? v[0] : v)}
        onBlur={onBlur}
        disabled={readOnly}
        sx={{ mx: 1 }}
      />
      {(error ?? helperText) && <FormHelperText>{prose(error ?? helperText)}</FormHelperText>}
    </FormControl>
  );
}

function BooleanInput(props: PrimitiveInputProps) {
  const { widget, value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText } = props;
  const checked = value === true;
  const control =
    widget === 'checkbox' ? (
      <Checkbox
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        onBlur={onBlur}
        inputRef={inputRef}
        required={required}
      />
    ) : (
      <Switch
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        onBlur={onBlur}
        inputRef={inputRef}
        required={required}
      />
    );
  return (
    <FormControl error={error !== undefined} disabled={readOnly}>
      <FormControlLabel control={control} label={label} />
      {(error ?? helperText) && <FormHelperText>{prose(error ?? helperText)}</FormHelperText>}
    </FormControl>
  );
}

function SelectInput(props: PrimitiveInputProps) {
  const { schema, hints, value, onChange, onBlur, inputRef, label, required, readOnly, error, helperText } = props;
  const { t } = useTranslation(UI_KIT_NS);
  const values = schema.enum ?? [];
  const idx = values.findIndex((v) => v === value);
  return (
    <TextField
      select
      fullWidth
      label={label}
      required={required}
      value={idx < 0 ? '' : String(idx)}
      onChange={(e) => onChange(e.target.value === '' ? undefined : values[Number(e.target.value)])}
      onBlur={onBlur}
      inputRef={inputRef}
      error={error !== undefined}
      helperText={prose(error ?? helperText)}
      slotProps={{ input: { readOnly } }}
    >
      {!required && (
        <MenuItem value="">
          <em>{t('form.none')}</em>
        </MenuItem>
      )}
      {values.map((v, i) => (
        <MenuItem key={String(i)} value={String(i)}>
          {enumLabel(v, hints)}
        </MenuItem>
      ))}
    </TextField>
  );
}

function RadioInput(props: PrimitiveInputProps) {
  const { schema, hints, value, onChange, onBlur, label, required, readOnly, error, helperText } = props;
  const id = useId();
  const values = schema.enum ?? [];
  const idx = values.findIndex((v) => v === value);
  return (
    <FormControl component="fieldset" error={error !== undefined} required={required} disabled={readOnly}>
      <FormLabel component="legend" id={id}>
        {label}
      </FormLabel>
      <RadioGroup
        row
        aria-labelledby={id}
        value={idx < 0 ? '' : String(idx)}
        onChange={(e) => onChange(values[Number(e.target.value)])}
        onBlur={onBlur}
      >
        {values.map((v, i) => (
          <FormControlLabel key={String(i)} value={String(i)} control={<Radio />} label={enumLabel(v, hints)} />
        ))}
      </RadioGroup>
      {(error ?? helperText) && <FormHelperText>{prose(error ?? helperText)}</FormHelperText>}
    </FormControl>
  );
}

function ConstInput(props: PrimitiveInputProps) {
  const { schema, label, helperText, error } = props;
  return (
    <TextField
      fullWidth
      label={label}
      value={typeof schema.const === 'string' ? schema.const : JSON.stringify(schema.const)}
      error={error !== undefined}
      helperText={prose(error ?? helperText)}
      slotProps={{ input: { readOnly: true }, htmlInput: { 'aria-readonly': true } }}
    />
  );
}

/** Free-form JSON for `{}` schemas (placeholders, opaque blobs). Commits on blur when the text parses. */
function JsonInput(props: PrimitiveInputProps) {
  const { value, onChange, onBlur, label, required, readOnly, error, helperText } = props;
  const theme = useTheme();
  const { t } = useTranslation(UI_KIT_NS);
  const external = value === undefined ? '' : JSON.stringify(value, null, 2);
  const [text, setText] = useState(external);
  const [parseError, setParseError] = useState<string | undefined>(undefined);
  const [focused, setFocused] = useState(false);
  useEffect(() => {
    if (!focused) {
      setText(external);
      setParseError(undefined);
    }
  }, [external, focused]);
  const commit = () => {
    setFocused(false);
    if (text.trim() === '') {
      onChange(undefined);
      setParseError(undefined);
    } else {
      try {
        onChange(JSON.parse(text));
        setParseError(undefined);
      } catch {
        setParseError(t('form.invalidJson'));
      }
    }
    onBlur?.();
  };
  const shown = parseError ?? error;
  return (
    <TextField
      fullWidth
      multiline
      minRows={3}
      label={label}
      required={required}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onFocus={() => setFocused(true)}
      onBlur={commit}
      error={shown !== undefined}
      helperText={prose(shown ?? helperText ?? t('form.jsonValue'))}
      slotProps={{
        input: { readOnly, sx: { fontFamily: theme.vrx.monoFontFamily } },
        htmlInput: { spellCheck: false, ...LTR, 'aria-readonly': readOnly || undefined },
      }}
    />
  );
}

/** Array of strings as chips (`widget: 'chips'`). */
export function ChipsInput(props: PrimitiveInputProps & { ltr?: boolean | undefined }) {
  const { value, onChange, onBlur, label, required, readOnly, error, helperText, ltr = false } = props;
  const theme = useTheme();
  const items = Array.isArray(value) ? value.map(String) : [];
  return (
    <Autocomplete
      multiple
      freeSolo
      autoSelect
      options={[]}
      value={items}
      readOnly={readOnly}
      onChange={(_e, v) => onChange(v.map(String))}
      onBlur={onBlur}
      // Identifier chips (prefixes, names) are isolated LTR so `2001:db8::/64` never reorders in an RTL page (RTL-1).
      {...(ltr
        ? {
            renderValue: (vals: readonly string[], getItemProps: (a: { index: number }) => Record<string, unknown> & { key: number }) =>
              vals.map((v, index) => {
                const { key, ...itemProps } = getItemProps({ index });
                return <Chip key={key} {...itemProps} label={<bdi dir="ltr">{v}</bdi>} />;
              }),
          }
        : {})}
      renderInput={(params) => (
        <TextField
          {...params}
          label={label}
          required={required && items.length === 0}
          error={error !== undefined}
          helperText={prose(error ?? helperText)}
          slotProps={{
            input: { ...params.InputProps, sx: { fontFamily: theme.vrx.monoFontFamily } },
            htmlInput: { ...params.inputProps, spellCheck: false, ...(ltr ? LTR : {}) },
          }}
        />
      )}
    />
  );
}

/** Array of enum values (`uniqueItems` or `widget: 'multiselect'`). */
export function MultiSelectInput(props: PrimitiveInputProps & { choices: unknown[] }) {
  const { choices, hints, value, onChange, onBlur, label, required, readOnly, error, helperText } = props;
  const selected = Array.isArray(value) ? value : [];
  return (
    <Autocomplete
      multiple
      disableCloseOnSelect
      options={choices}
      value={selected}
      readOnly={readOnly}
      getOptionLabel={(o) => enumLabel(o, hints)}
      isOptionEqualToValue={(a, b) => a === b}
      onChange={(_e, v) => onChange(v)}
      onBlur={onBlur}
      renderInput={(params) => (
        <TextField
          {...params}
          label={label}
          required={required && selected.length === 0}
          error={error !== undefined}
          helperText={prose(error ?? helperText)}
        />
      )}
    />
  );
}
