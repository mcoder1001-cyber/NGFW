import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, useNavigate, useSearchParams } from 'react-router';
import { useAuth } from '../auth/AuthProvider';
import type { LoginFailure } from '../auth/session';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, isLanguage } from '../i18n-config';
import { useUiSettings } from '../settings/UiSettings';

const LTR = { dir: 'ltr' } as const;
const USERNAME_INPUT = { dir: 'ltr', spellCheck: false, autoCapitalize: 'none' } as const;

/** Only same-app paths are accepted as `next` (no open redirect to another origin). */
export function safeNext(next: string | null): string {
  // review L5: no `\` (URL parsing turns it into `/`), no control characters, and the result must stay same-origin
  // eslint-disable-next-line no-control-regex -- rejecting control characters is the point
  if (!next || !next.startsWith('/') || next.startsWith('//') || /[\\\u0000-\u001f\u007f]/.test(next)) return '/';
  try {
    const base = globalThis.location?.origin ?? 'http://localhost';
    const u = new URL(next, base);
    if (u.origin !== new URL(base).origin || u.pathname.startsWith('/login')) return '/';
    return `${u.pathname}${u.search}${u.hash}`;
  } catch {
    return '/';
  }
}

function failureKey(f: LoginFailure): string {
  if (f.status === 0) return 'error.unreachable';
  if (f.status === 401) return 'error.invalid';
  if (f.status === 429) return 'error.rateLimited';
  if (f.status === 400) return 'error.badRequest';
  return 'error.other';
}

/** Username/password → P06 session (access token in memory, refresh cookie). Language can be switched before login. */
export function LoginPage() {
  const { t } = useTranslation('auth');
  const { session, state } = useAuth();
  const { settings, update } = useUiSettings();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<LoginFailure | null>(null);
  const next = safeNext(params.get('next'));

  if (state.status === 'authenticated') return <Navigate to={next} replace />;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setFailure(null);
    const f = await session.login(username.trim(), password);
    setBusy(false);
    if (f) {
      setFailure(f);
      setPassword('');
      return;
    }
    void navigate(next, { replace: true });
  };

  return (
    <Box component="main" id="main" sx={{ display: 'grid', placeItems: 'center', minBlockSize: '100vh', p: 2, bgcolor: 'background.default' }}>
      <Card sx={{ inlineSize: '100%', maxInlineSize: 400 }} variant="outlined">
        <CardContent>
          <Stack component="form" gap={2} onSubmit={(e) => void submit(e)} noValidate aria-labelledby="login-title">
            <Box>
              <Typography id="login-title" component="h1" variant="h5">
                {t('title')}
              </Typography>
              <Typography color="text.secondary">{t('subtitle')}</Typography>
            </Box>
            {state.endReason === 'expired' && !failure && <Alert severity="warning">{t('sessionExpired')}</Alert>}
            {state.endReason === 'signedOut' && !failure && <Alert severity="info">{t('signedOut')}</Alert>}
            {failure && (
              <Alert severity="error" role="alert">
                {t(failureKey(failure))}
                {failure.status === 429 && failure.detail ? ` (${failure.detail})` : ''}
              </Alert>
            )}
            <TextField
              label={t('username')}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
              slotProps={{ htmlInput: USERNAME_INPUT }}
            />
            <TextField
              label={t('passwordLabel')}
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
              slotProps={{ htmlInput: LTR }}
            />
            <Button type="submit" variant="contained" disabled={busy || username.trim() === '' || password === ''}>
              {busy ? t('signingIn') : t('signIn')}
            </Button>
            <TextField
              select
              size="small"
              label={t('lang.label', { ns: 'common' })}
              value={settings.lang}
              onChange={(e) => {
                if (isLanguage(e.target.value)) update({ lang: e.target.value });
              }}
            >
              {SUPPORTED_LANGUAGES.map((l) => (
                <MenuItem key={l} value={l} lang={l}>
                  {LANGUAGE_NAMES[l]}
                </MenuItem>
              ))}
            </TextField>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
}
