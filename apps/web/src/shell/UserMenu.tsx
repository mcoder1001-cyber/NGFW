import AccountCircle from '@mui/icons-material/AccountCircle';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import ListItemText from '@mui/material/ListItemText';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api';
import { call } from '../api-problem';
import { useAuth } from '../auth/AuthProvider';
import { confirmStore } from '../config/confirm-store';
import { ProblemAlert } from '../config/ProblemAlert';

/** P06 requires ≥ 12 characters for a new password (PasswordBody). */
const MIN_PASSWORD = 12;

function ChangePasswordDialog({ open, onClose, onChanged }: { open: boolean; onClose: () => void; onChanged: () => void }) {
  const { t } = useTranslation('auth');
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [repeat, setRepeat] = useState('');
  const change = useMutation({
    mutationFn: async () => call(api.POST('/api/v1/auth/password', { body: { current, password: next } })),
    onSuccess: onChanged,
  });
  const tooShort = next.length > 0 && next.length < MIN_PASSWORD;
  const mismatch = repeat.length > 0 && repeat !== next;
  return (
    <Dialog open={open} onClose={onClose} aria-labelledby="pw-title" maxWidth="xs" fullWidth>
      <DialogTitle id="pw-title">{t('password.title')}</DialogTitle>
      <DialogContent>
        <Stack gap={2} sx={{ pt: 1 }}>
          <Alert severity="info">{t('password.endsSessions')}</Alert>
          <TextField label={t('password.current')} type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" />
          <TextField
            label={t('password.new')}
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            error={tooShort}
            helperText={t('password.minLength', { min: MIN_PASSWORD })}
          />
          <TextField
            label={t('password.repeat')}
            type="password"
            value={repeat}
            onChange={(e) => setRepeat(e.target.value)}
            autoComplete="new-password"
            error={mismatch}
            helperText={mismatch ? t('password.mismatch') : undefined}
          />
          {change.isError && <ProblemAlert error={change.error} />}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('cancel')}</Button>
        <Button
          variant="contained"
          onClick={() => change.mutate()}
          disabled={change.isPending || current === '' || next.length < MIN_PASSWORD || next !== repeat}
        >
          {t('password.submit')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** Signed-in user, role, self-service password change (P06: allowed for every role) and sign-out. */
export function UserMenu() {
  const { t } = useTranslation('auth');
  const { session, state } = useAuth();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [pw, setPw] = useState(false);
  if (!state.user) return null;
  const signOut = () => {
    setAnchor(null);
    confirmStore.reset();
    void session.logout();
  };
  return (
    <>
      <Button
        color="inherit"
        startIcon={<AccountCircle />}
        onClick={(e) => setAnchor(e.currentTarget)}
        aria-haspopup="menu"
        aria-label={t('menu.label', { user: state.user.username })}
        sx={{ textTransform: 'none', marginInlineEnd: 1 }}
        data-testid="user-menu"
      >
        <span dir="ltr">{state.user.username}</span>
      </Button>
      <Menu anchorEl={anchor} open={anchor !== null} onClose={() => setAnchor(null)}>
        <MenuItem disabled>
          <ListItemText primary={state.user.username} secondary={t(`role.${state.user.role}`)} />
        </MenuItem>
        <Divider />
        <MenuItem
          onClick={() => {
            setAnchor(null);
            setPw(true);
          }}
        >
          {t('password.title')}
        </MenuItem>
        <MenuItem onClick={signOut}>{t('signOut')}</MenuItem>
      </Menu>
      <ChangePasswordDialog
        open={pw}
        onClose={() => setPw(false)}
        onChanged={() => {
          setPw(false);
          // P06 revokes every refresh family of the user on a password change: sign in again with the new password
          void session.logout();
        }}
      />
    </>
  );
}
