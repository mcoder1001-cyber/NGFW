import BlockIcon from '@mui/icons-material/Block';
import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import LinearProgress from '@mui/material/LinearProgress';
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
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import {
  autoBlockKeys,
  useAutoBlock,
  useManualBlock,
  useUnblock,
  type BlockedEntry,
} from './queries';

const LTR = 'ltr' as const;

const REASON_COLOR: Record<string, 'default' | 'warning' | 'error' | 'info'> = {
  webLogin: 'warning',
  ssh: 'warning',
  vpnAuth: 'warning',
  portScan: 'error',
  manual: 'info',
};

/** F-bruteforce-block: `/firewall/auto-block` — the live set of temporarily blocked source IPs. */
export function AutoBlockPage() {
  const { t } = useTranslation('auto-block');
  const fmt = useFormatters();
  const qc = useQueryClient();
  const q = useAutoBlock();
  const unblock = useUnblock();
  const [dlgOpen, setDlgOpen] = useState(false);
  const items = q.data?.items ?? [];

  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>

      <Stack direction="row" gap={1} sx={{ mb: 1 }}>
        <Box sx={{ flex: 1 }} />
        <Button startIcon={<BlockIcon />} onClick={() => setDlgOpen(true)}>
          {t('blockByHand')}
        </Button>
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: autoBlockKeys.list })}
        >
          {t('refresh')}
        </Button>
      </Stack>

      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {unblock.isError && <ProblemAlert error={unblock.error} sx={{ mb: 1 }} />}
      {q.data && items.length === 0 && <Alert severity="success">{t('empty')}</Alert>}

      {items.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.source')}</TableCell>
                <TableCell>{t('col.reason')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.hits')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.offences')}</TableCell>
                <TableCell dir="ltr">{t('col.blockedAt')}</TableCell>
                <TableCell dir="ltr">{t('col.expiresAt')}</TableCell>
                <TableCell>{t('col.note')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map((e) => (
                <Row
                  key={e.source}
                  e={e}
                  onUnblock={() => unblock.mutate(e.source)}
                  busy={unblock.isPending}
                  reasonLabel={t(`reason.${e.reason}`, { defaultValue: e.reason })}
                  unblockLabel={t('unblock')}
                  dateTime={(iso) => fmt.dateTime(new Date(iso))}
                />
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Typography variant="body2" color="text.secondary" sx={{ mt: 2, maxInlineSize: 900 }}>
        {t('allowlistHint')}
      </Typography>

      <ManualBlockDialog open={dlgOpen} onClose={() => setDlgOpen(false)} />
    </Box>
  );
}

function Row({
  e,
  onUnblock,
  busy,
  reasonLabel,
  unblockLabel,
  dateTime,
}: {
  e: BlockedEntry;
  onUnblock: () => void;
  busy: boolean;
  reasonLabel: string;
  unblockLabel: string;
  dateTime: (iso: string) => string;
}) {
  return (
    <TableRow>
      <TableCell>
        <bdi>{e.source}</bdi>
      </TableCell>
      <TableCell>
        <Chip size="small" color={REASON_COLOR[e.reason] ?? 'default'} label={reasonLabel} />
      </TableCell>
      <TableCell sx={{ textAlign: 'end' }}>{e.hits}</TableCell>
      <TableCell sx={{ textAlign: 'end' }}>{e.offences}</TableCell>
      <TableCell dir="ltr">{dateTime(e.blockedAt)}</TableCell>
      <TableCell dir="ltr">{dateTime(e.expiresAt)}</TableCell>
      <TableCell>
        <bdi>{e.note}</bdi>
      </TableCell>
      <TableCell sx={{ textAlign: 'end' }}>
        <Button size="small" color="primary" onClick={onUnblock} disabled={busy}>
          {unblockLabel}
        </Button>
      </TableCell>
    </TableRow>
  );
}

function ManualBlockDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation('auto-block');
  const block = useManualBlock();
  const [source, setSource] = useState('');
  const [note, setNote] = useState('');

  const submit = () => {
    const trimmedNote = note.trim();
    block.mutate(
      { source: source.trim(), ...(trimmedNote ? { note: trimmedNote } : {}) },
      {
        onSuccess: () => {
          setSource('');
          setNote('');
          onClose();
        },
      },
    );
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t('dialog.title')}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {t('dialog.help')}
        </Typography>
        {block.isError && <ProblemAlert error={block.error} sx={{ mb: 2 }} />}
        <Stack gap={2} sx={{ mt: 1 }}>
          <TextField
            label={t('dialog.source')}
            value={source}
            onChange={(ev) => setSource(ev.target.value)}
            fullWidth
            autoFocus
            inputProps={{ dir: LTR }}
          />
          <TextField
            label={t('dialog.note')}
            value={note}
            onChange={(ev) => setNote(ev.target.value)}
            fullWidth
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('dialog.cancel')}</Button>
        <Button
          variant="contained"
          onClick={submit}
          disabled={source.trim() === '' || block.isPending}
        >
          {t('dialog.submit')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
