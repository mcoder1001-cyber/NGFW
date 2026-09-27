import AddIcon from '@mui/icons-material/Add';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload';
import DeleteIcon from '@mui/icons-material/Delete';
import DownloadIcon from '@mui/icons-material/Download';
import EditIcon from '@mui/icons-material/Edit';
import RefreshIcon from '@mui/icons-material/Refresh';
import UploadFileIcon from '@mui/icons-material/UploadFile';
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
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { ApiError, call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { readText } from '../../firewall/acl/ImportDialog';
import { localizeSchema } from '../../firewall/host-acl-nftables/model';
import { useCandidateAcl, usePatchAcl } from '../../firewall/host-acl-nftables/queries';
import { objectModelWidgets } from '../../firewall/object-model';
import { useCandidate } from '../../routing/rpf-adl-pbr/queries';
import { formValue, intervalText, LIST_NAME_RE, listSchema, type GbList } from './model';
import {
  exportList,
  fetchListNow,
  importList,
  invalidateGb,
  useGbStatus,
  type GbListStatus,
  type GbPreview,
} from './queries';

const NS = 'global-blocking';
const FILE_ACCEPT = '.txt,.list,.csv,text/plain';
const DIFF_KINDS = ['added', 'removed'] as const;
const NEW_LIST: Dlg = { kind: 'edit' };
const EDIT = 'edit';
const IMPORT = 'import';
const FETCH = 'fetch';
const dialogFor = (kind: 'edit' | 'import' | 'fetch', name: string): Dlg => ({ kind, name });

type Dlg =
  | { kind: 'edit'; name?: string }
  | { kind: 'import'; name: string }
  | { kind: 'fetch'; name: string }
  | null;

/**
 * `/firewall/global-blocking` (F-global-blocking): IP block lists enforced ahead of the access lists on the chosen
 * interfaces and, with "Protect the box", on traffic to the appliance. A list's entries come from an uploaded file or
 * its server URL — always previewed (added / removed / invalid lines) before they are staged into the candidate; the
 * normal commit applies them. URL lists with a refresh interval are re-downloaded by the box on schedule.
 */
export function GlobalBlockingPage() {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const perms = usePermissions();
  const readOnly = !perms.editConfig;
  const status = useGbStatus();
  const acl = useCandidateAcl();
  const patch = usePatchAcl();
  const qc = useQueryClient();
  const [dlg, setDlg] = useState<Dlg>(null);
  const [error, setError] = useState<unknown>(null);
  const lists =
    (acl.data as { globalBlocking?: { lists?: Record<string, GbList> } } | undefined)
      ?.globalBlocking?.lists ?? {};
  const n = (v: number) => fmt.integer(v);

  const remove = async (name: string) => {
    setError(null);
    try {
      await patch.mutateAsync({ globalBlocking: { lists: { [name]: null } } });
      await invalidateGb(qc);
    } catch (e) {
      setError(e);
    }
  };
  const doExport = async (name: string) => {
    setError(null);
    try {
      await exportList(name, 'candidate');
    } catch (e) {
      setError(e);
    }
  };

  const s = status.data;
  const edit = dlg?.kind === 'edit' ? dlg : null;
  const transfer = dlg !== null && dlg.kind !== 'edit' ? dlg : null;
  const fetching = transfer?.kind === 'fetch';
  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mb: 2 }}>
        <Tooltip title={readOnly ? t('readonly') : ''}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={readOnly}
              onClick={() => setDlg(NEW_LIST)}
            >
              {t('add')}
            </Button>
          </span>
        </Tooltip>
        <Button startIcon={<RefreshIcon />} onClick={() => void invalidateGb(qc)}>
          {t('refresh')}
        </Button>
        {s && (
          <Typography variant="body2" color="text.secondary">
            {t('total', { total: n(s.totalEntries), max: n(s.maxEntries) })}
          </Typography>
        )}
      </Stack>
      {(status.isLoading || acl.isLoading) && (
        <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />
      )}
      {status.error && <ProblemAlert error={status.error} sx={{ mb: 1 }} />}
      {error !== null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      {s?.countersError && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('countersUnavailable', { reason: s.countersError })}
        </Alert>
      )}
      {s && s.lists.length === 0 && <Alert severity="info">{t('empty')}</Alert>}
      {s && s.lists.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('tableLabel')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.name')}</TableCell>
                <TableCell>{t('col.source')}</TableCell>
                <TableCell>{t('col.enforced')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.entries')}</TableCell>
                <TableCell>{t('col.lastFetch')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.hits')}</TableCell>
                <TableCell>{t('col.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {s.lists.map((l) => (
                <ListRow
                  key={l.name}
                  l={l}
                  readOnly={readOnly}
                  onEdit={() => setDlg(dialogFor(EDIT, l.name))}
                  onImport={() => setDlg(dialogFor(IMPORT, l.name))}
                  onFetch={() => setDlg(dialogFor(FETCH, l.name))}
                  onExport={() => void doExport(l.name)}
                  onDelete={() => void remove(l.name)}
                />
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      <Dialog open={edit !== null} onClose={() => setDlg(null)} fullWidth maxWidth="md">
        <DialogTitle>
          {edit?.name ? t('edit.title', { name: edit.name }) : t('edit.new')}
        </DialogTitle>
        <DialogContent>
          {edit && (
            <ListForm
              name={edit.name}
              lists={lists}
              readOnly={readOnly}
              onDone={() => setDlg(null)}
            />
          )}
        </DialogContent>
      </Dialog>
      <Dialog open={transfer !== null} onClose={() => setDlg(null)} fullWidth maxWidth="md">
        {transfer && (
          <>
            <DialogTitle>{t(`${transfer.kind}.title`, { name: transfer.name })}</DialogTitle>
            <ImportBody name={transfer.name} fetch={fetching} onClose={() => setDlg(null)} />
          </>
        )}
      </Dialog>
    </Box>
  );
}

function ListRow({
  l,
  readOnly,
  onEdit,
  onImport,
  onFetch,
  onExport,
  onDelete,
}: {
  l: GbListStatus;
  readOnly: boolean;
  onEdit: () => void;
  onImport: () => void;
  onFetch: () => void;
  onExport: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const n = (v: number) => fmt.integer(v);
  const f = l.fetch;
  return (
    <TableRow>
      <TableCell>
        <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
          <bdi>{l.name}</bdi>
          {!l.enabled && <Chip size="small" label={t('disabled')} />}
          {l.pending && (
            <Chip
              size="small"
              color="warning"
              variant="outlined"
              label={t(`pending.${l.pending}`)}
            />
          )}
        </Stack>
        {l.description && (
          <Typography variant="caption" color="text.secondary">
            {l.description}
          </Typography>
        )}
      </TableCell>
      <TableCell>
        {l.source.kind === 'url' ? (
          <>
            <Typography
              variant="body2"
              dir="ltr"
              sx={{ wordBreak: 'break-all', fontFamily: 'monospace' }}
            >
              {l.source.url}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {l.source.refreshSec
                ? t('refreshEvery', { interval: intervalText(l.source.refreshSec) })
                : t('refreshManual')}
            </Typography>
          </>
        ) : (
          t('source.upload')
        )}
      </TableCell>
      <TableCell>
        <Typography variant="body2">
          {l.allInterfaces ? t('allInterfaces') : <bdi>{l.interfaces.join(', ')}</bdi>}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {t(`direction.${l.direction}`)}
          {l.protectHost ? ` · ${t('protectHost')}` : ''}
        </Typography>
      </TableCell>
      <TableCell sx={{ textAlign: 'end' }}>
        {n(l.entries)}
        {l.entries !== l.runningEntries && (
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            {t('runningEntries', { n: n(l.runningEntries) })}
          </Typography>
        )}
      </TableCell>
      <TableCell>
        {f ? (
          <Tooltip title={f.lastError ?? ''}>
            <Stack gap={0.5} alignItems="flex-start">
              <Chip
                size="small"
                color={
                  f.lastResult === 'failed'
                    ? 'error'
                    : f.lastResult === 'deferred'
                      ? 'warning'
                      : 'success'
                }
                label={t(`result.${f.lastResult}`)}
              />
              <Typography variant="caption" color="text.secondary">
                {fmt.dateTime(new Date(f.lastFetchAt))}
              </Typography>
            </Stack>
          </Tooltip>
        ) : (
          '—'
        )}
        {l.nextRefreshAt && (
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            {t('nextRefresh', { at: fmt.dateTime(new Date(l.nextRefreshAt)) })}
          </Typography>
        )}
      </TableCell>
      <TableCell sx={{ textAlign: 'end' }}>
        {l.hits ? (
          <>
            {t('hits.dataplane', { n: n(l.hits.dataplanePackets) })}
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
              {t('hits.host', { n: n(l.hits.hostPackets) })}
            </Typography>
          </>
        ) : (
          '—'
        )}
      </TableCell>
      <TableCell>
        <Stack direction="row">
          <Tooltip title={t('actions.edit')}>
            <span>
              <IconButton size="small" aria-label={t('actions.edit')} onClick={onEdit}>
                <EditIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title={t('actions.import')}>
            <span>
              <IconButton
                size="small"
                aria-label={t('actions.import')}
                disabled={readOnly || l.pending === 'deleted'}
                onClick={onImport}
              >
                <UploadFileIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          {l.source.kind === 'url' && (
            <Tooltip title={t('actions.fetch')}>
              <span>
                <IconButton
                  size="small"
                  aria-label={t('actions.fetch')}
                  disabled={readOnly || l.pending === 'deleted'}
                  onClick={onFetch}
                >
                  <CloudDownloadIcon fontSize="small" />
                </IconButton>
              </span>
            </Tooltip>
          )}
          <Tooltip title={t('actions.export')}>
            <span>
              <IconButton
                size="small"
                aria-label={t('actions.export')}
                disabled={l.pending === 'deleted'}
                onClick={onExport}
              >
                <DownloadIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title={t('actions.delete')}>
            <span>
              <IconButton
                size="small"
                aria-label={t('actions.delete')}
                disabled={readOnly || l.pending === 'deleted'}
                onClick={onDelete}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        </Stack>
      </TableCell>
    </TableRow>
  );
}

/** Form problems relative to the list (the API answers with document pointers). */
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

function ListForm({
  name: initialName,
  lists,
  readOnly,
  onDone,
}: {
  name?: string | undefined;
  lists: Record<string, GbList>;
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation(NS);
  const qc = useQueryClient();
  const schema = useMemo(() => localizeSchema(listSchema(), (k, o) => t(k, o ?? {})), [t]);
  const ifs =
    useCandidate<Record<string, { subinterfaces?: Record<string, unknown> }>>('interfaces');
  const ifNames = useMemo(
    () =>
      Object.entries(ifs.data ?? {})
        .flatMap(([n, i]) => [n, ...Object.keys(i.subinterfaces ?? {}).map((s) => `${n}.${s}`)])
        .sort(),
    [ifs.data],
  );
  const editing = initialName !== undefined;
  const [name, setName] = useState(initialName ?? '');
  const [error, setError] = useState<unknown>(null);
  const taken = !editing && name !== '' && Object.hasOwn(lists, name);
  const nameOk = LIST_NAME_RE.test(name) && !taken;
  const current = editing ? lists[initialName] : undefined;

  const save = async (v: unknown) => {
    if (!nameOk) return;
    setError(null);
    const body = { ...(v as GbList), entries: current?.entries ?? [] };
    try {
      if (current) {
        // PUT: switching the source kind must drop the old kind's fields (a merge patch would keep them)
        await call(
          api.PUT('/api/v1/config/{path}', {
            params: { path: { path: `acl/globalBlocking/lists/${name}` } },
            body,
          }),
        );
      } else {
        await call(
          api.PATCH('/api/v1/config/{path}', {
            params: { path: { path: 'acl' } },
            body: { globalBlocking: { lists: { [name]: body } } },
          }),
        );
      }
      await invalidateGb(qc);
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/acl/globalBlocking/lists/${name}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('edit.name')}
        value={name}
        disabled={editing || readOnly}
        onChange={(e) => setName(e.target.value)}
        error={name !== '' && !nameOk}
        helperText={
          taken ? t('edit.taken') : name !== '' && !nameOk ? t('edit.badName') : t('edit.nameHelp')
        }
        size="small"
      />
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={formValue(current)}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        interfaceOptions={ifNames}
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

function ImportBody({
  name,
  fetch,
  onClose,
}: {
  name: string;
  fetch: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const qc = useQueryClient();
  const [file, setFile] = useState<{ name: string; text: string } | null>(null);
  const [check, setCheck] = useState<GbPreview | null>(null);
  const [done, setDone] = useState<GbPreview | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const n = (v: number) => fmt.integer(v);

  const pick = async (f: File | undefined) => {
    setCheck(null);
    setDone(null);
    setError(null);
    setFile(f ? { name: f.name, text: await readText(f) } : null);
  };
  const run = async (dryRun: boolean) => {
    setBusy(true);
    setError(null);
    try {
      const r = fetch
        ? await fetchListNow(name, dryRun)
        : await importList(name, file?.text ?? '', dryRun);
      if (dryRun) setCheck(r);
      else {
        setDone(r);
        await invalidateGb(qc);
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const ready = fetch || file !== null;
  return (
    <>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>
          {t(fetch ? 'fetch.help' : 'import.help')}
        </DialogContentText>
        {!fetch && (
          <Stack direction="row" gap={2} alignItems="center" flexWrap="wrap" sx={{ mb: 2 }}>
            <Button
              component="label"
              variant="outlined"
              startIcon={<UploadFileIcon />}
              disabled={busy}
            >
              {t('import.choose')}
              <input
                hidden
                type="file"
                accept={FILE_ACCEPT}
                onChange={(e) => void pick(e.target.files?.[0])}
              />
            </Button>
            <Typography variant="body2" color="text.secondary">
              {file ? <bdi>{file.name}</bdi> : t('import.noFile')}
            </Typography>
          </Stack>
        )}
        {busy && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
        {error !== null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
        {done && (
          <Alert severity="success" sx={{ mb: 1 }}>
            {done.staged ? t('preview.staged', { entries: n(done.entries) }) : t('preview.nothing')}
          </Alert>
        )}
        {check && !done && <PreviewView p={check} />}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{done ? t('close') : t('cancel')}</Button>
        {!done && (
          <Button variant="outlined" disabled={!ready || busy} onClick={() => void run(true)}>
            {t('preview.check')}
          </Button>
        )}
        {!done && (
          <Button
            variant="contained"
            disabled={
              !ready || busy || !check || check.entries === 0 || check.added + check.removed === 0
            }
            onClick={() => void run(false)}
          >
            {t('preview.stage')}
          </Button>
        )}
      </DialogActions>
    </>
  );
}

function PreviewView({ p }: { p: GbPreview }) {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const n = (v: number) => fmt.integer(v);
  return (
    <Stack gap={1} role="region" aria-label={t('preview.label')}>
      <Alert severity={p.entries === 0 ? 'error' : p.invalidCount > 0 ? 'warning' : 'success'}>
        {t('preview.summary', {
          lines: n(p.lines),
          entries: n(p.entries),
          added: n(p.added),
          removed: n(p.removed),
          invalid: n(p.invalidCount),
        })}{' '}
        {p.normalised + p.collapsed > 0 &&
          t('preview.normalised', { normalised: n(p.normalised), collapsed: n(p.collapsed) })}
      </Alert>
      {p.invalid.length > 0 && (
        <Box sx={{ maxBlockSize: 220, overflow: 'auto' }}>
          <Table size="small" aria-label={t('preview.invalid')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('preview.line')}</TableCell>
                <TableCell>{t('preview.text')}</TableCell>
                <TableCell>{t('preview.reason')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {p.invalid.map((i) => (
                <TableRow key={i.line}>
                  <TableCell>{n(i.line)}</TableCell>
                  <TableCell dir="ltr" sx={{ fontFamily: 'monospace' }}>
                    {i.text}
                  </TableCell>
                  <TableCell>{i.reason}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      )}
      {(p.addedSample.length > 0 || p.removedSample.length > 0) && (
        <Stack direction="row" gap={2} flexWrap="wrap">
          {DIFF_KINDS.map((k) => {
            const xs = k === 'added' ? p.addedSample : p.removedSample;
            return xs.length === 0 ? null : (
              <Box key={k} sx={{ minInlineSize: 220 }}>
                <Typography variant="subtitle2">
                  {t(`preview.${k}`, { n: n(k === 'added' ? p.added : p.removed) })}
                </Typography>
                <Box
                  component="pre"
                  dir="ltr"
                  sx={{ m: 0, maxBlockSize: 160, overflow: 'auto', fontSize: 12 }}
                >
                  {xs.join('\n')}
                </Box>
              </Box>
            );
          })}
        </Stack>
      )}
    </Stack>
  );
}
