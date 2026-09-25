import DeleteIcon from '@mui/icons-material/Delete';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import FormControlLabel from '@mui/material/FormControlLabel';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  putSecret,
  useCandidateSnmp,
  usePatchSnmp,
  useSnmpState,
  type SnmpConfig,
} from './queries';

const NS = 'snmp';
const NAME_RE = /^[A-Za-z0-9_.-]{1,64}$/;
const LTR = { dir: 'ltr' } as const;
const NUMERIC = { dir: 'ltr', inputMode: 'numeric' } as const;
const AUTH_PROTOCOLS = ['sha', 'sha256', 'sha512'] as const;
const DEFAULTS = { level: 'authPriv', auth: 'sha', priv: 'aes', version: 'v2c' } as const;
type GeneralText = 'sysName' | 'sysLocation' | 'sysContact' | 'engineId';
const GENERAL_TEXT: readonly GeneralText[] = ['sysName', 'sysLocation', 'sysContact', 'engineId'];
const K_ENABLED = 'enabled' as const;
const K_SUBAGENT = 'subagent' as const;

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card variant="outlined" sx={{ mb: 2 }}>
      <CardContent>
        <Typography variant="h6" component="h2" gutterBottom>
          {title}
        </Typography>
        {children}
      </CardContent>
    </Card>
  );
}

/** Services → SNMP (F-snmp): general, communities, SNMPv3 users, trap receivers, live state. Secrets go in, never out. */
export default function SnmpTab() {
  const { t } = useTranslation(NS);
  const perm = usePermissions();
  const cand = useCandidateSnmp();
  const state = useSnmpState();
  const patch = usePatchSnmp();
  const [error, setError] = useState<unknown>(null);
  const cfg: SnmpConfig = cand.data ?? {};
  const ro = !perm.editConfig;

  const run = async (body: Record<string, unknown>) => {
    setError(null);
    try {
      await patch.mutateAsync(body);
    } catch (e) {
      setError(e);
    }
  };

  return (
    <Box>
      {(cand.isPending || state.isPending) && <LinearProgress aria-label={t('loading')} sx={{ mb: 2 }} />}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
      {error !== null && <ProblemAlert error={error} sx={{ mb: 2 }} />}
      <StateCard state={state.data} />
      <General cfg={cfg} readOnly={ro} onSave={run} />
      <Communities
        cfg={cfg}
        readOnly={ro}
        canWriteSecrets={perm.manageUsers}
        onSave={run}
        onError={setError}
      />
      <Users
        cfg={cfg}
        readOnly={ro}
        canWriteSecrets={perm.manageUsers}
        onSave={run}
        onError={setError}
      />
      <Traps cfg={cfg} readOnly={ro} onSave={run} />
      <Typography variant="body2" color="text.secondary">
        {t('mibHint')}
      </Typography>
    </Box>
  );
}

type SnmpStateData = ReturnType<typeof useSnmpState>['data'];

function StateCard({ state }: { state: SnmpStateData }) {
  const { t } = useTranslation(NS);
  if (!state) return null;
  return (
    <Section title={t('state.title')}>
      <Stack direction="row" spacing={3} flexWrap="wrap" useFlexGap>
        <Typography>
          {t('state.configured', { value: state.configured ? t('yes') : t('no') })}
        </Typography>
        <Typography>
          {t('state.reachable', { value: state.daemon.reachable ? t('yes') : t('no') })}
        </Typography>
        <Typography>{t('state.sysName', { value: state.daemon.sysName || '—' })}</Typography>
        <Typography>{t('state.credential', { value: state.daemon.credential || '—' })}</Typography>
        <Typography>
          {t('state.subagent', {
            value: state.subagent.registered ? t('state.registered') : t('state.notRegistered'),
          })}
        </Typography>
      </Stack>
      {state.pendingAction !== '' && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          {t('state.pending', { action: state.pendingAction })}
        </Alert>
      )}
    </Section>
  );
}

interface EditProps {
  cfg: SnmpConfig;
  readOnly: boolean;
  onSave: (patch: Record<string, unknown>) => Promise<void>;
}

function General({ cfg, readOnly, onSave }: EditProps) {
  const { t } = useTranslation(NS);
  const [draft, setDraft] = useState<SnmpConfig | null>(null);
  const v = draft ?? cfg;
  const set = (k: keyof SnmpConfig, val: unknown) => setDraft({ ...v, [k]: val });
  const text = (k: GeneralText) => (
    <TextField
      key={k}
      label={t(`general.${k}`)}
      value={v[k] ?? ''}
      onChange={(e) => set(k, e.target.value)}
      disabled={readOnly}
      size="small"
      slotProps={{ htmlInput: LTR }}
    />
  );
  return (
    <Section title={t('general.title')}>
      <Stack spacing={2} sx={{ maxWidth: 480 }}>
        <FormControlLabel
          control={
            <Switch
              checked={v.enabled === true}
              onChange={(e) => set(K_ENABLED, e.target.checked)}
              disabled={readOnly}
            />
          }
          label={t('general.enabled')}
        />
        {GENERAL_TEXT.map(text)}
        <FormControlLabel
          control={
            <Switch
              checked={v.subagent?.enabled !== false}
              onChange={(e) => set(K_SUBAGENT, { enabled: e.target.checked })}
              disabled={readOnly}
            />
          }
          label={t('general.subagent')}
        />
        <Box>
          <Button
            variant="contained"
            disabled={readOnly || draft === null}
            onClick={() =>
              void onSave({
                enabled: v.enabled ?? false,
                sysName: v.sysName || null,
                sysLocation: v.sysLocation || null,
                sysContact: v.sysContact || null,
                engineId: v.engineId || null,
                subagent: v.subagent ?? null,
              }).then(() => setDraft(null))
            }
          >
            {t('save')}
          </Button>
        </Box>
      </Stack>
    </Section>
  );
}

interface SecretEditProps extends EditProps {
  canWriteSecrets: boolean;
  onError: (e: unknown) => void;
}

function Communities({ cfg, readOnly, canWriteSecrets, onSave, onError }: SecretEditProps) {
  const { t } = useTranslation(NS);
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [sources, setSources] = useState('');
  const entries = Object.entries(cfg.communities ?? {});
  const add = async () => {
    try {
      const ref = await putSecret(`snmp-${name}`, value);
      setValue('');
      const src = sources.split(/[\s,]+/).filter(Boolean);
      await onSave({ communities: { [name]: { secretRef: ref, access: 'ro', sources: src } } });
      setName('');
      setSources('');
    } catch (e) {
      onError(e);
    }
  };
  return (
    <Section title={t('communities.title')}>
      <Table size="small" aria-label={t('communities.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('name')}</TableCell>
            <TableCell>{t('communities.secret')}</TableCell>
            <TableCell>{t('communities.sources')}</TableCell>
            <TableCell />
          </TableRow>
        </TableHead>
        <TableBody>
          {entries.length === 0 && <EmptyRow text={t('communities.empty')} />}
          {entries.map(([n, c]) => (
            <TableRow key={n}>
              <TableCell>{n}</TableCell>
              <TableCell dir="ltr">{c.secretRef}</TableCell>
              <TableCell dir="ltr">{(c.sources ?? []).join(', ') || t('any')}</TableCell>
              <TableCell>
                <IconButton
                  aria-label={t('remove', { name: n })}
                  disabled={readOnly}
                  onClick={() => void onSave({ communities: { [n]: null } })}
                >
                  <DeleteIcon />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {!canWriteSecrets && <Typography variant="body2">{t('adminOnly')}</Typography>}
      <Stack direction="row" spacing={1} sx={{ mt: 2 }} flexWrap="wrap" useFlexGap>
        <TextField
          size="small"
          label={t('name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          disabled={readOnly || !canWriteSecrets}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          size="small"
          type="password"
          autoComplete="new-password"
          label={t('communities.value')}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          disabled={readOnly || !canWriteSecrets}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          size="small"
          label={t('communities.sources')}
          value={sources}
          onChange={(e) => setSources(e.target.value)}
          disabled={readOnly || !canWriteSecrets}
          slotProps={{ htmlInput: LTR }}
        />
        <Button
          variant="outlined"
          disabled={readOnly || !canWriteSecrets || !NAME_RE.test(name) || value.length < 8}
          onClick={() => void add()}
        >
          {t('add')}
        </Button>
      </Stack>
    </Section>
  );
}

function Users({ cfg, readOnly, canWriteSecrets, onSave, onError }: SecretEditProps) {
  const { t } = useTranslation(NS);
  const [name, setName] = useState('');
  const [auth, setAuth] = useState('');
  const [priv, setPriv] = useState('');
  const [authProtocol, setAuthProtocol] = useState<'sha' | 'sha256' | 'sha512'>('sha256');
  const entries = Object.entries(cfg.v3Users ?? {});
  const add = async () => {
    try {
      const authRef = await putSecret(`snmp-${name}-auth`, auth);
      const privRef = await putSecret(`snmp-${name}-priv`, priv);
      setAuth('');
      setPriv('');
      await onSave({
        v3Users: {
          [name]: {
            securityLevel: 'authPriv',
            authProtocol,
            authRef,
            privProtocol: 'aes',
            privRef,
            access: 'ro',
          },
        },
      });
      setName('');
    } catch (e) {
      onError(e);
    }
  };
  const off = readOnly || !canWriteSecrets;
  return (
    <Section title={t('users.title')}>
      <Table size="small" aria-label={t('users.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('name')}</TableCell>
            <TableCell>{t('users.level')}</TableCell>
            <TableCell>{t('users.protocols')}</TableCell>
            <TableCell />
          </TableRow>
        </TableHead>
        <TableBody>
          {entries.length === 0 && <EmptyRow text={t('users.empty')} />}
          {entries.map(([n, u]) => (
            <TableRow key={n}>
              <TableCell>{n}</TableCell>
              <TableCell>{u.securityLevel ?? DEFAULTS.level}</TableCell>
              <TableCell dir="ltr">{`${u.authProtocol ?? DEFAULTS.auth} / ${u.privProtocol ?? DEFAULTS.priv}`}</TableCell>
              <TableCell>
                <IconButton
                  aria-label={t('remove', { name: n })}
                  disabled={readOnly}
                  onClick={() => void onSave({ v3Users: { [n]: null } })}
                >
                  <DeleteIcon />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <Stack direction="row" spacing={1} sx={{ mt: 2 }} flexWrap="wrap" useFlexGap>
        <TextField
          size="small"
          label={t('name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          disabled={off}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          select
          size="small"
          label={t('users.auth')}
          value={authProtocol}
          onChange={(e) => setAuthProtocol(e.target.value as typeof authProtocol)}
          disabled={off}
          sx={{ minWidth: 120 }}
        >
          {AUTH_PROTOCOLS.map((p) => (
            <MenuItem key={p} value={p}>
              {p}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          type="password"
          autoComplete="new-password"
          label={t('users.authPass')}
          value={auth}
          onChange={(e) => setAuth(e.target.value)}
          disabled={off}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          size="small"
          type="password"
          autoComplete="new-password"
          label={t('users.privPass')}
          value={priv}
          onChange={(e) => setPriv(e.target.value)}
          disabled={off}
          slotProps={{ htmlInput: LTR }}
        />
        <Button
          variant="outlined"
          disabled={off || !NAME_RE.test(name) || auth.length < 8 || priv.length < 8}
          onClick={() => void add()}
        >
          {t('add')}
        </Button>
      </Stack>
    </Section>
  );
}

function Traps({ cfg, readOnly, onSave }: EditProps) {
  const { t } = useTranslation(NS);
  const [address, setAddress] = useState('');
  const [port, setPort] = useState('162');
  const [target, setTarget] = useState('');
  const list = cfg.trapReceivers ?? [];
  const communities = Object.keys(cfg.communities ?? {});
  const users = Object.keys(cfg.v3Users ?? {});
  const add = () => {
    const [kind, name] = target.split(':') as ['c' | 'u', string];
    const r =
      kind === 'c'
        ? { address, port: Number(port), version: 'v2c', community: name }
        : { address, port: Number(port), version: 'v3', user: name };
    void onSave({ trapReceivers: [...list, r] }).then(() => setAddress(''));
  };
  return (
    <Section title={t('traps.title')}>
      <Table size="small" aria-label={t('traps.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('traps.address')}</TableCell>
            <TableCell>{t('traps.version')}</TableCell>
            <TableCell>{t('traps.credential')}</TableCell>
            <TableCell />
          </TableRow>
        </TableHead>
        <TableBody>
          {list.length === 0 && <EmptyRow text={t('traps.empty')} />}
          {list.map((r, i) => (
            <TableRow key={`${r.address}:${r.port ?? 162}:${r.version ?? 'v2c'}`}>
              <TableCell dir="ltr">{`${r.address}:${r.port ?? 162}`}</TableCell>
              <TableCell>{r.version ?? DEFAULTS.version}</TableCell>
              <TableCell>{r.community ?? r.user}</TableCell>
              <TableCell>
                <IconButton
                  aria-label={t('remove', { name: r.address })}
                  disabled={readOnly}
                  onClick={() => void onSave({ trapReceivers: list.filter((_, j) => j !== i) })}
                >
                  <DeleteIcon />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <Stack direction="row" spacing={1} sx={{ mt: 2 }} flexWrap="wrap" useFlexGap>
        <TextField
          size="small"
          label={t('traps.address')}
          value={address}
          onChange={(e) => setAddress(e.target.value)}
          disabled={readOnly}
          slotProps={{ htmlInput: LTR }}
        />
        <TextField
          size="small"
          label={t('traps.port')}
          value={port}
          onChange={(e) => setPort(e.target.value)}
          disabled={readOnly}
          slotProps={{ htmlInput: NUMERIC }}
          sx={{ width: 100 }}
        />
        <TextField
          select
          size="small"
          label={t('traps.credential')}
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          disabled={readOnly}
          sx={{ minWidth: 180 }}
        >
          {communities.map((c) => (
            <MenuItem key={`c:${c}`} value={`c:${c}`}>
              {t('traps.community', { name: c })}
            </MenuItem>
          ))}
          {users.map((u) => (
            <MenuItem key={`u:${u}`} value={`u:${u}`}>
              {t('traps.user', { name: u })}
            </MenuItem>
          ))}
        </TextField>
        <Button
          variant="outlined"
          disabled={readOnly || address === '' || target === ''}
          onClick={add}
        >
          {t('add')}
        </Button>
      </Stack>
    </Section>
  );
}

/** Guidance row for an empty list (screen readers get it as a normal table cell). */
function EmptyRow({ text }: { text: string }) {
  return (
    <TableRow>
      <TableCell colSpan={8}>
        <Typography variant="body2" color="text.secondary">
          {text}
        </Typography>
      </TableCell>
    </TableRow>
  );
}
