import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
  PgRefreshIcon as RefreshIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTableContainer as TableContainer,
  PgTabs as Tabs,
  PgTooltip as Tooltip,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel, useFormatters } from '@ngfw/ui-kit';
import { ExpiryChip } from '../ExpiryChip'; // F-rule-expiry
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { objectModelWidgets } from '../object-model';
import {
  attachmentSchema,
  banners,
  countersByRule,
  deleteListPatch,
  groupDigits,
  isTab,
  LIST_NAME_RE,
  listSchema,
  listsOf,
  localizeSchema,
  matchText,
  nextSequence,
  putRule,
  removeAt,
  ruleKey,
  ruleSchema,
  rulesOf,
  same,
  serviceText,
  settingsOf,
  settingsSchema,
  TABS,
  type HostAclConfig,
  type HostAclState,
  type HostAclTab,
  type HostAttachment,
  type HostList,
  type HostRule,
} from './model';
import {
  useCandidateAcl,
  useFreshCandidateAcl,
  useHostAclState,
  usePatchAcl,
  useRunningAcl,
} from './queries';

const NS = 'host-acl-nftables';
const TAB_PARAM = 'tab';
const LTR = { dir: 'ltr' } as const;
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** Server pointers below `prefix` → pointers relative to the edited value (the form's fields). */
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

function useLocalized(schema: () => JsonSchema): JsonSchema {
  const { t } = useTranslation(NS);
  return useMemo(() => localizeSchema(schema(), (k, o) => t(k, o ?? {})), [schema, t]);
}

/** LTR, monospace text inside an RTL page (addresses, rule text, interface names). */
function Code({ children, size = 12 }: { children: ReactNode; size?: number }) {
  return (
    <Box
      component="span"
      dir="ltr"
      sx={{
        fontFamily: (th) => th.ngfw.monoFontFamily,
        fontSize: size,
        textAlign: 'start',
        unicodeBidi: 'isolate',
      }}
    >
      {children}
    </Box>
  );
}

/**
 * F-host-acl-nftables: the Host ACL page (`/firewall/host-acl`) — host lists with their rules and live counters,
 * attachments to the input/output/forward hooks, the host firewall settings (default input policy, ICMP, anti-lockout)
 * and the table as the agent rendered it (nftables `inet ngfw`, HostAclState). Every edit is a merge patch of the
 * candidate's `acl`; the shell's pending-change bar shows the diff and commits.
 */
export function HostAclPage() {
  const { t } = useTranslation(NS);
  const [params, setParams] = useSearchParams();
  const tabParam = params.get(TAB_PARAM);
  const tab: HostAclTab = isTab(tabParam) ? tabParam : 'lists';
  const candidate = useCandidateAcl();
  const running = useRunningAcl();
  const state = useHostAclState();
  return (
    <PageHeader title={t('title')}>
      <Stack direction="row" spacing={2} sx={{ mb: 2, alignItems: 'flex-start' }}>
        <Typography color="text.secondary" sx={{ flex: 1 }}>
          {t('intro')}
        </Typography>
        {/* D-132: the state polls every 30 s; Refresh reads it (and the configuration) now */}
        <Button
          variant="outlined"
          size="small"
          startIcon={<RefreshIcon />}
          disabled={state.isFetching}
          onClick={() =>
            void Promise.all([state.refetch(), candidate.refetch(), running.refetch()])
          }
        >
          {t('refresh')}
        </Button>
      </Stack>
      <Banners settings={settingsOf(running.data)} state={state.data} running={running.data} />
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 1 }} />}
      <Tabs
        value={tab}
        onChange={(_e, v: HostAclTab) =>
          setParams(
            (p) => {
              p.set(TAB_PARAM, v);
              return p;
            },
            { replace: true },
          )
        }
        variant="scrollable"
        allowScrollButtonsMobile
        aria-label={t('tabsLabel')}
        sx={{ mb: 2, borderBlockEnd: 1, borderColor: 'divider' }}
      >
        {TABS.map((k) => (
          <Tab
            key={k}
            value={k}
            label={t(`tabs.${k}`)}
            id={`host-acl-tab-${k}`}
            aria-controls={`host-acl-panel-${k}`}
          />
        ))}
      </Tabs>
      {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} />}
      <Box role="tabpanel" id={`host-acl-panel-${tab}`} aria-labelledby={`host-acl-tab-${tab}`}>
        {candidate.isSuccess && tab === 'lists' && (
          <ListsPanel acl={candidate.data} running={running.data} state={state.data} />
        )}
        {candidate.isSuccess && tab === 'attachments' && (
          <AttachmentsPanel acl={candidate.data} running={running.data} />
        )}
        {candidate.isSuccess && tab === 'settings' && <SettingsPanel acl={candidate.data} />}
        {tab === 'rendered' && (
          <RenderedPanel state={state.data} loading={state.isPending && !state.isError} />
        )}
      </Box>
    </PageHeader>
  );
}

function Banners({
  settings,
  state,
  running,
}: {
  settings: ReturnType<typeof settingsOf>;
  state: HostAclState | undefined;
  running: HostAclConfig | undefined;
}) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  return (
    <Stack gap={1} sx={{ mb: 2 }}>
      {banners(settings, state, running, interfaceLabel).map((b) => (
        <Alert key={b.key} severity={b.severity} role={b.severity === 'error' ? 'alert' : 'status'}>
          {t(b.key, b.params)}
        </Alert>
      ))}
    </Stack>
  );
}

// ---------------------------------------------------------------------------------------------------------------------
// Lists and rules
// ---------------------------------------------------------------------------------------------------------------------

type RuleEdit = {
  list: string;
  index: number | undefined;
  rule: HostRule | undefined;
  defaultSequence: number;
};

function ListsPanel({
  acl,
  running,
  state,
}: {
  acl: HostAclConfig;
  running: HostAclConfig | undefined;
  state: HostAclState | undefined;
}) {
  const { t } = useTranslation(NS);
  const interfaceLabel = useInterfaceLabel();
  const fmt = useFormatters();
  const perms = usePermissions();
  const readOnly = !perms.editConfig;
  const patch = usePatchAcl();
  const fresh = useFreshCandidateAcl();
  const [editList, setEditList] = useState<{ name: string; value: HostList | undefined } | null>(
    null,
  );
  const [editRule, setEditRule] = useState<RuleEdit | null>(null);
  const [lastError, setLastError] = useState<unknown>(null);
  const counters = useMemo(() => countersByRule(state), [state]);
  const lists = listsOf(acl);

  const removeList = async (name: string) => {
    setLastError(null);
    try {
      await patch.mutateAsync(deleteListPatch(await fresh(), name));
    } catch (e) {
      setLastError(e);
    }
  };
  const removeRule = async (list: string, index: number) => {
    setLastError(null);
    try {
      const rules = (await fresh()).host?.[list]?.rules ?? [];
      await patch.mutateAsync({ host: { [list]: { rules: removeAt(rules, index) } } });
    } catch (e) {
      setLastError(e);
    }
  };

  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center">
        <Tooltip title={readOnly ? t('readonly') : ''}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={readOnly}
              onClick={() => setEditList({ name: '', value: undefined })}
            >
              {t('lists.add')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      {lastError !== null && <ProblemAlert error={lastError} sx={{ mb: 1 }} />}
      {lists.length === 0 && <Typography color="text.secondary">{t('lists.empty')}</Typography>}
      <Stack gap={2}>
        {lists.map(([name, list]) => {
          const runningList = running?.host?.[name];
          const pending = running !== undefined && !same(runningList, list);
          return (
            <Paper key={name} variant="outlined" sx={{ p: 1.5 }}>
              <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mb: 1 }}>
                <Typography variant="h6" component="h3">
                  <Code size={16}>{name}</Code>
                </Typography>
                {pending && (
                  <Chip size="small" color="warning" variant="outlined" label={t('pending')} />
                )}
                <Typography color="text.secondary" sx={{ flex: 1 }}>
                  {list.description}
                </Typography>
                <Button
                  size="small"
                  startIcon={<AddIcon />}
                  disabled={readOnly}
                  onClick={() =>
                    setEditRule({
                      list: name,
                      index: undefined,
                      rule: undefined,
                      defaultSequence: nextSequence(list.rules),
                    })
                  }
                >
                  {t('rules.add')}
                </Button>
                <IconButton
                  size="small"
                  aria-label={t('lists.editNamed', { name })}
                  disabled={readOnly}
                  onClick={() => setEditList({ name, value: list })}
                >
                  <EditIcon fontSize="small" />
                </IconButton>
                <IconButton
                  size="small"
                  aria-label={t('lists.deleteNamed', { name })}
                  disabled={readOnly || patch.isPending}
                  onClick={() => void removeList(name)}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Stack>
              <TableContainer>
                <ReactGeereedMaterial
                  tableProps={{ size: 'small', 'aria-label': t('rules.tableLabel', { name }) }}
                  columns={[
                    {
                      key: 'sequence',
                      headerRenderer: () => t('col.sequence'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'action',
                      headerRenderer: () => t('col.action'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'ipVersion',
                      headerRenderer: () => t('col.ipVersion'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'source',
                      headerRenderer: () => t('col.source'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'destination',
                      headerRenderer: () => t('col.destination'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'service',
                      headerRenderer: () => t('col.service'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'interface',
                      headerRenderer: () => t('col.interface'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'log',
                      headerRenderer: () => t('col.log'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'enabled',
                      headerRenderer: () => t('col.enabled'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'description',
                      headerRenderer: () => t('col.description'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'expiresAt',
                      headerRenderer: () => t('col.expiresAt'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'packets',
                      headerRenderer: () => t('col.packets'),
                      filterable: false,
                      renderer: (value) => <>{value}</>,
                    },
                    {
                      key: 'bytes',
                      headerRenderer: () => t('col.bytes'),
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
                    ...(rulesOf(list).map(({ rule, index }) =>
                      renderRuleRow({
                        interfaceLabel,
                        key: `${rule.sequence}-${index}`,
                        list: name,
                        rule: rule,
                        counters: counters.get(ruleKey(name, rule.sequence)),
                        readOnly: readOnly || patch.isPending,
                        onEdit: () =>
                          setEditRule({ list: name, index, rule, defaultSequence: rule.sequence }),
                        onDelete: () => void removeRule(name, index),
                        t,
                        fmt,
                      }),
                    ) ?? []),
                  ]}
                  rowKey={(item) => item.id}
                  rowProps={(item) => item.rowProps ?? {}}
                  cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
                  emptyContent={
                    list.rules.length === 0 ? (
                      <>
                        <Typography color="text.secondary">{t('rules.empty')}</Typography>
                      </>
                    ) : null
                  }
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
            </Paper>
          );
        })}
      </Stack>
      <Dialog open={editList !== null} onClose={() => setEditList(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {editList?.value ? t('lists.editTitle', { name: editList.name }) : t('lists.addTitle')}
        </DialogTitle>
        <DialogContent>
          {editList && (
            <ListForm
              key={editList.name || 'new'}
              initialName={editList.name}
              value={editList.value}
              acl={acl}
              readOnly={readOnly}
              onDone={() => setEditList(null)}
            />
          )}
        </DialogContent>
      </Dialog>
      <Dialog open={editRule !== null} onClose={() => setEditRule(null)} fullWidth maxWidth="md">
        <DialogTitle>
          {editRule?.rule
            ? t('rules.editTitle', { list: editRule.list, sequence: editRule.rule.sequence })
            : t('rules.addTitle', { list: editRule?.list ?? '' })}
        </DialogTitle>
        <DialogContent>
          {editRule && (
            <RuleForm
              key={`${editRule.list}-${editRule.index ?? 'new'}`}
              edit={editRule}
              readOnly={readOnly}
              onDone={() => setEditRule(null)}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function renderRuleRow({
  interfaceLabel,
  key,
  t,
  fmt,
  rule,
  counters,
  readOnly,
  onEdit,
  onDelete,
}: {
  interfaceLabel: (name: string) => string;
  key: string;
  t: ReturnType<typeof useTranslation>['t'];
  fmt: ReturnType<typeof useFormatters>;
  list: string;
  rule: HostRule;
  counters: { packets: string; bytes: string; nftRules: number } | undefined;
  readOnly: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return {
    id: key,
    sequence: fmt.digits(String(rule.sequence)),
    action: (
      <>
        <Chip
          size="small"
          color={rule.action === 'accept' ? 'success' : 'error'}
          variant="outlined"
          label={t(`action.${rule.action}`)}
        />
      </>
    ),
    ipVersion: t(`ipVersion.${rule.ipVersion}`),
    source: (
      <>
        <Code>{matchText(rule.source)}</Code>
      </>
    ),
    destination: (
      <>
        <Code>{matchText(rule.destination)}</Code>
      </>
    ),
    service: (
      <>
        <Code>{serviceText(rule.service)}</Code>
      </>
    ),
    interface: rule.interface ? <Code>{interfaceLabel(rule.interface)}</Code> : '*',
    log: rule.log ? t('yes') : t('no'),
    enabled: rule.enabled ? t('yes') : t('no'),
    description: rule.description,
    expiresAt: (
      <>
        <ExpiryChip expiresAt={rule.expiresAt} owner={rule.owner} ticket={rule.ticket} />
      </>
    ),
    packets: counters ? (
      <Tooltip title={t('rules.nftRules', { count: counters.nftRules })}>
        <span>{fmt.digits(groupDigits(counters.packets))}</span>
      </Tooltip>
    ) : (
      '—'
    ),
    bytes: counters ? fmt.digits(groupDigits(counters.bytes)) : '—',
    actions: (
      <>
        <IconButton
          size="small"
          aria-label={t('rules.deleteNamed', { sequence: rule.sequence })}
          disabled={readOnly}
          onClick={onDelete}
        >
          <DeleteIcon fontSize="small" />
        </IconButton>
      </>
    ),
    rowProps: {
      hover: true,
      sx: { cursor: 'pointer', opacity: rule.enabled ? 1 : 0.6 },
      onClick: onEdit,
    } satisfies ReturnType<NonNullable<GeereedTablePassthrough['rowProps']>> &
      Record<`data-${string}`, unknown>,
    cellProps: {
      actions: {
        sx: { textAlign: 'end', whiteSpace: 'nowrap' },
        onClick: (ev) => ev.stopPropagation(),
      },
    } satisfies Record<
      string,
      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
    >,
  };
}

function ListForm({
  initialName,
  value,
  acl,
  readOnly,
  onDone,
}: {
  initialName: string;
  value: HostList | undefined;
  acl: HostAclConfig;
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation(NS);
  const patch = usePatchAcl();
  const schema = useLocalized(listSchema);
  const editing = value !== undefined;
  const [name, setName] = useState(initialName);
  const [error, setError] = useState<unknown>(null);
  const taken = !editing && name !== '' && Object.hasOwn(acl.host ?? {}, name);
  const nameOk = LIST_NAME_RE.test(name) && !taken;
  const initial = value ? { description: value.description, tags: value.tags } : undefined;

  const save = async (v: unknown) => {
    if (!nameOk) return;
    setError(null);
    const body = v as { description?: string; tags?: string[] };
    try {
      await patch.mutateAsync({
        host: {
          [name]: editing
            ? { description: body.description ?? null, tags: body.tags ?? [] }
            : { ...body, rules: [] },
        },
      });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/acl/host/${esc(name)}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('lists.name')}
        value={name}
        disabled={editing}
        onChange={(e) => setName(e.target.value.trim())}
        error={name !== '' && !nameOk}
        helperText={taken ? t('lists.exists') : t('lists.nameHelp')}
        slotProps={{ htmlInput: LTR }}
      />
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={initial}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        problem={problem}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        onSubmit={save}
      >
        <Button onClick={onDone}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}

function RuleForm({
  edit,
  readOnly,
  onDone,
}: {
  edit: RuleEdit;
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation(NS);
  const patch = usePatchAcl();
  const fresh = useFreshCandidateAcl();
  const schema = useLocalized(ruleSchema);
  const [error, setError] = useState<unknown>(null);
  const [at, setAt] = useState<number | undefined>(edit.index);
  const [initial] = useState<Partial<HostRule>>(
    () => edit.rule ?? { sequence: edit.defaultSequence, action: 'accept' },
  );

  const save = async (v: unknown) => {
    setError(null);
    const rules = (await fresh()).host?.[edit.list]?.rules ?? [];
    const rule = v as HostRule;
    const next = putRule(rules, edit.index, rule);
    setAt(next.indexOf(rule));
    try {
      await patch.mutateAsync({ host: { [edit.list]: { rules: next } } });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/acl/host/${esc(edit.list)}/rules/${at ?? 0}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={initial}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        problem={problem}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        onSubmit={save}
      >
        <Button onClick={onDone}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}

// ---------------------------------------------------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------------------------------------------------

function AttachmentsPanel({
  acl,
  running,
}: {
  acl: HostAclConfig;
  running: HostAclConfig | undefined;
}) {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const perms = usePermissions();
  const readOnly = !perms.editConfig;
  const patch = usePatchAcl();
  const fresh = useFreshCandidateAcl();
  const [editing, setEditing] = useState<{
    index: number | undefined;
    value: HostAttachment | undefined;
  } | null>(null);
  const [lastError, setLastError] = useState<unknown>(null);
  const items = acl.hostAttachments ?? [];

  const remove = async (index: number) => {
    setLastError(null);
    try {
      await patch.mutateAsync({
        hostAttachments: removeAt((await fresh()).hostAttachments ?? [], index),
      });
    } catch (e) {
      setLastError(e);
    }
  };
  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }}>
        <Tooltip title={readOnly ? t('readonly') : ''}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={readOnly}
              onClick={() => setEditing({ index: undefined, value: undefined })}
            >
              {t('attachments.add')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      {lastError !== null && <ProblemAlert error={lastError} sx={{ mb: 1 }} />}
      <TableContainer component={Paper} variant="outlined">
        <ReactGeereedMaterial
          tableProps={{ size: 'small', 'aria-label': t('tabs.attachments') }}
          columns={[
            {
              key: 'list',
              headerRenderer: () => t('col.list'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
            {
              key: 'chain',
              headerRenderer: () => t('col.chain'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
            {
              key: 'priority',
              headerRenderer: () => t('col.priority'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
            {
              key: 'enabled',
              headerRenderer: () => t('col.enabled'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
            {
              key: 'description',
              headerRenderer: () => t('col.description'),
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
            ...(items.map((a, i) => {
              const pending =
                running !== undefined && !(running.hostAttachments ?? []).some((r) => same(r, a));
              return {
                id: `${a.list}-${a.chain}-${i}`,
                list: (
                  <>
                    <Stack direction="row" gap={0.5} alignItems="center">
                      <Code>{a.list}</Code>
                      {pending && (
                        <Chip
                          size="small"
                          color="warning"
                          variant="outlined"
                          label={t('pending')}
                        />
                      )}
                    </Stack>
                  </>
                ),
                chain: t(`hook.${a.chain}`),
                priority: fmt.digits(String(a.priority)),
                enabled: a.enabled ? t('yes') : t('no'),
                description: a.description,
                actions: (
                  <>
                    <IconButton
                      size="small"
                      aria-label={t('attachments.deleteNamed', { list: a.list, chain: a.chain })}
                      disabled={readOnly || patch.isPending}
                      onClick={() => void remove(i)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </>
                ),
                rowProps: {
                  hover: true,
                  sx: { cursor: 'pointer' },
                  onClick: () => setEditing({ index: i, value: a }),
                } satisfies ReturnType<NonNullable<GeereedTablePassthrough['rowProps']>> &
                  Record<`data-${string}`, unknown>,
                cellProps: {
                  actions: { sx: { textAlign: 'end' }, onClick: (ev) => ev.stopPropagation() },
                } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              };
            }) ?? []),
          ]}
          rowKey={(item) => item.id}
          rowProps={(item) => item.rowProps ?? {}}
          cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
          emptyContent={
            items.length === 0 ? (
              <>
                <Typography color="text.secondary">{t('attachments.empty')}</Typography>
              </>
            ) : null
          }
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
      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {editing?.value
            ? t('attachments.editTitle', { list: editing.value.list })
            : t('attachments.addTitle')}
        </DialogTitle>
        <DialogContent>
          {editing && (
            <AttachmentForm
              index={editing.index}
              value={editing.value}
              readOnly={readOnly}
              onDone={() => setEditing(null)}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function AttachmentForm({
  index,
  value,
  readOnly,
  onDone,
}: {
  index: number | undefined;
  value: HostAttachment | undefined;
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation(NS);
  const patch = usePatchAcl();
  const fresh = useFreshCandidateAcl();
  const schema = useLocalized(attachmentSchema);
  const [error, setError] = useState<unknown>(null);
  const [at, setAt] = useState<number | undefined>(index);
  const save = async (v: unknown) => {
    setError(null);
    const items = [...((await fresh()).hostAttachments ?? [])];
    const i = index !== undefined && index < items.length ? index : items.length;
    items[i] = v as HostAttachment;
    setAt(i);
    try {
      await patch.mutateAsync({ hostAttachments: items });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/acl/hostAttachments/${at ?? 0}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={value}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        problem={problem}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        onSubmit={save}
      >
        <Button onClick={onDone}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}

// ---------------------------------------------------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------------------------------------------------

function SettingsPanel({ acl }: { acl: HostAclConfig }) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const readOnly = !perms.editConfig;
  const patch = usePatchAcl();
  const schema = useLocalized(settingsSchema);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);
  const save = async (v: unknown) => {
    setError(null);
    setSaved(false);
    try {
      // the whole object: arrays (sources, interfaces, ports) are replaced, not merged
      await patch.mutateAsync({ hostSettings: v });
      setSaved(true);
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, '/acl/hostSettings');
  return (
    <Paper variant="outlined" sx={{ p: 2, maxWidth: 720 }}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('settings.intro')}
      </Typography>
      {error !== null && problem === null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      {saved && (
        <Alert severity="success" sx={{ mb: 1 }}>
          {t('settings.saved')}
        </Alert>
      )}
      <SchemaForm
        schema={schema}
        value={acl.hostSettings ?? settingsOf(undefined)}
        readOnly={readOnly}
        problem={problem}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        onSubmit={save}
      />
    </Paper>
  );
}

// ---------------------------------------------------------------------------------------------------------------------
// Rendered table (HostAclState)
// ---------------------------------------------------------------------------------------------------------------------

function RenderedPanel({ state, loading }: { state: HostAclState | undefined; loading: boolean }) {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  if (loading) return <LinearProgress aria-label={t('loading')} />;
  if (!state) return <Typography color="text.secondary">{t('rendered.unavailable')}</Typography>;
  return (
    <Stack gap={2}>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center">
        <Chip label={<Code>{`inet ${state.table}`}</Code>} variant="outlined" />
        <Chip label={t(`mode.${state.mode}`)} color="primary" variant="outlined" />
        <Chip
          label={state.present ? t('rendered.present') : t('rendered.absent')}
          color={state.present ? 'success' : 'default'}
          variant="outlined"
        />
        <Chip
          label={state.inSync ? t('rendered.inSync') : t('rendered.drift')}
          color={state.inSync ? 'success' : 'error'}
          variant="outlined"
        />
        {state.retrievedAt && (
          <Typography variant="body2" color="text.secondary">
            {t('rendered.retrievedAt', { at: fmt.dateTime(state.retrievedAt) })}
          </Typography>
        )}
      </Stack>
      {state.chains.length === 0 && (
        <Typography color="text.secondary">{t('rendered.noChains')}</Typography>
      )}
      {state.chains.map((c) => (
        <Paper key={c.name} variant="outlined" sx={{ p: 1.5 }}>
          <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center" sx={{ mb: 1 }}>
            <Typography variant="h6" component="h3">
              <Code size={16}>{c.name}</Code>
            </Typography>
            <Chip size="small" label={t(`hook.${c.hook}`, { defaultValue: c.hook })} />
            <Chip
              size="small"
              label={t('rendered.priority', { priority: fmt.digits(String(c.priority)) })}
            />
            <Chip
              size="small"
              color={c.policy === 'drop' ? 'error' : 'success'}
              variant="outlined"
              label={t('rendered.policy', {
                policy: t(`action.${c.policy}`, { defaultValue: c.policy }),
              })}
            />
            {c.list && (
              <Chip size="small" variant="outlined" label={t('rendered.list', { list: c.list })} />
            )}
          </Stack>
          <TableContainer>
            <ReactGeereedMaterial
              tableProps={{
                size: 'small',
                'aria-label': t('rendered.chainLabel', { name: c.name }),
              }}
              columns={[
                {
                  key: 'kind',
                  headerRenderer: () => t('rendered.col.kind'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'rule',
                  headerRenderer: () => t('rendered.col.rule'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'verdict',
                  headerRenderer: () => t('rendered.col.verdict'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'packets',
                  headerRenderer: () => t('rendered.col.packets'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'bytes',
                  headerRenderer: () => t('rendered.col.bytes'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
              ]}
              items={[
                ...(c.rules.map((r, i) => ({
                  id: `${r.comment}-${i}`,
                  kind: (
                    <>
                      {t(`kind.${r.kind}`)}
                      {r.kind === 'rule' && (
                        <Typography variant="caption" component="div" color="text.secondary">
                          <Code size={11}>{`${r.list}:${r.sequence}`}</Code>
                        </Typography>
                      )}
                    </>
                  ),
                  rule: (
                    <>
                      <Code>{r.text || r.comment || '—'}</Code>
                    </>
                  ),
                  verdict: r.verdict ? t(`action.${r.verdict}`, { defaultValue: r.verdict }) : '—',
                  packets: fmt.digits(groupDigits(r.packets)),
                  bytes: fmt.digits(groupDigits(r.bytes)),
                })) ?? []),
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
        </Paper>
      ))}
      {state.sets.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('rendered.sets') }}
            columns={[
              {
                key: 'column0',
                headerRenderer: () => t('rendered.setCol.name'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column1',
                headerRenderer: () => t('rendered.setCol.object'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column2',
                headerRenderer: () => t('rendered.setCol.type'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column3',
                headerRenderer: () => t('rendered.setCol.elements'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(state.sets.map((s) => ({
                id: s.name,
                column0: (
                  <>
                    <Code>{s.name}</Code>
                  </>
                ),
                column1: (
                  <>
                    <Code>{s.object || '—'}</Code>
                  </>
                ),
                column2: t(`rendered.family.${s.type === 'ipv6_addr' ? 'ipv6' : 'ipv4'}`),
                column3: (
                  <>
                    <Code>
                      {s.elements.length > 0 ? s.elements.join(', ') : t('rendered.emptySet')}
                    </Code>
                  </>
                ),
              })) ?? []),
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
    </Stack>
  );
}
