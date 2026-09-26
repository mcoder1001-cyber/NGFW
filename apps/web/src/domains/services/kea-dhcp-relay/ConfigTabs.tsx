import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import type { Theme } from '@mui/material/styles';
import { StatusChip, useFormatters, type VrxStatus } from '@ngfw/ui-kit';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
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

const MONO = { fontFamily: (th: Theme) => th.vrx.monoFontFamily, fontSize: 13 } as const;

// Non-UI literals (statuses, families, document paths) live here, outside JSX (i18next/no-literal-string).
const V4 = 'ipv4' as const;
const V6 = 'ipv6' as const;
const ST_UP: VrxStatus = 'up';
const ST_DEGRADED: VrxStatus = 'degraded';
const ST_OFF: VrxStatus = 'adminDown';
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

function RowActions({
  name,
  onEdit,
  onRemove,
  disabled,
}: {
  name: string;
  onEdit: () => void;
  onRemove: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation('kea-dhcp-relay');
  return (
    <TableCell sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
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
    </TableCell>
  );
}

function EmptyRow({ cols, text }: { cols: number; text: string }) {
  return (
    <TableRow>
      <TableCell colSpan={cols}>
        <Typography color="text.secondary">{text}</Typography>
      </TableCell>
    </TableRow>
  );
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
        title={status.error}
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
      <Table size="small" aria-label={t('servers.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.name')}</TableCell>
            <TableCell>{t('col.family')}</TableCell>
            <TableCell>{t('col.vrf')}</TableCell>
            <TableCell>{t('col.interfaces')}</TableCell>
            <TableCell>{t('col.subnets')}</TableCell>
            <TableCell>{t('col.enabled')}</TableCell>
            <TableCell>{t('col.status')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={8} text={t('servers.none')} />}
          {rows.map((r) => (
            <TableRow key={r.id} hover>
              <TableCell dir="ltr" sx={MONO}>
                {r.name}
              </TableCell>
              <TableCell>{t(`family.${r.family}`)}</TableCell>
              <TableCell dir="ltr">{r.vrf}</TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.interfaces.join(' ')}
              </TableCell>
              <TableCell>{r.subnets}</TableCell>
              <TableCell>{r.enabled ? t('yes') : t('no')}</TableCell>
              <TableCell>
                {r.enabled ? (
                  <DaemonChip status={byFamily.get(r.family)} />
                ) : (
                  <Chip size="small" variant="outlined" label={t('status.disabled')} />
                )}
              </TableCell>
              <RowActions
                name={r.name}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={() =>
                  setTarget({
                    keys: { name: r.name },
                    value: omit(r.cfg, SUBNETS),
                    editing: true,
                  })
                }
                onRemove={() => void ed.remove(serverPath(r.name))}
              />
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
      <Table size="small" aria-label={t('subnets.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.server')}</TableCell>
            <TableCell>{t('col.subnet')}</TableCell>
            <TableCell>{t('col.prefix')}</TableCell>
            <TableCell>{t('col.pools')}</TableCell>
            <TableCell>{t('col.reservations')}</TableCell>
            <TableCell>{t('col.usage')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={7} text={t('subnets.none')} />}
          {rows.map((r) => (
            <TableRow key={r.id} hover>
              <TableCell dir="ltr">{r.server}</TableCell>
              <TableCell dir="ltr">{r.name}</TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.prefix}
              </TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.pools.join(', ')}
              </TableCell>
              <TableCell>{r.reservations}</TableCell>
              <TableCell>
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
              </TableCell>
              <RowActions
                name={r.id}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={() =>
                  setTarget({
                    keys: { server: r.server, name: r.name },
                    value: omit(r.cfg, RESERVATIONS),
                    editing: true,
                  })
                }
                onRemove={() => void ed.remove(subnetPath(r.server, r.name))}
              />
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
      <Table size="small" aria-label={t('reservations.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.server')}</TableCell>
            <TableCell>{t('col.subnet')}</TableCell>
            <TableCell>{t('col.name')}</TableCell>
            <TableCell>{t('col.client')}</TableCell>
            <TableCell>{t('col.ip')}</TableCell>
            <TableCell>{t('col.hostname')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={7} text={t('reservations.none')} />}
          {rows.map((r) => (
            <TableRow key={r.id} hover>
              <TableCell dir="ltr">{r.server}</TableCell>
              <TableCell dir="ltr">{r.subnet}</TableCell>
              <TableCell dir="ltr">{r.name}</TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.client}
              </TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.ip}
              </TableCell>
              <TableCell dir="ltr">{r.hostname}</TableCell>
              <RowActions
                name={r.id}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={() =>
                  setTarget({
                    keys: { server: r.server, subnet: r.subnet, name: r.name },
                    value: r.cfg,
                    editing: true,
                  })
                }
                onRemove={() =>
                  void ed.remove(path({ server: r.server, subnet: r.subnet, name: r.name }))
                }
              />
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
      <Table size="small" aria-label={t('relays.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.name')}</TableCell>
            <TableCell>{t('col.vrf')}</TableCell>
            <TableCell>{t('col.serverVrf')}</TableCell>
            <TableCell>{t('col.dhcpServers')}</TableCell>
            <TableCell>{t('col.source')}</TableCell>
            <TableCell>{t('col.interfaces')}</TableCell>
            <TableCell>{t('col.status')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={8} text={t('relays.none')} />}
          {rows.map((r) => (
            <TableRow key={r.id} hover>
              <TableCell dir="ltr">{r.name}</TableCell>
              <TableCell dir="ltr">{r.vrf}</TableCell>
              <TableCell dir="ltr">{r.serverVrf}</TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.servers.join(' ')}
              </TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.source}
              </TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.interfaces.join(' ')}
              </TableCell>
              <TableCell>
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
              </TableCell>
              <RowActions
                name={r.name}
                disabled={ed.readOnly || ed.patch.isPending || r.cfg === undefined}
                onEdit={() => setTarget({ keys: { name: r.name }, value: r.cfg, editing: true })}
                onRemove={() => void ed.remove(relayPath(r.name))}
              />
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
