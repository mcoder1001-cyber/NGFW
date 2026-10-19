import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
  PgRefreshIcon as RefreshIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgMenuItem as MenuItem,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { ServerDataGrid, type GridColDef } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
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
  ucsKeys,
  useCandidateNode,
  useFreshNode,
  usePatchDomain,
  useSyslogState,
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
        {(() => {
          const nativeRows = targets.map((tg, i) => {
            const live = st?.targets.find((x) => x.index === i);
            return { tg, i, live };
          });
          return (
            <ReactGeereedMaterial
              footer={false}
              hideFilterBuilderComponent
              items={nativeRows}
              columns={[
                {
                  key: 'collector',
                  headerRenderer: () => <>{t('col.collector')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { tg }: (typeof nativeRows)[number]) => (
                    <>{`${tg.address}:${tg.port}`}</>
                  ),
                },
                {
                  key: 'transport',
                  headerRenderer: () => <>{t('col.transport')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { tg }: (typeof nativeRows)[number]) => (
                    <>{`${tg.protocol}${tg.format ? ` · ${tg.format}` : ''}`}</>
                  ),
                },
                {
                  key: 'filter',
                  headerRenderer: () => <>{t('col.filter')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { tg }: (typeof nativeRows)[number]) => (
                    <>{`${(tg.facilities ?? []).length ? tg.facilities!.join(',') : '*'}.${tg.severity}`}</>
                  ),
                },
                {
                  key: 'status',
                  headerRenderer: () => <>{t('col.status')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { live }: (typeof nativeRows)[number]) => (
                    <>
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
                    </>
                  ),
                },
                {
                  key: 'counters',
                  headerRenderer: () => <>{t('col.counters')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { live }: (typeof nativeRows)[number]) => (
                    <>
                      {live
                        ? t('syslog.counters', {
                            processed: live.processed,
                            failed: live.failed,
                            queued: live.queueSize,
                          })
                        : '—'}
                    </>
                  ),
                },
                {
                  key: 'actions',
                  headerRenderer: () => <></>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { tg, i }: (typeof nativeRows)[number]) => (
                    <>
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
                    </>
                  ),
                },
              ]}
              rowKey={(item) => {
                const { tg } = item as (typeof nativeRows)[number];
                return `${tg.address}-${tg.port}-${tg.protocol}`;
              }}
              cellProps={(_item, column) => {
                switch (column.key) {
                  case 'collector':
                    return { sx: { fontFamily: 'monospace' } };
                  case 'actions':
                    return { sx: { textAlign: 'end' } };
                  default:
                    return {};
                }
              }}
              tableProps={{ size: 'small', 'aria-label': t('syslog.targets') }}
              emptyContent={<>{t('syslog.none')}</>}
              noFormWrapper
              embedded
              memoEnable={false}
              hidePagination
              hideToolbar
              search={false}
              exportToCSV={false}
              sortable={false}
            />
          );
        })()}
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
      valueFormatter: (value: string) => serviceText(value),
      headerName: t('col.identifier'),
      width: 140,
      sortable: false,
      filterable: false,
    },
    {
      field: 'message',
      valueFormatter: (value: string) => serviceText(value),
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
