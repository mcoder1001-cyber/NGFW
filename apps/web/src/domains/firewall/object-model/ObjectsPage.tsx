import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgTravelExploreIcon as TravelExploreIcon,
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
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import {
  entriesOf,
  isObjectKind,
  itemSchema,
  localizeSchema,
  mergePatch,
  OBJECT_KINDS,
  OBJECT_NAME_RE,
  sameEntry,
  summary,
  tagsOf,
  variantOf,
  type AnyEntry,
  type FqdnItem,
  type ObjectKind,
  type ObjectsConfig,
} from './model';
import { objectModelWidgets } from './ObjectPicker';
import { FqdnCell, TagChips, UsageDrawer } from './parts';
import {
  useCandidateObjects,
  useFqdnState,
  useFreshCandidateObjects,
  usePatchObjects,
  useRunningObjects,
} from './queries';

const LTR = { dir: 'ltr' } as const;
const TAB_PARAM = 'tab';
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** Server pointers `/objects/<kind>/<name>/…` → pointers relative to the edited entry. */
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

/**
 * F-object-model: the Objects page (`/firewall/objects`) — one tab per kind (`?tab=`), a list with pending marks, tag
 * chips in their colours, the FQDN resolution column (agent FqdnObjectState), a where-used drawer, and a schema-driven
 * edit dialog (SchemaForm with the object and tag pickers). Every edit is a merge patch of the candidate's `objects`.
 */
export function ObjectsPage() {
  const { t } = useTranslation('object-model');
  const [params, setParams] = useSearchParams();
  const tabParam = params.get(TAB_PARAM);
  const tab: ObjectKind = isObjectKind(tabParam) ? tabParam : 'addresses';
  const [usageOf, setUsageOf] = useState<string | null>(null);
  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <Tabs
        value={tab}
        onChange={(_e, v: ObjectKind) =>
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
        {OBJECT_KINDS.map((k) => (
          <Tab
            key={k}
            value={k}
            label={t(`tabs.${k}`)}
            id={`objects-tab-${k}`}
            aria-controls={`objects-panel-${k}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`objects-panel-${tab}`} aria-labelledby={`objects-tab-${tab}`}>
        <KindTable key={tab} kind={tab} onUsage={setUsageOf} />
      </Box>
      <UsageDrawer name={usageOf} onClose={() => setUsageOf(null)} />
    </PageHeader>
  );
}

function KindTable({ kind, onUsage }: { kind: ObjectKind; onUsage: (name: string) => void }) {
  const { t } = useTranslation('object-model');
  const perms = usePermissions();
  const candidate = useCandidateObjects();
  const running = useRunningObjects();
  const fqdn = useFqdnState();
  const patch = usePatchObjects();
  const [filter, setFilter] = useState('');
  const [editing, setEditing] = useState<{ name: string; value: AnyEntry | undefined } | null>(
    null,
  );
  const [lastError, setLastError] = useState<unknown>(null);
  const readOnly = !perms.editConfig;

  const fqdnBy = useMemo(
    () => new Map<string, FqdnItem>((fqdn.data?.items ?? []).map((i) => [i.name, i])),
    [fqdn.data],
  );
  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return entriesOf(candidate.data, kind).filter(
      ([name, e]) =>
        !q ||
        `${name} ${summary(kind, e)} ${tagsOf(e).join(' ')} ${'description' in e ? (e.description ?? '') : ''}`
          .toLowerCase()
          .includes(q),
    );
  }, [candidate.data, kind, filter]);
  const runningOf = (name: string) =>
    ((running.data?.[kind] ?? {}) as Record<string, AnyEntry>)[name];

  const remove = async (name: string) => {
    setLastError(null);
    try {
      await patch.mutateAsync({ [kind]: { [name]: null } });
    } catch (e) {
      setLastError(e);
    }
  };

  const showFqdn = kind === 'addresses';
  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center" flexWrap="wrap">
        <Tooltip title={readOnly ? t('readonly') : ''}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={readOnly}
              onClick={() => setEditing({ name: '', value: undefined })}
            >
              {t('add', { kind: t(`kindOne.${kind}`) })}
            </Button>
          </span>
        </Tooltip>
        <Box sx={{ flex: 1 }} />
        <TextField
          size="small"
          label={t('filter')}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </Stack>
      {lastError !== null && <ProblemAlert error={lastError} sx={{ mb: 1 }} />}
      {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} />}
      <TableContainer component={Paper} variant="outlined">
        <ReactGeereedMaterial
          items={rows.map(([name, entry]) => ({ name, entry }))}
          columns={[
            {
              key: 'name',
              title: t('col.name'),
              filterable: false,
              renderer: (_value, { name, entry }) => (
                <Stack direction="row" gap={0.5} alignItems="center">
                  <Box
                    component="span"
                    dir="ltr"
                    sx={{ fontFamily: (th) => th.ngfw.monoFontFamily }}
                  >
                    {name}
                  </Box>
                  {running.isSuccess && !sameEntry(runningOf(name), entry) && (
                    <Chip size="small" color="warning" variant="outlined" label={t('pending')} />
                  )}
                </Stack>
              ),
            },
            ...(kind === 'addresses' || kind === 'services' || kind === 'schedules'
              ? [
                  {
                    key: 'type',
                    title: t('col.type'),
                    filterable: false,
                    renderer: (_value: unknown, row: { entry: (typeof rows)[number][1] }) => (
                      <>
                        {t('variant.' + variantOf(kind, row.entry), {
                          defaultValue: variantOf(kind, row.entry),
                        })}
                      </>
                    ),
                  },
                ]
              : []),
            {
              key: 'value',
              title: t('col.value.' + kind),
              filterable: false,
              renderer: (_value, { name, entry }) =>
                kind === 'tags' ? (
                  <TagChips tags={[name]} objects={candidate.data} />
                ) : (
                  <Box
                    component="span"
                    dir="ltr"
                    sx={{
                      fontFamily: (th) => th.ngfw.monoFontFamily,
                      fontSize: 12,
                      textAlign: 'start',
                    }}
                  >
                    {summary(kind, entry) || '—'}
                  </Box>
                ),
            },
            ...(showFqdn
              ? [
                  {
                    key: 'resolved',
                    title: t('col.resolved'),
                    filterable: false,
                    renderer: (
                      _value: unknown,
                      { name, entry }: { name: string; entry: (typeof rows)[number][1] },
                    ) => (
                      <>
                        {kind === 'addresses' && variantOf(kind, entry) === 'fqdn' ? (
                          <FqdnCell
                            item={fqdnBy.get(name)}
                            applied={runningOf(name) !== undefined}
                            unavailable={fqdn.isError}
                          />
                        ) : null}
                      </>
                    ),
                  },
                ]
              : []),
            ...(kind !== 'tags'
              ? [
                  {
                    key: 'tags',
                    title: t('col.tags'),
                    filterable: false,
                    renderer: (_value: unknown, { entry }: { entry: (typeof rows)[number][1] }) => (
                      <TagChips tags={tagsOf(entry)} objects={candidate.data} />
                    ),
                  },
                ]
              : []),
            {
              key: 'description',
              title: t('col.description'),
              filterable: false,
              renderer: (_value, { entry }) => (
                <>{'description' in entry ? entry.description : null}</>
              ),
            },
            {
              key: 'actions',
              title: t('col.actions'),
              filterable: false,
              headerCellProps: { sx: { textAlign: 'end' } },
              renderer: (_value, { name }) => (
                <>
                  <Tooltip title={t('whereUsed')}>
                    <IconButton
                      size="small"
                      aria-label={t('whereUsedOf', { name })}
                      onClick={() => onUsage(name)}
                    >
                      <TravelExploreIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <IconButton
                    size="small"
                    aria-label={t('deleteNamed', { name })}
                    disabled={readOnly || patch.isPending}
                    onClick={() => void remove(name)}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </>
              ),
            },
          ]}
          rowKey={(row) => row.name}
          rowProps={({ name, entry }) => ({
            hover: true,
            sx: { cursor: 'pointer' },
            onClick: () => setEditing({ name, value: entry }),
          })}
          cellProps={(_row, column) =>
            column.key === 'actions'
              ? {
                  sx: { textAlign: 'end', whiteSpace: 'nowrap' },
                  onClick: (event) => event.stopPropagation(),
                }
              : {}
          }
          tableProps={{ size: 'small', 'aria-label': t('tabs.' + kind) }}
          emptyContent={
            candidate.isSuccess && (
              <Typography color="text.secondary">
                {filter ? t('noMatch') : t('empty', { kind: t('tabs.' + kind) })}
              </Typography>
            )
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
            ? t('dialog.editTitle', { kind: t(`kindOne.${kind}`), name: editing.name })
            : t('dialog.addTitle', { kind: t(`kindOne.${kind}`) })}
        </DialogTitle>
        <DialogContent>
          {editing && (
            <ObjectForm
              key={editing.name || 'new'}
              kind={kind}
              initialName={editing.name}
              value={editing.value}
              objects={candidate.data}
              readOnly={readOnly}
              onDone={() => setEditing(null)}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ObjectForm({
  kind,
  initialName,
  value,
  objects,
  readOnly,
  onDone,
}: {
  kind: ObjectKind;
  initialName: string;
  value: AnyEntry | undefined;
  objects: Partial<ObjectsConfig> | undefined;
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation('object-model');
  const patch = usePatchObjects();
  const fresh = useFreshCandidateObjects();
  const editing = value !== undefined;
  const [name, setName] = useState(initialName);
  const [error, setError] = useState<unknown>(null);
  const schema = useMemo(
    () => localizeSchema(itemSchema(kind), (k, o) => t(k, o ?? {}), kind),
    [kind, t],
  );
  const taken = !editing && name !== '' && Object.hasOwn((objects?.[kind] ?? {}) as object, name);
  const nameOk = OBJECT_NAME_RE.test(name) && !taken;

  const save = async (v: unknown) => {
    if (!nameOk) return;
    setError(null);
    // edits send only what changed against the entry as it is in the candidate now (another session may have changed it)
    const current = ((await fresh())[kind] as Record<string, unknown> | undefined)?.[name];
    const body = current === undefined ? v : mergePatch(current, v);
    try {
      await patch.mutateAsync({ [kind]: { [name]: body } });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const remove = async () => {
    setError(null);
    try {
      await patch.mutateAsync({ [kind]: { [name]: null } });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/objects/${kind}/${esc(name)}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('dialog.name')}
        value={name}
        disabled={editing}
        onChange={(e) => setName(e.target.value.trim())}
        error={name !== '' && !nameOk}
        helperText={taken ? t('dialog.exists') : t('dialog.nameHelp')}
        slotProps={{ htmlInput: LTR }}
      />
      {kind === 'addressGroups' || kind === 'serviceGroups' ? (
        <Alert severity="info">{t('dialog.groupHint')}</Alert>
      ) : null}
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={value}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        problem={problem}
        submitLabel={t('dialog.save')}
        resetLabel={t('dialog.reset')}
        onSubmit={save}
      >
        {editing && (
          <Button
            color="error"
            variant="outlined"
            startIcon={<DeleteIcon />}
            disabled={readOnly || patch.isPending}
            onClick={() => void remove()}
          >
            {t('dialog.remove')}
          </Button>
        )}
        <Button onClick={onDone}>{t('dialog.cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
