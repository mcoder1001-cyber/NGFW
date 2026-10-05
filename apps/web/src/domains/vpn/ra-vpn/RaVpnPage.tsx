import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { localizeSchema } from '../../interfaces/model';
import { activationAllowed, stepSchema, steps, type Profile } from './model';
import {
  useCapabilities,
  useDisconnect,
  useProfiles,
  useSaveProfile,
  useSessions,
} from './queries';

export default function RaVpnPage() {
  const { t } = useTranslation('ra-vpn');
  const permissions = usePermissions();
  const capability = useCapabilities();
  const profiles = useProfiles();
  const [selected, setSelected] = useState('');
  const [cursor, setCursor] = useState('');
  const [editing, setEditing] = useState<{
    name: string;
    value: Partial<Profile>;
    isNew: boolean;
  } | null>(null);
  const [step, setStep] = useState(0);
  const [blocked, setBlocked] = useState(false);
  const [disconnectTarget, setDisconnectTarget] = useState<{ profile: string; id: string } | null>(
    null,
  );
  const save = useSaveProfile();
  const disconnect = useDisconnect();
  const sessions = useSessions(selected, cursor, capability.data?.operational === true);
  const schema = useMemo(() => localizeSchema(stepSchema(step), t), [step, t]);
  const nameValid =
    !!editing &&
    /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/.test(editing.name) &&
    (!editing.isNew || !Object.hasOwn(profiles.data ?? {}, editing.name));
  const advance = async (value: unknown) => {
    if (!editing || !permissions.editConfig || !nameValid) return;
    const merged = { ...editing.value, ...(value as Partial<Profile>) };
    setEditing({ ...editing, value: merged });
    setBlocked(false);
    if (step < 3) {
      setStep(step + 1);
      return;
    }
    if (!activationAllowed(merged, capability.data)) {
      setBlocked(true);
      return;
    }
    await save.mutateAsync({ name: editing.name, profile: merged });
    setEditing(null);
  };
  return (
    <Stack spacing={2}>
      <Typography variant="h5">{t('title')}</Typography>
      {capability.error && <ProblemAlert error={capability.error} />}
      {profiles.error && <ProblemAlert error={profiles.error} />}
      {!capability.data?.operational && <Alert severity="warning">{t('unavailable')}</Alert>}
      <Alert severity="info">{t('hints')}</Alert>
      <Stack direction="row" spacing={2}>
        <TextField
          select
          label={t('profile')}
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setCursor('');
          }}
          sx={{ minWidth: 220 }}
        >
          <MenuItem value="">{t('choose')}</MenuItem>
          {Object.keys(profiles.data ?? {}).map((name) => (
            <MenuItem key={name} value={name}>
              {name}
            </MenuItem>
          ))}
        </TextField>
        <Button
          disabled={!permissions.editConfig || profiles.isPending}
          onClick={() => {
            setStep(0);
            setBlocked(false);
            setEditing({
              name: '',
              value: {
                enabled: false,
                auth: 'eap-mschapv2',
                vrf: 'default',
                underlayVrf: 'default',
                pools: [],
                splitTunnel: [],
                users: [],
              },
              isNew: true,
            });
          }}
        >
          {t('add')}
        </Button>
        <Button
          disabled={!permissions.editConfig || !selected}
          onClick={() => {
            setStep(0);
            setBlocked(false);
            setEditing({
              name: selected,
              value: structuredClone(profiles.data?.[selected] ?? {}),
              isNew: false,
            });
          }}
        >
          {t('edit')}
        </Button>
        <Button
          onClick={() => {
            void capability.refetch();
            if (capability.data?.operational) void sessions.refetch();
          }}
        >
          {t('refresh')}
        </Button>
      </Stack>
      {sessions.error && <ProblemAlert error={sessions.error} />}
      {disconnect.error && <ProblemAlert error={disconnect.error} />}
      {disconnect.data?.disconnected === false && (
        <Alert severity="warning">{t('notRemoved')}</Alert>
      )}
      <Table aria-label={t('sessions')}>
        <TableHead>
          <TableRow>
            {['identity', 'addresses', 'uptime', 'bytesIn', 'bytesOut', 'action'].map((key) => (
              <TableCell key={key}>{t(key)}</TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {(sessions.data?.items ?? []).map((session) => (
            <TableRow key={session.id}>
              <TableCell>{session.identity}</TableCell>
              <TableCell dir="ltr">{session.addresses.join(', ')}</TableCell>
              <TableCell dir="ltr">{session.establishedSeconds}</TableCell>
              <TableCell dir="ltr">{session.bytesIn}</TableCell>
              <TableCell dir="ltr">{session.bytesOut}</TableCell>
              <TableCell>
                <Button
                  disabled={
                    permissions.role !== 'admin' ||
                    disconnect.isPending ||
                    !capability.data?.operational
                  }
                  onClick={() => setDisconnectTarget({ profile: session.profile, id: session.id })}
                >
                  {t('disconnect')}
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {sessions.isSuccess && sessions.data.items.length === 0 && (
        <Typography>{t('empty')}</Typography>
      )}
      <Stack direction="row">
        <Button disabled={!cursor} onClick={() => setCursor('')}>
          {t('first')}
        </Button>
        <Button
          disabled={!sessions.data?.nextCursor}
          onClick={() => setCursor(sessions.data!.nextCursor)}
        >
          {t('nextPage')}
        </Button>
      </Stack>
      <Dialog open={!!editing} onClose={() => setEditing(null)} fullWidth maxWidth="md">
        <DialogTitle>{t(`step.${step}`)}</DialogTitle>
        <DialogContent>
          <TextField
            label={t('name')}
            value={editing?.name ?? ''}
            disabled={!editing?.isNew}
            error={!!editing?.name && !nameValid}
            onChange={(e) => editing && setEditing({ ...editing, name: e.target.value })}
            fullWidth
            sx={{ my: 2 }}
          />
          {blocked && <Alert severity="error">{t('activationBlocked')}</Alert>}
          {save.error && <ProblemAlert error={save.error} />}
          {editing && (
            <SchemaForm
              key={`${editing.name}:${step}`}
              schema={schema}
              value={Object.fromEntries(
                Object.entries(editing.value).filter(([key]) =>
                  steps[step]!.includes(key as never),
                ),
              )}
              readOnly={!permissions.editConfig || !nameValid || save.isPending}
              onSubmit={advance}
              submitLabel={t(step === 3 ? 'save' : 'next')}
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button disabled={step === 0} onClick={() => setStep(step - 1)}>
            {t('back')}
          </Button>
          <Button onClick={() => setEditing(null)}>{t('close')}</Button>
        </DialogActions>
      </Dialog>
      <Dialog open={!!disconnectTarget} onClose={() => setDisconnectTarget(null)}>
        <DialogTitle>{t('confirmDisconnect')}</DialogTitle>
        <DialogActions>
          <Button onClick={() => setDisconnectTarget(null)}>{t('close')}</Button>
          <Button
            disabled={permissions.role !== 'admin' || disconnect.isPending}
            onClick={() => {
              if (disconnectTarget)
                void disconnect.mutateAsync(disconnectTarget).then(() => setDisconnectTarget(null));
            }}
          >
            {t('disconnect')}
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
