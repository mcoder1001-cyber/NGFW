import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import DownloadIcon from '@mui/icons-material/Download';
import EditIcon from '@mui/icons-material/Edit';
import KeyIcon from '@mui/icons-material/Key';
import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { createMergePatch, localizeSchema } from '../../interfaces/model';
import {
  browserKeypair,
  clientConfig,
  foldPeerEvents,
  ifaceChip,
  interfaceFormSchema,
  peerChip,
  peerFormSchema,
  peerStatus,
  type LivePeer,
  type PeerEventData,
  type WgInterfaceState,
  type WireguardInterface,
  type WireguardPeer,
} from './model';
import {
  useCandidateWireguard,
  useFreshWireguard,
  useKeypair,
  usePatchWireguard,
  useWireguardState,
} from './queries';

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
const LTR = { dir: 'ltr' } as const;

/** Server pointers `/vpn/wireguard/interfaces/<name>[/peers/<peer>]/…` → pointers relative to the edited form. */
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
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');
const short = (k: string) => (k.length > 12 ? `${k.slice(0, 10)}…` : k);

function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

/** The WireGuard tab of the VPN page (F-wireguard): interfaces with their peers, live handshake status, key helpers. */
export function WireguardPage() {
  const { t } = useTranslation('wireguard');
  const fmt = useFormatters();
  const perms = usePermissions();
  const state = useWireguardState();
  const candidate = useCandidateWireguard();
  const patch = usePatchWireguard();
  const [live, setLive] = useState<ReadonlyMap<string, LivePeer>>(() => new Map());
  useTopic<PeerEventData>('wireguard.events', {
    onBatch: (batch) =>
      setLive((prev) =>
        foldPeerEvents(
          prev,
          batch.map((m) => m.data),
        ),
      ),
  });
  // client private keys generated in this browser session only (never stored, never sent)
  const [clientKeys, setClientKeys] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [editIface, setEditIface] = useState<{ name: string; isNew: boolean } | null>(null);
  const [editPeer, setEditPeer] = useState<{ iface: string; name: string; isNew: boolean } | null>(
    null,
  );
  const [keypairOpen, setKeypairOpen] = useState(false);

  const ifaces = candidate.data ?? {};
  const byVpp = new Map<string, WgInterfaceState>(
    (state.data?.interfaces ?? []).map((i) => [i.vppName, i]),
  );
  const names = Object.keys(ifaces).sort();
  const unconfigured = (state.data?.interfaces ?? []).filter((i) => i.name === null);

  const remove = async (body: Record<string, unknown>) => {
    try {
      await patch.mutateAsync(body);
    } catch {
      // shown below from patch.error
    }
  };

  return (
    <Box>
      <Stack direction="row" spacing={1} sx={{ mb: 2, alignItems: 'center', flexWrap: 'wrap' }}>
        <Typography variant="h6" sx={{ flexGrow: 1 }}>
          {t('title')}
        </Typography>
        <Chip
          size="small"
          variant="outlined"
          label={state.data?.eventsActive ? t('live.on') : t('live.off')}
          color={state.data?.eventsActive ? 'success' : 'default'}
        />
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
        {perms.manageUsers && (
          <Button size="small" startIcon={<KeyIcon />} onClick={() => setKeypairOpen(true)}>
            {t('keypair.button')}
          </Button>
        )}
        {perms.editConfig && (
          <Button
            size="small"
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setEditIface({ name: '', isNew: true })}
          >
            {t('add')}
          </Button>
        )}
      </Stack>
      {state.error !== null && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
      {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
      {candidate.isSuccess && names.length === 0 && <Alert severity="info">{t('empty')}</Alert>}
      {unconfigured.length > 0 && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('unconfigured', { names: unconfigured.map((i) => i.vppName).join(', ') })}
        </Alert>
      )}
      {names.map((name) => {
        const cfg = ifaces[name]!;
        const vpp = `wg${cfg.instance}`;
        const st = byVpp.get(vpp);
        const peers = Object.entries(cfg.peers ?? {}).sort(([a], [b]) => a.localeCompare(b));
        return (
          <Paper key={name} variant="outlined" sx={{ p: 2, mb: 2 }} data-testid={`wg-${name}`}>
            <Stack
              direction="row"
              spacing={1}
              sx={{ alignItems: 'center', flexWrap: 'wrap', mb: 1 }}
            >
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                {name}
              </Typography>
              <Typography variant="body2" color="text.secondary" {...LTR}>
                {vpp}
              </Typography>
              <StatusChip
                status={ifaceChip(st)}
                label={
                  st ? (st.adminUp ? t('state.up') : t('state.adminDown')) : t('state.missing')
                }
              />
              <Box sx={{ flexGrow: 1 }} />
              {perms.editConfig && (
                <>
                  <Button
                    size="small"
                    startIcon={<AddIcon />}
                    onClick={() => setEditPeer({ iface: name, name: '', isNew: true })}
                  >
                    {t('peer.add')}
                  </Button>
                  <IconButton
                    size="small"
                    aria-label={t('edit')}
                    onClick={() => setEditIface({ name, isNew: false })}
                  >
                    <EditIcon fontSize="small" />
                  </IconButton>
                  <IconButton
                    size="small"
                    aria-label={t('delete')}
                    onClick={() => void remove({ [name]: null })}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </>
              )}
            </Stack>
            <Stack direction={{ xs: 'column', md: 'row' }} spacing={3} sx={{ mb: 1 }}>
              <Typography variant="body2">
                {t('col.listen')}: <span dir="ltr">{`${cfg.listenAddress}:${cfg.listenPort}`}</span>
              </Typography>
              <Typography variant="body2">
                {t('col.address')}: <span dir="ltr">{(cfg.address ?? []).join(' ') || '—'}</span>
              </Typography>
              <Typography variant="body2">
                {t('col.publicKey')}: <span dir="ltr">{st?.publicKey ?? '—'}</span>
              </Typography>
              {st && (
                <Typography variant="body2">
                  {t('col.traffic')}:{' '}
                  {t('bytesRxTx', { rx: fmt.integer(st.rxBytes), tx: fmt.integer(st.txBytes) })}
                </Typography>
              )}
            </Stack>
            <Table size="small" aria-label={t('peers', { name })}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('col.peer')}</TableCell>
                  <TableCell>{t('col.status')}</TableCell>
                  <TableCell>{t('col.publicKey')}</TableCell>
                  <TableCell>{t('col.endpoint')}</TableCell>
                  <TableCell>{t('col.allowedIps')}</TableCell>
                  <TableCell>{t('col.lastHandshake')}</TableCell>
                  <TableCell />
                </TableRow>
              </TableHead>
              <TableBody>
                {peers.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={7}>{t('peer.none')}</TableCell>
                  </TableRow>
                )}
                {peers.map(([pname, p]) => {
                  const ps = st?.peers.find((x) => x.publicKey === p.publicKey);
                  const status =
                    st && ps
                      ? peerStatus(st, ps, live, state.data?.retrievedAt ?? null)
                      : undefined;
                  const endpoint = ps?.endpoint
                    ? `${ps.endpoint}:${ps.endpointPort}`
                    : p.endpoint
                      ? `${p.endpoint.address}:${p.endpoint.port}`
                      : '—';
                  return (
                    <TableRow key={pname} data-testid={`wg-peer-${name}-${pname}`}>
                      <TableCell>{pname}</TableCell>
                      <TableCell>
                        <StatusChip
                          status={peerChip(status)}
                          label={t(`peer.status.${status ?? 'unknown'}`)}
                        />
                      </TableCell>
                      <TableCell>
                        <Tooltip title={p.publicKey}>
                          <span dir="ltr">{short(p.publicKey)}</span>
                        </Tooltip>
                      </TableCell>
                      <TableCell dir="ltr">{endpoint}</TableCell>
                      <TableCell dir="ltr">{p.allowedIps.join(' ')}</TableCell>
                      <TableCell>
                        {ps?.lastHandshake ? fmt.dateTime(ps.lastHandshake) : '—'}
                      </TableCell>
                      <TableCell sx={{ whiteSpace: 'nowrap' }}>
                        <Tooltip title={t('export.hint')}>
                          <IconButton
                            size="small"
                            aria-label={t('export.button')}
                            onClick={() =>
                              download(
                                `${name}-${pname}.conf`,
                                clientConfig({
                                  clientPrivateKey: clientKeys.get(`${name}|${pname}`),
                                  peer: p,
                                  iface: cfg,
                                  serverPublicKey: st?.publicKey,
                                }),
                              )
                            }
                          >
                            <DownloadIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        {perms.editConfig && (
                          <>
                            <IconButton
                              size="small"
                              aria-label={t('edit')}
                              onClick={() =>
                                setEditPeer({ iface: name, name: pname, isNew: false })
                              }
                            >
                              <EditIcon fontSize="small" />
                            </IconButton>
                            <IconButton
                              size="small"
                              aria-label={t('delete')}
                              onClick={() => void remove({ [name]: { peers: { [pname]: null } } })}
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
          </Paper>
        );
      })}
      {editIface && (
        <InterfaceDialog
          key={editIface.name || 'new'}
          target={editIface}
          existing={ifaces}
          onClose={() => setEditIface(null)}
        />
      )}
      {editPeer && (
        <PeerDialog
          key={`${editPeer.iface}/${editPeer.name}`}
          target={editPeer}
          iface={ifaces[editPeer.iface]}
          onClose={() => setEditPeer(null)}
          onClientKey={(peer, priv) =>
            setClientKeys((m) => new Map(m).set(`${editPeer.iface}|${peer}`, priv))
          }
        />
      )}
      {keypairOpen && <KeypairDialog onClose={() => setKeypairOpen(false)} />}
    </Box>
  );
}

/** Create or edit `vpn.wireguard.interfaces.<name>` (everything but peers) with the one schema's form. */
function InterfaceDialog({
  target,
  existing,
  onClose,
}: {
  target: { name: string; isNew: boolean };
  existing: Record<string, WireguardInterface>;
  onClose: () => void;
}) {
  const { t } = useTranslation('wireguard');
  const perms = usePermissions();
  const patch = usePatchWireguard();
  const fresh = useFreshWireguard();
  const keypair = useKeypair();
  const schema = useMemo(() => localizeSchema(interfaceFormSchema(), (k, o) => t(k, o ?? {})), [t]);
  const [name, setName] = useState(target.name);
  const base = useMemo(() => {
    const c = existing[target.name];
    if (!c) return undefined;
    const rest: Record<string, unknown> = { ...c };
    delete rest['peers'];
    return rest;
  }, [existing, target.name]);
  const [value, setValue] = useState<Record<string, unknown> | undefined>(base);
  const [pub, setPub] = useState<string | null>(null);
  const nameOk = NAME_RE.test(name) && (!target.isNew || !(name in existing));

  const save = async (v: unknown) => {
    const current = (await fresh())[name];
    const cur: Record<string, unknown> | undefined = current ? { ...current } : undefined;
    if (cur) delete cur['peers'];
    const body = cur === undefined ? v : createMergePatch(cur, v);
    try {
      await patch.mutateAsync({ [name]: body });
      onClose();
    } catch {
      // rendered from patch.error
    }
  };
  const generate = async () => {
    try {
      const r = await keypair.mutateAsync(`wg-${name}`);
      setPub(r.publicKey);
      setValue((v) => ({ ...(v ?? {}), privateKeyRef: r.ref }));
    } catch {
      // shown below
    }
  };
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>{target.isNew ? t('add') : t('editTitle', { name })}</DialogTitle>
      <DialogContent>
        {target.isNew && (
          <TextField
            label={t('name')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={name !== '' && !nameOk}
            helperText={t('nameHint')}
            fullWidth
            sx={{ my: 1 }}
            slotProps={{ htmlInput: LTR }}
          />
        )}
        {perms.manageUsers && (
          <Stack direction="row" spacing={1} sx={{ my: 1, alignItems: 'center' }}>
            <Button
              size="small"
              startIcon={<KeyIcon />}
              disabled={!nameOk || keypair.isPending}
              onClick={() => void generate()}
            >
              {t('keypair.forInterface')}
            </Button>
            {pub && (
              <Typography variant="body2" dir="ltr" data-testid="wg-generated-public-key">
                {t('keypair.public')}: {pub}
              </Typography>
            )}
          </Stack>
        )}
        {keypair.error !== null && <ProblemAlert error={keypair.error} sx={{ mb: 1 }} />}
        <SchemaForm
          schema={schema}
          value={value}
          readOnly={!perms.editConfig || !nameOk}
          problem={problemFor(patch.error, `/vpn/wireguard/interfaces/${esc(name)}`)}
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

/** Create or edit a peer; "generate client keys" makes a key pair in the browser (the private key stays in this session). */
function PeerDialog({
  target,
  iface,
  onClose,
  onClientKey,
}: {
  target: { iface: string; name: string; isNew: boolean };
  iface: WireguardInterface | undefined;
  onClose: () => void;
  onClientKey: (peer: string, privateKey: string) => void;
}) {
  const { t } = useTranslation('wireguard');
  const perms = usePermissions();
  const patch = usePatchWireguard();
  const schema = useMemo(
    () =>
      localizeSchema(peerFormSchema(), (k, o) =>
        t(`peerField.${k.replace(/^field\./, '')}`, o ?? {}),
      ),
    [t],
  );
  const [name, setName] = useState(target.name);
  const [value, setValue] = useState<WireguardPeer | Record<string, unknown> | undefined>(
    iface?.peers?.[target.name],
  );
  const [keyError, setKeyError] = useState<string | null>(null);
  const [pendingKey, setPendingKey] = useState<string | null>(null);
  const nameOk = NAME_RE.test(name) && (!target.isNew || !(name in (iface?.peers ?? {})));

  const save = async (v: unknown) => {
    const before = iface?.peers?.[name];
    const body = before === undefined ? v : createMergePatch(before, v);
    try {
      await patch.mutateAsync({ [target.iface]: { peers: { [name]: body } } });
      if (pendingKey) onClientKey(name, pendingKey);
      onClose();
    } catch {
      // rendered from patch.error
    }
  };
  const generate = async () => {
    try {
      const kp = await browserKeypair();
      setPendingKey(kp.privateKey);
      setValue((v) => ({ ...(v ?? {}), publicKey: kp.publicKey }));
      setKeyError(null);
    } catch (e) {
      setKeyError(e instanceof Error ? e.message : String(e));
    }
  };
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>
        {target.isNew ? t('peer.add') : t('peer.editTitle', { name, iface: target.iface })}
      </DialogTitle>
      <DialogContent>
        {target.isNew && (
          <TextField
            label={t('peer.name')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={name !== '' && !nameOk}
            helperText={t('nameHint')}
            fullWidth
            sx={{ my: 1 }}
            slotProps={{ htmlInput: LTR }}
          />
        )}
        {perms.editConfig && (
          <Stack direction="row" spacing={1} sx={{ my: 1, alignItems: 'center' }}>
            <Button size="small" startIcon={<KeyIcon />} onClick={() => void generate()}>
              {t('peer.generateClient')}
            </Button>
            {pendingKey && <Alert severity="info">{t('peer.clientKeyHint')}</Alert>}
          </Stack>
        )}
        {keyError && (
          <Alert severity="error">{t('peer.clientKeyError', { error: keyError })}</Alert>
        )}
        <SchemaForm
          schema={schema}
          value={value}
          readOnly={!perms.editConfig || !nameOk}
          problem={problemFor(
            patch.error,
            `/vpn/wireguard/interfaces/${esc(target.iface)}/peers/${esc(name)}`,
          )}
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

/** Admin: generate a key pair on the server; the private key is stored as a secret and never shown. */
function KeypairDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation('wireguard');
  const keypair = useKeypair();
  const [name, setName] = useState('');
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t('keypair.title')}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 1 }}>
          {t('keypair.explain')}
        </Typography>
        <TextField
          label={t('keypair.name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          fullWidth
          sx={{ my: 1 }}
          slotProps={{ htmlInput: LTR }}
        />
        {keypair.error !== null && <ProblemAlert error={keypair.error} sx={{ mb: 1 }} />}
        {keypair.data && (
          <Alert severity="success" data-testid="wg-keypair-result">
            <div>
              {t('keypair.ref')}: <span dir="ltr">{keypair.data.ref}</span>
            </div>
            <div>
              {t('keypair.public')}: <span dir="ltr">{keypair.data.publicKey}</span>
            </div>
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('close')}</Button>
        <Button
          variant="contained"
          disabled={!NAME_RE.test(name) || keypair.isPending}
          onClick={() => keypair.mutate(name)}
        >
          {t('keypair.generate')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
