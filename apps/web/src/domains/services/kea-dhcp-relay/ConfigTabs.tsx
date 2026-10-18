import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgIconButton as IconButton,
  PgLinearProgress as LinearProgress,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import type { Theme } from '@amnafzar/common-ui-lib/styles';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { serviceText } from '../../../product-text';
import { StatusChip, useFormatters, type NgfwStatus } from '@ngfw/ui-kit';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { createMergePatch } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { EditDialog, type EditTarget, type KeyField } from './EditDialog';
import {
  esc,
  localize,
  nestPatch,
  relayItemSchema,
  relayRows,
  reservationItemSchema,
  reservationRows,
  serverFormSchema,
  serverRows,
  subnetFormSchema,
  subnetRows,
  usagePercent,
  type DhcpCfg,
  type RelayStateName,
  type ServerStatus,
} from './model';
import {
  useCandidateServices,
  useDhcpStatus,
  useFreshServices,
  usePatchServices,
  useRelayState,
} from './queries';

const MONO = { fontFamily: (th: Theme) => th.ngfw.monoFontFamily, fontSize: 13 } as const;

// Non-UI literals (statuses, families, document paths) live here, outside JSX (i18next/no-literal-string).
const V4 = 'ipv4' as const;
const V6 = 'ipv6' as const;
const ST_UP: NgfwStatus = 'up';
const ST_DEGRADED: NgfwStatus = 'degraded';
const ST_OFF: NgfwStatus = 'adminDown';
const SUBNETS = ['subnets'];
const RESERVATIONS = ['reservations'];
const serverPath = (name: string) => ['dhcp', 'servers', name];
const subnetPath = (server: string, name: string) => [...serverPath(server), 'subnets', name];
const relayPath = (name: string) => ['dhcp', 'relays', name];

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function getAt(root: unknown, path: readonly string[]): unknown {
  let cur: unknown = root;
  for (const p of path) cur = isObject(cur) ? cur[p] : undefined;
  return cur;
}

function omit(v: unknown, keys: readonly string[]): unknown {
  if (!isObject(v)) return v;
  return Object.fromEntries(Object.entries(v).filter(([k]) => !keys.includes(k)));
}

/** Shared editing plumbing: the candidate `services`, merge patches below it, interface choices. */
function useDhcpEditor() {
  const candidate = useCandidateServices();
  const fresh = useFreshServices();
  const patch = usePatchServices();
  const ifaces = useCandidateInterfaces();
  const perms = usePermissions();
  const [error, setError] = useState<unknown>(null);
  const interfaceOptions = useMemo(() => {
    const out: string[] = [];
    for (const [name, itf] of Object.entries(ifaces.data ?? {})) {
      out.push(name);
      for (const id of Object.keys(
        (itf as { subinterfaces?: Record<string, unknown> }).subinterfaces ?? {},
      ))
        out.push(`${name}.${id}`);
    }
    return out.sort();
  }, [ifaces.data]);

  /**
   * Save `value` at services/<path>: only what changed against the current candidate (fields of other tabs untouched).
   * The form value is sent as-is: an optional section switched on with only its defaults is a real choice
   * (no dropPhantomOptionals — WEB-1 deprecates it).
   */
  const save = async (path: string[], exclude: string[], value: unknown): Promise<boolean> => {
    setError(null);
    const current = omit(getAt(await fresh(), path), exclude);
    const body = current === undefined ? value : createMergePatch(current, value);
    try {
      await patch.mutateAsync(nestPatch(path, body));
      return true;
    } catch (e) {
      setError(e);
      return false;
    }
  };
  const remove = async (path: string[]) => {
    setError(null);
    try {
      await patch.mutateAsync(nestPatch(path, null));
    } catch (e) {
      setError(e);
    }
  };
  const dhcp: DhcpCfg | undefined = candidate.data?.dhcp;
  return {
    candidate,
    dhcp,
    patch,
    save,
    remove,
    error,
    setError,
    readOnly: !perms.editConfig,
    interfaceOptions,
  };
}

type Editor = ReturnType<typeof useDhcpEditor>;

function Toolbar({
  title,
  onAdd,
  addLabel,
  disabled,
}: {
  title: string;
  onAdd: () => void;
  addLabel: string;
  disabled: boolean;
}) {
  return (
    <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
      <Typography component="h3" variant="subtitle1" sx={{ flex: 1 }}>
        {title}
      </Typography>
      <Button
        size="small"
        variant="contained"
        startIcon={<AddIcon />}
        disabled={disabled}
        onClick={onAdd}
      >
        {addLabel}
      </Button>
    </Stack>
  );
}

function renderRowActions({
  t,
  name,
  onEdit,
  onRemove,
  disabled,
}: {
  t: ReturnType<typeof useTranslation>['t'];
  name: string;
  onEdit: () => void;
  onRemove: () => void;
  disabled: boolean;
}) {
  return (
    <Box component="span" sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
      <IconButton
        size="small"
        aria-label={t('actions.edit', { name })}
        disabled={disabled}
        onClick={onEdit}
      >
        <EditIcon fontSize="small" />
      </IconButton>
      <IconButton
        size="small"
        aria-label={t('actions.remove', { name })}
        disabled={disabled}
        onClick={onRemove}
      >
        <DeleteIcon fontSize="small" />
      </IconButton>
    </Box>
  );
}

function renderEmptyContent({ text }: { text: string }) {
  return <Typography color="text.secondary">{text}</Typography>;
}

function Frame({ ed, children }: { ed: Editor; children: ReactNode }) {
  const { t } = useTranslation('kea-dhcp-relay');
  return (
    <Box>
      {ed.candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {ed.candidate.isError && <ProblemAlert error={ed.candidate.error} sx={{ mb: 1 }} />}
      {children}
    </Box>
  );
}

/** Daemon state of one family from `servers[]` of the lease endpoint. */
function DaemonChip({ status }: { status: ServerStatus | undefined }) {
  const { t } = useTranslation('kea-dhcp-relay');
  if (!status) return <Chip size="small" variant="outlined" label={t('status.unknown')} />;
  if (status.error)
    return (
      <StatusChip
        size="small"
        status={ST_DEGRADED}
        label={t('status.error')}
        title={serviceText(status.error)}
      />
    );
  if (status.actionRequired === 'start')
    return <StatusChip size="small" status={ST_DEGRADED} label={t('status.startRequired')} />;
  // no configuration the agent wrote (no subnet, nothing bound): not a fault, whether the daemon runs idle or not
  if (!status.active && status.subnets.length === 0)
    return <Chip size="small" variant="outlined" label={t('status.notConfigured')} />;
  if (status.running) return <StatusChip size="small" status={ST_UP} label={t('status.running')} />;
  return <StatusChip size="small" status={ST_OFF} label={t('status.stopped')} />;
}

// ─── Servers ───────────────────────────────────────────────────────────────

export function ServersTab() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('kea-dhcp-relay');
  const ed = useDhcpEditor();
  const status = useDhcpStatus();
  const schema = useMemo(
    () => localize(serverFormSchema(), (k, o) => t(k, o ?? {}), 'server'),
    [t],
  );
  const [target, setTarget] = useState<EditTarget | null>(null);
  const rows = serverRows(ed.dhcp);
  const byFamily = new Map((status.data?.servers ?? []).map((s) => [s.family, s]));
  const keyFields: KeyField[] = [{ id: 'name', label: t('col.name') }];
  const submit = async (keys: Record<string, string>, value: unknown) => {
    if (await ed.save(serverPath(keys['name']!), SUBNETS, value)) setTarget(null);
  };
  return (
    <Frame ed={ed}>
      <Stack direction="row" gap={1} sx={{ mb: 2 }} alignItems="center">
        <Typography variant="body2">{t('daemon.v4')}</Typography>
        <DaemonChip status={byFamily.get(V4)} />
        <Typography variant="body2" sx={{ marginInlineStart: 2 }}>
          {t('daemon.v6')}
        </Typography>
        <DaemonChip status={byFamily.get(V6)} />
      </Stack>
      {[...byFamily.values()].some((s) => s.actionRequired === 'start') && (
        <Alert severity="warning" sx={{ mb: 1 }}>
          {t('daemon.startHint')}
        </Alert>
      )}
      <Toolbar
        title={t('servers.title')}
        addLabel={t('servers.add')}
        disabled={ed.readOnly}
        onAdd={() => setTarget({ keys: { name: '' }, value: undefined, editing: false })}
      />
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      {(() => {
        const nativeRows = rows.map((r) => {
          return { r };
        });
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={nativeRows}
            columns={[
              {
                key: 'name',
                headerRenderer: () => <>{t('col.name')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.name}</>,
              },
              {
                key: 'family',
                headerRenderer: () => <>{t('col.family')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{t(`family.${r.family}`)}</>
                ),
              },
              {
                key: 'vrf',
                headerRenderer: () => <>{t('col.vrf')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.vrf}</>,
              },
              {
                key: 'interfaces',
                headerRenderer: () => <>{t('col.interfaces')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.interfaces.join(' ')}</>
                ),
              },
              {
                key: 'subnets',
                headerRenderer: () => <>{t('col.subnets')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.subnets}</>,
              },
              {
                key: 'enabled',
                headerRenderer: () => <>{t('col.enabled')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.enabled ? t('yes') : t('no')}</>
                ),
              },
              {
                key: 'status',
                headerRenderer: () => <>{t('col.status')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.enabled ? (
                      <DaemonChip status={byFamily.get(r.family)} />
                    ) : (
                      <Chip size="small" variant="outlined" label={t('status.disabled')} />
                    )}
                  </>
                ),
              },
              {
                key: 'actions',
                headerRenderer: () => <>{t('col.actions')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {renderRowActions({
                      name: r.name,
                      disabled: ed.readOnly || ed.patch.isPending,
                      onEdit: () =>
                        setTarget({
                          keys: { name: r.name },
                          value: omit(r.cfg, SUBNETS),
                          editing: true,
                        }),
                      onRemove: () => void ed.remove(serverPath(r.name)),
                      t,
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.id;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'name':
                  return { dir: 'ltr', sx: MONO };
                case 'vrf':
                  return { dir: 'ltr' };
                case 'interfaces':
                  return { dir: 'ltr', sx: MONO };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('servers.title') }}
            emptyContent={renderEmptyContent({ text: t('servers.none') })}
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
      <EditDialog
        open={target !== null}
        title={
          target?.editing
            ? t('servers.editTitle', { name: target.keys['name'] })
            : t('servers.addTitle')
        }
        target={target}
        keyFields={keyFields}
        existing={() => rows.map((r) => r.name)}
        schema={schema}
        pointerPrefix={(k) => `/services/dhcp/servers/${esc(k['name'] ?? '')}`}
        error={ed.error}
        pending={ed.patch.isPending}
        interfaceOptions={ed.interfaceOptions}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(k, v) => void submit(k, v)}
      />
    </Frame>
  );
}

// ─── Subnets & pools ─────────────────────────────────────────────────────────

/** Pool utilisation bar (assigned / total of the subnet's pools, from Kea's statistics). */
export function UsageBar({
  assigned,
  total,
  label,
}: {
  assigned: string;
  total: string;
  label: string;
}) {
  const { t } = useTranslation('kea-dhcp-relay');
  const fmt = useFormatters();
  const pct = usagePercent(assigned, total);
  return (
    <Stack gap={0.25} sx={{ minInlineSize: 140 }}>
      <LinearProgress
        variant="determinate"
        value={pct}
        aria-label={label}
        color={pct >= 90 ? 'error' : pct >= 75 ? 'warning' : 'primary'}
      />
      <Typography variant="caption" color="text.secondary">
        {t('subnets.usage', {
          assigned: fmt.integer(Number(assigned)),
          total: total.length > 15 ? total : fmt.integer(Number(total)),
          pct: pct.toFixed(1),
        })}
      </Typography>
    </Stack>
  );
}

export function SubnetsTab() {
  const { t } = useTranslation('kea-dhcp-relay');
  const ed = useDhcpEditor();
  const status = useDhcpStatus();
  const schema = useMemo(
    () => localize(subnetFormSchema(), (k, o) => t(k, o ?? {}), 'subnet'),
    [t],
  );
  const [target, setTarget] = useState<EditTarget | null>(null);
  const servers = serverRows(ed.dhcp).map((s) => s.name);
  const rows = subnetRows(ed.dhcp, status.data?.servers);
  const keyFields: KeyField[] = [
    { id: 'server', label: t('col.server'), options: () => servers },
    { id: 'name', label: t('col.subnet') },
  ];
  const submit = async (keys: Record<string, string>, value: unknown) => {
    if (await ed.save(subnetPath(keys['server']!, keys['name']!), RESERVATIONS, value))
      setTarget(null);
  };
  return (
    <Frame ed={ed}>
      <Toolbar
        title={t('subnets.title')}
        addLabel={t('subnets.add')}
        disabled={ed.readOnly || servers.length === 0}
        onAdd={() =>
          setTarget({
            keys: { server: servers[0] ?? '', name: '' },
            value: undefined,
            editing: false,
          })
        }
      />
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      {(() => {
        const nativeRows = rows.map((r) => {
          return { r };
        });
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={nativeRows}
            columns={[
              {
                key: 'server',
                headerRenderer: () => <>{t('col.server')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.server}</>,
              },
              {
                key: 'subnet',
                headerRenderer: () => <>{t('col.subnet')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.name}</>,
              },
              {
                key: 'prefix',
                headerRenderer: () => <>{t('col.prefix')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.prefix}</>,
              },
              {
                key: 'pools',
                headerRenderer: () => <>{t('col.pools')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.pools.join(', ')}</>
                ),
              },
              {
                key: 'reservations',
                headerRenderer: () => <>{t('col.reservations')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.reservations}</>
                ),
              },
              {
                key: 'usage',
                headerRenderer: () => <>{t('col.usage')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.usage ? (
                      <UsageBar
                        assigned={r.usage.assigned}
                        total={r.usage.total}
                        label={t('subnets.usageLabel', { name: r.id })}
                      />
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        {t('subnets.noUsage')}
                      </Typography>
                    )}
                  </>
                ),
              },
              {
                key: 'actions',
                headerRenderer: () => <>{t('col.actions')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {renderRowActions({
                      name: r.id,
                      disabled: ed.readOnly || ed.patch.isPending,
                      onEdit: () =>
                        setTarget({
                          keys: { server: r.server, name: r.name },
                          value: omit(r.cfg, RESERVATIONS),
                          editing: true,
                        }),
                      onRemove: () => void ed.remove(subnetPath(r.server, r.name)),
                      t,
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.id;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'server':
                  return { dir: 'ltr' };
                case 'subnet':
                  return { dir: 'ltr' };
                case 'prefix':
                  return { dir: 'ltr', sx: MONO };
                case 'pools':
                  return { dir: 'ltr', sx: MONO };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('subnets.title') }}
            emptyContent={renderEmptyContent({ text: t('subnets.none') })}
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
      <EditDialog
        open={target !== null}
        title={
          target?.editing
            ? t('subnets.editTitle', { name: `${target.keys['server']}/${target.keys['name']}` })
            : t('subnets.addTitle')
        }
        target={target}
        keyFields={keyFields}
        existing={(k) => rows.filter((r) => r.server === k['server']).map((r) => r.name)}
        schema={schema}
        pointerPrefix={(k) =>
          `/services/dhcp/servers/${esc(k['server'] ?? '')}/subnets/${esc(k['name'] ?? '')}`
        }
        error={ed.error}
        pending={ed.patch.isPending}
        interfaceOptions={ed.interfaceOptions}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(k, v) => void submit(k, v)}
      />
    </Frame>
  );
}

// ─── Reservations ────────────────────────────────────────────────────────────

export function ReservationsTab() {
  const { t } = useTranslation('kea-dhcp-relay');
  const ed = useDhcpEditor();
  const schema = useMemo(
    () => localize(reservationItemSchema(), (k, o) => t(k, o ?? {}), 'reservation'),
    [t],
  );
  const [target, setTarget] = useState<EditTarget | null>(null);
  const subnets = subnetRows(ed.dhcp, undefined);
  const servers = [...new Set(subnets.map((s) => s.server))];
  const rows = reservationRows(ed.dhcp);
  const keyFields: KeyField[] = [
    { id: 'server', label: t('col.server'), options: () => servers },
    {
      id: 'subnet',
      label: t('col.subnet'),
      options: (k) => subnets.filter((s) => s.server === k['server']).map((s) => s.name),
    },
    { id: 'name', label: t('col.name') },
  ];
  const path = (k: Record<string, string>) => [
    'dhcp',
    'servers',
    k['server']!,
    'subnets',
    k['subnet']!,
    'reservations',
    k['name']!,
  ];
  const submit = async (keys: Record<string, string>, value: unknown) => {
    if (await ed.save(path(keys), [], value)) setTarget(null);
  };
  const first = subnets[0];
  return (
    <Frame ed={ed}>
      <Toolbar
        title={t('reservations.title')}
        addLabel={t('reservations.add')}
        disabled={ed.readOnly || first === undefined}
        onAdd={() =>
          setTarget({
            keys: { server: first?.server ?? '', subnet: first?.name ?? '', name: '' },
            value: undefined,
            editing: false,
          })
        }
      />
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {t('reservations.hint')}
      </Typography>
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      {(() => {
        const nativeRows = rows.map((r) => {
          return { r };
        });
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={nativeRows}
            columns={[
              {
                key: 'server',
                headerRenderer: () => <>{t('col.server')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.server}</>,
              },
              {
                key: 'subnet',
                headerRenderer: () => <>{t('col.subnet')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.subnet}</>,
              },
              {
                key: 'name',
                headerRenderer: () => <>{t('col.name')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.name}</>,
              },
              {
                key: 'client',
                headerRenderer: () => <>{t('col.client')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.client}</>,
              },
              {
                key: 'ip',
                headerRenderer: () => <>{t('col.ip')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.ip}</>,
              },
              {
                key: 'hostname',
                headerRenderer: () => <>{t('col.hostname')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.hostname}</>
                ),
              },
              {
                key: 'actions',
                headerRenderer: () => <>{t('col.actions')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {renderRowActions({
                      name: r.id,
                      disabled: ed.readOnly || ed.patch.isPending,
                      onEdit: () =>
                        setTarget({
                          keys: { server: r.server, subnet: r.subnet, name: r.name },
                          value: r.cfg,
                          editing: true,
                        }),
                      onRemove: () =>
                        void ed.remove(path({ server: r.server, subnet: r.subnet, name: r.name })),
                      t,
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.id;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'server':
                  return { dir: 'ltr' };
                case 'subnet':
                  return { dir: 'ltr' };
                case 'name':
                  return { dir: 'ltr' };
                case 'client':
                  return { dir: 'ltr', sx: MONO };
                case 'ip':
                  return { dir: 'ltr', sx: MONO };
                case 'hostname':
                  return { dir: 'ltr' };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('reservations.title') }}
            emptyContent={renderEmptyContent({ text: t('reservations.none') })}
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
      <EditDialog
        open={target !== null}
        title={
          target?.editing
            ? t('reservations.editTitle', { name: target.keys['name'] })
            : t('reservations.addTitle')
        }
        target={target}
        keyFields={keyFields}
        existing={(k) =>
          rows
            .filter((r) => r.server === k['server'] && r.subnet === k['subnet'])
            .map((r) => r.name)
        }
        schema={schema}
        pointerPrefix={(k) =>
          `/services/dhcp/servers/${esc(k['server'] ?? '')}/subnets/${esc(k['subnet'] ?? '')}/reservations/${esc(k['name'] ?? '')}`
        }
        error={ed.error}
        pending={ed.patch.isPending}
        interfaceOptions={ed.interfaceOptions}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(k, v) => void submit(k, v)}
      />
    </Frame>
  );
}

// ─── Relays ──────────────────────────────────────────────────────────────────

const RELAY_STATUS: Record<RelayStateName, 'up' | 'down' | 'degraded' | 'adminDown'> = {
  applied: 'up',
  drift: 'degraded',
  missing: 'down',
  disabled: 'adminDown',
  unmanaged: 'degraded',
};

export function RelaysTab() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('kea-dhcp-relay');
  const ed = useDhcpEditor();
  const state = useRelayState();
  const schema = useMemo(() => localize(relayItemSchema(), (k, o) => t(k, o ?? {}), 'relay'), [t]);
  const [target, setTarget] = useState<EditTarget | null>(null);
  const rows = relayRows(ed.dhcp, state.data?.items);
  const keyFields: KeyField[] = [{ id: 'name', label: t('col.name') }];
  const submit = async (keys: Record<string, string>, value: unknown) => {
    if (await ed.save(relayPath(keys['name']!), [], value)) setTarget(null);
  };
  return (
    <Frame ed={ed}>
      <Toolbar
        title={t('relays.title')}
        addLabel={t('relays.add')}
        disabled={ed.readOnly}
        onAdd={() => setTarget({ keys: { name: '' }, value: undefined, editing: false })}
      />
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {t('relays.hint')}
      </Typography>
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 1 }} />}
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      {(() => {
        const nativeRows = rows.map((r) => {
          return { r };
        });
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={nativeRows}
            columns={[
              {
                key: 'name',
                headerRenderer: () => <>{t('col.name')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.name}</>,
              },
              {
                key: 'vrf',
                headerRenderer: () => <>{t('col.vrf')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.vrf}</>,
              },
              {
                key: 'serverVrf',
                headerRenderer: () => <>{t('col.serverVrf')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.serverVrf}</>
                ),
              },
              {
                key: 'dhcpServers',
                headerRenderer: () => <>{t('col.dhcpServers')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.servers.join(' ')}</>
                ),
              },
              {
                key: 'source',
                headerRenderer: () => <>{t('col.source')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.source}</>,
              },
              {
                key: 'interfaces',
                headerRenderer: () => <>{t('col.interfaces')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.interfaces.join(' ')}</>
                ),
              },
              {
                key: 'status',
                headerRenderer: () => <>{t('col.status')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.state === 'pending' ? (
                      <Chip
                        size="small"
                        variant="outlined"
                        color="warning"
                        label={t('relayState.pending')}
                      />
                    ) : (
                      <StatusChip
                        size="small"
                        status={RELAY_STATUS[r.state]}
                        label={t(`relayState.${r.state}`)}
                      />
                    )}
                  </>
                ),
              },
              {
                key: 'actions',
                headerRenderer: () => <>{t('col.actions')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {renderRowActions({
                      name: r.name,
                      disabled: ed.readOnly || ed.patch.isPending || r.cfg === undefined,
                      onEdit: () =>
                        setTarget({ keys: { name: r.name }, value: r.cfg, editing: true }),
                      onRemove: () => void ed.remove(relayPath(r.name)),
                      t,
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.id;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'name':
                  return { dir: 'ltr' };
                case 'vrf':
                  return { dir: 'ltr' };
                case 'serverVrf':
                  return { dir: 'ltr' };
                case 'dhcpServers':
                  return { dir: 'ltr', sx: MONO };
                case 'source':
                  return { dir: 'ltr', sx: MONO };
                case 'interfaces':
                  return { dir: 'ltr', sx: MONO };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('relays.title') }}
            emptyContent={renderEmptyContent({ text: t('relays.none') })}
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
      <EditDialog
        open={target !== null}
        title={
          target?.editing
            ? t('relays.editTitle', { name: target.keys['name'] })
            : t('relays.addTitle')
        }
        target={target}
        keyFields={keyFields}
        existing={() => rows.filter((r) => r.cfg !== undefined).map((r) => r.name)}
        schema={schema}
        pointerPrefix={(k) => `/services/dhcp/relays/${esc(k['name'] ?? '')}`}
        error={ed.error}
        pending={ed.patch.isPending}
        interfaceOptions={ed.interfaceOptions}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(k, v) => void submit(k, v)}
      />
    </Frame>
  );
}
