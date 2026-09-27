import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Checkbox from '@mui/material/Checkbox';
import FormControlLabel from '@mui/material/FormControlLabel';
import LinearProgress from '@mui/material/LinearProgress';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  downloadCapture,
  useCaptures,
  useDeleteCapture,
  useStartCapture,
  type CaptureFile,
  type CaptureRequest,
} from './queries';

export const NS = 'capture-trace';
const DIRS = ['both', 'rx', 'tx'] as const;
const LIMITS = ['maxPackets', 'seconds', 'snaplen'] as const;
const TABS = ['capture', 'trace', 'pg'] as const;
const COLS = ['id', 'state', 'interface', 'packets', 'size', 'actions'] as const;
const BPF_INPUT = { dir: 'ltr', 'data-testid': 'bpf' } as const;
/** Same alphabet as the API and agent (pcap-filter(7), no quotes / ";" / "$" / backslash). */
const BPF_RE = /^[A-Za-z0-9 .:/[\]()&|!=<>\-+*%^~_,]*$/;

function pointerErrors(err: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (err instanceof ApiError) {
    for (const e of err.body.errors ?? []) {
      const k = (e.pointer ?? '').replace(/^\//, '');
      if (k) out[k] = e.message ?? '';
    }
  }
  return out;
}

function Running({ c }: { c: CaptureFile }) {
  const { t } = useTranslation(NS);
  const elapsed = c.startedAt
    ? Math.max(0, Math.round((Date.now() - Date.parse(c.startedAt)) / 1000))
    : 0;
  const pct = Math.min(100, (elapsed / Math.max(1, c.seconds)) * 100);
  return (
    <Alert severity="info" sx={{ mb: 2 }} data-testid="capture-running">
      {t('running', { id: c.id, iface: c.interface, elapsed, seconds: c.seconds })}
      <LinearProgress variant="determinate" value={pct} sx={{ mt: 1 }} aria-label={t('progress')} />
    </Alert>
  );
}

function CaptureForm({ disabled }: { disabled: boolean }) {
  const { t } = useTranslation(NS);
  const start = useStartCapture();
  const [f, setF] = useState<CaptureRequest>({
    interface: 'any',
    direction: 'both',
    drop: false,
    bpf: '',
    maxPackets: 1000,
    seconds: 30,
    snaplen: 9000,
  });
  const local = BPF_RE.test(f.bpf) ? '' : t('bpfInvalid');
  const server = pointerErrors(start.error);
  const set = (p: Partial<CaptureRequest>) => setF((o) => ({ ...o, ...p }));
  const num = (k: 'maxPackets' | 'seconds' | 'snaplen') => ({
    type: 'number',
    value: f[k],
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => set({ [k]: Number(e.target.value) }),
    error: Boolean(server[k]),
    helperText: server[k] ?? t(`${k}Help`),
    label: t(k),
    size: 'small' as const,
  });
  return (
    <Box
      component="form"
      onSubmit={(e) => {
        e.preventDefault();
        if (!local) start.mutate(f);
      }}
    >
      {start.isError && !Object.keys(server).length && (
        <ProblemAlert error={start.error} sx={{ mb: 2 }} />
      )}
      {start.isSuccess && (
        <Alert severity="success" sx={{ mb: 2 }}>
          {t('started', { id: start.data.id })}
        </Alert>
      )}
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} sx={{ mb: 2 }}>
        <TextField
          label={t('interface')}
          size="small"
          value={f.interface}
          onChange={(e) => set({ interface: e.target.value })}
          error={Boolean(server['interface'])}
          helperText={server['interface'] ?? t('interfaceHelp')}
        />
        <TextField
          select
          label={t('direction')}
          size="small"
          value={f.direction}
          onChange={(e) => set({ direction: e.target.value as CaptureRequest['direction'] })}
          sx={{ minInlineSize: 140 }}
        >
          {DIRS.map((d) => (
            <MenuItem key={d} value={d}>
              {t(`dir.${d}`)}
            </MenuItem>
          ))}
        </TextField>
        <FormControlLabel
          control={<Checkbox checked={f.drop} onChange={(e) => set({ drop: e.target.checked })} />}
          label={t('drop')}
        />
      </Stack>
      <TextField
        fullWidth
        label={t('bpf')}
        size="small"
        value={f.bpf}
        onChange={(e) => set({ bpf: e.target.value })}
        error={Boolean(local || server['bpf'])}
        helperText={local || server['bpf'] || t('bpfHelp')}
        inputProps={BPF_INPUT}
        sx={{ mb: 2 }}
      />
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} sx={{ mb: 2 }}>
        {LIMITS.map((k) => (
          <TextField key={k} {...num(k)} />
        ))}
      </Stack>
      <Button
        type="submit"
        variant="contained"
        disabled={disabled || start.isPending || Boolean(local)}
      >
        {t('start')}
      </Button>
    </Box>
  );
}

/** Tools › Packet capture (F-capture-trace, WBS D8.2): start a pcap capture, watch it, download / delete the files. */
export function CapturePage() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const admin = perms.manageUsers;
  const q = useCaptures();
  const del = useDeleteCapture();
  const [tab, setTab] = useState<(typeof TABS)[number]>(TABS[0]);
  const [dlError, setDlError] = useState<unknown>(null);
  const running = q.data?.captures.find((c) => c.state === 'running');
  return (
    <Box>
      <Typography variant="h5" component="h1" sx={{ mb: 2 }}>
        {t('title')}
      </Typography>
      <Tabs
        value={tab}
        onChange={(_, v: typeof tab) => setTab(v)}
        aria-label={t('tabs')}
        sx={{ mb: 2 }}
      >
        {TABS.map((v) => (
          <Tab key={v} value={v} label={t(`tab.${v}`)} />
        ))}
      </Tabs>
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 2 }} />}
      {q.isPending && <LinearProgress aria-label={t('loading')} />}
      {tab === 'trace' && (
        <Alert severity="warning" data-testid="trace-unavailable">
          {t('notAvailable')} {q.data?.trace.reason}
        </Alert>
      )}
      {tab === 'pg' && (
        <Alert severity="warning" data-testid="pg-unavailable">
          {t('notAvailable')} {q.data?.pg.reason}
        </Alert>
      )}
      {tab === 'capture' && (
        <>
          {running && <Running c={running} />}
          <CaptureForm disabled={!perms.editConfig || Boolean(running)} />
          {dlError !== null && <ProblemAlert error={dlError} sx={{ mt: 2 }} />}
          {del.isError && <ProblemAlert error={del.error} sx={{ mt: 2 }} />}
          <Typography variant="h6" component="h2" sx={{ mt: 3, mb: 1 }}>
            {t('files', { max: q.data?.maxFiles ?? 0 })}
          </Typography>
          <Table size="small" aria-label={t('files', { max: q.data?.maxFiles ?? 0 })}>
            <TableHead>
              <TableRow>
                {COLS.map((h) => (
                  <TableCell key={h}>{t(`col.${h}`)}</TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {(q.data?.captures ?? []).map((c) => (
                <TableRow key={c.id}>
                  <TableCell dir="ltr">{c.id}</TableCell>
                  <TableCell>{t(`state.${c.state}`, { defaultValue: c.state })}</TableCell>
                  <TableCell dir="ltr">{c.interface}</TableCell>
                  <TableCell>{c.packets}</TableCell>
                  <TableCell>{c.size}</TableCell>
                  <TableCell>
                    {admin && c.state !== 'running' && c.size !== '0' && (
                      <Button size="small" onClick={() => downloadCapture(c.id).catch(setDlError)}>
                        {t('download')}
                      </Button>
                    )}
                    {admin && c.state !== 'running' && (
                      <Button size="small" color="error" onClick={() => del.mutate(c.id)}>
                        {t('delete')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {!admin && (
            <Typography variant="body2" sx={{ mt: 1 }}>
              {t('adminOnly')}
            </Typography>
          )}
        </>
      )}
    </Box>
  );
}
