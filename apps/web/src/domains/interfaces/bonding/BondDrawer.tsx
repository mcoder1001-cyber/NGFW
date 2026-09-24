import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../InterfaceDrawer';
import { createMergePatch, dropPhantomOptionals, localizeSchema } from '../model';
import { useCandidateInterfaces, useFreshCandidate, useInterfacesState, usePatchInterfaces } from '../queries';
import { bondFormSchema, bondStatus, eligibleMembers, memberSchema, memberStatus, type BondConfig, type BondMemberConfig, type LiveMember } from './model';
import { useBondsState } from './queries';

const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** The bond leaf without its `members` (edited in their own table). */
function withoutMembers(b: BondConfig | undefined): Partial<BondConfig> | undefined {
  if (!b) return undefined;
  return Object.fromEntries(Object.entries(b).filter(([k]) => k !== 'members')) as Partial<BondConfig>;
}

/** Opens from the end of the reading direction (MUI flips `anchor="right"` in RTL), like P08's interface drawer. */
export function BondDrawer({ name, onClose }: { name: string | null; onClose: () => void }) {
  return (
    <Drawer anchor="right" open={name !== null} onClose={onClose} sx={{ zIndex: (th) => th.zIndex.modal }} slotProps={{ paper: { sx: { inlineSize: { xs: '100%', md: 720 } } } }}>
      {name !== null && <DrawerBody key={name} name={name} onClose={onClose} />}
    </Drawer>
  );
}

function lacpText(m: LiveMember, t: (k: string, o?: Record<string, unknown>) => string): string {
  if (!m.lacp) return '—';
  const mux = t(`lacp.mux.${m.lacp.muxState}`, { defaultValue: m.lacp.muxState });
  const rx = t(`lacp.rx.${m.lacp.rxState}`, { defaultValue: m.lacp.rxState });
  return `${mux} · ${rx}`;
}

function DrawerBody({ name, onClose }: { name: string; onClose: () => void }) {
  const { t } = useTranslation(['bonding', 'interfaces']);
  const perms = usePermissions();
  const bonds = useBondsState();
  const ifState = useInterfacesState();
  const candidate = useCandidateInterfaces();
  const fresh = useFreshCandidate();
  const patch = usePatchInterfaces();
  const [saved, setSaved] = useState(false);
  const [memberDialog, setMemberDialog] = useState<{ name: string; value: BondMemberConfig | undefined } | null>(null);
  const [memberError, setMemberError] = useState<unknown>(null);
  const readOnly = !perms.editConfig;

  const tr = (k: string, o?: Record<string, unknown>) => t(k, o ?? {});
  const formSchema = useMemo(() => localizeSchema(bondFormSchema(), (k, o) => t(k, o ?? {})), [t]);
  const mSchema = useMemo(() => localizeSchema(memberSchema(), (k, o) => t(k, o ?? {})), [t]);
  const item = bonds.data?.items.find((i) => i.name === name);
  const live = item?.state ?? null;
  const itf = candidate.data?.[name];
  const config = itf?.bond;
  const members = Object.entries(config?.members ?? {}).sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }));
  const eligible = useMemo(
    () => eligibleMembers(name, candidate.data ?? {}, ifState.data?.items ?? []).filter((m) => !(m in (config?.members ?? {}))),
    [name, candidate.data, ifState.data, config?.members],
  );

  // As in P08's drawer (review N4): the form edits the value it opened with and saves only what changed against it.
  const [opened, setOpened] = useState<Partial<BondConfig> | undefined | null>(null);
  if (candidate.isSuccess && opened === null) setOpened(withoutMembers(config));

  const saveBond = async (value: unknown) => {
    setSaved(false);
    const base = opened ?? undefined;
    const cleaned = dropPhantomOptionals(formSchema, base, value);
    const body = base === undefined ? cleaned : createMergePatch(base, cleaned);
    try {
      await patch.mutateAsync({ [name]: { bond: body } });
      setOpened(cleaned as Partial<BondConfig>);
      setSaved(true);
    } catch {
      // rendered from patch.error (pointers mapped onto the fields)
    }
  };

  const removeBond = async () => {
    try {
      await patch.mutateAsync({ [name]: null });
      onClose();
    } catch {
      // shown below
    }
  };

  const saveMember = async (member: string, value: unknown) => {
    setMemberError(null);
    const current = await fresh();
    const prev = current[name]?.bond?.members?.[member];
    const cleaned = dropPhantomOptionals(mSchema, prev, value);
    const body: Record<string, unknown> = { [name]: { bond: { members: { [member]: prev === undefined ? cleaned : createMergePatch(prev, cleaned) } } } };
    // a member needs its own interfaces entry (interfaces.bonding-member-exists): a NIC not configured yet is added enabled
    if (current[member] === undefined) body[member] = { enabled: true };
    try {
      await patch.mutateAsync(body);
      setMemberDialog(null);
    } catch (e) {
      setMemberError(e);
    }
  };

  const removeMember = async (member: string) => {
    setMemberError(null);
    try {
      await patch.mutateAsync({ [name]: { bond: { members: { [member]: null } } } });
    } catch (e) {
      setMemberError(e);
    }
  };

  const liveBy = new Map((live?.members ?? []).map((m) => [m.interface, m]));
  const status = bondStatus(live);

  return (
    <Box sx={{ p: 2 }} role="region" aria-label={t('drawer.label', { name })}>
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, flex: 1, textAlign: 'start' }}>
          {name}
        </Typography>
        {status && <StatusChip size="small" status={status} label={t(`status.${status}`)} />}
        <IconButton aria-label={t('close')} onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Stack>
      {(bonds.isPending || candidate.isPending) && <LinearProgress aria-label={t('loading')} />}
      {!live && bonds.isSuccess && <Alert severity="info">{bonds.data.live ? t('drawer.notInVpp') : t('drawer.noLive')}</Alert>}
      {live && (
        <Table size="small" aria-label={t('drawer.liveTitle')} sx={{ mb: 2 }}>
          <TableBody>
            {(
              [
                [t('live.vppName'), live.vppName],
                [t('live.swIfIndex'), String(live.swIfIndex)],
                [t('col.mode'), `${t(`mode.${live.mode}`, { defaultValue: live.mode })} · ${live.loadBalance}`],
                [t('col.active'), t('live.activeOf', { active: live.activeMemberCount, total: live.memberCount })],
              ] as const
            ).map(([k, v]) => (
              <TableRow key={k}>
                <TableCell component="th" sx={{ inlineSize: 200 }}>
                  {k}
                </TableCell>
                <TableCell dir="ltr" sx={{ textAlign: 'start' }}>
                  {v}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('drawer.interfaceHint')}{' '}
        <RouterLink to="/interfaces">{t('drawer.interfaceLink')}</RouterLink>
      </Alert>
      <Divider sx={{ mb: 2 }} />

      <Typography component="h4" variant="subtitle1" gutterBottom>
        {t('drawer.configTitle')}
      </Typography>
      {!config && candidate.isSuccess && <Alert severity="info" sx={{ mb: 1 }}>{t('drawer.notConfigured')}</Alert>}
      {patch.isError && !memberDialog && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {saved && !patch.isError && (
        <Alert severity="success" sx={{ mb: 1 }}>
          {t('drawer.saved')}
        </Alert>
      )}
      {candidate.isSuccess && opened !== null && (
        <SchemaForm
          schema={formSchema}
          value={opened}
          readOnly={readOnly}
          submitLabel={t('drawer.save')}
          resetLabel={t('drawer.reset')}
          problem={problemFor(patch.error, `/interfaces/${esc(name)}/bond`)}
          onSubmit={saveBond}
        >
          {itf && (
            <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={readOnly || patch.isPending} onClick={() => void removeBond()}>
              {t('drawer.remove')}
            </Button>
          )}
        </SchemaForm>
      )}

      <Divider sx={{ my: 2 }} />
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography component="h4" variant="subtitle1" sx={{ flex: 1 }}>
          {t('members.title')}
        </Typography>
        <AddMember options={eligible} disabled={readOnly || !config} onPick={(m) => setMemberDialog({ name: m, value: undefined })} />
      </Stack>
      {memberError !== null && !memberDialog && <ProblemAlert error={memberError} sx={{ mb: 1 }} />}
      <Table size="small" aria-label={t('members.title')}>
        <TableHead>
          <TableRow>
            <TableCell>{t('members.interface')}</TableCell>
            <TableCell>{t('members.link')}</TableCell>
            <TableCell>{t('members.options')}</TableCell>
            <TableCell>{t('members.lacp')}</TableCell>
            <TableCell sx={{ textAlign: 'end' }}>{t('members.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {members.length === 0 && (
            <TableRow>
              <TableCell colSpan={5}>
                <Typography color="text.secondary">{t('members.none')}</Typography>
              </TableCell>
            </TableRow>
          )}
          {members.map(([m, mc]) => {
            const lm = liveBy.get(m);
            const opts = [mc.passive ? t('field.passive.title') : '', mc.longTimeout ? t('field.longTimeout.title') : '', mc.weight !== undefined ? t('members.weight', { weight: mc.weight }) : '']
              .filter(Boolean)
              .join(', ');
            return (
              <TableRow key={m} hover sx={{ cursor: readOnly ? 'default' : 'pointer' }} onClick={() => !readOnly && setMemberDialog({ name: m, value: mc })}>
                <TableCell dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                  {m}
                </TableCell>
                <TableCell>{lm ? <StatusChip size="small" status={memberStatus(lm)} label={t(`status.${memberStatus(lm)}`)} /> : t('notInVpp')}</TableCell>
                <TableCell>{opts || '—'}</TableCell>
                <TableCell>
                  {lm?.lacp ? (
                    <Stack>
                      <span>{lacpText(lm, tr)}</span>
                      <Typography variant="caption" color="text.secondary" dir="ltr">
                        {t('lacp.detail', { flags: lm.lacp.actor.stateFlags.join(' ') || '—', system: lm.lacp.partner.system || '—' })}
                      </Typography>
                    </Stack>
                  ) : (
                    '—'
                  )}
                </TableCell>
                <TableCell sx={{ textAlign: 'end' }}>
                  <IconButton
                    size="small"
                    aria-label={t('members.remove', { name: m })}
                    disabled={readOnly || patch.isPending}
                    onClick={(e) => {
                      e.stopPropagation();
                      void removeMember(m);
                    }}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>

      <Dialog open={memberDialog !== null} onClose={() => setMemberDialog(null)} fullWidth maxWidth="sm">
        <DialogTitle>{memberDialog ? t(memberDialog.value ? 'members.editTitle' : 'members.addTitle', { name: memberDialog.name, bond: name }) : ''}</DialogTitle>
        <DialogContent>
          {memberError !== null && <ProblemAlert error={memberError} sx={{ mb: 1 }} />}
          {memberDialog && (
            <SchemaForm
              schema={mSchema}
              value={memberDialog.value}
              readOnly={readOnly}
              submitLabel={t('drawer.save')}
              resetLabel={t('drawer.reset')}
              problem={problemFor(memberError, `/interfaces/${esc(name)}/bond/members/${esc(memberDialog.name)}`)}
              onSubmit={(v) => saveMember(memberDialog.name, v)}
            />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}

/** Member picker: only the NICs that may join this bond (eligibleMembers). */
function AddMember({ options, disabled, onPick }: { options: string[]; disabled: boolean; onPick: (name: string) => void }) {
  const { t } = useTranslation('bonding');
  const [value, setValue] = useState('');
  return (
    <Stack direction="row" gap={1} alignItems="center">
      <TextField
        select
        size="small"
        label={t('members.pick')}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        disabled={disabled || options.length === 0}
        helperText={options.length === 0 ? t('members.noneEligible') : ' '}
        sx={{ minInlineSize: 200 }}
      >
        {options.map((o) => (
          <MenuItem key={o} value={o} dir="ltr">
            {o}
          </MenuItem>
        ))}
      </TextField>
      <Button
        size="small"
        startIcon={<AddIcon />}
        disabled={disabled || value === ''}
        onClick={() => {
          onPick(value);
          setValue('');
        }}
      >
        {t('members.add')}
      </Button>
    </Stack>
  );
}
