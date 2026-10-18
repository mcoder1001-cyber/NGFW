import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgArrowDownwardIcon as ArrowDownwardIcon,
  PgArrowUpwardIcon as ArrowUpwardIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
  PgRefreshIcon as RefreshIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgMenuItem as MenuItem,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTabs as Tabs,
  PgTooltip as Tooltip,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { canonicalPrefix, DEFAULT_VRF, jsonPointer } from '@ngfw/schema';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { createMergePatch, localizeSchema } from '../../interfaces/model';
import {
  canon,
  canonLocalSid,
  canonPolicy,
  canonSteering,
  effectiveEncapSource,
  globalsFormSchema,
  isIpv6,
  localSidFormSchema,
  localSidRows,
  move,
  policyFormSchema,
  policyRows,
  rowChip,
  sidListProblems,
  sortSteering,
  SRV6_MAX_SIDS,
  steeredBsids,
  steeringKey,
  steeringRows,
  trafficOf,
  type SidListDraft,
  type Srv6LocalSidConfig,
  type Srv6PolicyConfig,
  type Srv6State,
  type Srv6SteeringConfig,
  type Srv6View,
} from './model';
import {
  useCandidateInterfaceNames,
  useCandidateSrv6,
  useCandidateVrfNames,
  useFreshSrv6,
  usePatchSrv6,
  useSrv6State,
} from './queries';

const LTR = { dir: 'ltr' } as const;
const SUB_TABS = ['sids', 'policies', 'steering'] as const;
type SubTab = (typeof SUB_TABS)[number];
const STEER_TYPES = ['l3', 'l2'] as const;
const DEFAULT_VRFS: readonly string[] = [DEFAULT_VRF];
const SRV6_POINTER = jsonPointer('routing', 'srv6');
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** Server pointers under `prefix` → pointers relative to the edited form. */
function problemFor(error: unknown, prefix: string): ProblemDetails | null {
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

/**
 * The SRv6 tab of the VPN page (F-srv6): local SIDs with their counters, policies with their segment lists and the
 * steering entries, each joined with the live state of the data plane. Configuration is `routing.srv6` of the
 * candidate (generic config routes); the state is `GET /api/v1/state/srv6`, refreshed on demand or every 30 s (D-132).
 */
export function Srv6Page() {
  const { t } = useTranslation('srv6');
  const fmt = useFormatters();
  const state = useSrv6State();
  const candidate = useCandidateSrv6();
  const [tab, setTab] = useState<SubTab>('sids');
  const cfg = candidate.data;

  return (
    <Box>
      <Stack direction="row" spacing={1} sx={{ mb: 2, alignItems: 'center', flexWrap: 'wrap' }}>
        <Typography variant="h6" sx={{ flexGrow: 1 }}>
          {t('title')}
        </Typography>
        {state.data?.retrievedAt && (
          <Typography variant="body2" color="text.secondary">
            {t('retrievedAt', { at: fmt.dateTime(state.data.retrievedAt) })}
          </Typography>
        )}
        <Tooltip title={t('refreshHint')}>
          <span>
            <Button
              size="small"
              startIcon={<RefreshIcon />}
              onClick={() => void state.refetch()}
              disabled={state.isFetching}
            >
              {t('refresh')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      <Alert severity="info" sx={{ mb: 2 }} data-testid="srv6-proxy-note">
        {t('proxyNote')}
      </Alert>
      {state.error !== null && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
      {candidate.error !== null && <ProblemAlert error={candidate.error} sx={{ mb: 2 }} />}
      <Tabs
        value={tab}
        onChange={(_, v: SubTab) => setTab(v)}
        aria-label={t('sections')}
        variant="scrollable"
        sx={{ mb: 2 }}
      >
        {SUB_TABS.map((id) => (
          <Tab
            key={id}
            value={id}
            label={t(`tabs.${id}`)}
            id={`srv6-tab-${id}`}
            aria-controls="srv6-panel"
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id="srv6-panel" aria-labelledby={`srv6-tab-${tab}`}>
        {cfg !== undefined && tab === 'sids' && <LocalSidsPanel cfg={cfg} state={state.data} />}
        {cfg !== undefined && tab === 'policies' && <PoliciesPanel cfg={cfg} state={state.data} />}
        {cfg !== undefined && tab === 'steering' && <SteeringPanel cfg={cfg} state={state.data} />}
      </Box>
    </Box>
  );
}

function StatusCell({ status }: { status: 'installed' | 'missing' | 'unmanaged' }) {
  const { t } = useTranslation('srv6');
  return <StatusChip status={rowChip(status)} label={t(`status.${status}`)} />;
}

// ---- local SIDs -------------------------------------------------------------------------------------------

function LocalSidsPanel({ cfg, state }: { cfg: Srv6View; state: Srv6State | undefined }) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('srv6');
  const fmt = useFormatters();
  const perms = usePermissions();
  const patch = usePatchSrv6();
  const [edit, setEdit] = useState<{ sid: string; isNew: boolean } | null>(null);
  const rows = localSidRows(cfg, state);
  const remove = async (sid: string) => {
    try {
      await patch.mutateAsync({ localSids: { [sid]: null } });
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Box>
      <Stack direction="row" sx={{ mb: 1, alignItems: 'center' }}>
        <Typography variant="subtitle1" sx={{ flexGrow: 1 }}>
          {t('sids.title')}
        </Typography>
        {perms.editConfig && (
          <Button
            size="small"
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setEdit({ sid: '', isNew: true })}
          >
            {t('sids.add')}
          </Button>
        )}
      </Stack>
      {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
      <ReactGeereedMaterial
        hideFilterBuilderComponent
        items={rows}
        columns={[
          {
            key: 'sid',
            headerRenderer: () => t('col.sid'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return <>{r.sid}</>;
            },
          },
          {
            key: 'behavior',
            headerRenderer: () => t('col.behavior'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const behavior = r.config?.behavior ?? r.state?.behavior ?? '';
              const psp = r.config?.psp ?? r.state?.psp ?? false;
              return <>{psp ? t('behaviorPsp', { behavior }) : behavior}</>;
            },
          },
          {
            key: 'vrf',
            headerRenderer: () => t('col.vrf'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const vrf = r.config?.vrf ?? r.state?.vrf ?? '';
              return <>{vrf}</>;
            },
          },
          {
            key: 'target',
            headerRenderer: () => t('col.target'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const iface = r.config?.interface ?? r.state?.interface ?? null;
              const nh = r.config?.nextHop ?? r.state?.nextHop ?? null;
              const lookup = r.config?.lookupVrf ?? r.state?.lookupVrf ?? null;
              const target = [iface, nh ? `via ${nh}` : null, lookup ? `→ ${lookup}` : null]
                .filter((x) => x !== null)
                .join(' ');
              return <>{target || '—'}</>;
            },
          },
          {
            key: 'status',
            headerRenderer: () => t('col.status'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return (
                <>
                  <StatusCell status={r.status} />
                </>
              );
            },
          },
          {
            key: 'good',
            headerRenderer: () => t('col.good'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return (
                <>
                  {r.state
                    ? t('counters', {
                        packets: fmt.integer(r.state.goodPackets),
                        bytes: fmt.integer(r.state.goodBytes),
                      })
                    : '—'}
                </>
              );
            },
          },
          {
            key: 'bad',
            headerRenderer: () => t('col.bad'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return (
                <>
                  {r.state
                    ? t('counters', {
                        packets: fmt.integer(r.state.badPackets),
                        bytes: fmt.integer(r.state.badBytes),
                      })
                    : '—'}
                </>
              );
            },
          },
          {
            key: 'actions',
            headerRenderer: () => null,
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return (
                <>
                  {perms.editConfig && r.config && (
                    <>
                      <IconButton
                        size="small"
                        aria-label={t('edit')}
                        onClick={() => setEdit({ sid: r.sid, isNew: false })}
                      >
                        <EditIcon fontSize="small" />
                      </IconButton>
                      <IconButton
                        size="small"
                        aria-label={t('delete')}
                        onClick={() => void remove(r.sid)}
                      >
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </>
                  )}
                </>
              );
            },
          },
        ]}
        rowKey={(r) => r.sid}
        rowProps={(r) => ({ role: 'row', 'data-testid': `srv6-sid-${r.sid}` })}
        cellProps={(_item, column) => {
          if (column.key === 'sid') return { dir: 'ltr' };
          if (column.key === 'behavior') return { dir: 'ltr' };
          if (column.key === 'target') return { dir: 'ltr' };
          if (column.key === 'actions') return { sx: { whiteSpace: 'nowrap' } };
          return {};
        }}
        tableProps={{ size: 'small', 'aria-label': t('sids.title') }}

        emptyContent={t('sids.empty')}
        noFormWrapper
        embedded
        memoEnable={false}
        hidePagination
        hideToolbar
        footer={false}
        search={false}
        sortable={false}
        exportToCSV={false}
      />
      {edit && (
        <LocalSidDialog
          key={edit.sid || 'new'}
          target={edit}
          cfg={cfg}
          onClose={() => setEdit(null)}
        />
      )}
    </Box>
  );
}

function LocalSidDialog({
  target,
  cfg,
  onClose,
}: {
  target: { sid: string; isNew: boolean };
  cfg: Srv6View;
  onClose: () => void;
}) {
  const { t } = useTranslation('srv6');
  const perms = usePermissions();
  const patch = usePatchSrv6();
  const fresh = useFreshSrv6();
  const ifaces = useCandidateInterfaceNames();
  const schema = useMemo(
    () => localizeSchema(localSidFormSchema(), (k, o) => t(`sidForm.${k}`, o ?? {})),
    [t],
  );
  const [sidText, setSidText] = useState(target.sid);
  const sid = canon(sidText);
  const taken =
    Object.keys(cfg.localSids).map(canon).includes(sid) ||
    Object.keys(cfg.policies).map(canon).includes(sid);
  const sidOk = isIpv6(sidText) && (!target.isNew || !taken);

  const save = async (v: unknown) => {
    const value = canonLocalSid(v as Srv6LocalSidConfig);
    const before = (await fresh()).localSids[sid];
    const body = before === undefined ? value : createMergePatch(before, value);
    try {
      await patch.mutateAsync({ localSids: { [sid]: body } });
      onClose();
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>
        {target.isNew ? t('sids.add') : t('sids.editTitle', { sid: target.sid })}
      </DialogTitle>
      <DialogContent dividers>
        {target.isNew && (
          <TextField
            label={t('sids.sid')}
            value={sidText}
            onChange={(e) => setSidText(e.target.value)}
            error={sidText !== '' && !sidOk}
            helperText={taken && target.isNew ? t('sids.taken') : t('sids.sidHint')}
            fullWidth
            sx={{ my: 1 }}
            slotProps={{ htmlInput: LTR }}
          />
        )}
        <SchemaForm
          schema={schema}
          value={cfg.localSids[target.sid]}
          readOnly={!perms.editConfig || !sidOk}
          interfaceOptions={ifaces.data ?? []}
          problem={problemFor(patch.error, `/routing/srv6/localSids/${esc(sid)}`)}
          onSubmit={save}
          submitLabel={t('save')}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('close')}</Button>
      </DialogActions>
    </Dialog>
  );
}

// ---- policies -----------------------------------------------------------------------------------------

function sidListText(sids: readonly string[]): string {
  return sids.join(' → ');
}

function PoliciesPanel({ cfg, state }: { cfg: Srv6View; state: Srv6State | undefined }) {
  const { t } = useTranslation('srv6');
  const perms = usePermissions();
  const patch = usePatchSrv6();
  const [edit, setEdit] = useState<{ bsid: string; isNew: boolean } | null>(null);
  const rows = policyRows(cfg, state);
  const steered = steeredBsids(cfg);
  const remove = async (bsid: string) => {
    try {
      await patch.mutateAsync({ policies: { [bsid]: null } });
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Box>
      <GlobalsCard cfg={cfg} />
      <Stack direction="row" sx={{ mb: 1, alignItems: 'center' }}>
        <Typography variant="subtitle1" sx={{ flexGrow: 1 }}>
          {t('policies.title')}
        </Typography>
        {perms.editConfig && (
          <Button
            size="small"
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setEdit({ bsid: '', isNew: true })}
          >
            {t('policies.add')}
          </Button>
        )}
      </Stack>
      {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
      <ReactGeereedMaterial
        hideFilterBuilderComponent
        items={rows}
        columns={[
          {
            key: 'bsid',
            headerRenderer: () => t('col.bsid'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return <>{r.bsid}</>;
            },
          },
          {
            key: 'type',
            headerRenderer: () => t('col.type'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return <>{t(`policyType.${r.config?.type ?? r.state?.type ?? 'default'}`)}</>;
            },
          },
          {
            key: 'mode',
            headerRenderer: () => t('col.mode'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const encap = r.config?.encap ?? r.state?.encap ?? false;
              return <>{encap ? t('mode.encap') : t('mode.insert')}</>;
            },
          },
          {
            key: 'vrf',
            headerRenderer: () => t('col.vrf'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return <>{r.config?.vrf ?? r.state?.vrf ?? ''}</>;
            },
          },
          {
            key: 'source',
            headerRenderer: () => t('col.source'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const source = r.config
                ? effectiveEncapSource(r.config, cfg)
                : (r.state?.encapSource ?? undefined);
              const inherited = r.config?.encap === true && r.config.encapSource === undefined;
              return (
                <>
                  {source ? <span dir="ltr">{source}</span> : '—'}
                  {source && inherited ? ` ${t('policies.inherited')}` : ''}
                </>
              );
            },
          },
          {
            key: 'sidLists',
            headerRenderer: () => t('col.sidLists'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const lists = r.config?.sidLists ?? r.state?.sidLists ?? [];
              return (
                <>
                  {lists.map((l, i) => (
                    <div key={i} dir="ltr">
                      {`${sidListText(l.sids)} (w ${l.weight ?? 1})`}
                    </div>
                  ))}
                </>
              );
            },
          },
          {
            key: 'status',
            headerRenderer: () => t('col.status'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return (
                <>
                  <StatusCell status={r.status} />
                </>
              );
            },
          },
          {
            key: 'actions',
            headerRenderer: () => null,
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const inUse = steered.has(canon(r.bsid));
              return (
                <>
                  {perms.editConfig && r.config && (
                    <>
                      <IconButton
                        size="small"
                        aria-label={t('edit')}
                        onClick={() => setEdit({ bsid: r.bsid, isNew: false })}
                      >
                        <EditIcon fontSize="small" />
                      </IconButton>
                      <Tooltip title={inUse ? t('policies.inUse') : ''}>
                        <span>
                          <IconButton
                            size="small"
                            aria-label={t('delete')}
                            disabled={inUse}
                            onClick={() => void remove(r.bsid)}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    </>
                  )}
                </>
              );
            },
          },
        ]}
        rowKey={(r) => r.bsid}
        rowProps={(r) => ({
          role: 'row',
          'data-testid': `srv6-policy-${r.bsid}`,
        })}
        cellProps={(_item, column) => {
          if (column.key === 'bsid') return { dir: 'ltr' };
          if (column.key === 'actions') return { sx: { whiteSpace: 'nowrap' } };
          return {};
        }}
        tableProps={{ size: 'small', 'aria-label': t('policies.title') }}

        emptyContent={t('policies.empty')}
        noFormWrapper
        embedded
        memoEnable={false}
        hidePagination
        hideToolbar
        footer={false}
        search={false}
        sortable={false}
        exportToCSV={false}
      />
      {edit && (
        <PolicyDialog
          key={edit.bsid || 'new'}
          target={edit}
          cfg={cfg}
          onClose={() => setEdit(null)}
        />
      )}
    </Box>
  );
}

/** The two VPP-wide settings (encapSource, encapHopLimit): applied by the globals owner only, never read back. */
function GlobalsCard({ cfg }: { cfg: Srv6View }) {
  const { t } = useTranslation('srv6');
  const perms = usePermissions();
  const patch = usePatchSrv6();
  const fresh = useFreshSrv6();
  const schema = useMemo(
    () => localizeSchema(globalsFormSchema(), (k, o) => t(`globalsForm.${k}`, o ?? {})),
    [t],
  );
  const value = useMemo(
    () => ({
      ...(cfg.encapSource !== undefined ? { encapSource: cfg.encapSource } : {}),
      ...(cfg.encapHopLimit !== undefined ? { encapHopLimit: cfg.encapHopLimit } : {}),
    }),
    [cfg.encapSource, cfg.encapHopLimit],
  );
  const save = async (v: unknown) => {
    const next = { ...(v as { encapSource?: string; encapHopLimit?: number }) };
    if (next.encapSource !== undefined) next.encapSource = canon(next.encapSource);
    const cur = await fresh();
    const before = {
      ...(cur.encapSource !== undefined ? { encapSource: cur.encapSource } : {}),
      ...(cur.encapHopLimit !== undefined ? { encapHopLimit: cur.encapHopLimit } : {}),
    };
    try {
      await patch.mutateAsync(createMergePatch(before, next) as Record<string, unknown>);
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 2 }} data-testid="srv6-globals">
      <Typography variant="subtitle1">{t('globals.title')}</Typography>
      <Alert severity="info" sx={{ my: 1 }}>
        {t('globals.note')}
      </Alert>
      {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      <SchemaForm
        schema={schema}
        value={value}
        readOnly={!perms.editConfig}
        problem={problemFor(patch.error, SRV6_POINTER)}
        onSubmit={save}
        submitLabel={t('save')}
      />
    </Paper>
  );
}

function PolicyDialog({
  target,
  cfg,
  onClose,
}: {
  target: { bsid: string; isNew: boolean };
  cfg: Srv6View;
  onClose: () => void;
}) {
  const { t } = useTranslation('srv6');
  const perms = usePermissions();
  const patch = usePatchSrv6();
  const fresh = useFreshSrv6();
  const schema = useMemo(
    () => localizeSchema(policyFormSchema(), (k, o) => t(`policyForm.${k}`, o ?? {})),
    [t],
  );
  const existing = cfg.policies[target.bsid];
  const formValue = useMemo(() => {
    if (!existing) return undefined;
    const rest: Record<string, unknown> = { ...existing };
    delete rest['sidLists'];
    return rest;
  }, [existing]);
  const [bsidText, setBsidText] = useState(target.bsid);
  const [lists, setLists] = useState<SidListDraft[]>(
    existing?.sidLists.map((l) => ({
      sids: [...l.sids],
      weight: l.weight,
    })) ?? [{ sids: [''], weight: 1 }],
  );
  const bsid = canon(bsidText);
  const taken =
    Object.keys(cfg.policies).map(canon).includes(bsid) ||
    Object.keys(cfg.localSids).map(canon).includes(bsid);
  const bsidOk = isIpv6(bsidText) && (!target.isNew || !taken);
  const listProblems = sidListProblems(lists);
  const formId = 'srv6-policy-form';

  const save = async (v: unknown) => {
    const value = canonPolicy({
      ...(v as Omit<Srv6PolicyConfig, 'sidLists'>),
      sidLists: lists.map((l) => ({ sids: l.sids, weight: l.weight })),
    } as Srv6PolicyConfig);
    const before = (await fresh()).policies[bsid];
    const body = before === undefined ? value : createMergePatch(before, value);
    try {
      await patch.mutateAsync({ policies: { [bsid]: body } });
      onClose();
    } catch {
      // rendered from patch.error
    }
  };
  const readOnly = !perms.editConfig || !bsidOk;
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>
        {target.isNew ? t('policies.add') : t('policies.editTitle', { bsid: target.bsid })}
      </DialogTitle>
      <DialogContent dividers>
        {target.isNew && (
          <TextField
            label={t('policies.bsid')}
            value={bsidText}
            onChange={(e) => setBsidText(e.target.value)}
            error={bsidText !== '' && !bsidOk}
            helperText={taken && target.isNew ? t('policies.taken') : t('policies.bsidHint')}
            fullWidth
            sx={{ my: 1 }}
            slotProps={{ htmlInput: LTR }}
          />
        )}
        <SchemaForm
          id={formId}
          hideActions
          schema={schema}
          value={formValue}
          readOnly={readOnly}
          problem={problemFor(patch.error, `/routing/srv6/policies/${esc(bsid)}`)}
          onSubmit={save}
        />
        <SidListEditor lists={lists} onChange={setLists} readOnly={readOnly} />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('close')}</Button>
        <Button
          type="submit"
          form={formId}
          variant="contained"
          disabled={readOnly || listProblems.length > 0 || patch.isPending}
        >
          {t('save')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** The SID-list editor: ordered segment lists (≤ 16 SIDs each, first visited first) with their weights. */
export function SidListEditor({
  lists,
  onChange,
  readOnly,
}: {
  lists: readonly SidListDraft[];
  onChange: (lists: SidListDraft[]) => void;
  readOnly: boolean;
}) {
  const { t } = useTranslation('srv6');
  const problems = sidListProblems(lists);
  const set = (i: number, l: SidListDraft) => onChange(lists.map((x, j) => (j === i ? l : x)));
  return (
    <Box sx={{ mt: 2 }} data-testid="srv6-sid-lists">
      <Stack direction="row" sx={{ alignItems: 'center', mb: 1 }}>
        <Typography variant="subtitle2" sx={{ flexGrow: 1 }}>
          {t('sidLists.title')}
        </Typography>
        {!readOnly && (
          <Button
            size="small"
            startIcon={<AddIcon />}
            onClick={() => onChange([...lists, { sids: [''], weight: 1 }])}
          >
            {t('sidLists.addList')}
          </Button>
        )}
      </Stack>
      {problems.some((p) => p.problem === 'noList') && (
        <Alert severity="error">{t('sidLists.problem.noList')}</Alert>
      )}
      {lists.map((l, i) => {
        const problem = problems.find((p) => p.list === i)?.problem;
        return (
          <Paper
            key={i}
            variant="outlined"
            sx={{ p: 1.5, mb: 1 }}
            data-testid={`srv6-sid-list-${i}`}
          >
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 1 }}>
              <Typography variant="body2" sx={{ flexGrow: 1 }}>
                {t('sidLists.list', { n: i + 1 })}
              </Typography>
              <TextField
                size="small"
                type="number"
                label={t('sidLists.weight')}
                value={l.weight}
                disabled={readOnly}
                onChange={(e) => set(i, { ...l, weight: Number(e.target.value) })}
                sx={{ width: 120 }}
                slotProps={{ htmlInput: { min: 1, max: 65535 } }}
              />
              {!readOnly && (
                <IconButton
                  size="small"
                  aria-label={t('sidLists.removeList', { n: i + 1 })}
                  onClick={() => onChange(lists.filter((_, j) => j !== i))}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              )}
            </Stack>
            {l.sids.map((s, k) => (
              <Stack key={k} direction="row" spacing={1} sx={{ alignItems: 'center', mb: 0.5 }}>
                <Typography variant="body2" sx={{ minWidth: 24, textAlign: 'end' }}>
                  {k + 1}.
                </Typography>
                <TextField
                  size="small"
                  fullWidth
                  value={s}
                  disabled={readOnly}
                  error={s !== '' && !isIpv6(s)}
                  onChange={(e) =>
                    set(i, {
                      ...l,
                      sids: l.sids.map((x, m) => (m === k ? e.target.value : x)),
                    })
                  }
                  onBlur={() =>
                    isIpv6(s) &&
                    set(i, {
                      ...l,
                      sids: l.sids.map((x, m) => (m === k ? canon(x) : x)),
                    })
                  }
                  slotProps={{
                    htmlInput: {
                      ...LTR,
                      'aria-label': t('sidLists.segment', {
                        n: k + 1,
                        list: i + 1,
                      }),
                    },
                  }}
                />
                {!readOnly && (
                  <>
                    <IconButton
                      size="small"
                      aria-label={t('sidLists.up')}
                      disabled={k === 0}
                      onClick={() => set(i, { ...l, sids: move(l.sids, k, -1) })}
                    >
                      <ArrowUpwardIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={t('sidLists.down')}
                      disabled={k === l.sids.length - 1}
                      onClick={() => set(i, { ...l, sids: move(l.sids, k, 1) })}
                    >
                      <ArrowDownwardIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={t('sidLists.removeSid')}
                      onClick={() => set(i, { ...l, sids: l.sids.filter((_, m) => m !== k) })}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </>
                )}
              </Stack>
            ))}
            {!readOnly && (
              <Button
                size="small"
                startIcon={<AddIcon />}
                disabled={l.sids.length >= SRV6_MAX_SIDS}
                onClick={() => set(i, { ...l, sids: [...l.sids, ''] })}
              >
                {t('sidLists.addSid', { max: SRV6_MAX_SIDS })}
              </Button>
            )}
            {problem !== undefined && (
              <Typography variant="body2" color="error">
                {t(`sidLists.problem.${problem}`, { max: SRV6_MAX_SIDS })}
              </Typography>
            )}
          </Paper>
        );
      })}
    </Box>
  );
}

// ---- steering -------------------------------------------------------------------------------------------

function SteeringPanel({ cfg, state }: { cfg: Srv6View; state: Srv6State | undefined }) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('srv6');
  const perms = usePermissions();
  const patch = usePatchSrv6();
  const fresh = useFreshSrv6();
  const [adding, setAdding] = useState(false);
  const rows = steeringRows(cfg, state);
  const remove = async (key: string) => {
    const cur = await fresh();
    const next = sortSteering(cur.steering.filter((s) => steeringKey(s) !== key));
    try {
      await patch.mutateAsync({ steering: next });
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Box>
      <Stack direction="row" sx={{ mb: 1, alignItems: 'center' }}>
        <Typography variant="subtitle1" sx={{ flexGrow: 1 }}>
          {t('steering.title')}
        </Typography>
        {perms.editConfig && (
          <Button
            size="small"
            variant="contained"
            startIcon={<AddIcon />}
            disabled={Object.keys(cfg.policies).length === 0}
            onClick={() => setAdding(true)}
          >
            {t('steering.add')}
          </Button>
        )}
      </Stack>
      {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
      <ReactGeereedMaterial
        hideFilterBuilderComponent
        items={rows}
        columns={[
          {
            key: 'match',
            headerRenderer: () => t('col.match'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const c = r.config;
              const s = r.state;
              const what = c
                ? c.type === 'l2'
                  ? c.interface
                  : c.prefix
                : (s?.interface ?? s?.prefix ?? '');
              return <>{what}</>;
            },
          },
          {
            key: 'vrf',
            headerRenderer: () => t('col.vrf'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const c = r.config;
              const s = r.state;
              const vrf = c ? (c.type === 'l3' ? c.vrf : null) : (s?.vrf ?? null);
              return <>{vrf ?? '—'}</>;
            },
          },
          {
            key: 'traffic',
            headerRenderer: () => t('col.traffic'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const c = r.config;
              const s = r.state;
              const traffic = s?.trafficType ?? trafficOf(c);
              return <>{t(`traffic.${traffic}`)}</>;
            },
          },
          {
            key: 'bsid',
            headerRenderer: () => t('col.bsid'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const c = r.config;
              const s = r.state;
              return <>{c?.bsid ?? s?.bsid ?? ''}</>;
            },
          },
          {
            key: 'status',
            headerRenderer: () => t('col.status'),
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              return (
                <>
                  <StatusCell status={r.status} />
                </>
              );
            },
          },
          {
            key: 'actions',
            headerRenderer: () => null,
            sortable: false,
            filterable: false,
            renderer: (_value, r: (typeof rows)[number]) => {
              const c = r.config;
              return (
                <>
                  {perms.editConfig && c && (
                    <IconButton
                      size="small"
                      aria-label={t('delete')}
                      onClick={() => void remove(r.key)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  )}
                </>
              );
            },
          },
        ]}
        rowKey={(r) => r.key}
        rowProps={(r) => ({
          role: 'row',
          'data-testid': `srv6-steer-${r.key}`,
        })}
        cellProps={(_item, column) => {
          if (column.key === 'match') return { dir: 'ltr' };
          if (column.key === 'bsid') return { dir: 'ltr' };
          return {};
        }}
        tableProps={{ size: 'small', 'aria-label': t('steering.title') }}

        emptyContent={t('steering.empty')}
        noFormWrapper
        embedded
        memoEnable={false}
        hidePagination
        hideToolbar
        footer={false}
        search={false}
        sortable={false}
        exportToCSV={false}
      />
      {adding && <SteeringDialog cfg={cfg} onClose={() => setAdding(false)} />}
    </Box>
  );
}

function SteeringDialog({ cfg, onClose }: { cfg: Srv6View; onClose: () => void }) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('srv6');
  const patch = usePatchSrv6();
  const fresh = useFreshSrv6();
  const vrfs = useCandidateVrfNames();
  const ifaces = useCandidateInterfaceNames();
  const bsids = Object.keys(cfg.policies).sort();
  const [type, setType] = useState<'l3' | 'l2'>('l3');
  const [prefix, setPrefix] = useState('');
  const [vrf, setVrf] = useState('default');
  const [iface, setIface] = useState('');
  const [bsid, setBsid] = useState(bsids[0] ?? '');
  const entry: Srv6SteeringConfig | undefined =
    type === 'l3'
      ? canonicalPrefix(prefix.trim()) !== undefined
        ? canonSteering({ type: 'l3', prefix, vrf, bsid })
        : undefined
      : iface !== ''
        ? canonSteering({ type: 'l2', interface: iface, bsid })
        : undefined;
  const dup =
    entry !== undefined && cfg.steering.some((s) => steeringKey(s) === steeringKey(entry));
  const ok = entry !== undefined && bsid !== '' && !dup;

  const save = async () => {
    if (!entry) return;
    const cur = await fresh();
    try {
      await patch.mutateAsync({
        steering: sortSteering([...cur.steering, entry]),
      });
      onClose();
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t('steering.add')}</DialogTitle>
      <DialogContent dividers>
        <TextField
          select
          label={t('steering.type')}
          value={type}
          onChange={(e) => setType(e.target.value as 'l3' | 'l2')}
          fullWidth
          sx={{ my: 1 }}
        >
          {STEER_TYPES.map((st) => (
            <MenuItem key={st} value={st}>
              {t(`steering.${st}`)}
            </MenuItem>
          ))}
        </TextField>
        {type === 'l3' ? (
          <>
            <TextField
              label={t('steering.prefix')}
              value={prefix}
              onChange={(e) => setPrefix(e.target.value)}
              error={prefix !== '' && canonicalPrefix(prefix.trim()) === undefined}
              helperText={t('steering.prefixHint')}
              fullWidth
              sx={{ my: 1 }}
              slotProps={{ htmlInput: LTR }}
            />
            <TextField
              select
              label={t('col.vrf')}
              value={vrf}
              onChange={(e) => setVrf(e.target.value)}
              fullWidth
              sx={{ my: 1 }}
            >
              {(vrfs.data ?? DEFAULT_VRFS).map((v) => (
                <MenuItem key={v} value={v}>
                  {v}
                </MenuItem>
              ))}
            </TextField>
          </>
        ) : (
          <>
            <TextField
              select
              label={t('steering.interface')}
              value={iface}
              onChange={(e) => setIface(e.target.value)}
              helperText={t('steering.l2Hint')}
              fullWidth
              sx={{ my: 1 }}
            >
              {(ifaces.data ?? []).map((n) => (
                <MenuItem key={n} value={n} dir="ltr">
                  {interfaceLabel(n)}
                </MenuItem>
              ))}
            </TextField>
          </>
        )}
        <TextField
          select
          label={t('col.bsid')}
          value={bsid}
          onChange={(e) => setBsid(e.target.value)}
          fullWidth
          sx={{ my: 1 }}
        >
          {bsids.map((b) => (
            <MenuItem key={b} value={b} dir="ltr">
              {b}
            </MenuItem>
          ))}
        </TextField>
        {dup && <Alert severity="error">{t('steering.duplicate')}</Alert>}
        {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mt: 1 }} />}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('close')}</Button>
        <Button variant="contained" disabled={!ok || patch.isPending} onClick={() => void save()}>
          {t('save')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
