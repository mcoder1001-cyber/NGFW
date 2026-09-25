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
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { DaemonStatus, PendingActions, StateProblem } from './common';
import { fieldKey, NS, problemUnder, resolverSchema, vppCacheSchema } from './model';
import {
  useCandidate,
  useDnsLookup,
  useDnsState,
  useFreshCandidate,
  usePatchCandidate,
  type DnsResolver,
  type ServicesDnsConfig,
} from './queries';

const DNS_PATH = 'services/dns';
const DAEMON = 'unbound';
const CACHE_POINTER = '/services/dns/vppCache';
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;

/**
 * Services › DNS (F-unbound-chrony-syslog): the Unbound resolvers (list + schema-driven form, `services.dns.resolvers`),
 * their live state (running, queries, forward zones, local zones, pending start/restart requests), the VPP DNS cache
 * (`services.dns.vppCache`, write-only in VPP: shown as configured) and a lookup through it. Edits go to the candidate;
 * the pending-change bar commits them.
 */
export default function DnsTab() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const cand = useCandidate<ServicesDnsConfig>(DNS_PATH);
  const fresh = useFreshCandidate<ServicesDnsConfig>(DNS_PATH);
  const state = useDnsState();
  const patch = usePatchCandidate(DNS_PATH);
  const [editing, setEditing] = useState<{ name: string; value: DnsResolver | null } | null>(null);
  const [newName, setNewName] = useState('');
  const [cacheOpen, setCacheOpen] = useState(false);
  const schema = useMemo(() => resolverSchema(), []);
  const cacheSchema = useMemo(() => vppCacheSchema(), []);
  const translate = (path: string, fallback: string) =>
    t(fieldKey('resolver', path), { defaultValue: fallback });

  const resolvers = cand.data?.resolvers ?? {};
  const names = Object.keys(resolvers).sort();
  const st = state.data;
  const zonesUp = new Set((st?.localZones ?? []).map((z) => z.zone));

  const submit = async (value: unknown) => {
    if (!editing) return;
    const name = editing.value ? editing.name : newName.trim();
    const current = (await fresh()).resolvers ?? {};
    if (!editing.value && name in current) return;
    try {
      await patch.mutateAsync({ resolvers: { [name]: value } });
      setEditing(null);
    } catch {
      // shown from patch.error on the form
    }
  };

  const remove = (name: string) =>
    void patch.mutateAsync({ resolvers: { [name]: null } }).catch(() => undefined);

  return (
    <Box>
      <DaemonStatus
        daemon={DAEMON}
        running={st?.running}
        query={state}
        configPath={st?.configPath}
      />
      <StateProblem error={state.error} partial={st?.error} />
      <PendingActions actions={st?.pendingActions} />
      {st?.running && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {t('dns.stats', {
            queries: st.stats['total.num.queries'] ?? '0',
            hits: st.stats['total.num.cachehits'] ?? '0',
            misses: st.stats['total.num.cachemiss'] ?? '0',
          })}
        </Typography>
      )}

      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="h6">{t('dns.resolvers')}</Typography>
        <Button
          startIcon={<AddIcon />}
          disabled={!perms.editConfig}
          onClick={() => {
            patch.reset();
            setNewName('');
            setEditing({ name: '', value: null });
          }}
        >
          {t('add')}
        </Button>
      </Stack>
      {patch.error && !editing && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
      <TableContainer component={Paper} variant="outlined" sx={{ mb: 3 }}>
        <Table size="small" aria-label={t('dns.resolvers')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.name')}</TableCell>
              <TableCell>{t('col.listen')}</TableCell>
              <TableCell>{t('col.forwarders')}</TableCell>
              <TableCell>{t('col.localZones')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {names.length === 0 && (
              <TableRow>
                <TableCell colSpan={6}>{t('dns.none')}</TableCell>
              </TableRow>
            )}
            {names.map((name) => {
              const r = resolvers[name]!;
              const zones = r.localZones.map((z) =>
                (z.zone.endsWith('.') ? z.zone : `${z.zone}.`).toLowerCase(),
              );
              const served = zones.filter((z) => zonesUp.has(z)).length;
              return (
                <TableRow key={name}>
                  <TableCell>{name}</TableCell>
                  <TableCell sx={{ fontFamily: 'monospace' }}>
                    {r.listen.map((l) => `${l.address}:${l.port}`).join(', ')}
                  </TableCell>
                  <TableCell sx={{ fontFamily: 'monospace' }}>
                    {[
                      ...r.forwarders.map((f) => f.address),
                      ...r.forwardZones.map(
                        (z) => `${z.zone} → ${z.forwarders.map((f) => f.address).join(' ')}`,
                      ),
                    ].join(', ') || t('dns.recursion')}
                  </TableCell>
                  <TableCell>{t('dns.zonesServed', { served, total: zones.length })}</TableCell>
                  <TableCell>
                    {r.enabled === false ? (
                      <Chip size="small" label={t('status.disabled')} />
                    ) : (
                      <Chip
                        size="small"
                        color={st?.running ? 'success' : 'default'}
                        label={t(st?.running ? 'status.serving' : 'status.notServing')}
                      />
                    )}
                  </TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>
                    <IconButton
                      aria-label={t('edit', { name })}
                      disabled={!perms.editConfig}
                      onClick={() => {
                        patch.reset();
                        setEditing({ name, value: r });
                      }}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      aria-label={t('delete', { name })}
                      disabled={!perms.editConfig}
                      onClick={() => remove(name)}
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

      {st && (st.forwards.length > 0 || st.localZones.length > 0) && (
        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" gutterBottom>
            {t('dns.live')}
          </Typography>
          {st.forwards.map((f) => (
            <Chip
              key={`f-${f.zone}`}
              sx={{ mb: 1, marginInlineEnd: 1 }}
              label={`${f.zone} → ${f.addresses.join(' ')}`}
              variant="outlined"
            />
          ))}
          {st.localZones
            .filter(
              (z) =>
                names.length > 0 &&
                z.type !== 'transparent' &&
                !z.zone.endsWith('in-addr.arpa.') &&
                !z.zone.endsWith('ip6.arpa.'),
            )
            .slice(0, 12)
            .map((z) => (
              <Chip
                key={`z-${z.zone}`}
                sx={{ mb: 1, marginInlineEnd: 1 }}
                label={`${z.zone} (${z.type})`}
              />
            ))}
          {st.localData.length > 0 && (
            <Typography
              variant="body2"
              component="pre"
              sx={{ fontFamily: 'monospace', mt: 1, whiteSpace: 'pre-wrap' }}
            >
              {st.localData.slice(0, 20).join('\n')}
            </Typography>
          )}
        </Box>
      )}

      <VppCachePanel
        configured={cand.data?.vppCache}
        live={st?.vppCache ?? null}
        canEdit={perms.editConfig}
        onEdit={() => {
          patch.reset();
          setCacheOpen(true);
        }}
      />
      <LookupPanel canRun={perms.editConfig} />

      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="md">
        <DialogTitle>
          {editing?.value ? t('dns.editTitle', { name: editing.name }) : t('dns.addTitle')}
        </DialogTitle>
        <DialogContent>
          {editing && !editing.value && (
            <TextField
              label={t('col.name')}
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              error={newName !== '' && (!NAME_RE.test(newName) || newName in resolvers)}
              helperText={newName in resolvers ? t('dns.nameTaken') : t('dns.nameHelp')}
              sx={{ mt: 1, mb: 2 }}
              fullWidth
            />
          )}
          {editing && (
            <SchemaForm
              id="dns-resolver-form"
              schema={schema}
              value={editing.value ?? undefined}
              onSubmit={submit}
              problem={problemUnder(
                patch.error,
                `/services/dns/resolvers/${editing.value ? editing.name : newName.trim()}`,
              )}
              translateLabel={translate}
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)}>{t('config:cancel')}</Button>
          <Button
            type="submit"
            form="dns-resolver-form"
            variant="contained"
            disabled={
              patch.isPending ||
              (editing !== null &&
                !editing.value &&
                (!NAME_RE.test(newName.trim()) || newName.trim() in resolvers))
            }
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={cacheOpen} onClose={() => setCacheOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>{t('dns.vppCache')}</DialogTitle>
        <DialogContent>
          <SchemaForm
            id="dns-vpp-cache-form"
            schema={cacheSchema}
            value={cand.data?.vppCache ?? undefined}
            onSubmit={async (value) => {
              try {
                await patch.mutateAsync({ vppCache: value });
                setCacheOpen(false);
              } catch {
                // shown on the form
              }
            }}
            problem={problemUnder(patch.error, CACHE_POINTER)}
            translateLabel={(p, f) => t(fieldKey('vppCache', p), { defaultValue: f })}
            hideActions
          />
        </DialogContent>
        <DialogActions>
          {cand.data?.vppCache && (
            <Button
              color="error"
              onClick={() =>
                void patch.mutateAsync({ vppCache: null }).then(
                  () => setCacheOpen(false),
                  () => undefined,
                )
              }
            >
              {t('dns.removeCache')}
            </Button>
          )}
          <Button onClick={() => setCacheOpen(false)}>{t('config:cancel')}</Button>
          <Button
            type="submit"
            form="dns-vpp-cache-form"
            variant="contained"
            disabled={patch.isPending}
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

function VppCachePanel({
  configured,
  live,
  canEdit,
  onEdit,
}: {
  configured: ServicesDnsConfig['vppCache'];
  live: { configured: boolean; appliedByThisAgent: boolean; upstreams: string[] } | null;
  canEdit: boolean;
  onEdit: () => void;
}) {
  const { t } = useTranslation(NS);
  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography variant="h6">{t('dns.vppCache')}</Typography>
        <Button startIcon={<EditIcon />} disabled={!canEdit} onClick={onEdit}>
          {t('configure')}
        </Button>
      </Stack>
      <Typography variant="body2" sx={{ mt: 1 }}>
        {configured?.enabled
          ? t('dns.cacheOn', { upstreams: configured.upstreams.join(', ') })
          : t('dns.cacheOff')}
      </Typography>
      <Alert severity="info" sx={{ mt: 1 }}>
        {t('dns.cacheWriteOnly')}{' '}
        {live?.configured
          ? t(live.appliedByThisAgent ? 'dns.cacheOwner' : 'dns.cacheRequired')
          : ''}
      </Alert>
    </Paper>
  );
}

function LookupPanel({ canRun }: { canRun: boolean }) {
  const { t } = useTranslation(NS);
  const lookup = useDnsLookup();
  const [name, setName] = useState('');
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Typography variant="h6" gutterBottom>
        {t('lookup.title')}
      </Typography>
      <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
        <TextField
          size="small"
          label={t('lookup.name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          sx={{ minWidth: 280 }}
        />
        <Button
          variant="contained"
          disabled={!canRun || name.trim() === '' || lookup.isPending}
          onClick={() => lookup.mutate({ name: name.trim() })}
        >
          {t('lookup.run')}
        </Button>
      </Stack>
      <Typography variant="caption" color="text.secondary">
        {t('lookup.help')}
      </Typography>
      {lookup.error && <ProblemAlert error={lookup.error} sx={{ mt: 1 }} />}
      {lookup.data && (
        <Alert severity={lookup.data.ok ? 'success' : 'warning'} sx={{ mt: 1 }}>
          {lookup.data.addresses.map((a) => `${a.type} ${a.address}`).join('  ') ||
            lookup.data.summary}
        </Alert>
      )}
    </Paper>
  );
}
