import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
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
import Typography from '@mui/material/Typography';
import { objectName } from '@ngfw/schema';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import {
  attachmentKey,
  localize,
  NS,
  pathText,
  policyViews,
  problemUnder,
  schemas,
  statusColour,
  withChoices,
  type PbrAttachmentConfig,
  type PbrConfig,
  type PbrPolicyConfig,
} from './model';
import { useCandidate, useFreshPbr, usePatch, usePbrState, useRunning } from './queries';

const MONO = { fontFamily: 'monospace' } as const;
const NAME_INPUT = { dir: 'ltr', 'data-testid': 'policy-name' } as const;

interface RoutingDoc {
  pbr?: PbrConfig;
}

/**
 * Routing › Policy routing (F-rpf-adl-pbr, VPP ABF): the policies of `routing.pbr` — ACL, priority, paths — with the live
 * status from GET /api/v1/state/pbr, and their interface attachments. Edits are merge patches of the candidate's
 * `routing` (the generic pointer route); the pending-change bar shows the diff and commits it.
 */
export function PbrPage() {
  const { t } = useTranslation([NS, 'config']);
  const fmt = useFormatters();
  const perms = usePermissions();
  const routing = useCandidate<RoutingDoc>('routing');
  const running = useRunning<RoutingDoc>('routing');
  const acls = useCandidate<{ lists?: Record<string, unknown> }>('acl');
  const vrfs = useCandidate<Record<string, unknown>>('vrfs');
  const ifs =
    useCandidate<Record<string, { subinterfaces?: Record<string, unknown> }>>('interfaces');
  const state = usePbrState();
  const patch = usePatch('routing');
  const fresh = useFreshPbr();
  const [editing, setEditing] = useState<{
    name: string | null;
    value: PbrPolicyConfig | undefined;
  } | null>(null);
  const [newName, setNewName] = useState('');
  const [attaching, setAttaching] = useState(false);

  const readOnly = !perms.editConfig;
  const pbr = routing.data?.pbr;
  const views = useMemo(
    () => policyViews(pbr, state.data, running.data?.pbr),
    [pbr, state.data, running.data],
  );
  const aclNames = useMemo(() => Object.keys(acls.data?.lists ?? {}).sort(), [acls.data]);
  const vrfNames = useMemo(
    () => [
      'default',
      ...Object.keys(vrfs.data ?? {})
        .filter((v) => v !== 'default')
        .sort(),
    ],
    [vrfs.data],
  );
  const ifNames = useMemo(() => {
    const out: string[] = [];
    for (const [name, itf] of Object.entries(ifs.data ?? {})) {
      out.push(name);
      for (const id of Object.keys(itf.subinterfaces ?? {})) out.push(`${name}.${id}`);
    }
    return out.sort();
  }, [ifs.data]);
  const policyNames = useMemo(() => Object.keys(pbr?.policies ?? {}).sort(), [pbr]);

  const policySchema = useMemo(
    () =>
      localize(
        withChoices(schemas.policy(), { acl: aclNames, 'paths.items.vrf': vrfNames }),
        (k, o) => t(k, o ?? {}),
        'form.policy',
      ),
    [aclNames, vrfNames, t],
  );
  const attachSchema = useMemo(
    () =>
      localize(
        withChoices(schemas.attachment(), { policy: policyNames }),
        (k, o) => t(k, o ?? {}),
        'form.attachment',
      ),
    [policyNames, t],
  );
  const nameError = newName !== '' && !objectName.safeParse(newName).success;
  const liveAttach = new Map(
    (state.data?.attachments.items ?? []).map((a) => [attachmentKey(a), a]),
  );

  const savePolicy = async (value: unknown) => {
    if (!editing) return;
    const name = editing.name ?? newName;
    try {
      await patch.mutateAsync({ pbr: { policies: { [name]: value } } });
      setEditing(null);
      setNewName('');
    } catch {
      // shown by the form (pointers) or the alert
    }
  };
  const deletePolicy = async (name: string) => {
    const cur = await fresh();
    const attachments = (cur.attachments ?? []).filter((a) => a.policy !== name);
    await patch
      .mutateAsync({ pbr: { policies: { [name]: null }, attachments } })
      .catch(() => undefined);
  };
  const saveAttachment = async (value: unknown) => {
    const cur = await fresh();
    try {
      // canonical order (policy, interface, family) — the order Retrieve reports, so the drift view stays clean
      const next = [...(cur.attachments ?? []), value as PbrAttachmentConfig].sort((a, b) =>
        attachmentKey(a) < attachmentKey(b) ? -1 : attachmentKey(a) > attachmentKey(b) ? 1 : 0,
      );
      await patch.mutateAsync({ pbr: { attachments: next } });
      setAttaching(false);
    } catch {
      // shown by the form
    }
  };
  const deleteAttachment = async (key: string) => {
    const cur = await fresh();
    await patch
      .mutateAsync({
        pbr: { attachments: (cur.attachments ?? []).filter((a) => attachmentKey(a) !== key) },
      })
      .catch(() => undefined);
  };
  const editingBase = `/routing/pbr/policies/${(editing?.name ?? newName).replace(/~/g, '~0').replace(/\//g, '~1')}`;

  return (
    <PageHeader title={t('pbr.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('pbr.intro')}
      </Typography>
      {state.data?.pendingChange && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="pbr-pending">
          {t('pbr.pending')}
        </Alert>
      )}
      {aclNames.length === 0 && acls.isSuccess && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('pbr.noAcl')}
        </Alert>
      )}
      {(routing.isPending || state.isPending) && (
        <LinearProgress aria-label={t('config:loading')} />
      )}
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 1 }} />}
      {!editing && !attaching && patch.isError && (
        <ProblemAlert error={patch.error} sx={{ mb: 1 }} />
      )}

      <Stack direction="row" alignItems="center" sx={{ mb: 1 }} gap={2}>
        <Typography component="h3" variant="h6" sx={{ flexGrow: 1 }}>
          {t('pbr.policies')}
        </Typography>
        {state.data?.retrievedAt && (
          <Typography variant="body2" color="text.secondary">
            {t('pbr.retrievedAt', { time: fmt.time(new Date(state.data.retrievedAt)) })}
          </Typography>
        )}
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          disabled={readOnly}
          onClick={() => {
            patch.reset();
            setNewName('');
            setEditing({ name: null, value: undefined });
          }}
        >
          {t('pbr.addPolicy')}
        </Button>
      </Stack>
      <TableContainer component={Paper} variant="outlined" sx={{ mb: 3 }}>
        <Table size="small" aria-label={t('pbr.policies')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.name')}</TableCell>
              <TableCell>{t('col.acl')}</TableCell>
              <TableCell>{t('col.priority')}</TableCell>
              <TableCell>{t('col.paths')}</TableCell>
              <TableCell>{t('col.attachments')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {views.length === 0 && routing.isSuccess && (
              <TableRow>
                <TableCell colSpan={7}>
                  <Typography color="text.secondary">{t('pbr.none')}</Typography>
                </TableCell>
              </TableRow>
            )}
            {views.map((v) => {
              const shown = v.config ?? v.live;
              const paths = (shown?.paths ?? []) as {
                address?: string;
                interface?: string;
                vrf?: string;
                weight?: number;
              }[];
              return (
                <TableRow key={v.name} data-testid={`policy-${v.name}`}>
                  <TableCell>
                    <Box component="span" dir="ltr" sx={MONO}>
                      {v.name}
                    </Box>
                  </TableCell>
                  <TableCell dir="ltr">{shown?.acl ?? ''}</TableCell>
                  <TableCell>{fmt.integer(shown?.priority ?? 100)}</TableCell>
                  <TableCell>
                    <Stack>
                      {paths.map((p, i) => (
                        <Box key={i} component="span" dir="ltr" sx={MONO}>
                          {pathText(p, (k, o) => t(k, o ?? {}))}
                        </Box>
                      ))}
                    </Stack>
                  </TableCell>
                  <TableCell>
                    {fmt.integer(
                      (pbr?.attachments ?? []).filter((a) => a.policy === v.name).length,
                    )}
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" gap={0.5} flexWrap="wrap">
                      {v.live && (
                        <StatusChip
                          status={statusColour(v.live.status)}
                          label={t(`status.${v.live.status}`)}
                        />
                      )}
                      {v.pending && (
                        <Chip
                          size="small"
                          color={v.pending === 'removed' ? 'error' : 'warning'}
                          label={t(`pending.${v.pending}`)}
                        />
                      )}
                    </Stack>
                  </TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>
                    {v.config && (
                      <>
                        <IconButton
                          size="small"
                          aria-label={t('pbr.edit', { name: v.name })}
                          disabled={readOnly}
                          onClick={() => {
                            patch.reset();
                            setEditing({ name: v.name, value: v.config });
                          }}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          aria-label={t('pbr.delete', { name: v.name })}
                          disabled={readOnly || patch.isPending}
                          onClick={() => void deletePolicy(v.name)}
                        >
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>

      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" sx={{ flexGrow: 1 }}>
          {t('pbr.attachments')}
        </Typography>
        <Button
          variant="outlined"
          startIcon={<AddIcon />}
          disabled={readOnly || policyNames.length === 0}
          onClick={() => {
            patch.reset();
            setAttaching(true);
          }}
        >
          {t('pbr.addAttachment')}
        </Button>
      </Stack>
      <TableContainer component={Paper} variant="outlined">
        <Table size="small" aria-label={t('pbr.attachments')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.policy')}</TableCell>
              <TableCell>{t('col.interface')}</TableCell>
              <TableCell>{t('col.family')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(pbr?.attachments ?? []).length === 0 && (
              <TableRow>
                <TableCell colSpan={5}>
                  <Typography color="text.secondary">{t('pbr.noAttachments')}</Typography>
                </TableCell>
              </TableRow>
            )}
            {(pbr?.attachments ?? []).map((a) => {
              const key = attachmentKey(a);
              const live = liveAttach.get(key);
              return (
                <TableRow key={key}>
                  <TableCell dir="ltr">{a.policy}</TableCell>
                  <TableCell>
                    <Box component="span" dir="ltr" sx={MONO}>
                      {a.interface}
                    </Box>
                  </TableCell>
                  <TableCell>{t(`family.${a.family ?? 'ipv4'}`)}</TableCell>
                  <TableCell>
                    {live ? (
                      <StatusChip
                        status={statusColour(live.status)}
                        label={t(`status.${live.status}`)}
                      />
                    ) : (
                      <Chip size="small" color="warning" label={t('pending.new')} />
                    )}
                  </TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>
                    <IconButton
                      size="small"
                      aria-label={t('pbr.detach', { policy: a.policy, interface: a.interface })}
                      disabled={readOnly || patch.isPending}
                      onClick={() => void deleteAttachment(key)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
        {state.data?.counters.available === false ? t('pbr.noCounters') : ''}
      </Typography>

      <Dialog
        open={editing !== null}
        onClose={patch.isPending ? undefined : () => setEditing(null)}
        maxWidth="md"
        fullWidth
        aria-labelledby="pbr-policy-title"
      >
        <DialogTitle id="pbr-policy-title">
          {editing?.name ? t('pbr.editTitle', { name: editing.name }) : t('pbr.addTitle')}
        </DialogTitle>
        <DialogContent dividers>
          {editing?.name === null && (
            <TextField
              label={t('form.name.title')}
              helperText={nameError ? t('form.name.invalid') : t('form.name.help')}
              error={nameError}
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              fullWidth
              size="small"
              sx={{ mb: 2 }}
              slotProps={{ htmlInput: NAME_INPUT }}
            />
          )}
          {editing && (
            <SchemaForm
              id="pbr-policy-form"
              schema={policySchema}
              value={editing.value}
              onSubmit={savePolicy}
              interfaceOptions={ifNames}
              problem={patch.error ? problemUnder(patch.error, editingBase) : null}
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)} disabled={patch.isPending}>
            {t('config:cancel')}
          </Button>
          <Button
            type="submit"
            form="pbr-policy-form"
            variant="contained"
            disabled={patch.isPending || (editing?.name === null && (newName === '' || nameError))}
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={attaching}
        onClose={patch.isPending ? undefined : () => setAttaching(false)}
        maxWidth="sm"
        fullWidth
        aria-labelledby="pbr-attach-title"
      >
        <DialogTitle id="pbr-attach-title">{t('pbr.addAttachment')}</DialogTitle>
        <DialogContent dividers>
          {attaching && (
            <SchemaForm
              id="pbr-attach-form"
              schema={attachSchema}
              onSubmit={saveAttachment}
              interfaceOptions={ifNames}
              problem={
                patch.error
                  ? problemUnder(
                      patch.error,
                      `/routing/pbr/attachments/${(pbr?.attachments ?? []).length}`,
                    )
                  : null
              }
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAttaching(false)} disabled={patch.isPending}>
            {t('config:cancel')}
          </Button>
          <Button
            type="submit"
            form="pbr-attach-form"
            variant="contained"
            disabled={patch.isPending}
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
