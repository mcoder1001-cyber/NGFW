import { serviceText } from '../../../product-text';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { useAaaTest, useMfaActivate, useMfaSetup, useMfaStatus } from './queries';
import { RecoveryCodes } from './RecoveryCodes';

const LTR = { dir: 'ltr', spellCheck: false } as const;
const OTP_INPUT = {
  dir: 'ltr',
  spellCheck: false,
  inputMode: 'numeric',
  autoComplete: 'one-time-code',
} as const;
/** Protocol names (not translated). */
const METHODS = [
  { id: 'radius', label: 'RADIUS' },
  { id: 'ldap', label: 'LDAP' },
] as const;

/** F-aaa-login: the caller's own TOTP factor — status, voluntary set-up (password step-up), activation. */
export function MyMfaCard() {
  const { t } = useTranslation('aaa');
  const status = useMfaStatus();
  const setup = useMfaSetup();
  const activate = useMfaActivate();
  const [current, setCurrent] = useState('');
  const [token, setToken] = useState('');
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<string[] | null>(null);

  const start = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await setup.mutateAsync({ current, token: token.trim() });
    } catch {
      // rendered from setup.error
    }
    setCurrent('');
  };
  const turnOn = async (e: FormEvent) => {
    e.preventDefault();
    try {
      const r = await activate.mutateAsync(code);
      setCodes(r.recoveryCodes);
      setup.reset();
    } catch {
      // rendered from activate.error
    }
    setCode('');
  };

  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 3, maxInlineSize: 720 }} data-testid="my-mfa">
      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {t('mine.title')}
      </Typography>
      {status.isError && <ProblemAlert error={status.error} />}
      {status.data && (
        <Stack gap={1} sx={{ mb: 2 }}>
          <Stack direction="row" gap={1} alignItems="center">
            <Chip
              size="small"
              color={status.data.enrolled ? 'success' : 'default'}
              label={status.data.enrolled ? t('mine.enrolled') : t('mine.notEnrolled')}
            />
            {status.data.required && (
              <Chip size="small" color="warning" variant="outlined" label={t('mine.required')} />
            )}
          </Stack>
          {status.data.enrolled && (
            <Typography variant="body2" color="text.secondary">
              {t('mine.left', { count: status.data.recoveryCodesLeft })}
            </Typography>
          )}
        </Stack>
      )}
      {codes && <RecoveryCodes codes={codes} onDone={() => setCodes(null)} />}
      {activate.isSuccess && !codes && <Alert severity="success">{t('mine.activated')}</Alert>}
      {status.data && !status.data.enrolled && !setup.data && (
        <Stack
          component="form"
          direction="row"
          gap={1}
          alignItems="flex-start"
          onSubmit={(e) => void start(e)}
        >
          <TextField
            size="small"
            type="password"
            label={t('mine.current')}
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            slotProps={{ htmlInput: LTR }}
          />
          <TextField
            size="small"
            label={t('step.token')}
            helperText={t('mine.tokenHelp')}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            autoComplete="off"
            slotProps={{ htmlInput: LTR }}
          />
          <Button
            type="submit"
            variant="contained"
            disabled={
              current === '' || !/^[A-Za-z0-9_-]{32}$/.test(token.trim()) || setup.isPending
            }
          >
            {t('mine.setup')}
          </Button>
        </Stack>
      )}
      {setup.isError && <ProblemAlert error={setup.error} sx={{ mt: 1 }} />}
      {setup.data && (
        <Stack component="form" gap={1} onSubmit={(e) => void turnOn(e)} data-testid="mfa-setup">
          <Typography variant="subtitle2">{t('step.secret')}</Typography>
          <Box
            component="code"
            dir="ltr"
            sx={{ wordBreak: 'break-all', fontFamily: (th) => th.ngfw.monoFontFamily }}
          >
            {setup.data.secret}
          </Box>
          <Typography variant="subtitle2">{t('step.uri')}</Typography>
          <Link href={setup.data.otpauthUri} dir="ltr" sx={{ wordBreak: 'break-all' }}>
            {setup.data.otpauthUri}
          </Link>
          <Alert severity="warning">{t('step.shownOnce')}</Alert>
          <Stack direction="row" gap={1} alignItems="flex-start">
            <TextField
              size="small"
              label={t('step.code')}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              slotProps={{ htmlInput: OTP_INPUT }}
            />
            <Button
              type="submit"
              variant="contained"
              disabled={!/^[0-9]{6}$/.test(code) || activate.isPending}
            >
              {t('mine.activate')}
            </Button>
          </Stack>
          {activate.isError && <ProblemAlert error={activate.error} />}
        </Stack>
      )}
      <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
        {t('mine.lost')}
      </Typography>
    </Paper>
  );
}

/** F-aaa-login: admin "test login" against one configured backend (no session is created). */
export function AaaTestPanel() {
  const { t } = useTranslation('aaa');
  const test = useAaaTest();
  const [method, setMethod] = useState<'radius' | 'ldap'>('radius');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const run = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await test.mutateAsync({ method, username: username.trim(), password });
    } catch {
      // rendered from test.error
    }
    setPassword('');
  };
  const r = test.data;
  return (
    <Paper variant="outlined" sx={{ p: 2, maxInlineSize: 720 }} data-testid="aaa-test">
      <Typography component="h3" variant="subtitle1">
        {t('test.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {t('test.intro')}
      </Typography>
      <Stack
        component="form"
        direction={{ xs: 'column', sm: 'row' }}
        gap={1}
        onSubmit={(e) => void run(e)}
      >
        <TextField
          select
          size="small"
          label={t('test.method')}
          value={method}
          onChange={(e) => setMethod(e.target.value as 'radius' | 'ldap')}
          sx={{ minInlineSize: 120 }}
        >
          {METHODS.map((m) => (
            <MenuItem key={m.id} value={m.id}>
              {m.label}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('test.username')}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="off"
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          size="small"
          type="password"
          label={t('test.password')}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          slotProps={{ htmlInput: LTR }}
        />
        <Button
          type="submit"
          variant="contained"
          disabled={test.isPending || username.trim() === '' || password === ''}
        >
          {test.isPending ? t('test.running') : t('test.run')}
        </Button>
      </Stack>
      {test.isError && <ProblemAlert error={test.error} sx={{ mt: 2 }} />}
      {r && (
        <Stack gap={1} sx={{ mt: 2 }} data-testid="aaa-test-result">
          <Stack direction="row" gap={1}>
            <Chip
              size="small"
              color={r.reachable ? 'success' : 'error'}
              label={r.reachable ? t('test.reachable') : t('test.unreachable')}
            />
            {r.reachable && (
              <Chip
                size="small"
                color={r.authenticated ? 'success' : 'warning'}
                label={r.authenticated ? t('test.accepted') : t('test.rejected')}
              />
            )}
          </Stack>
          {r.authenticated && (
            <>
              <Typography variant="body2">
                {t('test.groups')}: <span dir="ltr">{r.groups.join(', ') || '—'}</span>
              </Typography>
              <Typography variant="body2">
                {t('test.role')}: {r.role ?? t('test.noRole')}
              </Typography>
            </>
          )}
          <Typography variant="body2" color="text.secondary" dir="ltr">
            {serviceText(r.detail)}
          </Typography>
        </Stack>
      )}
    </Paper>
  );
}

/** F-aaa-login: `/system/aaa` — own second factor for everyone, backend test for admins. */
export function AaaPage() {
  const { t } = useTranslation('aaa');
  const perms = usePermissions();
  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>
      <MyMfaCard />
      {perms.role === 'admin' ? (
        <AaaTestPanel />
      ) : (
        <Alert severity="info" sx={{ maxInlineSize: 720 }}>
          {t('test.adminOnly')}
        </Alert>
      )}
    </Box>
  );
}
