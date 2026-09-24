import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
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
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import type { UserConfig } from '@ngfw/schema';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api';
import { ApiError, call } from '../api-problem';
import { useAuth, usePermissions } from '../auth/AuthProvider';
import { ProblemAlert } from '../config/ProblemAlert';
import { invalidateConfig, qk } from '../config/queries';
import { domainSchemas } from '../schema/registry';
import { PageHeader } from '../shell/PageHeader';

/** A configured user as the API returns it: redacted, so `passwordHash` is never present (D-046/D-070). */
export type ConfigUser = Omit<UserConfig, 'passwordHash'> & { passwordHash?: string };

type Translate = (key: string, opts?: Record<string, unknown>) => string;

const USERS_POINTER = '/management/users';

/** `management.users[]` item schema — the one schema, never a hand-written form (00-CONTEXT rule 5). */
export function userItemSchema(): JsonSchema {
  const mgmt = domainSchemas.management as { properties?: { users?: { items?: JsonSchema } } };
  const item = mgmt.properties?.users?.items;
  if (!item) throw new Error('management.users schema not found');
  return item;
}

/** Titles and help of the user schema in the UI language (the schema's English text stays the fallback). */
export function localizeUserSchema(schema: JsonSchema, t: Translate): JsonSchema {
  const props = (schema.properties ?? {}) as Record<string, JsonSchema>;
  const localized: Record<string, JsonSchema> = {};
  for (const [name, prop] of Object.entries(props)) {
    const hints = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
    const help = t(`field.${name}.help`, { defaultValue: '' });
    localized[name] = {
      ...prop,
      title: t(`field.${name}.title`, { defaultValue: prop.title ?? name }),
      'x-vrx-ui': { ...hints, ...(help ? { help } : {}) },
    } as JsonSchema;
  }
  return { ...schema, properties: localized } as JsonSchema;
}

/** Server pointers `/management/users/<i>/…` → pointers relative to the edited item, for `<SchemaForm problem>`. */
export function problemForItem(error: unknown, index: number): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  const prefix = `${USERS_POINTER}/${index}`;
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({ ...e, pointer: e.pointer.startsWith(prefix) ? e.pointer.slice(prefix.length) || '' : e.pointer })),
  };
}

function Why({ reason, children }: { reason: string | undefined; children: ReactElement }) {
  return reason ? (
    <Tooltip title={reason}>
      <span>{children}</span>
    </Tooltip>
  ) : (
    children
  );
}

/** The signed-in (bootstrap) admin as a configured user; P06 keeps its stored password hash (hydrated by username). */
export function adminEntry(username: string): ConfigUser {
  return { username, role: 'admin', scope: '*', sshKeys: [], disabled: false };
}

type RowState = 'new' | 'changed' | 'removed' | undefined;

/** Candidate vs running, matched by username (the natural key P06 also uses for hashes). */
export function rowStates(candidate: readonly ConfigUser[], running: readonly ConfigUser[]): { user: ConfigUser; index: number; state: RowState }[] {
  const byName = new Map(running.map((u) => [u.username, u]));
  const rows: { user: ConfigUser; index: number; state: RowState }[] = candidate.map((u, index) => {
    const r = byName.get(u.username);
    return { user: u, index, state: r === undefined ? 'new' : JSON.stringify(r) === JSON.stringify(u) ? undefined : 'changed' };
  });
  const names = new Set(candidate.map((u) => u.username));
  for (const r of running) if (!names.has(r.username)) rows.push({ user: r, index: -1, state: 'removed' });
  return rows;
}

function useManagement(which: 'running' | 'candidate') {
  return useQuery({
    queryKey: which === 'candidate' ? qk.candidate('management') : (['config', 'running', 'management'] as const),
    queryFn: async ({ signal }) => {
      const r =
        which === 'candidate'
          ? await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'management' } }, signal }))
          : await call(api.GET('/api/v1/config/{path}', { params: { path: { path: 'management' } }, signal }));
      return ((r.data as { users?: ConfigUser[] } | undefined)?.users ?? []) as ConfigUser[];
    },
    refetchInterval: 5_000,
  });
}

/**
 * System › Users (P07 §9): CRUD on `management.users` in the candidate via the schema form; changes go through the
 * pending-change bar → commit like every other edit. Admin only (P06 RBAC); other roles see the list, actions disabled.
 */
export function UsersPage() {
  const { t } = useTranslation(['users', 'config', 'auth']);
  const fmt = useFormatters();
  const perms = usePermissions();
  const { state } = useAuth();
  const qc = useQueryClient();
  const candidate = useManagement('candidate');
  const running = useManagement('running');
  const [editing, setEditing] = useState<{ index: number; user: ConfigUser | null } | null>(null);
  const [deleting, setDeleting] = useState<{ index: number; user: ConfigUser } | null>(null);

  const schema = useMemo(() => localizeUserSchema(userItemSchema(), (k, o) => t(k, o ?? {})), [t]);
  const users = useMemo(() => candidate.data ?? [], [candidate.data]);
  const rows = useMemo(() => rowStates(users, running.data ?? []), [users, running.data]);

  const save = useMutation({
    mutationFn: async (next: ConfigUser[]) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'management' } }, body: { users: next } })),
    onSuccess: () => invalidateConfig(qc),
  });

  const blocked = perms.manageUsers ? undefined : t('adminOnly', { role: t(`auth:role.${perms.role ?? 'readonly'}`) });
  const me = state.user?.username ?? '';
  const meListed = users.some((u) => u.username === me);

  const openEditor = (e: { index: number; user: ConfigUser | null }) => {
    save.reset();
    setEditing(e);
  };

  const submitUser = async (value: unknown) => {
    if (!editing) return;
    const user = value as ConfigUser;
    const next = [...users];
    if (editing.index < 0) next.push(user);
    else next[editing.index] = user;
    try {
      await save.mutateAsync(next);
      setEditing(null);
    } catch {
      // the problem (pointers mapped onto the fields) is rendered from `save.error`
    }
  };

  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      {blocked && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="users-readonly">
          {blocked}
        </Alert>
      )}
      {candidate.isSuccess && users.length === 0 && (
        <Alert severity="info" sx={{ mb: 2 }}>
          <AlertTitle>{t('empty.title')}</AlertTitle>
          {t('empty.body', { user: me })}
          {perms.manageUsers && !meListed && (
            <Box sx={{ mt: 1 }}>
              <Button
                size="small"
                variant="outlined"
                disabled={save.isPending}
                onClick={() => save.mutate([adminEntry(me)])}
              >
                {t('empty.addMe', { user: me })}
              </Button>
            </Box>
          )}
        </Alert>
      )}
      <Stack direction="row" sx={{ mb: 1 }}>
        <Why reason={blocked}>
          <Button variant="contained" startIcon={<AddIcon />} disabled={blocked !== undefined} onClick={() => openEditor({ index: -1, user: null })}>
            {t('add')}
          </Button>
        </Why>
      </Stack>
      {(candidate.isPending || running.isPending) && <LinearProgress aria-label={t('config:loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} />}
      {!editing && save.isError && <ProblemAlert error={save.error} sx={{ mb: 1 }} />}
      <TableContainer component={Paper} variant="outlined">
        <Table size="small" aria-label={t('title')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.username')}</TableCell>
              <TableCell>{t('col.fullName')}</TableCell>
              <TableCell>{t('col.role')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
              <TableCell>{t('col.sshKeys')}</TableCell>
              <TableCell>{t('col.pending')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.length === 0 && candidate.isSuccess && (
              <TableRow>
                <TableCell colSpan={7}>
                  <Typography color="text.secondary">{t('none')}</Typography>
                </TableCell>
              </TableRow>
            )}
            {rows.map(({ user, index, state: rowState }) => (
              <TableRow key={`${user.username}:${index}`} sx={rowState === 'removed' ? { '& td': { textDecoration: 'line-through', color: 'text.disabled' } } : undefined}>
                <TableCell>
                  <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                    {user.username}
                  </Box>
                  {user.username === me && <Chip size="small" label={t('you')} sx={{ marginInlineStart: 1 }} />}
                </TableCell>
                <TableCell>{user.fullName ?? ''}</TableCell>
                <TableCell>
                  <Chip size="small" variant="outlined" label={t(`auth:role.${user.role}`)} />
                </TableCell>
                <TableCell>{user.disabled ? t('status.disabled') : t('status.enabled')}</TableCell>
                <TableCell>{fmt.integer(user.sshKeys?.length ?? 0)}</TableCell>
                <TableCell>{rowState && <Chip size="small" color={rowState === 'removed' ? 'error' : 'warning'} label={t(`pending.${rowState}`)} />}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>
                  {rowState !== 'removed' && (
                    <>
                      <Why reason={blocked}>
                        <IconButton
                          size="small"
                          aria-label={t('edit', { user: user.username })}
                          disabled={blocked !== undefined}
                          onClick={() => openEditor({ index, user })}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Why>
                      <Why reason={blocked}>
                        <IconButton
                          size="small"
                          aria-label={t('delete', { user: user.username })}
                          disabled={blocked !== undefined}
                          onClick={() => {
                            save.reset();
                            setDeleting({ index, user });
                          }}
                        >
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </Why>
                    </>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
        {t('bootstrapNote')}
      </Typography>

      <Dialog open={editing !== null} onClose={save.isPending ? undefined : () => setEditing(null)} maxWidth="sm" fullWidth aria-labelledby="user-title">
        <DialogTitle id="user-title">{editing?.user ? t('editTitle', { user: editing.user.username }) : t('addTitle')}</DialogTitle>
        <DialogContent dividers>
          <Alert severity="info" sx={{ mb: 2 }}>
            {t('passwordNote')}
          </Alert>
          {editing && (
            <SchemaForm
              id="user-form"
              schema={schema}
              value={editing.user ?? undefined}
              onSubmit={submitUser}
              problem={save.error ? problemForItem(save.error, editing.index < 0 ? users.length : editing.index) : null}
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)} disabled={save.isPending}>
            {t('config:cancel')}
          </Button>
          <Button type="submit" form="user-form" variant="contained" disabled={save.isPending}>
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={deleting !== null} onClose={() => setDeleting(null)} aria-labelledby="del-title">
        <DialogTitle id="del-title">{t('deleteTitle', { user: deleting?.user.username ?? '' })}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('deleteBody')}</DialogContentText>
          {deleting?.user.username === me && (
            <Alert severity="warning" sx={{ mt: 1 }}>
              {t('deleteSelf')}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleting(null)}>{t('config:cancel')}</Button>
          <Button
            color="error"
            variant="contained"
            disabled={save.isPending}
            onClick={() => {
              if (!deleting) return;
              save.mutate(
                users.filter((_, i) => i !== deleting.index),
                { onSuccess: () => setDeleting(null) },
              );
            }}
          >
            {t('deleteConfirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
