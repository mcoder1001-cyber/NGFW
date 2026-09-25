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
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../api-problem';
import { usePermissions } from '../../auth/AuthProvider';
import { ProblemAlert } from '../../config/ProblemAlert';
import {
  adminStatus,
  createMergePatch,
  interfaceFormSchema,
  linkStatus,
  localizeSchema,
  subinterfaceSchema,
  type InterfaceConfig,
  type InterfaceItem,
  type SubinterfaceConfig,
} from './model';
import { useCandidateInterfaces, useFreshCandidate, useInterfacesState, usePatchInterfaces } from './queries';
import type { Rate } from './rates';
import { Sparkline } from './Sparkline';

/** Server pointers `/interfaces/<name>[/subinterfaces/<id>]/…` → pointers relative to the edited form. */
export function problemFor(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({ ...e, pointer: e.pointer.startsWith(prefix) ? e.pointer.slice(prefix.length) : e.pointer })),
  };
}

const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');
const SUB_ID_RE = /^[0-9]{1,10}$/;
const BPS = 'bps' as const;
const SUB_ID_INPUT = { dir: 'ltr', inputMode: 'numeric' } as const;

/** Two configuration values are equal when the merge patch from one to the other is empty. */
function sameValue(a: unknown, b: unknown): boolean {
  const p = createMergePatch(a ?? {}, b ?? {});
  return typeof p === 'object' && p !== null && Object.keys(p).length === 0;
}

/** The interface without its `subinterfaces` member (edited in their own table). */
function withoutSubs(c: InterfaceConfig | undefined): Partial<InterfaceConfig> | undefined {
  if (!c) return undefined;
  return Object.fromEntries(Object.entries(c).filter(([k]) => k !== 'subinterfaces')) as Partial<InterfaceConfig>;
}

/** Opens from the end of the reading direction: MUI flips `anchor="right"` to the left in RTL. */
export function InterfaceDrawer({ name, onClose, rates }: { name: string | null; onClose: () => void; rates: Map<string, Rate> }) {
  return (
    <Drawer anchor="right" open={name !== null} onClose={onClose} sx={{ zIndex: (th) => th.zIndex.modal }} slotProps={{ paper: { sx: { inlineSize: { xs: '100%', md: 640 } } } }}>
      {name !== null && <DrawerBody key={name} name={name} onClose={onClose} rates={rates} />}
    </Drawer>
  );
}

function DrawerBody({ name, onClose, rates }: { name: string; onClose: () => void; rates: Map<string, Rate> }) {
  const { t } = useTranslation('interfaces');
  const fmt = useFormatters();
  const perms = usePermissions();
  const state = useInterfacesState();
  const candidate = useCandidateInterfaces();
  const fresh = useFreshCandidate();
  const patch = usePatchInterfaces();
  const [saved, setSaved] = useState(false);
  const [subDialog, setSubDialog] = useState<{ id: string; value: SubinterfaceConfig | undefined } | null>(null);
  const [subError, setSubError] = useState<unknown>(null);

  const item: InterfaceItem | undefined = state.data?.items.find((i) => i.name === name);
  const parentName = item?.kind === 'subinterface' ? (item.parent ?? '') : name;
  const isSub = item?.kind === 'subinterface';
  const config: InterfaceConfig | undefined = candidate.data?.[parentName];
  const formSchema = useMemo(() => localizeSchema(interfaceFormSchema(), (k, o) => t(k, o ?? {})), [t]);
  const subSchema = useMemo(() => localizeSchema(subinterfaceSchema(), (k, o) => t(k, o ?? {})), [t]);
  const subs = Object.entries(config?.subinterfaces ?? {}).sort(([a], [b]) => Number(a) - Number(b));
  const live = item?.state;
  const rate = rates.get(live?.vppName ?? name);
  const readOnly = !perms.editConfig;

  // The form edits the value it opened with (review N4): a refetch of the candidate never remounts it (unsaved edits
  // stay), and Save sends only what changed against that value — a field another session changed meanwhile is not
  // written back with a stale value. "Reload" opens the form again on the current candidate.
  const [reload, setReload] = useState(0);
  const [opened, setOpened] = useState<{ reload: number; value: Partial<InterfaceConfig> | undefined } | null>(null);
  const [changedElsewhere, setChangedElsewhere] = useState(false);
  if (candidate.isSuccess && (opened === null || opened.reload !== reload)) {
    setOpened({ reload, value: withoutSubs(config) });
  }
  const formValue = opened?.value;

  const saveInterface = async (value: unknown) => {
    setSaved(false);
    const base = opened?.value;
    const current = withoutSubs((await fresh())[name]);
    setChangedElsewhere(!sameValue(base, current));
    // No phantom clean-up: SchemaForm keeps an optional object (dhcpClient) absent until its presence switch is on
    // (WEB-1), so an object in `value` is one the user enabled, possibly holding nothing but its defaults.
    const body = base === undefined ? value : createMergePatch(base, value);
    try {
      await patch.mutateAsync({ [name]: body });
      setOpened({ reload, value: value as Partial<InterfaceConfig> }); // the next save diffs against what is saved now
      setSaved(true);
    } catch {
      // rendered from patch.error (pointers mapped onto the fields)
    }
  };

  const removeInterface = async () => {
    try {
      await patch.mutateAsync({ [name]: null });
      onClose();
    } catch {
      // shown below
    }
  };

  const saveSub = async (id: string, value: unknown) => {
    setSubError(null);
    const current = (await fresh())[parentName]?.subinterfaces?.[id];
    const body = current === undefined ? value : createMergePatch(current, value);
    try {
      await patch.mutateAsync({ [parentName]: { subinterfaces: { [id]: body } } });
      setSubDialog(null);
    } catch (e) {
      setSubError(e);
    }
  };

  const removeSub = async (id: string) => {
    setSubError(null);
    try {
      await patch.mutateAsync({ [parentName]: { subinterfaces: { [id]: null } } });
    } catch (e) {
      setSubError(e);
    }
  };

  const liveRows: [string, string][] = live
    ? [
        [t('live.type'), live.type],
        [t('live.swIfIndex'), String(live.swIfIndex)],
        [t('live.mac'), live.mac],
        [t('live.mtu'), `${fmt.integer(live.mtu)} (${t('live.linkMtu')} ${fmt.integer(live.linkMtu)})`],
        [t('live.addresses'), [...live.ipv4, ...live.ipv6].join(' ') || '—'],
        [t('live.vrf'), `${live.vrf} (${live.tableId})`],
        [t('live.rxMode'), live.rxMode || '—'],
      ]
    : [];

  return (
    <Box sx={{ p: 2 }} role="region" aria-label={t('drawer.label', { name })}>
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, flex: 1, textAlign: 'start' }}>
          {name}
        </Typography>
        {live && <StatusChip size="small" status={adminStatus(live)!} label={`${t('col.admin')}: ${t(`status.${adminStatus(live)!}`)}`} />}
        {live && <StatusChip size="small" status={linkStatus(live)!} label={`${t('col.link')}: ${t(`status.${linkStatus(live)!}`)}`} />}
        <IconButton aria-label={t('close')} onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Stack>
      {(state.isPending || candidate.isPending) && <LinearProgress aria-label={t('loading')} />}
      {!live && state.isSuccess && <Alert severity="info">{t('drawer.notInVpp')}</Alert>}
      {live && (
        <Table size="small" aria-label={t('drawer.liveTitle')} sx={{ mb: 2 }}>
          <TableBody>
            {liveRows.map(([k, v]) => (
              <TableRow key={k}>
                <TableCell component="th" sx={{ inlineSize: 180 }}>
                  {k}
                </TableCell>
                <TableCell dir="ltr" sx={{ textAlign: 'start' }}>
                  {v}
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell component="th">{t('live.rates')}</TableCell>
              <TableCell>
                {rate ? (
                  <Stack direction="row" gap={1} alignItems="center">
                    <span>{t('col.rx')}</span>
                    <bdi dir="ltr">{fmt.rate(rate.rxBps, BPS)}</bdi>
                    <span>{t('col.tx')}</span>
                    <bdi dir="ltr">{fmt.rate(rate.txBps, BPS)}</bdi>
                    <Sparkline values={rate.history} label={t('trendLabel', { name })} />
                  </Stack>
                ) : (
                  t('live.noRates')
                )}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      )}
      <Divider sx={{ mb: 2 }} />

      {isSub ? (
        <Alert severity="info">{t('drawer.subHint', { parent: parentName })}</Alert>
      ) : (
        <>
          <Typography component="h4" variant="subtitle1" gutterBottom>
            {t('drawer.configTitle')}
          </Typography>
          {!config && candidate.isSuccess && <Alert severity="info" sx={{ mb: 1 }}>{t('drawer.notConfigured')}</Alert>}
          {patch.isError && !subDialog && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
          {saved && !patch.isError && (
            <Alert severity="success" sx={{ mb: 1 }}>
              {t('drawer.saved')}
            </Alert>
          )}
          {changedElsewhere && (
            <Alert
              severity="warning"
              sx={{ mb: 1 }}
              action={
                <Button color="inherit" size="small" onClick={() => { setChangedElsewhere(false); setSaved(false); setReload((r) => r + 1); }}>
                  {t('drawer.reload')}
                </Button>
              }
            >
              {t('drawer.changedElsewhere')}
            </Alert>
          )}
          {candidate.isSuccess && opened !== null && (
            <SchemaForm
              key={`${name}:${reload}`}
              schema={formSchema}
              value={formValue}
              readOnly={readOnly}
              submitLabel={t('drawer.save')}
              resetLabel={t('drawer.reset')}
              problem={problemFor(patch.error, `/interfaces/${esc(name)}`)}
              onSubmit={saveInterface}
            >
              {config && (
                <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={readOnly || patch.isPending} onClick={() => void removeInterface()}>
                  {t('drawer.remove')}
                </Button>
              )}
            </SchemaForm>
          )}

          <Divider sx={{ my: 2 }} />
          <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
            <Typography component="h4" variant="subtitle1" sx={{ flex: 1 }}>
              {t('sub.title')}
            </Typography>
            <Button size="small" startIcon={<AddIcon />} disabled={readOnly || !config} onClick={() => setSubDialog({ id: '', value: undefined })}>
              {t('sub.add')}
            </Button>
          </Stack>
          {subError !== null && !subDialog && <ProblemAlert error={subError} sx={{ mb: 1 }} />}
          <Table size="small" aria-label={t('sub.title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('sub.id')}</TableCell>
                <TableCell>{t('sub.vlan')}</TableCell>
                <TableCell>{t('col.admin')}</TableCell>
                <TableCell>{t('col.addresses')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('sub.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {subs.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5}>
                    <Typography color="text.secondary">{t('sub.none')}</Typography>
                  </TableCell>
                </TableRow>
              )}
              {subs.map(([id, sub]) => {
                const subLive = state.data?.items.find((i) => i.name === `${name}.${id}`)?.state;
                return (
                  <TableRow key={id} hover sx={{ cursor: readOnly ? 'default' : 'pointer' }} onClick={() => !readOnly && setSubDialog({ id, value: sub })}>
                    <TableCell dir="ltr">{`${name}.${id}`}</TableCell>
                    <TableCell>{sub.vlanId}</TableCell>
                    <TableCell>{subLive ? <StatusChip size="small" status={adminStatus(subLive)!} /> : t('notInVpp')}</TableCell>
                    <TableCell dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: 12 }}>
                      {[...(sub.ipv4 ?? []), ...(sub.ipv6 ?? [])].join(' ')}
                    </TableCell>
                    <TableCell sx={{ textAlign: 'end' }}>
                      <IconButton
                        size="small"
                        aria-label={t('sub.remove', { id })}
                        disabled={readOnly || patch.isPending}
                        onClick={(e) => {
                          e.stopPropagation();
                          void removeSub(id);
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
        </>
      )}

      <Dialog open={subDialog !== null} onClose={() => setSubDialog(null)} fullWidth maxWidth="sm">
        <DialogTitle>{subDialog?.value ? t('sub.editTitle', { name: `${name}.${subDialog.id}` }) : t('sub.addTitle', { name })}</DialogTitle>
        <DialogContent>
          {subDialog && (
            <SubForm
              key={subDialog.id || 'new'}
              initialId={subDialog.id}
              editing={subDialog.value !== undefined}
              existingIds={subs.map(([id]) => id)}
              value={subDialog.value}
              schema={subSchema}
              pointerPrefix={(id) => `/interfaces/${esc(parentName)}/subinterfaces/${esc(id)}`}
              error={subError}
              pending={patch.isPending}
              onCancel={() => setSubDialog(null)}
              onSubmit={(id, v) => void saveSub(id, v)}
            />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}

function SubForm({
  initialId,
  editing,
  existingIds,
  value,
  schema,
  pointerPrefix,
  error,
  pending,
  onCancel,
  onSubmit,
}: {
  initialId: string;
  editing: boolean;
  existingIds: string[];
  value: SubinterfaceConfig | undefined;
  schema: ReturnType<typeof subinterfaceSchema>;
  pointerPrefix: (id: string) => string;
  error: unknown;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (id: string, value: unknown) => void;
}) {
  const { t } = useTranslation('interfaces');
  const [id, setId] = useState(initialId);
  // review N5: server pointers are mapped with the id typed here (not the empty id of "add"), and "add" never
  // silently merges over an existing sub-interface
  const taken = !editing && existingIds.includes(id);
  const idOk = SUB_ID_RE.test(id) && !taken;
  const problem = problemFor(error, pointerPrefix(id));
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('sub.id')}
        value={id}
        disabled={editing}
        onChange={(e) => setId(e.target.value)}
        error={id !== '' && !idOk}
        helperText={taken ? t('sub.idExists', { id }) : t('sub.idHelp')}
        slotProps={{ htmlInput: SUB_ID_INPUT }}
      />
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={value}
        problem={problem}
        submitLabel={t('drawer.save')}
        resetLabel={t('drawer.reset')}
        onSubmit={(v) => {
          if (idOk && !pending) onSubmit(id, v);
        }}
      >
        <Button onClick={onCancel}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
