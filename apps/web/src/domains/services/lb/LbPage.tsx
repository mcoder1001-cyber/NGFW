import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgKeyboardArrowDownIcon as KeyboardArrowDownIcon,
  PgKeyboardArrowUpIcon as KeyboardArrowUpIcon,
  PgRefreshIcon as RefreshIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { StatusChip } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
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
          {(() => {
            const nativeRows = names.map((name) => {
              const v = vips[name] ?? {};
              const it: LbVipItem | undefined = live.get(name);
              const inUse = it?.servers.filter((s) => s.inUse).length ?? 0;
              return { name, v, it, inUse };
            });
            return (
              <ReactGeereedMaterial
                footer={false}
                hideFilterBuilderComponent
                items={nativeRows}
                columns={[
                  {
                    key: 'details',
                    headerRenderer: () => <></>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { name }: (typeof nativeRows)[number]) => (
                      <>
                        <IconButton
                          size="small"
                          aria-label={t('vips.expand', { name })}
                          onClick={() => setOpen((o) => ({ ...o, [name]: !o[name] }))}
                        >
                          {open[name] ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                        </IconButton>
                      </>
                    ),
                  },
                  {
                    key: 'name',
                    headerRenderer: () => <>{t('col.name')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { name }: (typeof nativeRows)[number]) => (
                      <>{name}</>
                    ),
                  },
                  {
                    key: 'prefix',
                    headerRenderer: () => <>{t('col.prefix')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { v }: (typeof nativeRows)[number]) => (
                      <>{v.prefix}</>
                    ),
                  },
                  {
                    key: 'protocol',
                    headerRenderer: () => <>{t('col.protocol')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { v }: (typeof nativeRows)[number]) => (
                      <>{protoPort(v)}</>
                    ),
                  },
                  {
                    key: 'encap',
                    headerRenderer: () => <>{t('col.encap')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { v }: (typeof nativeRows)[number]) => (
                      <>{v.encap}</>
                    ),
                  },
                  {
                    key: 'servers',
                    headerRenderer: () => <>{t('col.servers')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { v, inUse }: (typeof nativeRows)[number]) => (
                      <>{t('servers.count', { inUse, configured: (v.servers ?? []).length })}</>
                    ),
                  },
                  {
                    key: 'state',
                    headerRenderer: () => <>{t('col.state')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { it }: (typeof nativeRows)[number]) => (
                      <>
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
                      </>
                    ),
                  },
                  {
                    key: 'removed',
                    headerRenderer: () => <>{t('col.removed')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { it }: (typeof nativeRows)[number]) => (
                      <>{it ? removedCopies(it) : ''}</>
                    ),
                  },
                  {
                    key: 'actions',
                    headerRenderer: () => <>{t('col.actions')}</>,
                    sortable: false,
                    filterable: false,
                    renderer: (_value: unknown, { name, v, it }: (typeof nativeRows)[number]) => (
                      <>
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
                      </>
                    ),
                  },
                ]}
                rowKey={(item) => {
                  const { name } = item as (typeof nativeRows)[number];
                  return name;
                }}
                rowProps={() => {
                  return { hover: true };
                }}
                cellProps={(_item, column) => {
                  switch (column.key) {
                    case 'details':
                      return { padding: CHECKBOX };
                    case 'prefix':
                      return { dir: 'ltr' };
                    case 'actions':
                      return { align: RIGHT };
                    default:
                      return {};
                  }
                }}
                moreDetailTable={{
                  hideToggle: true,
                  isExpanded: (item) => Boolean(open[(item as (typeof nativeRows)[number]).name]),
                  customComponent: {
                    customRowComponent: (item) => {
                      const { name, it, v } = item as (typeof nativeRows)[number];
                      return (
                        <>
                          <VipServers name={name} vip={v} item={it} />
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
                        </>
                      );
                    },
                  },
                }}
                tableProps={{ size: 'small', 'aria-label': t('vips.title') }}
                emptyContent={
                  <>
                    <Typography color="text.secondary">{t('vips.empty')}</Typography>
                  </>
                }
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

/** Native grid for applied servers and configured addresses awaiting VPP. */
function VipServers({
  name,
  vip,
  item,
}: {
  name: string;
  vip: Partial<LbVipConfig>;
  item: LbVipItem | undefined;
}) {
  const { t } = useTranslation(NS);
  const rows = [
    ...(item?.servers ?? []).map((server, index) => ({
      key: `${server.address}-${index}`,
      address: server.address,
      inUse: server.inUse as boolean | undefined,
      configured: server.configured,
    })),
    ...(vip.servers ?? [])
      .filter(
        (server) =>
          !(item?.servers ?? []).some((live) => live.inUse && live.address === server.address),
      )
      .map((server) => ({
        key: `cfg-${server.address}`,
        address: server.address,
        inUse: undefined,
        configured: true,
      })),
  ];
  return (
    <ReactGeereedMaterial
      footer={false}
      hideFilterBuilderComponent
      items={rows}
      columns={[
        {
          key: 'address',
          headerRenderer: () => <>{t('servers.address')}</>,
          sortable: false,
          filterable: false,
          renderer: (_value: unknown, row: (typeof rows)[number]) => <>{row.address}</>,
        },
        {
          key: 'state',
          headerRenderer: () => <>{t('servers.state')}</>,
          sortable: false,
          filterable: false,
          renderer: (_value: unknown, row: (typeof rows)[number]) =>
            row.inUse === undefined ? (
              <Typography variant="body2" color="text.secondary">
                {t('servers.notInVpp')}
              </Typography>
            ) : (
              <StatusChip
                size="small"
                status={serverStatus(row.inUse)}
                label={t(row.inUse ? 'servers.inUse' : 'servers.removed')}
              />
            ),
        },
        {
          key: 'configured',
          headerRenderer: () => <>{t('servers.configured')}</>,
          sortable: false,
          filterable: false,
          renderer: (_value: unknown, row: (typeof rows)[number]) => (
            <>{row.configured ? t('yes') : t('no')}</>
          ),
        },
      ]}
      rowKey={(item) => (item as (typeof rows)[number]).key}
      cellProps={(_item, column) => (column.key === 'address' ? { dir: 'ltr' } : {})}
      tableProps={{ size: 'small', 'aria-label': t('servers.title', { name }), sx: { mb: 1 } }}
      emptyContent={null}
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
}
