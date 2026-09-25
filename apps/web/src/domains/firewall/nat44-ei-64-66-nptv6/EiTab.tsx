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
import LinearProgress from '@mui/material/LinearProgress';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  EMPTY_FILTER,
  isSessionLevelFilter,
  sessionId,
  type Session,
  type SessionFilter,
} from '../nat44-ed-sessions/model';
import { useCandidateNat, usePatchNat } from '../nat44-ed-sessions/queries';
import { eiKillBodyOf, NS, type EiSessionsPage } from './model';
import { fetchEiSessions, keys, POLL_MS, useEiKill } from './queries';

interface Row extends Session {
  id: string;
}

const LTR = { dir: 'ltr' } as const;
const PAGE_SIZES = [25, 50, 100, 500, 1000];
const FILTER_FIELDS = ['inside', 'outside', 'external', 'port'] as const;
const PROTOCOLS = ['', 'tcp', 'udp', 'icmp'];
const ep = (a: string, p: number) => `${a}:${p}`;

/** NAT44 mode switch: EI applies the same NAT44 settings (Outbound, Static, Pools tabs) with nat44-ei. */
function ModeBar({ mode }: { mode: string }) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const patch = usePatchNat();
  const next = mode === 'ei' ? 'ed' : 'ei';
  return (
    <Box sx={{ mb: 2 }}>
      <Alert
        severity={mode === 'ei' ? 'success' : 'info'}
        action={
          <Button
            size="small"
            disabled={!perms.editConfig || patch.isPending}
            onClick={() => void patch.mutateAsync({ mode: next }).catch(() => undefined)}
          >
            {t(`ei.switchTo.${next}`)}
          </Button>
        }
        data-testid="nat-ei-mode"
      >
        {mode === 'ei' ? t('ei.modeEi') : t('ei.modeEd')}
      </Alert>
      {patch.isError && <ProblemAlert error={patch.error} sx={{ mt: 1 }} />}
    </Box>
  );
}

/**
 * NAT44-EI (shown when `nat.mode` is "ei"): the mode switch, then the paged EI session browser
 * (`GET /state/nat/ei/sessions`, server-side, pageSize ≤ 1000) with per-row kill by the inside endpoint
 * (`POST /actions/nat/ei/sessions/kill`, confirmed, audited). The NAT44 settings themselves are the Outbound, Static
 * and Pools tabs (shared by ED and EI).
 */
export function EiTab() {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const perms = usePermissions();
  const candidate = useCandidateNat();
  const kill = useEiKill();
  const qc = useQueryClient();
  const [draft, setDraft] = useState<SessionFilter>(EMPTY_FILTER);
  const [filter, setFilter] = useState<SessionFilter>(EMPTY_FILTER);
  const [meta, setMeta] = useState<Pick<
    EiSessionsPage,
    'total' | 'totalUsers' | 'truncated'
  > | null>(null);
  const [confirm, setConfirm] = useState<Session | null>(null);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [killError, setKillError] = useState<unknown>(null);
  const mode = typeof candidate.data?.['mode'] === 'string' ? candidate.data['mode'] : 'ed';
  // D-132: 30 s at most; a session-level filter makes the agent scan sessions: refreshed by hand only
  const manual = isSessionLevelFilter(filter);

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const page = await fetchEiSessions(req.page, req.pageSize, filter, signal);
      setMeta({ total: page.total, totalUsers: page.totalUsers, truncated: page.truncated });
      return { rows: page.items.map((s): Row => ({ ...s, id: sessionId(s) })), total: page.total };
    },
    [filter],
  );

  const columns = useMemo<GridColDef<Row>[]>(
    () => [
      { field: 'protocol', headerName: t('col.protocol'), width: 90, sortable: false },
      ...(['inside', 'outside', 'external'] as const).map((side): GridColDef<Row> => ({
        field: side,
        headerName: t(`col.${side}`),
        minWidth: 170,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) =>
          side === 'inside'
            ? ep(r.insideAddress, r.insidePort)
            : side === 'outside'
              ? ep(r.outsideAddress, r.outsidePort)
              : ep(r.externalAddress, r.externalPort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      })),
      { field: 'vrf', headerName: t('col.vrf'), width: 90, sortable: false },
      {
        field: 'static',
        headerName: t('col.flags'),
        width: 110,
        sortable: false,
        renderCell: (p) =>
          p.row.static ? (
            <Chip size="small" variant="outlined" label={t('sessions.static')} />
          ) : null,
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
        field: 'actions',
        headerName: t('col.actions'),
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
      const r = await kill.mutateAsync(eiKillBodyOf(confirm));
      setResult({ ok: true, text: r.summary });
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setResult({ ok: false, text: t('kill.gone') });
      else setKillError(e);
    }
    setConfirm(null);
  };

  if (candidate.isPending) return <LinearProgress aria-label={t('loading')} />;
  if (candidate.isError) return <ProblemAlert error={candidate.error} />;
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('ei.intro')}
      </Typography>
      <ModeBar mode={mode} />
      {mode === 'ei' && (
        <>
          <Stack
            component="form"
            direction="row"
            gap={1}
            flexWrap="wrap"
            sx={{ mb: 1 }}
            aria-label={t('sessions.filter')}
            onSubmit={(e) => {
              e.preventDefault();
              setFilter({ ...draft });
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
            <Button
              size="small"
              onClick={() => {
                setDraft(EMPTY_FILTER);
                setFilter(EMPTY_FILTER);
              }}
            >
              {t('filter.clear')}
            </Button>
          </Stack>
          <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
            <Typography variant="body2" role="status" data-testid="nat-ei-sessions-total">
              {meta
                ? t('sessions.total', {
                    sessions: fmt.integer(meta.total),
                    users: fmt.integer(meta.totalUsers),
                  })
                : t('loading')}
            </Typography>
            {meta?.truncated && (
              <Chip size="small" color="warning" label={t('sessions.truncated')} />
            )}
            <Button
              size="small"
              onClick={() => void qc.invalidateQueries({ queryKey: keys.eiSessions })}
            >
              {t('refresh')}
            </Button>
            {manual && <Chip size="small" variant="outlined" label={t('sessions.manualRefresh')} />}
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
          <Paper variant="outlined" sx={{ blockSize: 520 }}>
            <ServerDataGrid<Row>
              aria-label={t('ei.sessions')}
              columns={columns}
              queryKey={[...keys.eiSessions, filter]}
              fetchPage={fetchPage}
              refetchInterval={manual ? false : POLL_MS}
              initialPageSize={100}
              pageSizeOptions={PAGE_SIZES}
              disableColumnFilter
            />
          </Paper>
        </>
      )}
      <Dialog open={confirm !== null} onClose={() => setConfirm(null)} fullWidth maxWidth="xs">
        <DialogTitle>{t('kill.title')}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('kill.question')}</DialogContentText>
          {confirm && (
            <Typography
              component="p"
              dir="ltr"
              sx={{ mt: 1, fontFamily: (th) => th.vrx.monoFontFamily, textAlign: 'start' }}
              data-testid="nat-ei-kill-tuple"
            >
              {`${confirm.protocol} ${ep(confirm.insideAddress, confirm.insidePort)} (vrf ${confirm.vrf})`}
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
