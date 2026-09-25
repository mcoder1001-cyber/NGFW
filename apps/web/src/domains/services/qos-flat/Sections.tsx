import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
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
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import type { Theme } from '@mui/material/styles';
import { StatusChip, useFormatters, type VrxStatus } from '@ngfw/ui-kit';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { createMergePatch } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { AttachmentDialog, type AttachmentTarget } from './AttachmentDialog';
import { MapDialog, type MapTarget } from './MapDialog';
import {
  attachmentRows,
  burstFor,
  counterText,
  localize,
  mapRows,
  nestPatch,
  policerItemSchema,
  policerRows,
  QOS_SOURCES,
  shaperItemSchema,
  shaperRows,
  type PolicerItem,
  type QosCfg,
  type RowStatus,
} from './model';
import {
  useCandidateServices,
  useFreshServices,
  usePatchServices,
  useQosState,
  useResetPolicer,
} from './queries';
import { SchemaDialog, type EditTarget } from './SchemaDialog';

const MONO = { fontFamily: (th: Theme) => th.vrx.monoFontFamily, fontSize: 13 } as const;

// Non-UI literals (statuses, document paths) live here, outside JSX (i18next/no-literal-string).
const ST_UP: VrxStatus = 'up';
const ST_DOWN: VrxStatus = 'down';
const ST_DEGRADED: VrxStatus = 'degraded';
const POLICERS = 'policers';
const SHAPERS = 'shapers';
const MAPS = 'maps';
const INTERFACES = 'interfaces';
const UNITS = ['kbps', 'pps'] as const;
const NUMERIC_LTR = { dir: 'ltr', inputMode: 'numeric' } as const;
const qosPath = (collection: string, name: string) => ['qos', collection, name];

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function getAt(root: unknown, path: readonly string[]): unknown {
  let cur: unknown = root;
  for (const p of path) cur = isObject(cur) ? cur[p] : undefined;
  return cur;
}

/** Shared editing plumbing: the candidate `services`, merge patches below it, interface choices, live state. */
function useQosEditor() {
  const candidate = useCandidateServices();
  const fresh = useFreshServices();
  const patch = usePatchServices();
  const ifaces = useCandidateInterfaces();
  const state = useQosState();
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
   * Save `value` at services/<path>: only what changed against the current candidate. The form value is sent as-is
   * (no dropPhantomOptionals — WEB-1 deprecates it).
   */
  const save = async (path: string[], value: unknown): Promise<boolean> => {
    setError(null);
    const current = getAt(await fresh(), path);
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
  const qos: QosCfg | undefined = candidate.data?.qos;
  return {
    candidate,
    state,
    qos,
    patch,
    save,
    remove,
    error,
    setError,
    readOnly: !perms.editConfig,
    interfaceOptions,
  };
}

type Editor = ReturnType<typeof useQosEditor>;

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
      <Button size="small" variant="contained" startIcon={<AddIcon />} disabled={disabled} onClick={onAdd}>
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
  children,
}: {
  name: string;
  onEdit: (() => void) | undefined;
  onRemove: (() => void) | undefined;
  disabled: boolean;
  children?: ReactNode;
}) {
  const { t } = useTranslation('qos-flat');
  return (
    <TableCell sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
      {children}
      {onEdit && (
        <IconButton size="small" aria-label={t('actions.edit', { name })} disabled={disabled} onClick={onEdit}>
          <EditIcon fontSize="small" />
        </IconButton>
      )}
      {onRemove && (
        <IconButton size="small" aria-label={t('actions.remove', { name })} disabled={disabled} onClick={onRemove}>
          <DeleteIcon fontSize="small" />
        </IconButton>
      )}
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
  const { t } = useTranslation('qos-flat');
  return (
    <Box>
      {ed.candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {ed.candidate.isError && <ProblemAlert error={ed.candidate.error} sx={{ mb: 1 }} />}
      {ed.state.isError && (
        <Alert severity="warning" sx={{ mb: 1 }}>
          {t('state.unavailable')}
        </Alert>
      )}
      {ed.state.data?.countersError && (
        <Alert severity="warning" sx={{ mb: 1 }}>
          {t('state.countersError', { error: ed.state.data.countersError })}
        </Alert>
      )}
      {children}
    </Box>
  );
}

/** Applied / not in VPP / pending / unmanaged, from the live state and the candidate. */
function StateChip({ status }: { status: RowStatus }) {
  const { t } = useTranslation('qos-flat');
  switch (status) {
    case 'applied':
      return <StatusChip size="small" status={ST_UP} label={t('status.applied')} />;
    case 'missing':
      return <StatusChip size="small" status={ST_DOWN} label={t('status.missing')} />;
    case 'unmanaged':
      return <StatusChip size="small" status={ST_DEGRADED} label={t('status.unmanaged')} />;
    default:
      return <Chip size="small" variant="outlined" label={t('status.pending')} />;
  }
}

/** Conform / exceed / violate packets of a live policer (empty while it is not in VPP). */
function Counters({ item }: { item: PolicerItem | undefined }) {
  const { t } = useTranslation('qos-flat');
  const fmt = useFormatters();
  if (!item?.present) return <TableCell />;
  const n = (x: number) => fmt.integer(x);
  return (
    <TableCell dir="ltr" sx={{ ...MONO, whiteSpace: 'nowrap' }}>
      <span title={t('counters.conform')}>{counterText(item.conform, n)}</span>
      {' / '}
      <span title={t('counters.exceed')}>{counterText(item.exceed, n)}</span>
      {' / '}
      <span title={t('counters.violate')}>{counterText(item.violate, n)}</span>
    </TableCell>
  );
}

/** Reset button + confirmation (policer_reset refills the token buckets; VPP keeps the counters). */
function ResetButton({ vppName, label, disabled }: { vppName: string; label: string; disabled: boolean }) {
  const { t } = useTranslation('qos-flat');
  const reset = useResetPolicer();
  const [open, setOpen] = useState(false);
  return (
    <>
      <IconButton
        size="small"
        aria-label={t('actions.reset', { name: label })}
        disabled={disabled}
        onClick={() => {
          reset.reset();
          setOpen(true);
        }}
      >
        <RestartAltIcon fontSize="small" />
      </IconButton>
      <Dialog open={open} onClose={() => setOpen(false)}>
        <DialogTitle>{t('reset.title', { name: label })}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('reset.body')}</DialogContentText>
          {reset.isError && <ProblemAlert error={reset.error} sx={{ mt: 1 }} />}
          {reset.isSuccess && (
            <Alert severity="success" sx={{ mt: 1 }}>
              {t('reset.done', { name: label })}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>{reset.isSuccess ? t('dialog.close') : t('dialog.cancel')}</Button>
          {!reset.isSuccess && (
            <Button variant="contained" disabled={reset.isPending} onClick={() => reset.mutate(vppName)}>
              {t('reset.confirm')}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </>
  );
}

// ─── Policers ────────────────────────────────────────────────────────────────

/** The rate/burst helper of the policer form: burst = rate × window (bytes for kbit/s, packets for packets/s). */
function BurstHelper({ apply, value }: { apply: (patch: Record<string, unknown>) => void; value: unknown }) {
  const { t } = useTranslation('qos-flat');
  const v = isObject(value) ? value : {};
  const [unit, setUnit] = useState<'kbps' | 'pps'>(v['rateUnit'] === 'pps' ? 'pps' : 'kbps');
  const [rate, setRate] = useState(typeof v['cir'] === 'number' ? String(v['cir']) : '');
  const [windowMs, setWindowMs] = useState('10');
  const burst = burstFor(Number(rate), unit, Number(windowMs));
  return (
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        {t('helper.title')}
      </Typography>
      <Stack direction="row" gap={1.5} alignItems="center" flexWrap="wrap">
        <TextField
          size="small"
          select
          label={t('helper.unit')}
          value={unit}
          onChange={(e) => setUnit(UNITS.find((u) => u === e.target.value) ?? UNITS[0])}
          sx={{ inlineSize: 140 }}
        >
          {UNITS.map((u) => (
            <MenuItem key={u} value={u}>
              {t(`unit.${u}`)}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('helper.rate')}
          value={rate}
          onChange={(e) => setRate(e.target.value.trim())}
          slotProps={{ htmlInput: NUMERIC_LTR }}
          sx={{ inlineSize: 160 }}
        />
        <TextField
          size="small"
          label={t('helper.window')}
          value={windowMs}
          onChange={(e) => setWindowMs(e.target.value.trim())}
          slotProps={{ htmlInput: NUMERIC_LTR }}
          sx={{ inlineSize: 140 }}
        />
        <Typography variant="body2" aria-live="polite">
          {unit === 'pps' ? t('helper.resultPackets', { n: burst }) : t('helper.resultBytes', { n: burst })}
        </Typography>
        <Button
          size="small"
          variant="outlined"
          disabled={burst <= 0}
          onClick={() => apply({ rateUnit: unit, cir: Number(rate), cb: burst })}
        >
          {t('helper.use')}
        </Button>
      </Stack>
      <Typography variant="caption" color="text.secondary">
        {t('helper.note')}
      </Typography>
    </Paper>
  );
}

export function PolicersTab() {
  const { t } = useTranslation('qos-flat');
  const ed = useQosEditor();
  const schema = useMemo(() => localize(policerItemSchema(), (k, o) => t(k, o ?? {}), 'policer'), [t]);
  const [target, setTarget] = useState<EditTarget | null>(null);
  const rows = policerRows(ed.qos, ed.state.data?.items);
  const submit = async (name: string, value: unknown) => {
    if (await ed.save(qosPath(POLICERS, name), value)) setTarget(null);
  };
  return (
    <Frame ed={ed}>
      <Toolbar
        title={t('policers.title')}
        addLabel={t('policers.add')}
        disabled={ed.readOnly}
        onAdd={() => setTarget({ name: '', value: undefined, editing: false })}
      />
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      <Table size="small" aria-label={t('policers.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.name')}</TableCell>
            <TableCell>{t('col.algorithm')}</TableCell>
            <TableCell>{t('col.rates')}</TableCell>
            <TableCell>{t('col.bursts')}</TableCell>
            <TableCell>{t('col.counters')}</TableCell>
            <TableCell>{t('col.status')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={7} text={t('policers.none')} />}
          {rows.map((r) => (
            <TableRow key={r.name} hover>
              <TableCell dir="ltr" sx={MONO}>
                {r.name}
              </TableCell>
              <TableCell dir="ltr">{r.type}</TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.eir !== undefined
                  ? t('rates.two', { cir: r.cir, eir: r.eir, unit: t(`unit.${r.rateUnit}`) })
                  : t('rates.one', { cir: r.cir, unit: t(`unit.${r.rateUnit}`) })}
              </TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.eb !== undefined ? t('bursts.two', { cb: r.cb, eb: r.eb }) : t('bursts.one', { cb: r.cb })}
              </TableCell>
              <Counters item={r.state} />
              <TableCell>
                <StateChip status={r.status} />
              </TableCell>
              <RowActions
                name={r.name}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={r.cfg ? () => setTarget({ name: r.name, value: r.cfg, editing: true }) : undefined}
                onRemove={r.cfg ? () => void ed.remove(qosPath(POLICERS, r.name)) : undefined}
              >
                {r.state?.present && (
                  <ResetButton vppName={r.state.vppName} label={r.name} disabled={ed.readOnly} />
                )}
              </RowActions>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <SchemaDialog
        open={target !== null}
        title={target?.editing ? t('policers.editTitle', { name: target.name }) : t('policers.addTitle')}
        target={target}
        existing={rows.map((r) => r.name)}
        schema={schema}
        collection={POLICERS}
        error={ed.error}
        pending={ed.patch.isPending}
        extra={(apply, value) => <BurstHelper apply={apply} value={value} />}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(name, v) => void submit(name, v)}
      />
    </Frame>
  );
}

// ─── Rate limits (shapers) ───────────────────────────────────────────────────

export function ShapersTab() {
  const { t } = useTranslation('qos-flat');
  const fmt = useFormatters();
  const ed = useQosEditor();
  const schema = useMemo(() => localize(shaperItemSchema(), (k, o) => t(k, o ?? {}), 'shaper'), [t]);
  const [target, setTarget] = useState<EditTarget | null>(null);
  const rows = shaperRows(ed.qos, ed.state.data?.items);
  const submit = async (name: string, value: unknown) => {
    if (await ed.save(qosPath(SHAPERS, name), value)) setTarget(null);
  };
  return (
    <Frame ed={ed}>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('shapers.caveat')}
      </Alert>
      <Toolbar
        title={t('shapers.title')}
        addLabel={t('shapers.add')}
        disabled={ed.readOnly}
        onAdd={() => setTarget({ name: '', value: undefined, editing: false })}
      />
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      <Table size="small" aria-label={t('shapers.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.name')}</TableCell>
            <TableCell>{t('col.rate')}</TableCell>
            <TableCell>{t('col.burst')}</TableCell>
            <TableCell>{t('col.counters')}</TableCell>
            <TableCell>{t('col.status')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={6} text={t('shapers.none')} />}
          {rows.map((r) => (
            <TableRow key={r.name} hover>
              <TableCell dir="ltr" sx={MONO}>
                {r.name}
              </TableCell>
              <TableCell dir="ltr">{t('rates.one', { cir: r.rateKbps, unit: t('unit.kbps') })}</TableCell>
              <TableCell dir="ltr">
                {r.burstBytes !== undefined && r.cfg?.burstBytes !== undefined
                  ? t('bursts.bytes', { n: fmt.integer(r.burstBytes) })
                  : t('shapers.autoBurst', { n: fmt.integer(r.derivedBurst ?? 0) })}
              </TableCell>
              <Counters item={r.state} />
              <TableCell>
                <StateChip status={r.status} />
              </TableCell>
              <RowActions
                name={r.name}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={r.cfg ? () => setTarget({ name: r.name, value: r.cfg, editing: true }) : undefined}
                onRemove={r.cfg ? () => void ed.remove(qosPath(SHAPERS, r.name)) : undefined}
              >
                {r.state?.present && (
                  <ResetButton vppName={r.state.vppName} label={r.name} disabled={ed.readOnly} />
                )}
              </RowActions>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <SchemaDialog
        open={target !== null}
        title={target?.editing ? t('shapers.editTitle', { name: target.name }) : t('shapers.addTitle')}
        target={target}
        existing={rows.map((r) => r.name)}
        schema={schema}
        collection={SHAPERS}
        error={ed.error}
        pending={ed.patch.isPending}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(name, v) => void submit(name, v)}
      />
    </Frame>
  );
}

// ─── Marking maps ────────────────────────────────────────────────────────────

export function MapsTab() {
  const { t } = useTranslation('qos-flat');
  const ed = useQosEditor();
  const [target, setTarget] = useState<MapTarget | null>(null);
  const rows = mapRows(ed.qos);
  const submit = async (name: string, value: Record<string, unknown>) => {
    // rows are replaced as a whole (a merge patch cannot remove one array entry)
    if (await ed.save(qosPath(MAPS, name), value)) setTarget(null);
  };
  return (
    <Frame ed={ed}>
      <Toolbar
        title={t('maps.title')}
        addLabel={t('maps.add')}
        disabled={ed.readOnly}
        onAdd={() => setTarget({ name: '', cfg: undefined, editing: false })}
      />
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      <Table size="small" aria-label={t('maps.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.name')}</TableCell>
            <TableCell>{t('col.mapId')}</TableCell>
            <TableCell>{t('col.rows')}</TableCell>
            <TableCell>{t('col.usedBy')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={5} text={t('maps.none')} />}
          {rows.map((r) => (
            <TableRow key={r.name} hover>
              <TableCell dir="ltr" sx={MONO}>
                {r.name}
              </TableCell>
              <TableCell dir="ltr">{r.id !== undefined ? r.id : t('maps.autoId')}</TableCell>
              <TableCell>
                {QOS_SOURCES.filter((s) => r.entries[s] !== undefined)
                  .map((s) => t('maps.rowSummary', { source: t(`source.${s}`), n: r.entries[s] }))
                  .join(t('listSeparator'))}
              </TableCell>
              <TableCell dir="ltr" sx={MONO}>
                {r.usedBy.join(' ')}
              </TableCell>
              <RowActions
                name={r.name}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={() => setTarget({ name: r.name, cfg: r.cfg, editing: true })}
                onRemove={() => void ed.remove(qosPath(MAPS, r.name))}
              />
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <MapDialog
        target={target}
        existing={rows.map((r) => r.name)}
        error={ed.error}
        pending={ed.patch.isPending}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(name, v) => void submit(name, v)}
      />
    </Frame>
  );
}

// ─── Interface attachments ───────────────────────────────────────────────────

export function AttachmentsTab() {
  const { t } = useTranslation('qos-flat');
  const ed = useQosEditor();
  const [target, setTarget] = useState<AttachmentTarget | null>(null);
  const rows = attachmentRows(ed.qos);
  const submit = async (name: string, value: Record<string, unknown>) => {
    if (await ed.save(qosPath(INTERFACES, name), value)) setTarget(null);
  };
  const dash = (v: string | undefined) => v ?? t('none');
  return (
    <Frame ed={ed}>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('interfaces.writeOnlyNote')}
      </Alert>
      <Toolbar
        title={t('interfaces.title')}
        addLabel={t('interfaces.add')}
        disabled={ed.readOnly}
        onAdd={() => setTarget({ name: '', cfg: undefined, editing: false })}
      />
      {ed.error !== null && target === null && <ProblemAlert error={ed.error} sx={{ mb: 1 }} />}
      <Table size="small" aria-label={t('interfaces.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('col.interface')}</TableCell>
            <TableCell>{t('col.ingress')}</TableCell>
            <TableCell>{t('col.egress')}</TableCell>
            <TableCell>{t('col.record')}</TableCell>
            <TableCell>{t('col.store')}</TableCell>
            <TableCell>{t('col.mark')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && <EmptyRow cols={7} text={t('interfaces.nothing')} />}
          {rows.map((r) => (
            <TableRow key={r.name} hover>
              <TableCell dir="ltr" sx={MONO}>
                {r.name}
              </TableCell>
              <TableCell dir="ltr">{dash(r.input)}</TableCell>
              <TableCell dir="ltr">
                {r.shaper
                  ? t('interfaces.egressShaperValue', { name: r.shaper })
                  : dash(r.output)}
              </TableCell>
              <TableCell>{r.record ? t(`source.${r.record}`) : t('none')}</TableCell>
              <TableCell>
                {r.store
                  ? t('interfaces.storeValueText', { source: t(`source.${r.store.source ?? 'ip'}`), value: r.store.value })
                  : t('none')}
              </TableCell>
              <TableCell>
                {r.mark
                  ? t('interfaces.markValueText', { map: r.mark.map, output: t(`source.${r.mark.output ?? 'ip'}`) })
                  : t('none')}
              </TableCell>
              <RowActions
                name={r.name}
                disabled={ed.readOnly || ed.patch.isPending}
                onEdit={() => setTarget({ name: r.name, cfg: r.cfg, editing: true })}
                onRemove={() => void ed.remove(qosPath(INTERFACES, r.name))}
              />
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <AttachmentDialog
        target={target}
        existing={rows.map((r) => r.name)}
        interfaceOptions={ed.interfaceOptions}
        policers={Object.keys(ed.qos?.policers ?? {}).sort()}
        shapers={Object.keys(ed.qos?.shapers ?? {}).sort()}
        maps={Object.keys(ed.qos?.maps ?? {}).sort()}
        error={ed.error}
        pending={ed.patch.isPending}
        onCancel={() => {
          setTarget(null);
          ed.setError(null);
        }}
        onSubmit={(name, v) => void submit(name, v)}
      />
    </Frame>
  );
}
