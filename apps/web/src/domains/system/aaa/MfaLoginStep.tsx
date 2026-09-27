import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Link from '@mui/material/Link';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../../../auth/AuthProvider';
import type { LoginFailure, MfaEnrolment, MfaStep } from '../../../auth/session';
import { RecoveryCodes } from './RecoveryCodes';

const LTR = {
  dir: 'ltr',
  inputMode: 'numeric',
  autoComplete: 'one-time-code',
  spellCheck: false,
} as const;

/**
 * F-aaa-login: the second login step on the login page. With `enrolled: false` the MFA policy requires a factor the
 * user has not set up: the secret is fetched once (login-time enrolment) and the first code enables it. After a new
 * enrolment the recovery codes are shown once before `onDone`.
 */
export function MfaLoginStep({
  step,
  onDone,
  onCancel,
}: {
  step: MfaStep;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('aaa');
  const { session } = useAuth();
  const [enrolment, setEnrolment] = useState<MfaEnrolment | null>(null);
  const [recovery, setRecovery] = useState(false);
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<'invalid' | 'expired' | null>(null);
  const [codes, setCodes] = useState<string[] | null>(null);

  useEffect(() => {
    if (step.enrolled) return;
    let live = true;
    void session.mfaEnroll(step.challenge).then((r) => {
      if (!live) return;
      if ('secret' in r) setEnrolment(r);
      else setError('expired');
    });
    return () => {
      live = false;
    };
  }, [session, step]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const r = await session.mfaVerify(
      step.challenge,
      recovery ? { recoveryCode: value.trim() } : { code: value.trim() },
    );
    setBusy(false);
    setValue('');
    if ('status' in r) {
      setError((r as LoginFailure).status === 401 ? 'invalid' : 'expired');
      return;
    }
    if (r.recoveryCodes && r.recoveryCodes.length > 0) setCodes(r.recoveryCodes);
    else onDone();
  };

  if (codes) return <RecoveryCodes codes={codes} onDone={onDone} />;

  const valid = recovery ? /^[0-9a-fA-F]{10}$/.test(value.trim()) : /^[0-9]{6}$/.test(value.trim());
  return (
    <Stack
      component="form"
      gap={2}
      onSubmit={(e) => void submit(e)}
      noValidate
      aria-labelledby="mfa-title"
    >
      <Typography id="mfa-title" component="h1" variant="h5">
        {step.enrolled ? t('step.title') : t('step.enrolTitle')}
      </Typography>
      {!step.enrolled && (
        <>
          <Typography color="text.secondary">{t('step.enrolIntro')}</Typography>
          {enrolment && (
            <Box data-testid="mfa-enrolment">
              <Typography variant="subtitle2">{t('step.secret')}</Typography>
              <Box
                component="code"
                dir="ltr"
                sx={{
                  display: 'block',
                  wordBreak: 'break-all',
                  fontFamily: (th) => th.vrx.monoFontFamily,
                }}
              >
                {enrolment.secret}
              </Box>
              <Typography variant="subtitle2" sx={{ mt: 1 }}>
                {t('step.uri')}
              </Typography>
              <Link href={enrolment.otpauthUri} dir="ltr" sx={{ wordBreak: 'break-all' }}>
                {enrolment.otpauthUri}
              </Link>
              <Alert severity="warning" sx={{ mt: 1 }}>
                {t('step.shownOnce')}
              </Alert>
            </Box>
          )}
        </>
      )}
      {step.enrolled && <Typography color="text.secondary">{t('step.enter')}</Typography>}
      {error && (
        <Alert severity="error" role="alert">
          {t(`step.${error}`)}
        </Alert>
      )}
      <TextField
        label={recovery ? t('step.recovery') : t('step.code')}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        autoFocus
        required
        slotProps={{ htmlInput: LTR }}
      />
      <Button type="submit" variant="contained" disabled={busy || !valid || error === 'expired'}>
        {busy ? t('step.verifying') : t('step.verify')}
      </Button>
      {step.enrolled && (
        <Button
          size="small"
          onClick={() => {
            setRecovery(!recovery);
            setValue('');
          }}
        >
          {recovery ? t('step.useCode') : t('step.useRecovery')}
        </Button>
      )}
      <Button size="small" onClick={onCancel}>
        {t('step.cancel')}
      </Button>
    </Stack>
  );
}
