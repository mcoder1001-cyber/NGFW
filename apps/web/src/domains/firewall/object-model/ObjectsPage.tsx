import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import TravelExploreIcon from '@mui/icons-material/TravelExplore';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
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
import { useCandidateObjects, useFqdnState, useFreshCandidateObjects, usePatchObjects, useRunningObjects } from './queries';

const LTR = { dir: 'ltr' } as const;
const TAB_PARAM = 'tab';
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** Server pointers `/objects/<kind>/<name>/…` → pointers relative to the edited entry. */
export function problemFor(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({ ...e, pointer: e.pointer.startsWith(prefix) ? e.pointer.slice(prefix.length) : e.pointer })),
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
        onChange={(_e, v: ObjectKind) => setParams((p) => { p.set(TAB_PARAM, v); return p; }, { replace: true })}
        variant="scrollable"
        allowScrollButtonsMobile
        aria-label={t('tabsLabel')}
        sx={{ mb: 2, borderBlockEnd: 1, borderColor: 'divider' }}
      >
        {OBJECT_KINDS.map((k) => (
          <Tab key={k} value={k} label={t(`tabs.${k}`)} id={`objects-tab-${k}`} aria-controls={`objects-panel-${k}`} />
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
  const [editing, setEditing] = useState<{ name: string; value: AnyEntry | undefined } | null>(null);
  const [lastError, setLastError] = useState<unknown>(null);
  const readOnly = !perms.editConfig;

  const fqdnBy = useMemo(() => new Map<string, FqdnItem>((fqdn.data?.items ?? []).map((i) => [i.name, i])), [fqdn.data]);
  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return entriesOf(candidate.data, kind).filter(([name, e]) => !q || `${name} ${summary(kind, e)} ${tagsOf(e).join(' ')} ${'description' in e ? (e.description ?? '') : ''}`.toLowerCase().includes(q));
  }, [candidate.data, kind, filter]);
  const runningOf = (name: string) => ((running.data?.[kind] ?? {}) as Record<string, AnyEntry>)[name];

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
            <Button variant="contained" startIcon={<AddIcon />} disabled={readOnly} onClick={() => setEditing({ name: '', value: undefined })}>
              {t('add', { kind: t(`kindOne.${kind}`) })}
            </Button>
          </span>
        </Tooltip>
        <Box sx={{ flex: 1 }} />
        <TextField size="small" label={t('filter')} value={filter} onChange={(e) => setFilter(e.target.value)} />
      </Stack>
      {lastError !== null && <ProblemAlert error={lastError} sx={{ mb: 1 }} />}
      {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} />}
      <TableContainer component={Paper} variant="outlined">
        <Table size="small" aria-label={t(`tabs.${kind}`)}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.name')}</TableCell>
              {(kind === 'addresses' || kind === 'services' || kind === 'schedules') && <TableCell>{t('col.type')}</TableCell>}
              <TableCell>{t(`col.value.${kind}`)}</TableCell>
              {showFqdn && <TableCell>{t('col.resolved')}</TableCell>}
              {kind !== 'tags' && <TableCell>{t('col.tags')}</TableCell>}
              <TableCell>{t('col.description')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {candidate.isSuccess && rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={8}>
                  <Typography color="text.secondary">{filter ? t('noMatch') : t('empty', { kind: t(`tabs.${kind}`) })}</Typography>
                </TableCell>
              </TableRow>
            )}
            {rows.map(([name, e]) => {
              const pending = running.isSuccess && !sameEntry(runningOf(name), e);
              const variant = variantOf(kind, e);
              const isFqdn = kind === 'addresses' && variant === 'fqdn';
              return (
                <TableRow key={name} hover sx={{ cursor: 'pointer' }} onClick={() => setEditing({ name, value: e })}>
                  <TableCell>
                    <Stack direction="row" gap={0.5} alignItems="center">
                      <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                        {name}
                      </Box>
                      {pending && <Chip size="small" color="warning" variant="outlined" label={t('pending')} />}
                    </Stack>
                  </TableCell>
                  {(kind === 'addresses' || kind === 'services' || kind === 'schedules') && <TableCell>{t(`variant.${variant}`, { defaultValue: variant })}</TableCell>}
                  <TableCell>
                    {kind === 'tags' ? (
                      <TagChips tags={[name]} objects={candidate.data} />
                    ) : (
                      <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: 12, textAlign: 'start' }}>
                        {summary(kind, e) || '—'}
                      </Box>
                    )}
                  </TableCell>
                  {showFqdn && (
                    <TableCell>
                      {isFqdn ? <FqdnCell item={fqdnBy.get(name)} applied={runningOf(name) !== undefined} unavailable={fqdn.isError} /> : null}
                    </TableCell>
                  )}
                  {kind !== 'tags' && (
                    <TableCell>
                      <TagChips tags={tagsOf(e)} objects={candidate.data} />
                    </TableCell>
                  )}
                  <TableCell>{'description' in e ? e.description : null}</TableCell>
                  <TableCell sx={{ textAlign: 'end', whiteSpace: 'nowrap' }} onClick={(ev) => ev.stopPropagation()}>
                    <Tooltip title={t('whereUsed')}>
                      <IconButton size="small" aria-label={t('whereUsedOf', { name })} onClick={() => onUsage(name)}>
                        <TravelExploreIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    <IconButton size="small" aria-label={t('deleteNamed', { name })} disabled={readOnly || patch.isPending} onClick={() => void remove(name)}>
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {editing?.value ? t('dialog.editTitle', { kind: t(`kindOne.${kind}`), name: editing.name }) : t('dialog.addTitle', { kind: t(`kindOne.${kind}`) })}
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
  const schema = useMemo(() => localizeSchema(itemSchema(kind), (k, o) => t(k, o ?? {}), kind), [kind, t]);
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
      {kind === 'addressGroups' || kind === 'serviceGroups' ? <Alert severity="info">{t('dialog.groupHint')}</Alert> : null}
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
          <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={readOnly || patch.isPending} onClick={() => void remove()}>
            {t('dialog.remove')}
          </Button>
        )}
        <Button onClick={onDone}>{t('dialog.cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
