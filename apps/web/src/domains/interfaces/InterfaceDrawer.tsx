import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../api-problem';
import { usePermissions } from '../../auth/AuthProvider';
import { ProblemAlert } from '../../config/ProblemAlert';
import {
  adminStatus,
  createMergePatch,
  interfaceFormSchema,
  linkStatus,
  localizeSchema,
  subinterfaceSchema,
  type InterfaceConfig,
  type InterfaceItem,
  type SubinterfaceConfig,
} from './model';
import {
  useCandidateInterfaces,
  useFreshCandidate,
  useInterfacesState,
  usePatchInterfaces,
  usePppoeReconnect,
  useSetPhysicalOwner,
} from './queries';
import type { Rate } from './rates';
import { Sparkline } from './Sparkline';
import { SubinterfaceTable } from './subinterfaces/SubinterfaceTable';

/** Server pointers `/interfaces/<name>[/subinterfaces/<id>]/…` → pointers relative to the edited form. */
export function problemFor(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(prefix) ? e.pointer.slice(prefix.length) : e.pointer,
    })),
  };
}

const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');
const SUB_ID_RE = /^[0-9]{1,10}$/;
const BPS = 'bps' as const;
const SUB_ID_INPUT = { dir: 'ltr', inputMode: 'numeric' } as const;

/** Two configuration values are equal when the merge patch from one to the other is empty. */
function sameValue(a: unknown, b: unknown): boolean {
  const p = createMergePatch(a ?? {}, b ?? {});
  return typeof p === 'object' && p !== null && Object.keys(p).length === 0;
}

/**
 * The interface without the members the form does not edit: `subinterfaces` (their own table) and `physical`
 * (F-default-vpp-nics: read-only, changed only through Release / Reclaim) — so a save never sends `physical: null`.
 */
const NOT_IN_FORM = new Set(['subinterfaces', 'physical']);
function withoutSubs(c: InterfaceConfig | undefined): Partial<InterfaceConfig> | undefined {
  if (!c) return undefined;
  return Object.fromEntries(
    Object.entries(c).filter(([k]) => !NOT_IN_FORM.has(k)),
  ) as Partial<InterfaceConfig>;
}

/** Opens from the end of the reading direction: MUI flips `anchor="right"` to the left in RTL. */
export function InterfaceDrawer({
  name,
  onClose,
  rates,
}: {
  name: string | null;
  onClose: () => void;
  rates: Map<string, Rate>;
}) {
  return (
    <Drawer
      anchor="right"
      open={name !== null}
      onClose={onClose}
      sx={{ zIndex: (th) => th.zIndex.modal }}
      slotProps={{ paper: { sx: { inlineSize: { xs: '100%', md: 640 } } } }}
    >
      {name !== null && <DrawerBody key={name} name={name} onClose={onClose} rates={rates} />}
    </Drawer>
  );
}

function DrawerBody({
  name,
  onClose,
  rates,
}: {
  name: string;
  onClose: () => void;
  rates: Map<string, Rate>;
}) {
  const { t } = useTranslation('interfaces');
  const fmt = useFormatters();
  const perms = usePermissions();
  const state = useInterfacesState();
  const candidate = useCandidateInterfaces();
  const fresh = useFreshCandidate();
  const patch = usePatchInterfaces();
  const setOwner = useSetPhysicalOwner();
  const [saved, setSaved] = useState(false);
  // F-default-vpp-nics: pending release/reclaim confirmation ('host' = release, 'dataplane' = reclaim)
  const [ownerAction, setOwnerAction] = useState<'host' | 'dataplane' | null>(null);
  const [subDialog, setSubDialog] = useState<{
    id: string;
    value: SubinterfaceConfig | undefined;
  } | null>(null);
  const [subError, setSubError] = useState<unknown>(null);

  const item: InterfaceItem | undefined = state.data?.items.find((i) => i.name === name);
  const physical = item?.physical ?? null; // F-default-vpp-nics: seeded physical NIC marker
  // the owner this NIC would flip to (dataplane → host = release, host → dataplane = reclaim); computed outside JSX
  const nextOwner: 'host' | 'dataplane' = physical?.owner === 'dataplane' ? 'host' : 'dataplane';
  const parentName = item?.kind === 'subinterface' ? (item.parent ?? '') : name;
  const isSub = item?.kind === 'subinterface';
  const config: InterfaceConfig | undefined = candidate.data?.[parentName];
  const formSchema = useMemo(
    () => localizeSchema(interfaceFormSchema(), (k, o) => t(k, o ?? {})),
    [t],
  );
  const subSchema = useMemo(
    () => localizeSchema(subinterfaceSchema(), (k, o) => t(k, o ?? {})),
    [t],
  );
  const subs = Object.entries(config?.subinterfaces ?? {}).sort(
    ([a], [b]) => Number(a) - Number(b),
  );
  const live = item?.state;
  const rate = rates.get(live?.vppName ?? name);
  const readOnly = !perms.editConfig || item?.inventoryOnly === true;

  // The form edits the value it opened with (review N4): a refetch of the candidate never remounts it (unsaved edits
  // stay), and Save sends only what changed against that value — a field another session changed meanwhile is not
  // written back with a stale value. "Reload" opens the form again on the current candidate.
  const [reload, setReload] = useState(0);
  const [opened, setOpened] = useState<{
    reload: number;
    value: Partial<InterfaceConfig> | undefined;
  } | null>(null);
  const [changedElsewhere, setChangedElsewhere] = useState(false);
  if (candidate.isSuccess && (opened === null || opened.reload !== reload)) {
    setOpened({ reload, value: withoutSubs(config) });
  }
  const formValue = opened?.value;

  const saveInterface = async (value: unknown) => {
    setSaved(false);
    const base = opened?.value;
    const current = withoutSubs((await fresh())[name]);
    setChangedElsewhere(!sameValue(base, current));
    // No phantom clean-up: SchemaForm keeps an optional object (dhcpClient) absent until its presence switch is on
    // (WEB-1), so an object in `value` is one the user enabled, possibly holding nothing but its defaults.
    const body = base === undefined ? value : createMergePatch(base, value);
    try {
      await patch.mutateAsync({ [name]: body });
      setOpened({ reload, value: value as Partial<InterfaceConfig> }); // the next save diffs against what is saved now
      setSaved(true);
    } catch {
      // rendered from patch.error (pointers mapped onto the fields)
    }
  };

  const removeInterface = async () => {
    try {
      await patch.mutateAsync({ [name]: null });
      onClose();
    } catch {
      // shown below
    }
  };

  // Like saveInterface (review N4), an edit diffs against the value the dialog opened with, never against a fresher
  // candidate: a field another session set meanwhile (e.g. F-bridge-l2's `l2`) is not in the stale dialog, and a diff
  // against the fresh value would send `l2: null` and delete it (F-bridge-l2 review #3, TD-22).
  const saveSub = async (id: string, value: unknown) => {
    setSubError(null);
    const base = subDialog?.value;
    const current = (await fresh())[parentName]?.subinterfaces?.[id];
    const body =
      current === undefined || base === undefined ? value : createMergePatch(base, value);
    try {
      await patch.mutateAsync({ [parentName]: { subinterfaces: { [id]: body } } });
      setSubDialog(null);
    } catch (e) {
      setSubError(e);
    }
  };

  const removeSub = async (id: string) => {
    setSubError(null);
    try {
      await patch.mutateAsync({ [parentName]: { subinterfaces: { [id]: null } } });
    } catch (e) {
      setSubError(e);
    }
  };

  const liveRows: [string, string][] = live
    ? [
        [t('live.type'), live.type],
        [t('live.swIfIndex'), String(live.swIfIndex)],
        [t('live.mac'), live.mac],
        [
          t('live.mtu'),
          `${fmt.integer(live.mtu)} (${t('live.linkMtu')} ${fmt.integer(live.linkMtu)})`,
        ],
        [t('live.addresses'), [...live.ipv4, ...live.ipv6].join(' ') || '—'],
        [t('live.vrf'), `${live.vrf} (${live.tableId})`],
        [t('live.rxMode'), live.rxMode || '—'],
      ]
    : [];

  return (
    <Box sx={{ p: 2 }} role="region" aria-label={t('drawer.label', { name })}>
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography
          component="h3"
          variant="h6"
          dir="ltr"
          sx={{ fontFamily: (th) => th.ngfw.monoFontFamily, flex: 1, textAlign: 'start' }}
        >
          {name}
        </Typography>
        {live && (
          <StatusChip
            size="small"
            status={adminStatus(live)!}
            label={`${t('col.admin')}: ${t(`status.${adminStatus(live)!}`)}`}
          />
        )}
        {live && (
          <StatusChip
            size="small"
            status={linkStatus(live)!}
            label={`${t('col.link')}: ${t(`status.${linkStatus(live)!}`)}`}
          />
        )}
        <IconButton aria-label={t('close')} onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Stack>
      {(state.isPending || candidate.isPending) && <LinearProgress aria-label={t('loading')} />}
      {!live && !item?.inventoryOnly && state.isSuccess && (
        <Alert severity="info">{t('drawer.notInVpp')}</Alert>
      )}
      {live && (
        <Table size="small" aria-label={t('drawer.liveTitle')} sx={{ mb: 2 }}>
          <TableBody>
            {liveRows.map(([k, v]) => (
              <TableRow key={k}>
                <TableCell component="th" sx={{ inlineSize: 180 }}>
                  {k}
                </TableCell>
                <TableCell dir="ltr" sx={{ textAlign: 'start' }}>
                  {v}
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell component="th">{t('live.rates')}</TableCell>
              <TableCell>
                {rate ? (
                  <Stack direction="row" gap={1} alignItems="center">
                    <span>{t('col.rx')}</span>
                    <bdi dir="ltr">{fmt.rate(rate.rxBps, BPS)}</bdi>
                    <span>{t('col.tx')}</span>
                    <bdi dir="ltr">{fmt.rate(rate.txBps, BPS)}</bdi>
                    <Sparkline values={rate.history} label={t('trendLabel', { name })} />
                  </Stack>
                ) : (
                  t('live.noRates')
                )}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      )}
      {live?.pppoe && <PppoePanel name={name} pppoe={live.pppoe} readOnly={readOnly} />}
      <Divider sx={{ mb: 2 }} />

      {item?.inventoryOnly ? (
        <>
          <Alert severity="info" sx={{ mb: 1 }}>
            {t(
              item.hostInventory?.isManagement
                ? 'inventory.managementInfo'
                : 'inventory.readOnlyInfo',
            )}
          </Alert>
          <Table size="small" aria-label={t('inventory.title')}>
            <TableBody>
              {[
                [t('inventory.netdev'), item.hostInventory?.netdev || '—'],
                [t('inventory.pci'), item.hostInventory?.pci || '—'],
                [t('inventory.driver'), item.hostInventory?.driver || '—'],
                [t('live.mac'), item.hostInventory?.mac || '—'],
                [t('col.link'), t(item.hostInventory?.linkUp ? 'status.up' : 'status.down')],
              ].map(([label, value]) => (
                <TableRow key={label}>
                  <TableCell component="th">{label}</TableCell>
                  <TableCell>
                    <bdi dir="ltr">{value}</bdi>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>
      ) : isSub ? (
        <Alert severity="info">{t('drawer.subHint', { parent: parentName })}</Alert>
      ) : (
        <>
          <Typography component="h4" variant="subtitle1" gutterBottom>
            {t('drawer.configTitle')}
          </Typography>
          {physical !== null && (
            <Alert severity="info" sx={{ mb: 1 }}>
              {physical.owner === 'host'
                ? t('physical.releasedInfo', { pci: physical.pci })
                : item?.awaitingDataplane
                  ? t('physical.awaitingInfo', { pci: physical.pci })
                  : t('physical.builtInInfo', { pci: physical.pci })}
            </Alert>
          )}
          {physical !== null && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {t('physical.summary', {
                pci: `\u2068${physical.pci}\u2069`,
                owner: t(`physical.owners.${physical.owner}`),
              })}
            </Typography>
          )}
          {!config && candidate.isSuccess && (
            <Alert severity="info" sx={{ mb: 1 }}>
              {t('drawer.notConfigured')}
            </Alert>
          )}
          {patch.isError && !subDialog && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
          {saved && !patch.isError && (
            <Alert severity="success" sx={{ mb: 1 }}>
              {t('drawer.saved')}
            </Alert>
          )}
          {changedElsewhere && (
            <Alert
              severity="warning"
              sx={{ mb: 1 }}
              action={
                <Button
                  color="inherit"
                  size="small"
                  onClick={() => {
                    setChangedElsewhere(false);
                    setSaved(false);
                    setReload((r) => r + 1);
                  }}
                >
                  {t('drawer.reload')}
                </Button>
              }
            >
              {t('drawer.changedElsewhere')}
            </Alert>
          )}
          {candidate.isSuccess && opened !== null && (
            <SchemaForm
              key={`${name}:${reload}`}
              schema={formSchema}
              value={formValue}
              readOnly={readOnly}
              submitLabel={t('drawer.save')}
              resetLabel={t('drawer.reset')}
              problem={problemFor(patch.error, `/interfaces/${esc(name)}`)}
              onSubmit={saveInterface}
            >
              {config && physical === null && (
                <Button
                  color="error"
                  variant="outlined"
                  startIcon={<DeleteIcon />}
                  disabled={readOnly || patch.isPending}
                  onClick={() => void removeInterface()}
                >
                  {t('drawer.remove')}
                </Button>
              )}
              {/* F-default-vpp-nics: built-in physical NICs are never deleted; they are released to / reclaimed from the host */}
              {config && physical !== null && (
                <Button
                  color={physical.owner === 'dataplane' ? 'warning' : 'primary'}
                  variant="outlined"
                  disabled={readOnly || setOwner.isPending}
                  onClick={() => setOwnerAction(nextOwner)}
                >
                  {physical.owner === 'dataplane' ? t('physical.release') : t('physical.reclaim')}
                </Button>
              )}
            </SchemaForm>
          )}

          <Divider sx={{ my: 2 }} />
          <SubinterfaceTable
            parent={name}
            subs={subs}
            items={state.data?.items}
            readOnly={readOnly}
            canAdd={config !== undefined}
            busy={patch.isPending}
            error={subDialog ? null : subError}
            onAdd={() => setSubDialog({ id: '', value: undefined })}
            onEdit={(id, value) => setSubDialog({ id, value })}
            onRemove={(id) => void removeSub(id)}
          />
        </>
      )}

      {/* F-default-vpp-nics: release / reclaim confirmation — the change applies with the next dataplane apply */}
      <Dialog
        open={ownerAction !== null}
        onClose={() => setOwnerAction(null)}
        fullWidth
        maxWidth="xs"
      >
        <DialogTitle>
          {ownerAction === 'host' ? t('physical.release') : t('physical.reclaim')}
        </DialogTitle>
        <DialogContent>
          <Typography sx={{ mb: 1 }}>
            {ownerAction === 'host'
              ? t('physical.releaseConfirm', { name })
              : t('physical.reclaimConfirm', { name })}
          </Typography>
          <Alert severity="warning">{t('physical.applyHint')}</Alert>
          {setOwner.isError && <ProblemAlert error={setOwner.error} sx={{ mt: 1 }} />}
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setOwner.reset();
              setOwnerAction(null);
            }}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="contained"
            color={ownerAction === 'host' ? 'warning' : 'primary'}
            disabled={setOwner.isPending}
            onClick={() => {
              if (physical && ownerAction) {
                setOwner.mutate(
                  { name, pci: physical.pci, owner: ownerAction },
                  { onSuccess: () => setOwnerAction(null) },
                );
              }
            }}
          >
            {ownerAction === 'host' ? t('physical.release') : t('physical.reclaim')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={subDialog !== null} onClose={() => setSubDialog(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {subDialog?.value
            ? t('sub.editTitle', { name: `${name}.${subDialog.id}` })
            : t('sub.addTitle', { name })}
        </DialogTitle>
        <DialogContent>
          {subDialog && (
            <SubForm
              key={subDialog.id || 'new'}
              initialId={subDialog.id}
              editing={subDialog.value !== undefined}
              existingIds={subs.map(([id]) => id)}
              value={subDialog.value}
              schema={subSchema}
              pointerPrefix={(id) => `/interfaces/${esc(parentName)}/subinterfaces/${esc(id)}`}
              error={subError}
              pending={patch.isPending}
              onCancel={() => setSubDialog(null)}
              onSubmit={(id, v) => void saveSub(id, v)}
            />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}

function SubForm({
  initialId,
  editing,
  existingIds,
  value,
  schema,
  pointerPrefix,
  error,
  pending,
  onCancel,
  onSubmit,
}: {
  initialId: string;
  editing: boolean;
  existingIds: string[];
  value: SubinterfaceConfig | undefined;
  schema: ReturnType<typeof subinterfaceSchema>;
  pointerPrefix: (id: string) => string;
  error: unknown;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (id: string, value: unknown) => void;
}) {
  const { t } = useTranslation('interfaces');
  const [id, setId] = useState(initialId);
  // review N5: server pointers are mapped with the id typed here (not the empty id of "add"), and "add" never
  // silently merges over an existing sub-interface
  const taken = !editing && existingIds.includes(id);
  const idOk = SUB_ID_RE.test(id) && !taken;
  const problem = problemFor(error, pointerPrefix(id));
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('sub.id')}
        value={id}
        disabled={editing}
        onChange={(e) => setId(e.target.value)}
        error={id !== '' && !idOk}
        helperText={taken ? t('sub.idExists', { id }) : t('sub.idHelp')}
        slotProps={{ htmlInput: SUB_ID_INPUT }}
      />
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={value}
        problem={problem}
        submitLabel={t('drawer.save')}
        resetLabel={t('drawer.reset')}
        onSubmit={(v) => {
          if (idOk && !pending) onSubmit(id, v);
        }}
      >
        <Button onClick={onCancel}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}

type PppoeLive = NonNullable<NonNullable<InterfaceItem['state']>['pppoe']>;

/** F-pppoe-client: the live PPPoE session on a WAN interface, with a Reconnect action. */
function PppoePanel({
  name,
  pppoe,
  readOnly,
}: {
  name: string;
  pppoe: PppoeLive;
  readOnly: boolean;
}) {
  const { t } = useTranslation('interfaces');
  const reconnect = usePppoeReconnect();
  const phase = pppoe.phase === 'up' ? 'up' : pppoe.phase === 'failed' ? 'degraded' : 'down';
  const rows: [string, string][] = [];
  if (pppoe.localIpv4) rows.push([t('pppoe.local'), pppoe.localIpv4]);
  if (pppoe.peerIpv4) rows.push([t('pppoe.peer'), pppoe.peerIpv4]);
  if (pppoe.ipv6) rows.push([t('pppoe.ipv6'), pppoe.ipv6]);
  if (pppoe.dns.length > 0) rows.push([t('pppoe.dns'), pppoe.dns.join(', ')]);
  if (pppoe.since) rows.push([t('pppoe.since'), pppoe.since]);
  if (pppoe.sessionId > 0) rows.push([t('pppoe.sessionId'), String(pppoe.sessionId)]);
  if (pppoe.failCount > 0) rows.push([t('pppoe.failCount'), String(pppoe.failCount)]);
  if (pppoe.lastError) rows.push([t('pppoe.lastError'), pppoe.lastError]);
  return (
    <>
      <Divider sx={{ mb: 2 }} />
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography component="h4" variant="subtitle1" sx={{ flex: 1 }}>
          {t('pppoe.title')}
        </Typography>
        <StatusChip
          size="small"
          status={phase}
          label={`${t('pppoe.phase')}: ${t(`pppoe.phases.${pppoe.phase}`, { defaultValue: pppoe.phase })}`}
        />
        <Button
          size="small"
          variant="outlined"
          disabled={readOnly || reconnect.isPending}
          onClick={() => reconnect.mutate(name)}
        >
          {t('pppoe.reconnect')}
        </Button>
      </Stack>
      {reconnect.isError && <ProblemAlert error={reconnect.error} sx={{ mb: 1 }} />}
      {reconnect.isSuccess && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('pppoe.reconnecting')}
        </Alert>
      )}
      {rows.length > 0 && (
        <Table size="small" aria-label={t('pppoe.title')} sx={{ mb: 2 }}>
          <TableBody>
            {rows.map(([k, v]) => (
              <TableRow key={k}>
                <TableCell component="th" sx={{ inlineSize: 180 }}>
                  {k}
                </TableCell>
                <TableCell dir="ltr" sx={{ textAlign: 'start' }}>
                  {v}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </>
  );
}
