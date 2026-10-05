import type { paths } from '@ngfw/api-client';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Box, Button, MenuItem, Paper, Stack, TextField, Typography } from '@mui/material';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { usePermissions } from '../../../auth/AuthProvider';
import { ApiError } from '../../../api-problem';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { DiffView, type DiffChange } from '../../../config/DiffView';
import { invalidateConfig } from '../../../config/queries';
import { PageHeader } from '../../../shell/PageHeader';
import { domainSchemas } from '../../../schema/registry';
import { archiveBase64, download, json } from './transport';

const ENDPOINTS = {
  backup: '/actions/backup',
  restore: '/actions/restore',
  schedule: '/config/management/backup',
  support: '/actions/support-bundle',
};
const PUT = 'PUT';
const BACKUP_NAME = 'ngfw-backup.ngfwbackup';
const SUPPORT_NAME = 'ngfw-support.json';
const ARCHIVE_INPUT = { accept: '.ngfwbackup' };
const LTR = { dir: 'ltr', spellCheck: false } as const;
const SUCCESS = 'success';
const SCHEDULE_LABELS: Record<string, string> = {
  schedule: 'cron',
  type: 'targetType',
  hostKeySha256: 'hostKey',
};
type Template =
  paths['/api/v1/config-templates/{name}']['put']['requestBody']['content']['application/json'];
interface Stage {
  staged: true;
  diff: { baseRevision: number | null; changes: DiffChange[] };
}
export function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
        {title}
      </Typography>
      <Stack gap={2}>{children}</Stack>
    </Paper>
  );
}
export function BackupRestorePage() {
  const { t } = useTranslation('backup-restore');
  const permissions = usePermissions();
  return (
    <Box>
      <PageHeader title={t('title')} />
      {permissions.role === 'admin' ? (
        <BackupControls />
      ) : (
        <Alert severity="info">{t('adminOnly')}</Alert>
      )}
    </Box>
  );
}
function BackupControls() {
  const fmt = useFormatters();
  const { t } = useTranslation('backup-restore');
  const qc = useQueryClient();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [revisions, setRevisions] = useState(100);
  const [archive, setArchive] = useState<File | null>(null);
  const [changes, setChanges] = useState<DiffChange[] | null>(null);
  const templates = useQuery({
    queryKey: ['config', 'backup-templates'],
    queryFn: () => json<{ items: Record<string, Template> }>('/config-templates'),
    retry: false,
  });
  const runs = useQuery({
    queryKey: ['state', 'backup'],
    queryFn: () =>
      json<{ runs: { at: string; result: string; filename?: string; error?: string }[] }>(
        '/state/backup',
      ),
    retry: false,
    refetchInterval: 30_000,
  });
  const schedule = useQuery({
    queryKey: ['config', 'candidate', 'management/backup'],
    queryFn: async () => {
      try {
        return await json<unknown>('/config/candidate/management/backup');
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return {};
        throw e;
      }
    },
    retry: false,
  });
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [parameterDefinition, setParameterDefinition] = useState('{}');
  const [patchText, setPatchText] = useState('{}');
  const selected = templates.data?.items[name];
  const action = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(undefined);
    setMessage('');
    try {
      await fn();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const staged = async (result?: Stage) => {
    if (result) setChanges(result.diff.changes);
    await invalidateConfig(qc);
    setMessage(t('staged'));
  };
  const templateSchema: JsonSchema = {
    type: 'object',
    additionalProperties: false,
    properties: Object.fromEntries(
      Object.entries(selected?.parameters ?? {}).map(([key, def]) => [
        key,
        { type: def.type, title: key },
      ]),
    ),
    required: Object.entries(selected?.parameters ?? {})
      .filter(([, def]) => def.required !== false)
      .map(([key]) => key),
  };
  const scheduleSchema = domainSchemas.management.properties?.backup;
  return (
    <Stack gap={3} sx={{ maxInlineSize: 1000 }}>
      {error !== undefined && <ProblemAlert error={error} />}
      {message && <Alert severity="success">{message}</Alert>}
      <Card title={t('backup')}>
        <TextField
          type="password"
          label={t('passphrase')}
          value={passphrase}
          onChange={(e) => setPassphrase(e.target.value)}
          autoComplete="new-password"
          helperText={t('passphraseHelp')}
        />
        <TextField
          type="number"
          label={t('revisions')}
          value={revisions}
          onChange={(e) => setRevisions(Number(e.target.value))}
          slotProps={{ htmlInput: { min: 1, max: 1000 } }}
        />
        <Button
          variant="contained"
          disabled={
            busy ||
            passphrase.length < 12 ||
            !Number.isInteger(revisions) ||
            revisions < 1 ||
            revisions > 1000
          }
          onClick={() =>
            void action(async () => {
              const secret = passphrase;
              setPassphrase('');
              await download(ENDPOINTS.backup, { passphrase: secret, revisions }, BACKUP_NAME);
            })
          }
        >
          {t('download')}
        </Button>
        <Typography>{t('restoreHelp')}</Typography>
        <TextField
          type="file"
          label={t('archive')}
          slotProps={{ inputLabel: { shrink: true }, htmlInput: ARCHIVE_INPUT }}
          onChange={(e) => setArchive((e.target as HTMLInputElement).files?.[0] ?? null)}
          helperText={t('archiveLimit')}
        />
        <Button
          disabled={busy || !archive || archive.size > 32 * 1024 * 1024 || passphrase.length < 12}
          onClick={() =>
            void action(async () => {
              if (!archive) return;
              const secret = passphrase;
              setPassphrase('');
              await staged(
                await json<Stage>(ENDPOINTS.restore, {
                  archive: await archiveBase64(archive),
                  passphrase: secret,
                }),
              );
            })
          }
        >
          {t('restore')}
        </Button>
        {changes !== null && (
          <>
            <Alert severity="info">{t('commitHelp')}</Alert>
            <DiffView changes={changes} />
          </>
        )}
      </Card>
      <Card title={t('schedule')}>
        {schedule.isPending && <Typography role="status">{t('loading')}</Typography>}
        {schedule.isError && <ProblemAlert error={schedule.error} />}
        {schedule.isSuccess && scheduleSchema && (
          <SchemaForm
            schema={scheduleSchema}
            translateLabel={(path, fallback) => {
              const key = path.split('.').pop() ?? '';
              return t(SCHEDULE_LABELS[key] ?? key, { defaultValue: fallback });
            }}
            value={schedule.data}
            readOnly={busy}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            onSubmit={(value) =>
              action(async () => {
                await json(ENDPOINTS.schedule, value, PUT);
                await staged();
              })
            }
          />
        )}
        {runs.isPending && <Typography role="status">{t('loading')}</Typography>}
        {runs.isSuccess && runs.data.runs.length === 0 && <Typography>{t('noRuns')}</Typography>}
        {runs.isError && <ProblemAlert error={runs.error} />}
        {runs.data?.runs.map((run, i) => (
          <Typography key={`${run.at}-${i}`} color={run.result === SUCCESS ? undefined : 'error'}>
            {t('runSummary', {
              at: fmt.dateTime(run.at),
              result: run.result === SUCCESS ? t('success') : t('failure'),
              detail: run.filename ?? run.error ?? '',
            })}
          </Typography>
        ))}
      </Card>
      <Card title={t('templates')}>
        {templates.isPending && <Typography role="status">{t('loading')}</Typography>}
        {templates.isSuccess && Object.keys(templates.data.items).length === 0 && (
          <Typography>{t('noTemplates')}</Typography>
        )}
        {templates.isError && <ProblemAlert error={templates.error} />}
        <TextField
          select
          label={t('selectTemplate')}
          value={selected ? name : ''}
          onChange={(e) => {
            setName(e.target.value);
            const item = templates.data?.items[e.target.value];
            if (item) {
              setDescription(item.description ?? '');
              setParameterDefinition(JSON.stringify(item.parameters ?? {}, null, 2));
              setPatchText(JSON.stringify(item.patch, null, 2));
            } else {
              setDescription('');
              setParameterDefinition('{}');
              setPatchText('{}');
            }
          }}
        >
          <MenuItem value="">{t('newTemplate')}</MenuItem>
          {Object.keys(templates.data?.items ?? {}).map((key) => (
            <MenuItem key={key} value={key}>
              {key}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          label={t('name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          label={t('description')}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <TextField
          multiline
          minRows={3}
          label={t('parameterDefinition')}
          value={parameterDefinition}
          onChange={(e) => setParameterDefinition(e.target.value)}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          multiline
          minRows={4}
          label={t('patch')}
          value={patchText}
          onChange={(e) => setPatchText(e.target.value)}
          slotProps={{ htmlInput: LTR }}
        />
        <Button
          disabled={busy || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(name)}
          onClick={() =>
            void action(async () => {
              let parameters: unknown;
              let patch: unknown;
              try {
                parameters = JSON.parse(parameterDefinition);
                patch = JSON.parse(patchText);
              } catch {
                throw new ApiError(400, { detail: t('invalidJson') }, false);
              }
              await json(
                `/config-templates/${encodeURIComponent(name)}`,
                { description, parameters, patch },
                PUT,
              );
              await staged();
            })
          }
        >
          {t('saveTemplate')}
        </Button>
        {selected && (
          <>
            <Typography>{selected.description}</Typography>
            <SchemaForm
              key={name}
              schema={templateSchema}
              readOnly={busy}
              submitLabel={t('apply')}
              resetLabel={t('reset')}
              onSubmit={(parameters) =>
                action(async () => {
                  await staged(
                    await json<Stage>(`/config-templates/${encodeURIComponent(name)}/apply`, {
                      parameters,
                    }),
                  );
                })
              }
            />
          </>
        )}
      </Card>
      <Card title={t('support')}>
        <Typography>{t('supportHelp')}</Typography>
        <Button
          disabled={busy}
          onClick={() => void action(() => download(ENDPOINTS.support, {}, SUPPORT_NAME))}
        >
          {t('downloadSupport')}
        </Button>
      </Card>
    </Stack>
  );
}
