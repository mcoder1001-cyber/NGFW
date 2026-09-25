import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import { createMergePatch, localizeSchema } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { NAT_POINTER, pickSchema, pickValue } from './model';
import { useCandidateNat, useFreshCandidateNat, useNatSummary, usePatchNat } from './queries';
import { NAT_NS } from './tabs';

/** Outbound NAT44 settings edited by the form (`enabled` has its own tri-state control, see below). */
const FORM_KEYS = [
  'mode',
  'inside',
  'outside',
  'outputFeature',
  'insideVrf',
  'outsideVrf',
  'forwarding',
  'sessionLimit',
  'timeouts',
] as const;

type Enabled = 'auto' | 'on' | 'off';
const ENABLED: readonly Enabled[] = ['auto', 'on', 'off'];

function enabledOf(nat: Record<string, unknown> | undefined): Enabled {
  if (nat?.['enabled'] === true) return 'on';
  if (nat?.['enabled'] === false) return 'off';
  return 'auto';
}

/** Live status line of NAT44-ED from the agent (NatSummary). */
export function NatStatus() {
  const { t } = useTranslation(NAT_NS);
  const fmt = useFormatters();
  const summary = useNatSummary();
  if (summary.isPending) return <LinearProgress aria-label={t('loading')} />;
  if (summary.isError) return <ProblemAlert error={summary.error} sx={{ mb: 1 }} />;
  const s = summary.data;
  return (
    <Stack
      direction="row"
      gap={1}
      flexWrap="wrap"
      sx={{ mb: 2 }}
      role="status"
      aria-label={t('status.label')}
      data-testid="nat-status"
    >
      <Chip
        size="small"
        color={s.enabled ? 'success' : 'default'}
        label={s.enabled ? t('status.enabled') : t('status.disabled')}
      />
      {s.enabled && (
        <Chip
          size="small"
          variant="outlined"
          label={t('status.sessionLimit', { limit: fmt.integer(s.sessionLimit) })}
        />
      )}
      <Chip
        size="small"
        variant="outlined"
        label={t('status.totals', {
          sessions: fmt.integer(s.totalSessions),
          users: fmt.integer(s.totalUsers),
        })}
      />
      {s.truncated && <Chip size="small" color="warning" label={t('status.truncated')} />}
    </Stack>
  );
}

export function OutboundTab() {
  const { t } = useTranslation(NAT_NS);
  const perms = usePermissions();
  const candidate = useCandidateNat();
  const fresh = useFreshCandidateNat();
  const patch = usePatchNat();
  const ifs = useCandidateInterfaces();
  const [saved, setSaved] = useState(false);
  const schema = useMemo(() => localizeSchema(pickSchema(FORM_KEYS), (k, o) => t(k, o ?? {})), [t]);
  const interfaceOptions = useMemo(() => Object.keys(ifs.data ?? {}).sort(), [ifs.data]);
  // The form edits the value it opened with (P08 review N4): Save sends only the edits against it.
  const [opened, setOpened] = useState<Record<string, unknown> | null>(null);
  if (candidate.isSuccess && opened === null) setOpened(pickValue(candidate.data, FORM_KEYS));
  const readOnly = !perms.editConfig;

  const save = async (value: unknown) => {
    setSaved(false);
    const base = opened ?? {};
    const cleaned = value; // WEB-1: SchemaForm keeps absent optionals absent (no phantom to drop)
    const body = createMergePatch(base, cleaned) as Record<string, unknown>;
    try {
      await patch.mutateAsync(body);
      setOpened(cleaned as Record<string, unknown>);
      setSaved(true);
    } catch {
      // shown from patch.error
    }
  };

  const setEnabled = async (v: Enabled) => {
    setSaved(false);
    const current = enabledOf(await fresh());
    if (current === v) return;
    try {
      await patch.mutateAsync({ enabled: v === 'auto' ? null : v === 'on' });
      setSaved(true);
    } catch {
      // shown from patch.error
    }
  };

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('outbound.intro')}
      </Typography>
      <NatStatus />
      {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} sx={{ mb: 1 }} />}
      {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {saved && !patch.isError && (
        <Alert severity="success" sx={{ mb: 1 }}>
          {t('saved')}
        </Alert>
      )}
      {candidate.isSuccess && (
        <>
          <TextField
            select
            label={t('field.enabled.title')}
            helperText={t('field.enabled.help')}
            value={enabledOf(candidate.data)}
            disabled={readOnly || patch.isPending}
            onChange={(e) => void setEnabled(e.target.value as Enabled)}
            sx={{ mb: 2, minInlineSize: 280 }}
          >
            {ENABLED.map((v) => (
              <MenuItem key={v} value={v}>
                {t(`outbound.enabled.${v}`)}
              </MenuItem>
            ))}
          </TextField>
          {opened !== null && (
            <SchemaForm
              schema={schema}
              value={opened}
              readOnly={readOnly}
              interfaceOptions={interfaceOptions}
              submitLabel={t('save')}
              resetLabel={t('reset')}
              problem={problemFor(patch.error, NAT_POINTER)}
              onSubmit={save}
            />
          )}
        </>
      )}
    </Box>
  );
}
