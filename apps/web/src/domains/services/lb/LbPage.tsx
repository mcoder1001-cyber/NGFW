import AddIcon from '@mui/icons-material/Add';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Collapse from '@mui/material/Collapse';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { Fragment, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import { createMergePatch, localizeSchema } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import {
  protoPort,
  removedCopies,
  serverStatus,
  settingsFormSchema,
  statusOf,
  vipSchema,
  type LbVipConfig,
  type LbVipItem,
} from './model';
import { useCandidateLb, useFlushVip, useLbState, usePatchLb } from './queries';

const NS = 'lb';
const RIGHT = 'right' as const;
const CHECKBOX = 'checkbox' as const;
const DENSE = 'dense' as const;
const LTR_INPUT = { dir: 'ltr' } as const;
const LB_POINTER = '/services/lb';
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

interface Editing {
  name: string;
  isNew: boolean;
  value: Partial<LbVipConfig> | undefined;
}

/**
 * Load balancer (F-lb, VPP lb plugin; tier T3): the VIPs of the candidate with their live state (agent LbState through
 * `GET /api/v1/state/lb/vips`, refreshed every 30 s or on demand — D-132), an application-server sub-table per VIP, the
 * flush action and a schema-driven form for VIPs, the global settings and the NAT interfaces. Edits go to the
 * candidate (pending-change bar → commit). The notice explains V20: lb objects are write-only and deleted VIPs linger.
 */
export function LbPage() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const state = useLbState();
  const candidate = useCandidateLb();
  const ifs = useCandidateInterfaces();
  const patch = usePatchLb();
  const flush = useFlushVip();
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [editing, setEditing] = useState<Editing | null>(null);
  const [nameError, setNameError] = useState('');
  const [flushed, setFlushed] = useState('');

  const vipForm = useMemo(() => localizeSchema(vipSchema(), (k, o) => t(k, o ?? {})), [t]);
  const settingsForm = useMemo(
    () => localizeSchema(settingsFormSchema(), (k, o) => t(k, o ?? {})),
    [t],
  );
  const lb = candidate.data;
  const vips = lb?.vips ?? {};
  const live = useMemo(
    () => new Map((state.data?.items ?? []).map((i) => [i.name, i])),
    [state.data],
  );
  const names = Object.keys(vips).sort();

  const saveVip = async (value: unknown) => {
    if (editing === null) return;
    if (editing.isNew && !NAME_RE.test(editing.name)) {
      setNameError(t('vip.nameInvalid'));
      return;
    }
    if (editing.isNew && vips[editing.name] !== undefined) {
      setNameError(t('vip.nameTaken'));
      return;
    }
    // only the edits against the value the form opened with (another session's changes are not overwritten)
    const before = editing.isNew ? undefined : editing.value;
    const body = {
      vips: { [editing.name]: before === undefined ? value : createMergePatch(before, value) },
    };
    await patch.mutateAsync(body).then(
      () => setEditing(null),
      () => undefined,
    );
  };
  const removeVip = (name: string) =>
    void patch.mutateAsync({ vips: { [name]: null } }).catch(() => undefined);
  const saveSettings = async (value: unknown) => {
    const current: Record<string, unknown> = { ...(lb ?? {}) };
    delete current['vips'];
    await patch
      .mutateAsync(lb === undefined ? value : createMergePatch(current, value))
      .catch(() => undefined);
  };
  const doFlush = (name: string) =>
    void flush.mutateAsync(name).then(
      (r) => setFlushed(t('flush.done', { name, vip: r.vip })),
      () => setFlushed(''),
    );

  const vipProblem = editing
    ? problemFor(patch.error, `/services/lb/vips/${esc(editing.name)}`)
    : null;

  return (
    <Stack gap={2}>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
        <Typography component="h3" variant="h6">
          {t('title')}
        </Typography>
        <Chip size="small" color="secondary" label={t('tier')} />
        <Typography color="text.secondary" sx={{ flex: 1 }}>
          {t('intro')}
        </Typography>
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void state.refetch()}
          disabled={state.isFetching}
        >
          {t('refresh')}
        </Button>
      </Stack>
      <Alert severity="warning" data-testid="lb-writeonly-notice">
        <strong>{t('notice.title')}</strong> {t('notice.body')}
      </Alert>
      {state.isError && <ProblemAlert error={state.error} />}
      {patch.isError && editing === null && <ProblemAlert error={patch.error} />}
      {flush.isError && <ProblemAlert error={flush.error} />}
      {flushed && (
        <Alert severity="success" onClose={() => setFlushed('')}>
          {flushed}
        </Alert>
      )}

      <Paper variant="outlined">
        <Stack direction="row" alignItems="center" sx={{ p: 1.5 }}>
          <Typography sx={{ flex: 1 }} variant="subtitle1">
            {t('vips.title')}
            {state.data && (
              <Typography
                component="span"
                color="text.secondary"
                variant="body2"
                sx={{ marginInlineStart: 1 }}
              >
                {t('vips.vppTotal', { count: state.data.totalVppVips })}
              </Typography>
            )}
          </Typography>
          <Button
            startIcon={<AddIcon />}
            variant="contained"
            disabled={!perms.editConfig}
            onClick={() => {
              setNameError('');
              patch.reset();
              setEditing({ name: '', isNew: true, value: undefined });
            }}
          >
            {t('vips.add')}
          </Button>
        </Stack>
        <TableContainer>
          <Table size="small" aria-label={t('vips.title')}>
            <TableHead>
              <TableRow>
                <TableCell />
                <TableCell>{t('col.name')}</TableCell>
                <TableCell>{t('col.prefix')}</TableCell>
                <TableCell>{t('col.protocol')}</TableCell>
                <TableCell>{t('col.encap')}</TableCell>
                <TableCell>{t('col.servers')}</TableCell>
                <TableCell>{t('col.state')}</TableCell>
                <TableCell>{t('col.removed')}</TableCell>
                <TableCell align={RIGHT}>{t('col.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {names.length === 0 && (
                <TableRow>
                  <TableCell colSpan={9}>
                    <Typography color="text.secondary">{t('vips.empty')}</Typography>
                  </TableCell>
                </TableRow>
              )}
              {names.map((name) => {
                const v = vips[name] ?? {};
                const it: LbVipItem | undefined = live.get(name);
                const inUse = it?.servers.filter((s) => s.inUse).length ?? 0;
                return (
                  <Fragment key={name}>
                    <TableRow hover>
                      <TableCell padding={CHECKBOX}>
                        <IconButton
                          size="small"
                          aria-label={t('vips.expand', { name })}
                          onClick={() => setOpen((o) => ({ ...o, [name]: !o[name] }))}
                        >
                          {open[name] ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                        </IconButton>
                      </TableCell>
                      <TableCell>{name}</TableCell>
                      <TableCell dir="ltr">{v.prefix}</TableCell>
                      <TableCell>{protoPort(v)}</TableCell>
                      <TableCell>{v.encap}</TableCell>
                      <TableCell>
                        {t('servers.count', { inUse, configured: (v.servers ?? []).length })}
                      </TableCell>
                      <TableCell>
                        {it ? (
                          <StatusChip
                            size="small"
                            status={statusOf(it.status)}
                            label={t(`status.${it.status}`)}
                          />
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            {t('status.pending')}
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>{it ? removedCopies(it) : ''}</TableCell>
                      <TableCell align={RIGHT}>
                        <Stack direction="row" gap={0.5} justifyContent="flex-end">
                          <Button
                            size="small"
                            disabled={
                              !perms.editConfig || it?.status !== 'active' || flush.isPending
                            }
                            onClick={() => doFlush(name)}
                          >
                            {t('flush.action')}
                          </Button>
                          <Button
                            size="small"
                            disabled={!perms.editConfig}
                            onClick={() => {
                              patch.reset();
                              setEditing({ name, isNew: false, value: v });
                            }}
                          >
                            {t('edit')}
                          </Button>
                          <Button
                            size="small"
                            color="error"
                            disabled={!perms.editConfig || patch.isPending}
                            onClick={() => removeVip(name)}
                          >
                            {t('delete')}
                          </Button>
                        </Stack>
                      </TableCell>
                    </TableRow>
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        sx={{ py: 0, borderBottom: open[name] ? undefined : 'none' }}
                      >
                        <Collapse in={open[name] ?? false} unmountOnExit>
                          <Table
                            size="small"
                            aria-label={t('servers.title', { name })}
                            sx={{ mb: 1 }}
                          >
                            <TableHead>
                              <TableRow>
                                <TableCell>{t('servers.address')}</TableCell>
                                <TableCell>{t('servers.state')}</TableCell>
                                <TableCell>{t('servers.configured')}</TableCell>
                              </TableRow>
                            </TableHead>
                            <TableBody>
                              {(it?.servers ?? []).map((s, i) => (
                                <TableRow key={`${s.address}-${i}`}>
                                  <TableCell dir="ltr">{s.address}</TableCell>
                                  <TableCell>
                                    <StatusChip
                                      size="small"
                                      status={serverStatus(s.inUse)}
                                      label={t(s.inUse ? 'servers.inUse' : 'servers.removed')}
                                    />
                                  </TableCell>
                                  <TableCell>{s.configured ? t('yes') : t('no')}</TableCell>
                                </TableRow>
                              ))}
                              {(v.servers ?? [])
                                .filter(
                                  (c) =>
                                    !(it?.servers ?? []).some(
                                      (s) => s.inUse && s.address === c.address,
                                    ),
                                )
                                .map((c) => (
                                  <TableRow key={`cfg-${c.address}`}>
                                    <TableCell dir="ltr">{c.address}</TableCell>
                                    <TableCell>
                                      <Typography variant="body2" color="text.secondary">
                                        {t('servers.notInVpp')}
                                      </Typography>
                                    </TableCell>
                                    <TableCell>{t('yes')}</TableCell>
                                  </TableRow>
                                ))}
                            </TableBody>
                          </Table>
                          {it && (
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                              {t('vips.detail', {
                                encap: it.vppEncap || '—',
                                entries: it.vppEntries,
                                dscp: it.dscp,
                                targetPort: it.targetPort,
                              })}
                            </Typography>
                          )}
                        </Collapse>
                      </TableCell>
                    </TableRow>
                  </Fragment>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      </Paper>

      <Paper variant="outlined" sx={{ p: 2 }}>
        <Typography variant="subtitle1" gutterBottom>
          {t('settings.title')}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('settings.intro')}
        </Typography>
        {candidate.isSuccess && (
          <SchemaForm
            schema={settingsForm}
            value={
              lb === undefined
                ? undefined
                : { settings: lb.settings, natInterfaces: lb.natInterfaces ?? [] }
            }
            readOnly={!perms.editConfig}
            interfaceOptions={Object.keys(ifs.data ?? {}).sort()}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            problem={editing === null ? problemFor(patch.error, LB_POINTER) : null}
            onSubmit={(v) => void saveSettings(v)}
          />
        )}
      </Paper>

      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="md">
        <DialogTitle>
          {editing?.isNew ? t('vip.addTitle') : t('vip.editTitle', { name: editing?.name ?? '' })}
        </DialogTitle>
        <DialogContent>
          {editing?.isNew && (
            <TextField
              label={t('vip.name')}
              value={editing.name}
              onChange={(e) => {
                setNameError('');
                setEditing({ ...editing, name: e.target.value });
              }}
              error={nameError !== ''}
              helperText={nameError || t('vip.nameHelp')}
              fullWidth
              margin={DENSE}
              inputProps={LTR_INPUT}
            />
          )}
          {editing && (
            <SchemaForm
              schema={vipForm}
              value={editing.value}
              submitLabel={t('save')}
              resetLabel={t('reset')}
              problem={vipProblem}
              onSubmit={(v) => void saveVip(v)}
            >
              <Button onClick={() => setEditing(null)}>{t('cancel')}</Button>
            </SchemaForm>
          )}
        </DialogContent>
      </Dialog>
    </Stack>
  );
}
