import AddIcon from '@mui/icons-material/Add';
import AutorenewIcon from '@mui/icons-material/Autorenew';
import DeleteIcon from '@mui/icons-material/Delete';
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
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import type { paths } from '@ngfw/api-client';
import { SECRET_KINDS, secretRef, type SecretKind } from '@ngfw/schema';
import { useFormatters } from '@ngfw/ui-kit';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useRef, useState, type CSSProperties, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api';
import { ApiError, call } from '../api-problem';
import { usePermissions } from '../auth/AuthProvider';
import { CollectionDrawer } from '../config/collection/CollectionDrawer';
import { ProblemAlert } from '../config/ProblemAlert';
import { IdText, Why } from '../config/widgets/cells';
import { KeyButton } from '../config/widgets/KeyButton';
import { LocalDataGrid } from '../config/widgets/LocalDataGrid';
import { PageHeader } from '../shell/PageHeader';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;

/** One entry of `GET /api/v1/secrets` as generated from the OpenAPI document: a reference, never a value. */
export type SecretMeta = Ok<NonNullable<paths['/api/v1/secrets']['get']>>[number];
/** `POST /api/v1/secrets` answer: the reference to put into the configuration and the stored version. */
export type SecretPutResult = Ok<NonNullable<paths['/api/v1/secrets']['post']>>;

/** Query keys mirror API paths. Only references are cached here — values never enter the query cache. */
export const secretKeys = { list: ['secrets'] as const };

/** Kinds whose values are multi-line PEM blocks (a password input would drop the line breaks). */
const MULTILINE: ReadonlySet<SecretKind> = new Set<SecretKind>(['key', 'cert']);
const LTR_INPUT = { dir: 'ltr', spellCheck: false, autoComplete: 'off' } as const;

/** `<kind>/<name>` is a valid reference per the ONE schema (`secretRef` of packages/schema, D-051). */
export function validRef(kind: SecretKind, name: string): boolean {
  return secretRef.safeParse(`${kind}/${name}`).success;
}

function useSecretList() {
  return useQuery({
    queryKey: secretKeys.list,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/secrets', { signal }))).data,
  });
}

type Target = { mode: 'create' } | { mode: 'rotate'; secret: SecretMeta };
const CREATE: Target = { mode: 'create' };
const rotate = (secret: SecretMeta): Target => ({ mode: 'rotate', secret });
const GRID_KEY = ['kit', 'secrets'] as const;
const SINGLE_LINE = { type: 'text' } as const;
const MULTI = { multiline: true, minRows: 6 } as const;
/**
 * The value input: LTR, no spell-check, testable without its (secret) label text, and opted out of every major
 * password manager's heuristics (review M2) — never `type=password` (see `MASK_STYLE`), `autoComplete="off"`, plus
 * the vendor "ignore this field" attributes (1Password, LastPass/Bitwarden-style, generic).
 */
const VALUE_INPUT = {
  dir: 'ltr',
  spellCheck: false,
  autoComplete: 'off',
  'data-1p-ignore': true,
  'data-lpignore': 'true',
  'data-bwignore': true,
  'data-form-type': 'other',
  'data-testid': 'secret-value',
} as const;
/**
 * Masks the single-line value visually without `type=password` (review M2: a password-type field inside a submitted
 * form is what makes Chromium/Firefox offer to save it). `-webkit-text-security` is Chromium/WebKit only — Firefox and
 * other engines show the value in clear text; there is no cross-browser masked plain-text input. jsdom does not paint
 * CSS, so this cannot be pinned by a unit test (review M2); the E2E step (Q2) verifies it in Chromium.
 */
const MASK_STYLE = { WebkitTextSecurity: 'disc' } as unknown as CSSProperties;

/**
 * System › Secrets (D-051): the store behind `<kind>/<name>` references in the configuration — list, create, rotate
 * (a new version; revisions keep theirs for rollback) and delete by reference. The secret VALUE is typed into an
 * uncontrolled input and read from the DOM only inside the request function: it is never React or form state, never a
 * mutation variable or result in the query cache, never logged, and the input is emptied after a successful store.
 * The value field is a plain text input masked with CSS, not `type=password`, and sits outside any `<form>` element,
 * so the browser's password manager is never offered a value to save (review M2).
 * Changes are immediate (not part of the candidate/commit flow), admin only; every call is audited by the API.
 */
export function SecretsPage() {
  const { t } = useTranslation('config');
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const list = useSecretList();
  const [selected, setSelected] = useState<string | null>(null);
  const [target, setTarget] = useState<Target | null>(null);
  const [deleting, setDeleting] = useState<SecretMeta | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const blocked = perms.role === 'admin' ? undefined : t('kit.secrets.adminOnly');

  const remove = useMutation({
    mutationFn: async (s: SecretMeta) => call(api.DELETE('/api/v1/secrets/{kind}/{name}', { params: { path: { kind: s.kind, name: s.name } } })),
    onSuccess: (_r, s) => {
      setDeleting(null);
      if (selected === s.ref) setSelected(null);
      setNotice(t('kit.secrets.deleted', { ref: s.ref }));
    },
    onSettled: () => qc.invalidateQueries({ queryKey: secretKeys.list }),
  });

  const resetRemove = remove.reset;
  const rows = useMemo(() => (list.data ?? []).map((s) => ({ ...s, id: s.ref })), [list.data]);
  const current = rows.find((r) => r.ref === selected);

  const columns = useMemo<GridColDef<SecretMeta & { id: string }>[]>(
    () => [
      {
        field: 'ref',
        headerName: t('kit.secrets.col.ref'),
        minWidth: 200,
        flex: 1,
        renderCell: (p) => (
          <KeyButton tabIndex={p.tabIndex} hasFocus={p.hasFocus} aria-label={t('kit.open', { key: p.row.ref })} onClick={() => setSelected(p.row.ref)}>
            {p.row.ref}
          </KeyButton>
        ),
      },
      { field: 'kind', headerName: t('kit.secrets.col.kind'), width: 150, renderCell: (p) => <Chip size="small" variant="outlined" label={t(`kit.secrets.kind.${p.row.kind}`)} /> },
      { field: 'name', headerName: t('kit.secrets.col.name'), minWidth: 160, flex: 1, renderCell: (p) => <IdText>{p.row.name}</IdText> },
      { field: 'createdAt', headerName: t('kit.secrets.col.created'), width: 190, valueFormatter: (v: string) => fmt.dateTime(v) },
      {
        field: 'actions',
        headerName: t('kit.col.actions'),
        width: 120,
        sortable: false,
        filterable: false,
        renderCell: (p) => (
          <Stack direction="row" sx={{ blockSize: '100%' }} alignItems="center">
            <Why reason={blocked}>
              <IconButton size="small" tabIndex={p.tabIndex} aria-label={t('kit.secrets.rotateRef', { ref: p.row.ref })} disabled={blocked !== undefined} onClick={(e) => { e.stopPropagation(); setTarget(rotate(p.row)); }}>
                <AutorenewIcon fontSize="small" />
              </IconButton>
            </Why>
            <Why reason={blocked}>
              <IconButton size="small" tabIndex={p.tabIndex} aria-label={t('kit.secrets.deleteRef', { ref: p.row.ref })} disabled={blocked !== undefined} onClick={(e) => { e.stopPropagation(); resetRemove(); setDeleting(p.row); }}>
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Why>
          </Stack>
        ),
      },
    ],
    [t, fmt, blocked, resetRemove],
  );

  return (
    <PageHeader title={t('kit.secrets.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('kit.secrets.intro')}
      </Typography>
      <Stack direction="row" sx={{ mb: 1 }}>
        <Why reason={blocked}>
          <Button variant="contained" startIcon={<AddIcon />} disabled={blocked !== undefined} onClick={() => setTarget(CREATE)}>
            {t('kit.secrets.add')}
          </Button>
        </Why>
      </Stack>
      {notice && (
        <Alert severity="success" sx={{ mb: 1 }} onClose={() => setNotice(null)} data-testid="secret-notice">
          {notice}
        </Alert>
      )}
      {list.isPending && <LinearProgress aria-label={t('loading')} />}
      {list.isError && <ProblemAlert error={list.error} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: 440 }}>
        <LocalDataGrid
          aria-label={t('kit.secrets.title')}
          gridKey={GRID_KEY}
          rows={rows}
          columns={columns}
          searchText={(r) => `${r.ref} ${r.kind} ${r.name}`}
        />
      </Paper>

      <CollectionDrawer open={current !== undefined} onClose={() => setSelected(null)} title={current?.ref ?? ''} label={t('kit.secrets.drawerLabel', { ref: current?.ref ?? '' })}>
        {current && (
          <>
            <Table size="small" aria-label={t('kit.secrets.details')} sx={{ mb: 2 }}>
              <TableBody>
                <TableRow>
                  <TableCell component="th" sx={{ inlineSize: 180 }}>{t('kit.secrets.col.ref')}</TableCell>
                  <TableCell><IdText>{current.ref}</IdText></TableCell>
                </TableRow>
                <TableRow>
                  <TableCell component="th">{t('kit.secrets.col.kind')}</TableCell>
                  <TableCell>{t(`kit.secrets.kind.${current.kind}`)}</TableCell>
                </TableRow>
                <TableRow>
                  <TableCell component="th">{t('kit.secrets.col.created')}</TableCell>
                  <TableCell>{fmt.dateTime(current.createdAt)}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
            <Alert severity="info" sx={{ mb: 2 }}>
              {t('kit.secrets.usage', { ref: current.ref })}
            </Alert>
            <Stack direction="row" gap={1}>
              <Why reason={blocked}>
                <Button variant="outlined" startIcon={<AutorenewIcon />} disabled={blocked !== undefined} onClick={() => setTarget(rotate(current))}>
                  {t('kit.secrets.rotate')}
                </Button>
              </Why>
              <Why reason={blocked}>
                <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={blocked !== undefined} onClick={() => { remove.reset(); setDeleting(current); }}>
                  {t('kit.secrets.delete')}
                </Button>
              </Why>
            </Stack>
          </>
        )}
      </CollectionDrawer>

      {target && (
        <SecretValueDialog
          target={target}
          onClose={() => setTarget(null)}
          onStored={(r) => {
            setTarget(null);
            setNotice(t(r.created ? 'kit.secrets.created' : 'kit.secrets.rotated', { ref: r.ref, version: fmt.integer(r.version) }));
          }}
        />
      )}

      <Dialog open={deleting !== null} onClose={() => setDeleting(null)} aria-labelledby="secret-del-title" maxWidth="sm" fullWidth>
        <DialogTitle id="secret-del-title">{t('kit.secrets.deleteTitle', { ref: deleting?.ref ?? '' })}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('kit.secrets.deleteBody')}</DialogContentText>
          {remove.isError && <ProblemAlert error={remove.error} sx={{ mt: 1 }} />}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleting(null)}>{t('cancel')}</Button>
          <Button color="error" variant="contained" disabled={remove.isPending || deleting === null} onClick={() => deleting && remove.mutate(deleting)}>
            {t('kit.secrets.deleteConfirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}

/** Where the typed value goes: only the server. Mutation variables hold the reference parts, never the value. */
interface PutArgs {
  kind: SecretKind;
  name: string;
  replace: boolean;
}

function SecretValueDialog({ target, onClose, onStored }: { target: Target; onClose: () => void; onStored: (r: SecretPutResult) => void }) {
  const { t } = useTranslation('config');
  const qc = useQueryClient();
  const rotating = target.mode === 'rotate';
  const [kind, setKind] = useState<SecretKind>(rotating ? target.secret.kind : 'psk');
  const [name, setName] = useState(rotating ? target.secret.name : '');
  // whether the value input is empty — a boolean, never the value itself
  const [hasValue, setHasValue] = useState(false);
  const valueInput = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null);
  const nameOk = rotating || validRef(kind, name);

  const put = useMutation({
    mutationFn: async ({ kind: k, name: n, replace }: PutArgs): Promise<SecretPutResult> => {
      const value = valueInput.current?.value ?? '';
      const { data } = await call(api.POST('/api/v1/secrets', { params: { query: replace ? { replace: 'true' } : {} }, body: { kind: k, name: n, value } }));
      return data;
    },
    onSuccess: (r) => {
      if (valueInput.current) valueInput.current.value = '';
      setHasValue(false);
      onStored(r);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: secretKeys.list }),
  });

  // server pointers: `/value` → the value field, `/name` → the name field; 409 secret-exists → the name field
  const pointers = put.error instanceof ApiError ? (put.error.body.errors ?? []).map((e) => e.pointer) : [];
  const exists = put.error instanceof ApiError && put.error.slug === 'secret-exists';
  const nameError = (name !== '' && !nameOk) || pointers.includes('/name') || exists;
  const valueError = pointers.includes('/value');
  const multiline = MULTILINE.has(kind);

  const submit = () => {
    if (nameOk && hasValue && !put.isPending) put.mutate({ kind, name, replace: rotating });
  };
  // no <form> wraps this dialog (review M2): Enter is wired by hand on the single-line fields instead of relying on
  // implicit form submission. A multi-line (PEM) field keeps Enter as a newline, as it would inside a real form too.
  const submitOnEnter = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      submit();
    }
  };

  return (
    <Dialog open onClose={put.isPending ? undefined : onClose} maxWidth="sm" fullWidth aria-labelledby="secret-value-title">
      <Box>
        <DialogTitle id="secret-value-title">{rotating ? t('kit.secrets.rotateTitle', { ref: target.secret.ref }) : t('kit.secrets.addTitle')}</DialogTitle>
        <DialogContent>
          <Stack gap={2} sx={{ pt: 1 }}>
            <Alert severity="info">{rotating ? t('kit.secrets.rotateNote') : t('kit.secrets.valueNote')}</Alert>
            <TextField select label={t('kit.secrets.col.kind')} value={kind} disabled={rotating} onChange={(e) => {
                const next = e.target.value as SecretKind;
                if (MULTILINE.has(next) !== multiline) setHasValue(false); // the value input is replaced (and empty)
                setKind(next);
              }}
            >
              {SECRET_KINDS.map((k) => (
                <MenuItem key={k} value={k}>
                  {t(`kit.secrets.kind.${k}`)}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              label={t('kit.secrets.col.name')}
              value={name}
              disabled={rotating}
              required
              onChange={(e) => setName(e.target.value.trim())}
              onKeyDown={submitOnEnter}
              error={nameError}
              helperText={exists ? t('kit.secrets.exists', { ref: `${kind}/${name}` }) : t('kit.secrets.nameHelp', { ref: `${kind}/${name || '…'}` })}
              slotProps={{ htmlInput: LTR_INPUT }}
            />
            <TextField
              key={multiline ? 'multi' : 'single'}
              inputRef={valueInput}
              label={t('kit.secrets.value')}
              {...(multiline ? MULTI : SINGLE_LINE)}
              required
              error={valueError}
              helperText={t(multiline ? 'kit.secrets.valueHelpPem' : 'kit.secrets.valueHelp')}
              onChange={(e) => setHasValue(e.target.value !== '')}
              onKeyDown={multiline ? undefined : submitOnEnter}
              slotProps={{ htmlInput: multiline ? VALUE_INPUT : { ...VALUE_INPUT, style: MASK_STYLE } }}
            />
            {put.isError && !exists && <ProblemAlert error={put.error} />}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} disabled={put.isPending}>
            {t('cancel')}
          </Button>
          <Button onClick={submit} variant="contained" disabled={!nameOk || !hasValue || put.isPending}>
            {rotating ? t('kit.secrets.rotateSubmit') : t('kit.secrets.addSubmit')}
          </Button>
        </DialogActions>
      </Box>
    </Dialog>
  );
}
