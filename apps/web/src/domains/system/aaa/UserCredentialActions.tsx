import KeyIcon from '@mui/icons-material/Key';
import PhonelinkEraseIcon from '@mui/icons-material/PhonelinkErase';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Checkbox from '@mui/material/Checkbox';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import FormControlLabel from '@mui/material/FormControlLabel';
import IconButton from '@mui/material/IconButton';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { useMfaReset, useSetPassword } from './queries';

/** P06/TD-2: ≥ 12 characters (the API enforces it too). */
const MIN_PASSWORD = 12;
const LTR = { dir: 'ltr' } as const;

/**
 * D-102 (F-aaa-login): the Users page sets an existing user's password through `POST /api/v1/users/{name}/password`
 * (argon2id server-side, sessions end, API keys revoked unless kept) instead of staging a hash in the configuration;
 * and an admin can remove a user's second factor (lost device).
 */
export function UserCredentialActions({
  username,
  disabled,
}: {
  username: string;
  disabled?: boolean;
}) {
  const { t } = useTranslation('aaa');
  const [pwOpen, setPwOpen] = useState(false);
  const [resetOpen, setResetOpen] = useState(false);
  const [password, setPassword] = useState('');
  const [repeat, setRepeat] = useState('');
  const [keep, setKeep] = useState(false);
  const setPw = useSetPassword();
  const reset = useMfaReset();
  const tooShort = password.length > 0 && password.length < MIN_PASSWORD;
  const mismatch = repeat.length > 0 && repeat !== password;
  const closePw = () => {
    setPwOpen(false);
    setPassword('');
    setRepeat('');
    setKeep(false);
  };
  return (
    <>
      <Tooltip title={t('password.action')}>
        <span>
          <IconButton
            size="small"
            aria-label={`${t('password.action')}: ${username}`}
            disabled={disabled}
            onClick={() => {
              setPw.reset();
              setPwOpen(true);
            }}
          >
            <KeyIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title={t('reset.action')}>
        <span>
          <IconButton
            size="small"
            aria-label={`${t('reset.action')}: ${username}`}
            disabled={disabled}
            onClick={() => {
              reset.reset();
              setResetOpen(true);
            }}
          >
            <PhonelinkEraseIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>

      <Dialog
        open={pwOpen}
        onClose={setPw.isPending ? undefined : closePw}
        maxWidth="xs"
        fullWidth
        aria-labelledby="setpw-title"
      >
        <DialogTitle id="setpw-title">{t('password.title', { user: username })}</DialogTitle>
        <DialogContent>
          <Stack gap={2} sx={{ pt: 1 }}>
            <Alert severity="info">{t('password.endsSessions')}</Alert>
            {setPw.isSuccess && <Alert severity="success">{t('password.done')}</Alert>}
            {setPw.isError && <ProblemAlert error={setPw.error} />}
            <TextField
              type="password"
              label={t('password.new')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              error={tooShort}
              helperText={t('password.minLength', { min: MIN_PASSWORD })}
              slotProps={{ htmlInput: LTR }}
            />
            <TextField
              type="password"
              label={t('password.repeat')}
              value={repeat}
              onChange={(e) => setRepeat(e.target.value)}
              autoComplete="new-password"
              error={mismatch}
              helperText={mismatch ? t('password.mismatch') : undefined}
              slotProps={{ htmlInput: LTR }}
            />
            <FormControlLabel
              control={<Checkbox checked={keep} onChange={(e) => setKeep(e.target.checked)} />}
              label={t('password.keepKeys')}
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={closePw}>{t('auth:cancel')}</Button>
          <Button
            variant="contained"
            disabled={setPw.isPending || password.length < MIN_PASSWORD || repeat !== password}
            onClick={() => {
              setPw.mutate(
                { name: username, password, keepApiKeys: keep },
                {
                  onSuccess: () => {
                    setPassword('');
                    setRepeat('');
                  },
                },
              );
            }}
          >
            {t('password.submit')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={resetOpen}
        onClose={reset.isPending ? undefined : () => setResetOpen(false)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>{t('reset.action')}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('reset.confirm', { user: username })}</DialogContentText>
          {reset.isSuccess && (
            <Alert severity="success" sx={{ mt: 1 }}>
              {t('reset.done')}
            </Alert>
          )}
          {reset.isError && <ProblemAlert error={reset.error} sx={{ mt: 1 }} />}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setResetOpen(false)}>{t('auth:cancel')}</Button>
          <Button
            color="warning"
            variant="contained"
            disabled={reset.isPending || reset.isSuccess}
            onClick={() => reset.mutate(username)}
          >
            {t('reset.action')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
