import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgBlockIcon as BlockIcon,
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
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
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

/**
 * F-bruteforce-block: the live set of temporarily blocked source IPs — the Blocked sources tab of the Security page
 * (`/firewall/security?tab=blocked`; the old `/firewall/auto-block` address redirects there, F-security-page). The
 * page's own heading is the tab, so this renders no title of its own.
 */
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
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
        {t('intro')}
      </Typography>

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
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('title') }}
            columns={[
              {
                key: 'source',
                headerRenderer: () => t('col.source'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'reason',
                headerRenderer: () => t('col.reason'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'hits',
                headerRenderer: () => t('col.hits'),
                headerCellProps: { sx: { textAlign: 'end' } },
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'offences',
                headerRenderer: () => t('col.offences'),
                headerCellProps: { sx: { textAlign: 'end' } },
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'blockedAt',
                headerRenderer: () => t('col.blockedAt'),
                headerCellProps: { dir: 'ltr' },
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'expiresAt',
                headerRenderer: () => t('col.expiresAt'),
                headerCellProps: { dir: 'ltr' },
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'note',
                headerRenderer: () => t('col.note'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'actions',
                headerRenderer: () => t('col.actions'),
                headerCellProps: { sx: { textAlign: 'end' } },
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(items.map((e) =>
                renderRow({
                  key: e.source,
                  e: e,
                  onUnblock: () => unblock.mutate(e.source),
                  busy: unblock.isPending,
                  reasonLabel: t(`reason.${e.reason}`, { defaultValue: e.reason }),
                  unblockLabel: t('unblock'),
                  dateTime: (iso) => fmt.dateTime(new Date(iso)),
                }),
              ) ?? []),
            ]}
            rowKey={(item) => item.id}
            rowProps={(item) => item.rowProps ?? {}}
            cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
            emptyContent={null}
            noFormWrapper
            embedded
            hideToolbar
            hidePagination
            hideFilterBuilderComponent
            search={false}
            sortable={false}
            exportToCSV={false}
            footer={false}
            memoEnable={false}
          />
        </TableContainer>
      )}

      <Typography variant="body2" color="text.secondary" sx={{ mt: 2, maxInlineSize: 900 }}>
        {t('allowlistHint')}
      </Typography>

      <ManualBlockDialog open={dlgOpen} onClose={() => setDlgOpen(false)} />
    </Box>
  );
}

function renderRow({
  key,
  e,
  onUnblock,
  busy,
  reasonLabel,
  unblockLabel,
  dateTime,
}: {
  key: string;
  e: BlockedEntry;
  onUnblock: () => void;
  busy: boolean;
  reasonLabel: string;
  unblockLabel: string;
  dateTime: (iso: string) => string;
}) {
  return {
    id: key,
    source: (
      <>
        <bdi>{e.source}</bdi>
      </>
    ),
    reason: (
      <>
        <Chip size="small" color={REASON_COLOR[e.reason] ?? 'default'} label={reasonLabel} />
      </>
    ),
    hits: e.hits,
    offences: e.offences,
    blockedAt: dateTime(e.blockedAt),
    expiresAt: dateTime(e.expiresAt),
    note: (
      <>
        <bdi>{e.note}</bdi>
      </>
    ),
    actions: (
      <>
        <Button size="small" color="primary" onClick={onUnblock} disabled={busy}>
          {unblockLabel}
        </Button>
      </>
    ),
    cellProps: {
      hits: { sx: { textAlign: 'end' } },
      offences: { sx: { textAlign: 'end' } },
      blockedAt: { dir: 'ltr' },
      expiresAt: { dir: 'ltr' },
      actions: { sx: { textAlign: 'end' } },
    } satisfies Record<
      string,
      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
    >,
  };
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
