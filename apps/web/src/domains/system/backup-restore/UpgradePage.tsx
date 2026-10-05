import type { paths } from '@ngfw/api-client';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  LinearProgress,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { usePermissions } from '../../../auth/AuthProvider';
import { ApiError } from '../../../api-problem';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { Card } from './BackupRestorePage';
import { json, request } from './transport';

const OPS = {
  stage: 'stage',
  activate: 'activate',
  confirm: 'confirm',
  rollback: 'rollback',
} as const;
const UPDATE_INPUT = { accept: '.tar' };
type UpgradeRequest = paths['/api/v1/actions/upgrade']['post']['requestBody']['content']['application/json'];
type Op = Exclude<UpgradeRequest['op'], 'status'>;
interface Result {
  lines: string[];
  done: { summary: string; exitCode: number; stats: Record<string, string> };
}
interface Status {
  active_slot: 'A' | 'B';
  default_slot: string;
  versions: Record<string, string | null>;
  pending_slot: string | null;
  staged_slot: string | null;
  confirmed: boolean;
}
export function parseStatus(result: Result): Status {
  if (result.done.exitCode !== 0) throw new ApiError(502, { detail: result.done.summary }, false);
  const value = JSON.parse(result.lines[0] ?? '') as Partial<Status>;
  if (
    (value.active_slot !== 'A' && value.active_slot !== 'B') ||
    typeof value.versions !== 'object' ||
    value.versions === null ||
    typeof value.confirmed !== 'boolean'
  )
    throw new ApiError(502, { title: 'invalid-upgrade-status' }, false);
  const slot = (v: unknown) => v === 'A' || v === 'B';
  if (
    !slot(value.default_slot) ||
    !(value.pending_slot === null || slot(value.pending_slot)) ||
    !(value.staged_slot === null || slot(value.staged_slot)) ||
    !['A', 'B'].every(
      (key) => value.versions?.[key] === null || typeof value.versions?.[key] === 'string',
    )
  )
    throw new ApiError(502, { title: 'invalid-upgrade-status' }, false);
  return value as Status;
}
export function UpgradePage() {
  const { t } = useTranslation('backup-restore');
  const perms = usePermissions();
  return (
    <Box>
      <PageHeader title={t('upgrade')} />
      {perms.role === 'admin' ? (
        <UpgradeControls />
      ) : (
        <Alert severity="info">{t('adminOnly')}</Alert>
      )}
    </Box>
  );
}
function UpgradeControls() {
  const { t } = useTranslation('backup-restore');
  const state = useQuery({
    queryKey: ['state', 'upgrade'],
    queryFn: async () => parseStatus(await json<Result>('/actions/upgrade', { op: 'status' })),
    retry: false,
    refetchInterval: 30_000,
  });
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [result, setResult] = useState<Result | null>(null);
  const [confirm, setConfirm] = useState<Op | null>(null);
  const data = state.isError ? undefined : state.data;
  const otherSlot = data?.active_slot === 'A' ? 'B' : 'A';
  const action = async (op: Op) => {
    setConfirm(null);
    setBusy(true);
    setError(undefined);
    setResult(null);
    try {
      let bundle: string | undefined;
      if (op === 'stage') {
        if (
          !file ||
          !/^ngfw-update-\d+\.\d+\.\d+\.tar$/.test(file.name) ||
          file.size === 0 ||
          file.size > 8 * 1024 ** 3
        )
          throw new ApiError(400, { detail: t('updateHelp') }, false);
        const uploaded = await request('/actions/upgrade-upload', {
          method: 'POST',
          headers: { 'content-type': 'application/vnd.ngfw.update', 'x-ngfw-filename': file.name },
          body: file,
        });
        bundle = ((await uploaded.json()) as { bundle: string }).bundle;
      }
      const response = await json<Result>('/actions/upgrade', {
        op,
        ...(bundle ? { bundle } : {}),
      });
      if (response.done.exitCode !== 0)
        throw new ApiError(502, { detail: response.done.summary }, false);
      setResult(response);
      await state.refetch();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Stack gap={3} sx={{ maxInlineSize: 900 }}>
      {state.isError && <ProblemAlert error={state.error} />}
      {error !== undefined && <ProblemAlert error={error} />}
      <Card title={t('slots')}>
        <Button disabled={busy || state.isFetching} onClick={() => void state.refetch()}>
          {t('refresh')}
        </Button>
        {data && (
          <>
            <Typography>
              {t('slotSummary', {
                label: t('activeSlot'),
                slot: data.active_slot,
                version: data.versions[data.active_slot] ?? t('unknown'),
              })}
            </Typography>
            <Typography>
              {t('slotSummary', {
                label: t('otherSlot'),
                slot: otherSlot,
                version: data.versions[otherSlot] ?? t('unknown'),
              })}
            </Typography>
            <Typography>
              {t('defaultSlot')}: {data.default_slot}
            </Typography>
            <Typography>
              {t('stagedSlot')}: {data.staged_slot ?? t('none')}
            </Typography>
            <Typography>
              {t('pendingSlot')}: {data.pending_slot ?? t('none')}
            </Typography>
            <Alert severity={data.confirmed ? 'success' : 'warning'}>
              {data.confirmed ? t('confirmed') : t('trial')}
            </Alert>
          </>
        )}
      </Card>
      <Card title={t('stage')}>
        <TextField
          type="file"
          label={t('updateFile')}
          slotProps={{ inputLabel: { shrink: true }, htmlInput: UPDATE_INPUT }}
          onChange={(e) => setFile((e.target as HTMLInputElement).files?.[0] ?? null)}
          helperText={t('updateHelp')}
        />
        <Button
          variant="contained"
          disabled={
            busy ||
            !file ||
            !/^ngfw-update-\d+\.\d+\.\d+\.tar$/.test(file.name) ||
            file.size === 0 ||
            file.size > 8 * 1024 ** 3 ||
            !data ||
            Boolean(data.pending_slot)
          }
          onClick={() => void action(OPS.stage)}
        >
          {t('uploadStage')}
        </Button>
        {busy && (
          <>
            <LinearProgress />
            <Typography>{t('working')}</Typography>
          </>
        )}
      </Card>
      <Card title={t('rollout')}>
        <Typography>{t('rolloutHelp')}</Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
          <Button
            disabled={busy || !data?.staged_slot || Boolean(data.pending_slot)}
            onClick={() => setConfirm(OPS.activate)}
          >
            {t('activate')}
          </Button>
          <Button
            disabled={busy || !data?.pending_slot || data.active_slot !== data.pending_slot}
            onClick={() => setConfirm(OPS.confirm)}
          >
            {t('confirm')}
          </Button>
          <Button
            color="warning"
            disabled={busy || !data?.pending_slot}
            onClick={() => setConfirm(OPS.rollback)}
          >
            {t('rollback')}
          </Button>
        </Stack>
        {result && <Alert severity="success">{result.done.summary}</Alert>}
      </Card>
      <Dialog open={confirm !== null} onClose={() => setConfirm(null)}>
        <DialogTitle>{confirm ? t(confirm) : ''}</DialogTitle>
        <DialogContent>
          <Typography>{confirm ? t(`${confirm}Warning`) : ''}</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirm(null)}>{t('cancel')}</Button>
          <Button
            onClick={() => {
              if (confirm) void action(confirm);
            }}
          >
            {t('proceed')}
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
