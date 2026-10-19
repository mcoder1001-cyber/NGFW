import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
  PgRestartAltIcon as RestartAltIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogContentText as DialogContentText,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgLinearProgress as LinearProgress,
  PgMenuItem as MenuItem,
  PgPaper as Paper,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import type { Theme } from '@amnafzar/common-ui-lib/styles';
import { StatusChip, useFormatters, type NgfwStatus } from '@ngfw/ui-kit';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
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

const MONO = { fontFamily: (th: Theme) => th.ngfw.monoFontFamily, fontSize: 13 } as const;

// Non-UI literals (statuses, document paths) live here, outside JSX (i18next/no-literal-string).
const ST_UP: NgfwStatus = 'up';
const ST_DOWN: NgfwStatus = 'down';
const ST_DEGRADED: NgfwStatus = 'degraded';
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
  children,
}: {
  t: ReturnType<typeof useTranslation>['t'];
  name: string;
  onEdit: (() => void) | undefined;
  onRemove: (() => void) | undefined;
  disabled: boolean;
  children?: ReactNode;
}) {
  return (
    <Box component="span" sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
      {children}
      {onEdit && (
        <IconButton
          size="small"
          aria-label={t('actions.edit', { name })}
          disabled={disabled}
          onClick={onEdit}
        >
          <EditIcon fontSize="small" />
        </IconButton>
      )}
      {onRemove && (
        <IconButton
          size="small"
          aria-label={t('actions.remove', { name })}
          disabled={disabled}
          onClick={onRemove}
        >
          <DeleteIcon fontSize="small" />
        </IconButton>
      )}
    </Box>
  );
}

function renderEmptyContent({ text }: { text: string }) {
  return <Typography color="text.secondary">{text}</Typography>;
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
          {t('state.countersError', { error: serviceText(ed.state.data.countersError) })}
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
function renderCounters({
  t,
  fmt,
  item,
}: {
  t: ReturnType<typeof useTranslation>['t'];
  fmt: ReturnType<typeof useFormatters>;
  item: PolicerItem | undefined;
}) {
  if (!item?.present) return <></>;
  const n = (x: number) => fmt.integer(x);
  return (
    <Box component="span" dir="ltr" sx={{ ...MONO, whiteSpace: 'nowrap' }}>
      <span title={t('counters.conform')}>{counterText(item.conform, n)}</span>
      {' / '}
      <span title={t('counters.exceed')}>{counterText(item.exceed, n)}</span>
      {' / '}
      <span title={t('counters.violate')}>{counterText(item.violate, n)}</span>
    </Box>
  );
}

/** Reset button + confirmation (policer_reset refills the token buckets; VPP keeps the counters). */
function ResetButton({
  vppName,
  label,
  disabled,
}: {
  vppName: string;
  label: string;
  disabled: boolean;
}) {
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
          <Button onClick={() => setOpen(false)}>
            {reset.isSuccess ? t('dialog.close') : t('dialog.cancel')}
          </Button>
          {!reset.isSuccess && (
            <Button
              variant="contained"
              disabled={reset.isPending}
              onClick={() => reset.mutate(vppName)}
            >
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
function BurstHelper({
  apply,
  value,
}: {
  apply: (patch: Record<string, unknown>) => void;
  value: unknown;
}) {
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
          {unit === 'pps'
            ? t('helper.resultPackets', { n: burst })
            : t('helper.resultBytes', { n: burst })}
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
  const fmt = useFormatters();
  const ed = useQosEditor();
  const schema = useMemo(
    () => localize(policerItemSchema(), (k, o) => t(k, o ?? {}), 'policer'),
    [t],
  );
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
                key: 'algorithm',
                headerRenderer: () => <>{t('col.algorithm')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.type}</>,
              },
              {
                key: 'rates',
                headerRenderer: () => <>{t('col.rates')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.eir !== undefined
                      ? t('rates.two', { cir: r.cir, eir: r.eir, unit: t(`unit.${r.rateUnit}`) })
                      : t('rates.one', { cir: r.cir, unit: t(`unit.${r.rateUnit}`) })}
                  </>
                ),
              },
              {
                key: 'bursts',
                headerRenderer: () => <>{t('col.bursts')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.eb !== undefined
                      ? t('bursts.two', { cb: r.cb, eb: r.eb })
                      : t('bursts.one', { cb: r.cb })}
                  </>
                ),
              },
              {
                key: 'counters',
                headerRenderer: () => <>{t('col.counters')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{renderCounters({ item: r.state, t, fmt })}</>
                ),
              },
              {
                key: 'status',
                headerRenderer: () => <>{t('col.status')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    <StateChip status={r.status} />
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
                      onEdit: r.cfg
                        ? () => setTarget({ name: r.name, value: r.cfg, editing: true })
                        : undefined,
                      onRemove: r.cfg ? () => void ed.remove(qosPath(POLICERS, r.name)) : undefined,
                      t,
                      children: (
                        <>
                          {r.state?.present && (
                            <ResetButton
                              vppName={r.state.vppName}
                              label={r.name}
                              disabled={ed.readOnly}
                            />
                          )}
                        </>
                      ),
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.name;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'name':
                  return { dir: 'ltr', sx: MONO };
                case 'algorithm':
                  return { dir: 'ltr' };
                case 'rates':
                  return { dir: 'ltr', sx: MONO };
                case 'bursts':
                  return { dir: 'ltr', sx: MONO };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('policers.title') }}
            emptyContent={renderEmptyContent({ text: t('policers.none') })}
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
      <SchemaDialog
        open={target !== null}
        title={
          target?.editing ? t('policers.editTitle', { name: target.name }) : t('policers.addTitle')
        }
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
  const schema = useMemo(
    () => localize(shaperItemSchema(), (k, o) => t(k, o ?? {}), 'shaper'),
    [t],
  );
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
                key: 'rate',
                headerRenderer: () => <>{t('col.rate')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{t('rates.one', { cir: r.rateKbps, unit: t('unit.kbps') })}</>
                ),
              },
              {
                key: 'burst',
                headerRenderer: () => <>{t('col.burst')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.burstBytes !== undefined && r.cfg?.burstBytes !== undefined
                      ? t('bursts.bytes', { n: fmt.integer(r.burstBytes) })
                      : t('shapers.autoBurst', { n: fmt.integer(r.derivedBurst ?? 0) })}
                  </>
                ),
              },
              {
                key: 'counters',
                headerRenderer: () => <>{t('col.counters')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{renderCounters({ item: r.state, t, fmt })}</>
                ),
              },
              {
                key: 'status',
                headerRenderer: () => <>{t('col.status')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    <StateChip status={r.status} />
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
                      onEdit: r.cfg
                        ? () => setTarget({ name: r.name, value: r.cfg, editing: true })
                        : undefined,
                      onRemove: r.cfg ? () => void ed.remove(qosPath(SHAPERS, r.name)) : undefined,
                      t,
                      children: (
                        <>
                          {r.state?.present && (
                            <ResetButton
                              vppName={r.state.vppName}
                              label={r.name}
                              disabled={ed.readOnly}
                            />
                          )}
                        </>
                      ),
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.name;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'name':
                  return { dir: 'ltr', sx: MONO };
                case 'rate':
                  return { dir: 'ltr' };
                case 'burst':
                  return { dir: 'ltr' };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('shapers.title') }}
            emptyContent={renderEmptyContent({ text: t('shapers.none') })}
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
      <SchemaDialog
        open={target !== null}
        title={
          target?.editing ? t('shapers.editTitle', { name: target.name }) : t('shapers.addTitle')
        }
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
                key: 'mapId',
                headerRenderer: () => <>{t('col.mapId')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.id !== undefined ? r.id : t('maps.autoId')}</>
                ),
              },
              {
                key: 'rows',
                headerRenderer: () => <>{t('col.rows')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {QOS_SOURCES.filter((s) => r.entries[s] !== undefined)
                      .map((s) =>
                        t('maps.rowSummary', { source: t(`source.${s}`), n: r.entries[s] }),
                      )
                      .join(t('listSeparator'))}
                  </>
                ),
              },
              {
                key: 'usedBy',
                headerRenderer: () => <>{t('col.usedBy')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.usedBy.join(' ')}</>
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
                      onEdit: () => setTarget({ name: r.name, cfg: r.cfg, editing: true }),
                      onRemove: () => void ed.remove(qosPath(MAPS, r.name)),
                      t,
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.name;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'name':
                  return { dir: 'ltr', sx: MONO };
                case 'mapId':
                  return { dir: 'ltr' };
                case 'usedBy':
                  return { dir: 'ltr', sx: MONO };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('maps.title') }}
            emptyContent={renderEmptyContent({ text: t('maps.none') })}
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
                key: 'interface',
                headerRenderer: () => <>{t('col.interface')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.name}</>,
              },
              {
                key: 'ingress',
                headerRenderer: () => <>{t('col.ingress')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{dash(r.input)}</>
                ),
              },
              {
                key: 'egress',
                headerRenderer: () => <>{t('col.egress')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.shaper
                      ? t('interfaces.egressShaperValue', { name: r.shaper })
                      : dash(r.output)}
                  </>
                ),
              },
              {
                key: 'record',
                headerRenderer: () => <>{t('col.record')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>{r.record ? t(`source.${r.record}`) : t('none')}</>
                ),
              },
              {
                key: 'store',
                headerRenderer: () => <>{t('col.store')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.store
                      ? t('interfaces.storeValueText', {
                          source: t(`source.${r.store.source ?? 'ip'}`),
                          value: r.store.value,
                        })
                      : t('none')}
                  </>
                ),
              },
              {
                key: 'mark',
                headerRenderer: () => <>{t('col.mark')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                  <>
                    {r.mark
                      ? t('interfaces.markValueText', {
                          map: r.mark.map,
                          output: t(`source.${r.mark.output ?? 'ip'}`),
                        })
                      : t('none')}
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
                      onEdit: () => setTarget({ name: r.name, cfg: r.cfg, editing: true }),
                      onRemove: () => void ed.remove(qosPath(INTERFACES, r.name)),
                      t,
                    })}
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { r } = item as (typeof nativeRows)[number];
              return r.name;
            }}
            rowProps={() => {
              return { hover: true };
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'interface':
                  return { dir: 'ltr', sx: MONO };
                case 'ingress':
                  return { dir: 'ltr' };
                case 'egress':
                  return { dir: 'ltr' };
                case 'actions':
                  return { sx: { textAlign: 'end', whiteSpace: 'nowrap' } };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('interfaces.title') }}
            emptyContent={renderEmptyContent({ text: t('interfaces.nothing') })}
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
