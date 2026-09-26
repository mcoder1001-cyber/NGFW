import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
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
import { ServerDataGrid, type GridColDef } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { DaemonStatus, PendingActions, StateProblem } from './common';
import {
  FACILITIES,
  fieldKey,
  NS,
  problemUnder,
  SEVERITIES,
  severityColor,
  syslogTargetSchema,
  WINDOWS_H,
} from './model';
import {
  fetchLogs,
  useCandidateNode,
  useFreshNode,
  usePatchDomain,
  useSyslogState,
  ucsKeys,
  type LogEntry,
  type SyslogTarget,
} from './queries';

const DAEMON = 'rsyslogd';

type Row = LogEntry & { id: string };

/**
 * Services › Logging (F-unbound-chrony-syslog, WBS D7.7): the remote-syslog targets (`management.syslog`, list +
 * schema-driven form with the D-086 keys, live counters from rsyslog impstats) and the log explorer — one bounded,
 * paged query of the local journal per page (server-side paging; severity, facility, time window and text filters).
 */
export default function LoggingTab() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const cand = useCandidateNode<SyslogTarget[]>('management', 'syslog');
  const fresh = useFreshNode<SyslogTarget[]>('management', 'syslog');
  const state = useSyslogState();
  const put = usePatchDomain('management');
  const [editing, setEditing] = useState<{ index: number; value: SyslogTarget | null } | null>(
    null,
  );
  const schema = useMemo(() => syslogTargetSchema(), []);
  const targets = cand.data ?? [];
  const st = state.data;

  const save = async (next: SyslogTarget[]) => {
    await put.mutateAsync({ syslog: next });
  };
  const submit = async (value: unknown) => {
    if (!editing) return;
    const current = [...((await fresh()) ?? [])];
    if (editing.index < 0 || editing.index >= current.length) current.push(value as SyslogTarget);
    else current[editing.index] = value as SyslogTarget;
    try {
      await save(current);
      setEditing(null);
    } catch {
      // shown on the form
    }
  };
  const remove = async (index: number) => {
    const current = [...((await fresh()) ?? [])];
    current.splice(index, 1);
    await save(current).catch(() => undefined);
  };

  return (
    <Box>
      <DaemonStatus daemon={DAEMON} running={undefined} query={state} configPath={st?.configPath} />
      <StateProblem error={state.error} partial={st?.error} />
      <PendingActions actions={st?.pendingActions} />
      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="h6">{t('syslog.targets')}</Typography>
        <Button
          startIcon={<AddIcon />}
          disabled={!perms.editConfig || targets.length >= 8}
          onClick={() => {
            put.reset();
            setEditing({ index: -1, value: null });
          }}
        >
          {t('add')}
        </Button>
      </Stack>
      {put.error && !editing && <ProblemAlert error={put.error} sx={{ mb: 2 }} />}
      <TableContainer component={Paper} variant="outlined" sx={{ mb: 3 }}>
        <Table size="small" aria-label={t('syslog.targets')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.collector')}</TableCell>
              <TableCell>{t('col.transport')}</TableCell>
              <TableCell>{t('col.filter')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
              <TableCell>{t('col.counters')}</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {targets.length === 0 && (
              <TableRow>
                <TableCell colSpan={6}>{t('syslog.none')}</TableCell>
              </TableRow>
            )}
            {targets.map((tg, i) => {
              const live = st?.targets.find((x) => x.index === i);
              return (
                <TableRow key={`${tg.address}-${tg.port}-${tg.protocol}`}>
                  <TableCell
                    sx={{ fontFamily: 'monospace' }}
                  >{`${tg.address}:${tg.port}`}</TableCell>
                  <TableCell>{`${tg.protocol}${tg.format ? ` · ${tg.format}` : ''}`}</TableCell>
                  <TableCell>{`${(tg.facilities ?? []).length ? tg.facilities!.join(',') : '*'}.${tg.severity}`}</TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      color={
                        !live
                          ? 'default'
                          : !live.reported
                            ? 'warning'
                            : live.failed > 0 || live.suspended > 0
                              ? 'error'
                              : live.queueSize > 0
                                ? 'warning'
                                : 'success'
                      }
                      label={t(
                        !live
                          ? 'syslog.notRendered'
                          : !live.reported
                            ? 'syslog.notReported'
                            : live.queueSize > 0
                              ? 'syslog.backlog'
                              : 'syslog.forwarding',
                      )}
                    />
                  </TableCell>
                  <TableCell>
                    {live
                      ? t('syslog.counters', {
                          processed: live.processed,
                          failed: live.failed,
                          queued: live.queueSize,
                        })
                      : '—'}
                  </TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>
                    <IconButton
                      aria-label={t('edit', { name: tg.address })}
                      disabled={!perms.editConfig}
                      onClick={() => {
                        put.reset();
                        setEditing({ index: i, value: tg });
                      }}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      aria-label={t('delete', { name: tg.address })}
                      disabled={!perms.editConfig}
                      onClick={() => void remove(i)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>

      {perms.manageUsers ? <LogExplorer /> : <Alert severity="info">{t('logs.adminOnly')}</Alert>}

      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="md">
        <DialogTitle>{editing?.value ? t('syslog.editTitle') : t('syslog.addTitle')}</DialogTitle>
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            {t('syslog.tlsNote')}
          </Alert>
          {editing && (
            <SchemaForm
              id="syslog-target-form"
              schema={schema}
              value={editing.value ?? undefined}
              onSubmit={submit}
              problem={problemUnder(
                put.error,
                `/management/syslog/${editing.index < 0 ? targets.length : editing.index}`,
              )}
              translateLabel={(p, f) => t(fieldKey('syslog', p), { defaultValue: f })}
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)}>{t('config:cancel')}</Button>
          <Button
            type="submit"
            form="syslog-target-form"
            variant="contained"
            disabled={put.isPending}
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

function LogExplorer() {
  const { t, i18n } = useTranslation(NS);
  const qc = useQueryClient();
  const [severity, setSeverity] = useState('');
  const [facility, setFacility] = useState('');
  const [hours, setHours] = useState<number>(1);
  const [text, setText] = useState('');
  const [q, setQ] = useState('');
  const [meta, setMeta] = useState<{ scanned: number; truncated: boolean } | null>(null);
  const fmt = useMemo(
    () => new Intl.DateTimeFormat(i18n.language, { dateStyle: 'short', timeStyle: 'medium' }),
    [i18n.language],
  );

  const columns: GridColDef<Row>[] = [
    {
      field: 'time',
      headerName: t('col.time'),
      width: 170,
      sortable: false,
      filterable: false,
      valueFormatter: (v: string | null) => (v ? fmt.format(new Date(v)) : ''),
    },
    {
      field: 'severity',
      headerName: t('col.severity'),
      width: 110,
      sortable: false,
      filterable: false,
      renderCell: (p) =>
        p.value ? (
          <Chip
            size="small"
            color={severityColor(String(p.value))}
            label={t(`severity.${String(p.value)}`)}
          />
        ) : null,
    },
    {
      field: 'facility',
      headerName: t('col.facility'),
      width: 100,
      sortable: false,
      filterable: false,
    },
    {
      field: 'identifier',
      headerName: t('col.identifier'),
      width: 140,
      sortable: false,
      filterable: false,
    },
    {
      field: 'message',
      headerName: t('col.message'),
      flex: 1,
      minWidth: 300,
      sortable: false,
      filterable: false,
    },
  ];

  const queryKey = [...ucsKeys.logs, { severity, facility, hours, q }];

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="h6">{t('logs.title')}</Typography>
        <Button
          size="small"
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: ucsKeys.logs })}
        >
          {t('refresh')}
        </Button>
      </Stack>
      <Stack direction="row" spacing={1} sx={{ mb: 1, flexWrap: 'wrap', rowGap: 1 }}>
        <TextField
          select
          size="small"
          label={t('logs.severity')}
          value={severity}
          onChange={(e) => setSeverity(e.target.value)}
          sx={{ minWidth: 160 }}
        >
          <MenuItem value="">{t('logs.all')}</MenuItem>
          {SEVERITIES.map((s) => (
            <MenuItem key={s} value={s}>
              {t('logs.atLeast', { severity: t(`severity.${s}`) })}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label={t('logs.facility')}
          value={facility}
          onChange={(e) => setFacility(e.target.value)}
          sx={{ minWidth: 140 }}
        >
          <MenuItem value="">{t('logs.all')}</MenuItem>
          {FACILITIES.map((f) => (
            <MenuItem key={f} value={f}>
              {f}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label={t('logs.window')}
          value={hours}
          onChange={(e) => setHours(Number(e.target.value))}
          sx={{ minWidth: 140 }}
        >
          {WINDOWS_H.map((h) => (
            <MenuItem key={h} value={h}>
              {t('logs.lastHours', { count: h })}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('logs.search')}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') setQ(text.trim());
          }}
          onBlur={() => setQ(text.trim())}
          slotProps={{ htmlInput: { maxLength: 128 } }}
        />
      </Stack>
      {meta && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mb: 1 }}>
          {t(meta.truncated ? 'logs.scannedTruncated' : 'logs.scanned', { count: meta.scanned })}
        </Typography>
      )}
      <Box sx={{ blockSize: 480 }}>
        <ServerDataGrid<Row>
          aria-label={t('logs.title')}
          columns={columns}
          queryKey={queryKey}
          initialPageSize={50}
          pageSizeOptions={[25, 50, 100]}
          fetchPage={async (req, signal) => {
            const page = await fetchLogs(
              {
                page: req.page + 1,
                pageSize: req.pageSize,
                since: new Date(Date.now() - hours * 3600_000).toISOString(),
                ...(severity ? { severity: severity as never } : {}),
                ...(facility ? { facility: facility as never } : {}),
                ...(q ? { q } : {}),
              },
              signal,
            );
            setMeta({ scanned: page.scanned, truncated: page.truncated });
            const base = req.page * req.pageSize;
            return {
              rows: page.items.map((e, i) => ({ ...e, id: `${base + i}-${e.time ?? ''}` })),
              total: page.total,
            };
          }}
        />
      </Box>
    </Paper>
  );
}
