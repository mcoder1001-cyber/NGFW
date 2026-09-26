import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Checkbox from '@mui/material/Checkbox';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import FormControlLabel from '@mui/material/FormControlLabel';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  attachmentForm,
  attachmentValue,
  esc,
  issuesUnder,
  QOS_SOURCES,
  type AttachmentCfg,
  type AttachmentForm,
} from './model';

const LTR = { dir: 'ltr' } as const;
const NUMERIC_LTR = { dir: 'ltr', inputMode: 'numeric' } as const;
const NONE = '';
const EGRESS_KINDS: readonly AttachmentForm['egress'][] = ['none', 'policer', 'shaper'];
const EGRESS_LABEL: Record<AttachmentForm['egress'], string> = {
  none: 'interfaces.none',
  policer: 'interfaces.egressPolicer',
  shaper: 'interfaces.egressShaper',
};

export interface AttachmentTarget {
  name: string;
  cfg: AttachmentCfg | undefined;
  editing: boolean;
}

/**
 * The interface attachment editor (`services.qos.interfaces.<if>`): ingress policer, egress policer or rate limit,
 * record, store and mark. Choices are the names of the candidate's policers, rate limits and maps and the schema's
 * QoS sources; the server's schema and semantic rules (400 with pointers) decide the rest and are shown per field.
 */
export function AttachmentDialog({
  target,
  existing,
  interfaceOptions,
  policers,
  shapers,
  maps,
  error,
  pending,
  onCancel,
  onSubmit,
}: {
  target: AttachmentTarget | null;
  existing: readonly string[];
  interfaceOptions: readonly string[];
  policers: readonly string[];
  shapers: readonly string[];
  maps: readonly string[];
  error: unknown;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (name: string, value: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation('qos-flat');
  return (
    <Dialog open={target !== null} onClose={onCancel} fullWidth maxWidth="md">
      <DialogTitle>
        {target?.editing
          ? t('interfaces.editTitle', { name: target.name })
          : t('interfaces.addTitle')}
      </DialogTitle>
      {target && (
        <Body
          key={target.name}
          target={target}
          existing={existing}
          interfaceOptions={interfaceOptions}
          policers={policers}
          shapers={shapers}
          maps={maps}
          error={error}
          pending={pending}
          onCancel={onCancel}
          onSubmit={onSubmit}
        />
      )}
    </Dialog>
  );
}

function Body({
  target,
  existing,
  interfaceOptions,
  policers,
  shapers,
  maps,
  error,
  pending,
  onCancel,
  onSubmit,
}: {
  target: AttachmentTarget;
  existing: readonly string[];
  interfaceOptions: readonly string[];
  policers: readonly string[];
  shapers: readonly string[];
  maps: readonly string[];
  error: unknown;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (name: string, value: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation('qos-flat');
  const [name, setName] = useState(target.name);
  const [f, setF] = useState<AttachmentForm>(() => attachmentForm(target.cfg));
  const set = <K extends keyof AttachmentForm>(k: K, v: AttachmentForm[K]) =>
    setF((cur) => ({ ...cur, [k]: v }));
  const taken = !target.editing && existing.includes(name);
  const prefix = `/services/qos/interfaces/${esc(name)}`;
  const issues = issuesUnder(error, prefix);
  /** Server message for one field (pointer relative to the attachment). */
  const at = (...rel: string[]) =>
    issues.find((e) => e.pointer === `${prefix}/${rel.join('/')}`)?.message;
  const whole = issues.filter((e) => e.pointer === prefix);
  const unmapped = error !== null && error !== undefined && issues.length === 0;
  // field setters and per-field server messages, prepared outside the JSX (i18next/no-literal-string)
  const on = {
    description: (v: string) => set('description', v),
    input: (v: string) => set('input', v),
    record: (v: string) => set('record', v),
    storeOn: (v: boolean) => set('storeOn', v),
    storeSource: (v: string) => set('storeSource', v),
    storeValue: (v: string) => set('storeValue', v),
    egress: (v: string) => set('egress', EGRESS_KINDS.find((k) => k === v) ?? 'none'),
    output: (v: string) => set('output', v),
    shaper: (v: string) => set('shaper', v),
    markOn: (v: boolean) => set('markOn', v),
    markMap: (v: string) => set('markMap', v),
    markOutput: (v: string) => set('markOutput', v),
  };
  const err = {
    input: at('policer', 'input'),
    output: at('policer', 'output'),
    shaper: at('shaper'),
    record: at('record'),
    storeSource: at('store', 'source'),
    storeValue: at('store', 'value'),
    markMap: at('mark', 'map'),
    markOutput: at('mark', 'output'),
  };
  const withCurrent = (opts: readonly string[], cur: string) =>
    cur && !opts.includes(cur) ? [...opts, cur] : opts;
  const nameOptions = withCurrent(interfaceOptions, name);

  const select = (
    label: string,
    value: string,
    options: readonly string[],
    onChange: (v: string) => void,
    opts: { none?: boolean; error?: string | undefined; helper?: string } = {},
  ) => (
    <TextField
      select
      label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      error={opts.error !== undefined}
      helperText={opts.error ?? opts.helper}
      sx={{ minInlineSize: 200, flex: 1 }}
    >
      {opts.none !== false && <MenuItem value={NONE}>{t('interfaces.none')}</MenuItem>}
      {options.map((o) => (
        <MenuItem key={o} value={o} dir="ltr">
          {o}
        </MenuItem>
      ))}
    </TextField>
  );
  const sourceLabel = (s: string) => t(`source.${s}`, { defaultValue: s });
  const sourceSelect = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    opts: { none?: boolean; error?: string | undefined; helper?: string } = {},
  ) => (
    <TextField
      select
      label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      error={opts.error !== undefined}
      helperText={opts.error ?? opts.helper}
      sx={{ minInlineSize: 200, flex: 1 }}
    >
      {opts.none !== false && <MenuItem value={NONE}>{t('interfaces.none')}</MenuItem>}
      {QOS_SOURCES.map((s) => (
        <MenuItem key={s} value={s}>
          {sourceLabel(s)}
        </MenuItem>
      ))}
    </TextField>
  );

  return (
    <>
      <DialogContent>
        <Stack gap={2} sx={{ pt: 1 }}>
          {target.editing ? (
            <TextField label={t('interfaces.interface')} value={name} disabled slotProps={{ htmlInput: LTR }} />
          ) : (
            select(t('interfaces.interface'), name, nameOptions, setName, {
              none: false,
              ...(taken ? { error: t('dialog.nameTaken', { name }) } : {}),
              helper: t('interfaces.interfaceHelp'),
            })
          )}
          <TextField
            label={t('interfaces.description')}
            value={f.description}
            onChange={(e) => on.description(e.target.value)}
          />
          <Typography variant="subtitle2">{t('interfaces.ingress')}</Typography>
          <Stack direction="row" gap={2} flexWrap="wrap">
            {select(t('interfaces.inputPolicer'), f.input, withCurrent(policers, f.input), on.input, {
              error: err.input,
            })}
            {sourceSelect(t('interfaces.record'), f.record, on.record, {
              error: err.record,
              helper: t('interfaces.recordHelp'),
            })}
          </Stack>
          <Stack direction="row" gap={2} flexWrap="wrap" alignItems="center">
            <FormControlLabel
              control={<Checkbox checked={f.storeOn} onChange={(e) => on.storeOn(e.target.checked)} />}
              label={t('interfaces.store')}
            />
            {f.storeOn && (
              <>
                {sourceSelect(t('interfaces.storeSource'), f.storeSource, on.storeSource, {
                  none: false,
                  error: err.storeSource,
                  helper: t('interfaces.storeHelp'),
                })}
                <TextField
                  label={t('interfaces.storeValue')}
                  value={f.storeValue}
                  onChange={(e) => on.storeValue(e.target.value)}
                  error={err.storeValue !== undefined}
                  helperText={err.storeValue}
                  slotProps={{ htmlInput: NUMERIC_LTR }}
                  sx={{ inlineSize: 160 }}
                />
              </>
            )}
          </Stack>
          <Typography variant="subtitle2">{t('interfaces.egress')}</Typography>
          <Stack direction="row" gap={2} flexWrap="wrap">
            <TextField
              select
              label={t('interfaces.egressKind')}
              value={f.egress}
              onChange={(e) => on.egress(e.target.value)}
              sx={{ minInlineSize: 200, flex: 1 }}
            >
              {EGRESS_KINDS.map((k) => (
                <MenuItem key={k} value={k}>
                  {t(EGRESS_LABEL[k])}
                </MenuItem>
              ))}
            </TextField>
            {f.egress === 'policer' &&
              select(t('interfaces.outputPolicer'), f.output, withCurrent(policers, f.output), on.output, {
                none: false,
                error: err.output,
              })}
            {f.egress === 'shaper' &&
              select(t('interfaces.shaper'), f.shaper, withCurrent(shapers, f.shaper), on.shaper, {
                none: false,
                error: err.shaper,
              })}
          </Stack>
          <Stack direction="row" gap={2} flexWrap="wrap" alignItems="center">
            <FormControlLabel
              control={<Checkbox checked={f.markOn} onChange={(e) => on.markOn(e.target.checked)} />}
              label={t('interfaces.mark')}
            />
            {f.markOn && (
              <>
                {select(t('interfaces.markMap'), f.markMap, withCurrent(maps, f.markMap), on.markMap, {
                  none: false,
                  error: err.markMap,
                })}
                {sourceSelect(t('interfaces.markOutput'), f.markOutput, on.markOutput, {
                  none: false,
                  error: err.markOutput,
                })}
              </>
            )}
          </Stack>
          <Alert severity="info">{t('interfaces.writeOnlyNote')}</Alert>
          {whole.map((e) => (
            <Alert key={e.message} severity="error">
              {e.message}
            </Alert>
          ))}
          {issues
            .filter((e) => e.pointer !== prefix && !isShown(e.pointer, prefix))
            .map((e) => (
              <Alert key={`${e.pointer}:${e.message}`} severity="error" dir="ltr">
                {e.pointer} — {e.message}
              </Alert>
            ))}
          {unmapped && <ProblemAlert error={error} />}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel}>{t('dialog.cancel')}</Button>
        <Button
          variant="contained"
          disabled={name === '' || taken || pending}
          onClick={() => onSubmit(name, attachmentValue(f))}
        >
          {t('dialog.save')}
        </Button>
      </DialogActions>
    </>
  );
}

/** Pointers the dialog shows next to a field. */
const FIELD_POINTERS = ['policer/input', 'policer/output', 'shaper', 'record', 'store/source', 'store/value', 'mark/map', 'mark/output'];

function isShown(pointer: string, prefix: string): boolean {
  return FIELD_POINTERS.some((p) => pointer === `${prefix}/${p}`);
}
