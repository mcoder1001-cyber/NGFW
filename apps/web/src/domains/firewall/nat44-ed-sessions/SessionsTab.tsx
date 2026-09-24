import DeleteForeverIcon from '@mui/icons-material/DeleteForever';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  EMPTY_FILTER,
  killBodyOf,
  sessionId,
  type Session,
  type SessionFilter,
  type SessionsPage,
} from './model';
import { fetchSessions, NAT_POLL_MS, natKeys, useKillSession } from './queries';
import { NAT_NS } from './tabs';

/** Grid row: one session plus its stable id. */
export interface SessionRow extends Session {
  id: string;
}

const LTR = { dir: 'ltr' } as const;
const PAGE_SIZES = [25, 50, 100, 500, 1000];
const FILTER_FIELDS = ['inside', 'outside', 'external', 'port'] as const;
const PROTOCOLS = ['', 'tcp', 'udp', 'icmp'];

function ep(a: string, p: number) {
  return `${a}:${p}`;
}

/**
 * Paged session browser (server-side: `GET /state/nat/sessions?page&pageSize&…`, pageSize ≤ 1000; only one page lives
 * in the browser) with per-row kill (`POST /actions/nat/sessions/kill`, confirmed, audited by the API).
 */
export function SessionsTab() {
  const { t } = useTranslation(NAT_NS);
  const fmt = useFormatters();
  const perms = usePermissions();
  const kill = useKillSession();
  const [draft, setDraft] = useState<SessionFilter>(EMPTY_FILTER);
  const [filter, setFilter] = useState<SessionFilter>(EMPTY_FILTER);
  const [meta, setMeta] = useState<Pick<SessionsPage, 'total' | 'totalUsers' | 'truncated'> | null>(
    null,
  );
  const [confirm, setConfirm] = useState<Session | null>(null);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [killError, setKillError] = useState<unknown>(null);

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const page = await fetchSessions(req.page, req.pageSize, filter, signal);
      setMeta({ total: page.total, totalUsers: page.totalUsers, truncated: page.truncated });
      return {
        rows: page.items.map((s): SessionRow => ({ ...s, id: sessionId(s) })),
        total: page.total,
      };
    },
    [filter],
  );

  const columns = useMemo<GridColDef<SessionRow>[]>(
    () => [
      { field: 'protocol', headerName: t('col.protocol'), width: 90, sortable: false },
      {
        field: 'inside',
        headerName: t('col.inside'),
        minWidth: 170,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) => ep(r.insideAddress, r.insidePort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      {
        field: 'outside',
        headerName: t('col.outside'),
        minWidth: 170,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) => ep(r.outsideAddress, r.outsidePort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      {
        field: 'external',
        headerName: t('col.externalHost'),
        minWidth: 170,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) => ep(r.externalAddress, r.externalPort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      { field: 'vrf', headerName: t('col.vrf'), width: 90, sortable: false },
      {
        field: 'flags',
        headerName: t('col.flags'),
        width: 150,
        sortable: false,
        renderCell: (p) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            {p.row.static && <Chip size="small" variant="outlined" label={t('sessions.static')} />}
            {p.row.twiceNat && (
              <Chip size="small" variant="outlined" label={t('sessions.twiceNat')} />
            )}
            {p.row.timedOut && (
              <Chip
                size="small"
                color="warning"
                variant="outlined"
                label={t('sessions.timedOut')}
              />
            )}
          </Stack>
        ),
      },
      {
        field: 'idleSeconds',
        headerName: t('col.idle'),
        type: 'number',
        width: 90,
        sortable: false,
        valueFormatter: (v: number) => fmt.integer(v),
      },
      {
        field: 'packets',
        headerName: t('col.packets'),
        type: 'number',
        width: 100,
        sortable: false,
        valueFormatter: (v: number) => fmt.integer(v),
      },
      {
        field: 'bytes',
        headerName: t('col.bytes'),
        type: 'number',
        width: 110,
        sortable: false,
        valueFormatter: (v: number) => fmt.integer(v),
      },
      {
        field: 'actions',
        headerName: t('list.actions'),
        width: 80,
        sortable: false,
        filterable: false,
        renderCell: (p) => (
          <Tooltip title={perms.editConfig ? '' : t('readonly')}>
            <span>
              <IconButton
                size="small"
                aria-label={t('kill.button', {
                  session: ep(p.row.insideAddress, p.row.insidePort),
                })}
                disabled={!perms.editConfig}
                onClick={() => setConfirm(p.row)}
              >
                <DeleteForeverIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        ),
      },
    ],
    [t, fmt, perms.editConfig],
  );

  const doKill = async () => {
    if (!confirm) return;
    setResult(null);
    setKillError(null);
    try {
      const r = await kill.mutateAsync(killBodyOf(confirm));
      setResult({ ok: true, text: r.summary });
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setResult({ ok: false, text: t('kill.gone') });
      else setKillError(e);
    }
    setConfirm(null);
  };

  const apply = () => setFilter({ ...draft });
  const clear = () => {
    setDraft(EMPTY_FILTER);
    setFilter(EMPTY_FILTER);
  };

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('sessions.intro')}
      </Typography>
      <Stack
        component="form"
        direction="row"
        gap={1}
        flexWrap="wrap"
        sx={{ mb: 1 }}
        aria-label={t('sessions.filter')}
        onSubmit={(e) => {
          e.preventDefault();
          apply();
        }}
      >
        {FILTER_FIELDS.map((f) => (
          <TextField
            key={f}
            size="small"
            label={t(`filter.${f}`)}
            value={draft[f]}
            onChange={(e) => setDraft({ ...draft, [f]: e.target.value })}
            slotProps={{ htmlInput: LTR }}
            sx={{ inlineSize: f === 'port' ? 110 : 160 }}
          />
        ))}
        <TextField
          select
          size="small"
          label={t('filter.protocol')}
          value={draft.protocol}
          onChange={(e) => setDraft({ ...draft, protocol: e.target.value })}
          sx={{ inlineSize: 130 }}
        >
          {PROTOCOLS.map((p) => (
            <MenuItem key={p} value={p}>
              {p === '' ? t('col.any') : p}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('filter.vrf')}
          value={draft.vrf}
          onChange={(e) => setDraft({ ...draft, vrf: e.target.value })}
          slotProps={{ htmlInput: LTR }}
          sx={{ inlineSize: 130 }}
        />
        <Button type="submit" variant="contained" size="small">
          {t('filter.apply')}
        </Button>
        <Button size="small" onClick={clear}>
          {t('filter.clear')}
        </Button>
      </Stack>
      <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
        <Typography variant="body2" role="status" data-testid="nat-sessions-total">
          {meta
            ? t('sessions.total', {
                sessions: fmt.integer(meta.total),
                users: fmt.integer(meta.totalUsers),
              })
            : t('loading')}
        </Typography>
        {meta?.truncated && <Chip size="small" color="warning" label={t('status.truncated')} />}
      </Stack>
      {result && (
        <Alert
          severity={result.ok ? 'success' : 'info'}
          sx={{ mb: 1 }}
          onClose={() => setResult(null)}
        >
          {result.text}
        </Alert>
      )}
      {killError !== null && <ProblemAlert error={killError} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: 560 }}>
        <ServerDataGrid<SessionRow>
          aria-label={t('sessions.title')}
          columns={columns}
          queryKey={[...natKeys.sessions, filter]}
          fetchPage={fetchPage}
          refetchInterval={NAT_POLL_MS}
          initialPageSize={100}
          pageSizeOptions={PAGE_SIZES}
          disableColumnFilter
        />
      </Paper>
      <Dialog open={confirm !== null} onClose={() => setConfirm(null)} fullWidth maxWidth="xs">
        <DialogTitle>{t('kill.title')}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('kill.question')}</DialogContentText>
          {confirm && (
            <Typography
              component="p"
              dir="ltr"
              sx={{ mt: 1, fontFamily: (th) => th.vrx.monoFontFamily, textAlign: 'start' }}
              data-testid="nat-kill-tuple"
            >
              {`${confirm.protocol} ${ep(confirm.insideAddress, confirm.insidePort)} → ${ep(confirm.externalAddress, confirm.externalPort)} (vrf ${confirm.vrf})`}
            </Typography>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirm(null)}>{t('cancel')}</Button>
          <Button
            color="error"
            variant="contained"
            disabled={kill.isPending}
            onClick={() => void doKill()}
          >
            {t('kill.confirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
