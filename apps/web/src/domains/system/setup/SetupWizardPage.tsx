import {
  Alert,
  Button,
  Checkbox,
  FormControlLabel,
  MenuItem,
  Stack,
  Step,
  StepLabel,
  Stepper,
  TextField,
  Typography,
} from '@mui/material';
import { RootConfig, SetupInputSchema, parentInterfaceName, type SetupInput } from '@ngfw/schema';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { api, authMiddleware } from '../../../api';
import { call } from '../../../api-problem';
import { useAuth, usePermissions } from '../../../auth/AuthProvider';
import { fetchInterfacesState, ifaceKeys } from '../../interfaces/queries';
import { useUiSettings } from '../../../settings/UiSettings';

const STEP_KEYS = ['time', 'password', 'identity', 'wan', 'lan', 'defaults', 'summary'];
const WAN_MODES = ['dhcp', 'static'];
const KEY = {
  time: 'time',
  password: 'password',
  identity: 'identity',
  wan: 'wan',
  lan: 'lan',
  defaults: 'defaults',
  summary: 'summary',
  rerun: 'rerun',
  language: 'language',
  timezone: 'timezone',
  ntp: 'ntp',
  hostname: 'hostname',
  wanMode: 'wanMode',
  dhcp: 'dhcp',
  static: 'static',
  wanAddress: 'wanAddress',
  wanGateway: 'wanGateway',
  lanAddress: 'lanAddress',
} as const;
const CONFIRM_PATH = '/api/v1/config/commit/confirm';
const LOGIN_PATH = '/login';
const EN = 'en';
const FA = 'fa';

export function SetupWizardPage() {
  const { t } = useTranslation('setup');
  const { session } = useAuth();
  const permissions = usePermissions();
  const settings = useUiSettings();
  const navigate = useNavigate();
  const cache = useQueryClient();
  const running = useQuery({
    queryKey: ['config', 'setup-running'],
    queryFn: async () => {
      const r = await call(api.GET('/api/v1/config'));
      return {
        doc: RootConfig.parse(r.data),
        revision: Number(r.response.headers.get('x-ngfw-revision') ?? 0),
      };
    },
  });
  const [values, setValues] = useState<Partial<SetupInput>>({
    language: settings.settings.lang,
    timezone: 'UTC',
    ntp: ['pool.ntp.org'],
    hostname: 'ngfw',
    wanMode: 'dhcp',
    lanAddress: '192.168.40.1/24',
    dhcp: true,
    rerun: false,
  });
  const [step, setStep] = useState(0);
  const interfaces = useQuery({
    queryKey: ifaceKeys.state,
    queryFn: ({ signal }) => fetchInterfacesState(signal),
    enabled: step === 3 || step === 4,
  });
  const interfacesUnavailable =
    interfaces.isError || (interfaces.data?.observationErrors?.length ?? 0) > 0;
  const interfaceNames = [
    ...new Set([
      ...Object.entries(running.data?.doc.interfaces ?? {})
        .filter(([name, config]) => name !== 'local0' && config.physical?.owner !== 'host')
        .map(([name]) => name),
      ...(interfaces.data?.items ?? [])
        .filter(
          (item) =>
            item.kind === 'interface' &&
            item.state !== null &&
            (running.data?.doc.interfaces[item.name] !== undefined ||
              (!item.state.managed &&
                item.state.vrf === 'default' &&
                ['dpdk', 'vmxnet3', 'virtio'].includes(item.state.type))) &&
            item.name !== 'local0' &&
            item.physical?.owner !== 'host' &&
            parentInterfaceName.safeParse(item.name).success &&
            running.data?.doc.interfaces[item.name]?.physical?.owner !== 'host',
        )
        .map((item) => item.name),
    ]),
  ].sort();
  const [current, setCurrent] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<unknown>();
  const [at, setAt] = useState('');
  const [deadline, setDeadline] = useState('');
  const set = <K extends keyof SetupInput>(key: K, value: SetupInput[K]) => {
    setValues((v) => ({ ...v, [key]: value }));
    setPreview(undefined);
  };
  async function request(path: 'preview' | 'stage', body: unknown): Promise<{ changes?: unknown }> {
    const response = await authMiddleware(session).onRequest({
      request: new Request(`${location.origin}/api/v1/config/setup/${path}`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
      }),
    });
    const data = (await response.json()) as { changes?: unknown; detail?: string };
    if (!response.ok) throw new Error(data.detail ?? t('failed'));
    return data;
  }
  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : t('failed'));
    } finally {
      setBusy(false);
    }
  };
  async function next() {
    await run(async () => {
      const fields: (keyof SetupInput)[][] = [
        ['language', 'timezone', 'ntp'],
        [],
        ['hostname'],
        ['wan', 'wanMode'],
        ['lan', 'lanAddress', 'dhcp'],
        [],
      ];
      if ((step === 3 || step === 4) && !values[step === 3 ? 'wan' : 'lan'])
        throw new Error(t('selectInterface'));
      for (const key of fields[step] ?? []) SetupInputSchema.shape[key].parse(values[key]);
      if (step === 1 && (password.length < 12 || current === '' || current === password))
        throw new Error(t('passwordPolicy'));
      if (step === 5) {
        const input = SetupInputSchema.parse(values);
        const completedAt = new Date().toISOString();
        const r = await request('preview', {
          input,
          completedAt,
          baseRevision: running.data!.revision,
        });
        setAt(completedAt);
        setPreview(r.changes);
      }
      setStep((s) => s + 1);
    });
  }
  async function commit() {
    await run(async () => {
      await request('stage', {
        input: SetupInputSchema.parse(values),
        completedAt: at,
        baseRevision: running.data!.revision,
        current,
        password,
      });
      setCurrent('');
      setPassword('');
      const validated = await call(api.POST('/api/v1/config/validate'));
      if (!validated.data.ok || validated.data.notApplied.length > 0) throw new Error(t('failed'));
      const r = await call(
        api.POST('/api/v1/config/commit', {
          params: { query: { confirm: 120, comment: 'First-boot setup' } },
        }),
      );
      if (r.data.status !== 'pending' || !r.data.confirmDeadline) throw new Error(t('failed'));
      setDeadline(r.data.confirmDeadline);
      settings.update({ lang: values.language ?? 'en' });
      await cache.invalidateQueries({ queryKey: ['config'] });
    });
  }
  const field = (key: keyof SetupInput, label = key) => (
    <TextField
      key={key}
      label={t(label)}
      value={String(values[key] ?? '')}
      onChange={(e) => set(key, e.target.value as never)}
      fullWidth
    />
  );
  if (!permissions.manageUsers) return <Alert severity="error">{t('adminOnly')}</Alert>;
  if (!running.data) return <Typography>{running.isError ? t('failed') : t('loading')}</Typography>;
  return (
    <Stack spacing={2} sx={{ maxWidth: 850 }}>
      <Typography variant="h4">{t('title')}</Typography>
      <Stepper activeStep={step} alternativeLabel>
        {STEP_KEYS.map((s) => (
          <Step key={s}>
            <StepLabel>{t(s)}</StepLabel>
          </Step>
        ))}
      </Stepper>
      {error && <Alert severity="error">{error}</Alert>}
      {running.data.doc.system.setup.completed && (
        <FormControlLabel
          control={
            <Checkbox
              checked={values.rerun ?? false}
              onChange={(e) => set(KEY.rerun, e.target.checked)}
            />
          }
          label={t('rerunWarning')}
        />
      )}
      {step === 0 && (
        <>
          {
            <TextField
              select
              label={t(KEY.language)}
              value={values.language}
              onChange={(e) => {
                set(KEY.language, e.target.value as 'en' | 'fa');
                settings.update({ lang: e.target.value as 'en' | 'fa' });
              }}
            >
              <MenuItem value={EN}>{t('english')}</MenuItem>
              <MenuItem value={FA}>{t('persian')}</MenuItem>
            </TextField>
          }
          {field(KEY.timezone)}
          <TextField
            label={t(KEY.ntp)}
            value={values.ntp?.join(', ')}
            onChange={(e) =>
              set(
                KEY.ntp,
                e.target.value
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              )
            }
          />
        </>
      )}
      {step === 1 && (
        <>
          <TextField
            type="password"
            autoComplete="current-password"
            label={t('current')}
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
          <TextField
            type="password"
            autoComplete="new-password"
            label={t('newPassword')}
            helperText={t('passwordPolicy')}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </>
      )}
      {step === 2 && field(KEY.hostname)}
      {(step === 3 || step === 4) && (
        <TextField
          select
          label={t(step === 3 ? KEY.wan : KEY.lan)}
          value={values[step === 3 ? KEY.wan : KEY.lan] ?? ''}
          onChange={(e) => set(step === 3 ? KEY.wan : KEY.lan, e.target.value)}
        >
          {interfaceNames
            .filter((name) => name !== values[step === 3 ? 'lan' : 'wan'])
            .map((name) => (
              <MenuItem key={name} value={name}>
                {name}
              </MenuItem>
            ))}
        </TextField>
      )}
      {(step === 3 || step === 4) && interfaces.isFetching && (
        <Typography>{t('loadingInterfaces')}</Typography>
      )}
      {(step === 3 || step === 4) && interfacesUnavailable && (
        <Alert severity="warning">
          {t('interfacesFailed')}
          <Button onClick={() => void interfaces.refetch()}>{t('retryInterfaces')}</Button>
        </Alert>
      )}
      {(step === 3 || step === 4) && !interfaces.isFetching && interfaceNames.length === 0 && (
        <Alert severity="warning">{t('noInterfaces')}</Alert>
      )}
      {step === 3 && (
        <>
          <TextField
            select
            label={t(KEY.wanMode)}
            value={values.wanMode}
            onChange={(e) => set(KEY.wanMode, e.target.value as SetupInput['wanMode'])}
          >
            {WAN_MODES.map((mode) => (
              <MenuItem key={mode} value={mode}>
                {t(mode)}
              </MenuItem>
            ))}
          </TextField>
          {values.wanMode === KEY.static && (
            <>
              {field(KEY.wanAddress)}
              {field(KEY.wanGateway)}
            </>
          )}
          <Alert severity="info">{t('pppoeHelp')}</Alert>
        </>
      )}
      {step === 4 && (
        <>
          {field(KEY.lanAddress)}
          <FormControlLabel
            control={
              <Checkbox
                checked={values.dhcp ?? false}
                onChange={(e) => set(KEY.dhcp, e.target.checked)}
              />
            }
            label={t('dhcpServer')}
          />
        </>
      )}
      {step === 5 && <Alert severity="warning">{t('defaultsHelp')}</Alert>}
      {step === 6 && (
        <>
          <Typography>{t('summaryHelp')}</Typography>
          <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
            {JSON.stringify(preview, null, 2)}
          </pre>
        </>
      )}
      {deadline ? (
        <>
          <Alert severity="warning">{t('confirmHelp', { deadline })}</Alert>
          <Button
            onClick={() =>
              void run(async () => {
                await call(api.POST(CONFIRM_PATH));
                await cache.invalidateQueries();
                navigate(LOGIN_PATH);
              })
            }
            disabled={busy}
          >
            {t('confirm')}
          </Button>
        </>
      ) : (
        <Stack direction="row" spacing={2}>
          <Button
            disabled={step === 0 || busy}
            onClick={() => {
              setStep((s) => s - 1);
              setPreview(undefined);
            }}
          >
            {t('back')}
          </Button>
          {step < 6 ? (
            <Button disabled={busy} onClick={() => void next()}>
              {t('next')}
            </Button>
          ) : (
            <Button disabled={busy || !preview} onClick={() => void commit()}>
              {t('commit')}
            </Button>
          )}
        </Stack>
      )}
    </Stack>
  );
}
