import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import FormControlLabel from '@mui/material/FormControlLabel';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import DeleteIcon from '@mui/icons-material/Delete';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  useCandidateHostStack,
  useHostStackState,
  usePutHostStack,
  type HostStackConfig,
  type HostStackRule,
} from './queries';

const NS = 'host-stack';
const DEFAULT_VRF = 'default';
const GLOBAL = 'global';
const TRANSPORTS: readonly HostStackRule['transport'][] = ['tcp', 'udp'];
const ACTIONS: readonly HostStackRule['action'][] = ['allow', 'deny'];
const RULE_COLS = ['tag', 'scope', 'transport', 'local', 'remote', 'action', 'namespace'] as const;
const RIGHT = { textAlign: 'right' } as const;
const BLOCK = { display: 'block' } as const;

function portText(p: number | undefined, any: string): string {
  return p === undefined || p === 0 ? any : String(p);
}

/** Services → Host stack (F-host-stack): session layer switch, app namespaces and the session-rules grid. */
export function HostStackTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const live = useHostStackState();
  const cand = useCandidateHostStack();
  const put = usePutHostStack();
  const hs: HostStackConfig = cand.data ?? {};
  const namespaces = hs.namespaces ?? {};
  const rules = hs.sessionRules ?? [];
  const ro = !perms.editConfig;

  const save = (next: HostStackConfig) => put.mutate({ ...hs, ...next });

  const [ns, setNs] = useState({ id: '', vrf: DEFAULT_VRF, iface: '' });
  const [rule, setRule] = useState<HostStackRule>({
    tag: '',
    transport: 'tcp',
    local: '',
    remote: '',
    action: 'deny',
    scope: 'global',
  });

  return (
    <Stack spacing={2}>
      <Alert severity="info" data-testid="host-stack-banner">
        {t('banner')}
      </Alert>
      {put.error !== null && <ProblemAlert error={put.error} />}

      <Paper sx={{ p: 2 }} aria-label={t('live')}>
        <Typography variant="subtitle1">{t('live')}</Typography>
        {live.isError ? (
          <Alert severity="warning">{t('stateError')}</Alert>
        ) : (
          live.data && (
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
              <Chip
                color={live.data.sessionEnabled ? 'success' : 'default'}
                label={live.data.sessionEnabled ? t('sessionOn') : t('sessionOff')}
                title={live.data.sessionDetail}
              />
              <Chip label={t('ruleCount', { own: live.data.ruleCount, total: live.data.ruleCountTotal })} />
              <Typography variant="body2">
                {t('appliedNs', { list: live.data.namespaces.join(', ') || t('none') })}
              </Typography>
            </Stack>
          )
        )}
      </Paper>

      <Paper sx={{ p: 2 }}>
        <FormControlLabel
          control={
            <Switch
              checked={hs.enabled === true}
              disabled={ro || put.isPending}
              onChange={(_, v) => save({ enabled: v })}
            />
          }
          label={t('enabled')}
        />
        <Typography variant="caption" sx={BLOCK} color="text.secondary">
          {t('enabledHelp')}
        </Typography>
      </Paper>

      <Paper sx={{ p: 2 }}>
        <Typography variant="subtitle1">{t('namespaces')}</Typography>
        <Table size="small" aria-label={t('namespaces')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.id')}</TableCell>
              <TableCell>{t('col.vrf')}</TableCell>
              <TableCell>{t('col.interface')}</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.keys(namespaces).length === 0 && (
              <TableRow>
                <TableCell colSpan={4}>{t('empty')}</TableCell>
              </TableRow>
            )}
            {Object.entries(namespaces).map(([id, n]) => (
              <TableRow key={id}>
                <TableCell>{id}</TableCell>
                <TableCell>{n.vrf ?? DEFAULT_VRF}</TableCell>
                <TableCell>{n.interface ?? ''}</TableCell>
                <TableCell sx={RIGHT}>
                  <IconButton
                    aria-label={`${t('delete')} ${id}`}
                    disabled={ro}
                    onClick={() => {
                      const next = { ...namespaces };
                      delete next[id];
                      save({ namespaces: next });
                    }}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <Stack direction="row" spacing={1} sx={{ mt: 1 }} flexWrap="wrap" useFlexGap>
          <TextField size="small" label={t('col.id')} value={ns.id} onChange={(e) => setNs({ ...ns, id: e.target.value })} />
          <TextField size="small" label={t('col.vrf')} value={ns.vrf} onChange={(e) => setNs({ ...ns, vrf: e.target.value })} />
          <TextField
            size="small"
            label={t('col.interface')}
            value={ns.iface}
            onChange={(e) => setNs({ ...ns, iface: e.target.value })}
          />
          <Button
            variant="outlined"
            disabled={ro || ns.id === ''}
            onClick={() => {
              save({
                namespaces: {
                  ...namespaces,
                  [ns.id]: { vrf: ns.vrf || DEFAULT_VRF, ...(ns.iface ? { interface: ns.iface } : {}) },
                },
              });
              setNs({ id: '', vrf: DEFAULT_VRF, iface: '' });
            }}
          >
            {t('add')}
          </Button>
        </Stack>
      </Paper>

      <Paper sx={{ p: 2 }}>
        <Typography variant="subtitle1">{t('rules')}</Typography>
        <Table size="small" aria-label={t('rules')}>
          <TableHead>
            <TableRow>
              {RULE_COLS.map((c) => (
                <TableCell key={c}>{t(`col.${c}`)}</TableCell>
              ))}
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {rules.length === 0 && (
              <TableRow>
                <TableCell colSpan={8}>{t('empty')}</TableCell>
              </TableRow>
            )}
            {rules.map((r, i) => (
              <TableRow key={r.tag}>
                <TableCell>{r.tag}</TableCell>
                <TableCell>{r.scope ?? GLOBAL}</TableCell>
                <TableCell>{r.transport}</TableCell>
                <TableCell>
                  {r.local} {t('port')} {portText(r.localPort, t('any'))}
                </TableCell>
                <TableCell>
                  {r.remote} {t('port')} {portText(r.remotePort, t('any'))}
                </TableCell>
                <TableCell>{r.action}</TableCell>
                <TableCell>{r.appNamespace ?? ''}</TableCell>
                <TableCell sx={RIGHT}>
                  <IconButton
                    aria-label={`${t('delete')} ${r.tag}`}
                    disabled={ro}
                    onClick={() => save({ sessionRules: rules.filter((_, j) => j !== i) })}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <Stack direction="row" spacing={1} sx={{ mt: 1 }} flexWrap="wrap" useFlexGap>
          <TextField size="small" label={t('col.tag')} value={rule.tag} onChange={(e) => setRule({ ...rule, tag: e.target.value })} />
          <TextField
            select
            size="small"
            label={t('col.transport')}
            value={rule.transport}
            onChange={(e) => setRule({ ...rule, transport: e.target.value as HostStackRule['transport'] })}
          >
            {TRANSPORTS.map((x) => (
              <MenuItem key={x} value={x}>
                {x}
              </MenuItem>
            ))}
          </TextField>
          <TextField size="small" label={t('col.local')} value={rule.local} onChange={(e) => setRule({ ...rule, local: e.target.value })} />
          <TextField size="small" label={t('col.remote')} value={rule.remote} onChange={(e) => setRule({ ...rule, remote: e.target.value })} />
          <TextField
            select
            size="small"
            label={t('col.action')}
            value={rule.action}
            onChange={(e) => setRule({ ...rule, action: e.target.value as HostStackRule['action'] })}
          >
            {ACTIONS.map((x) => (
              <MenuItem key={x} value={x}>
                {x}
              </MenuItem>
            ))}
          </TextField>
          <Button
            variant="outlined"
            disabled={ro || rule.tag === '' || rule.local === '' || rule.remote === ''}
            onClick={() => {
              save({ sessionRules: [...rules, rule] });
              setRule({ ...rule, tag: '' });
            }}
          >
            {t('add')}
          </Button>
        </Stack>
      </Paper>
    </Stack>
  );
}
