import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgCloseIcon as CloseIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
  PgInfoIcon as InfoIcon,
  PgRefreshIcon as RefreshIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogContentText as DialogContentText,
  PgDialogTitle as DialogTitle,
  PgDivider as Divider,
  PgDrawer as Drawer,
  PgIconButton as IconButton,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTooltip as Tooltip,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { ipsecObjectName, objectName } from '@ngfw/schema';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { createMergePatch } from '../../interfaces/model';
import { NativeActions } from '../ikev2-native/NativeActions';
import {
  localizeIpsecSchema,
  proposalFormSchema,
  tunnelChip,
  tunnelChipState,
  tunnelFormSchema,
} from './model';
import {
  ipsecKeys,
  useCandidateIpsec,
  useFreshIpsec,
  useIpsecSas,
  useIpsecTunnels,
  usePatchIpsec,
} from './queries';

const LTR = { dir: 'ltr' } as const;
const TUNNELS = 'tunnels' as const;
const PROPOSALS = 'proposals' as const;
type Section = typeof TUNNELS | typeof PROPOSALS;
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** The one name rule per section (from @ngfw/schema): tunnels ≤ 60 characters (D-089), proposals objectName. */
const nameValid = (section: Section, name: string) =>
  (section === TUNNELS ? ipsecObjectName : objectName).safeParse(name).success;

/** Server pointers `/vpn/ipsec/<section>/<name>/…` → pointers relative to the edited form. */
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

/** The route-based IPsec tab of the VPN page: tunnels with live status, proposals, and an SA inspector. */
export function IpsecPage() {
  const { t } = useTranslation('ipsec');
  const perms = usePermissions();
  const qc = useQueryClient();
  const state = useIpsecTunnels();
  const candidate = useCandidateIpsec();
  const patch = usePatchIpsec();
  const [edit, setEdit] = useState<{
    section: Section;
    name: string;
    isNew: boolean;
  } | null>(null);
  const [confirm, setConfirm] = useState<{
    section: Section;
    name: string;
  } | null>(null);
  const [inspect, setInspect] = useState<string | null>(null);
  // SA changes (EVENT_KIND_IPSEC_SA_CHANGED) refresh the state at once instead of waiting for the poll.
  useTopic('ipsec.events', {
    onBatch: () => {
      void qc.invalidateQueries({ queryKey: ipsecKeys.tunnels });
      void qc.invalidateQueries({ queryKey: ipsecKeys.sas });
    },
  });

  const tunnels = candidate.data?.tunnels ?? {};
  const proposals = candidate.data?.proposals ?? {};
  const liveByTunnel = new Map((state.data?.tunnels ?? []).map((x) => [x.tunnel, x]));
  const tunnelNames = Object.keys(tunnels).sort();
  const proposalNames = Object.keys(proposals).sort();

  const remove = async () => {
    if (!confirm) return;
    try {
      await patch.mutateAsync({ [confirm.section]: { [confirm.name]: null } });
      setConfirm(null);
    } catch {
      // shown in the dialog from patch.error
    }
  };

  return (
    <Box>
      <Stack direction="row" spacing={1} sx={{ mb: 2, alignItems: 'center', flexWrap: 'wrap' }}>
        <Typography variant="h6" sx={{ flexGrow: 1 }}>
          {t('title')}
        </Typography>
        {state.isSuccess && (
          <Tooltip title={t('live.hint')}>
            <Chip
              size="small"
              variant="outlined"
              label={state.data.eventsActive ? t('live.on') : t('live.off')}
              color={state.data.eventsActive ? 'success' : 'default'}
            />
          </Tooltip>
        )}
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
        {perms.editConfig && (
          <Button
            size="small"
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setEdit({ section: TUNNELS, name: '', isNew: true })}
          >
            {t('addTunnel')}
          </Button>
        )}
      </Stack>
      {(candidate.isPending || state.isPending) && <LinearProgress sx={{ mb: 2 }} />}
      {candidate.error !== null && <ProblemAlert error={candidate.error} sx={{ mb: 2 }} />}
      {state.error !== null && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('pskNotice')}
      </Alert>
      {state.data?.charonRestarted && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('restarted')}
        </Alert>
      )}
      {state.data?.pendingAction && (
        // The agent's action text is English and names the engine (D-155): show a fixed translated message only.
        <Alert severity="warning" sx={{ mb: 2 }} data-testid="ipsec-pending-restart">
          {t('pendingRestart')}
        </Alert>
      )}

      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
          {t('tunnels')}
        </Typography>
        {candidate.isSuccess && tunnelNames.length === 0 && (
          <Alert severity="info">{t('emptyTunnels')}</Alert>
        )}
        {tunnelNames.length > 0 && (
          <ReactGeereedMaterial
            hideFilterBuilderComponent
            items={tunnelNames.map((name) => ({ name }))}
            columns={[
              {
                key: 'name',
                headerRenderer: () => t('col.name'),
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  return <>{name}</>;
                },
              },
              {
                key: 'status',
                headerRenderer: () => t('col.status'),
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  const cfg = tunnels[name] as Record<string, unknown>;
                  const chip = tunnelChipState(state.isSuccess, liveByTunnel.get(name));
                  return (
                    <>
                      <StatusChip status={tunnelChip(chip)} label={t(`status.${chip}`)} />
                      {state.data?.daemonVersion === 'vpp-ikev2' &&
                        chip === 'down' &&
                        cfg['enabled'] !== false && <NativeActions tunnel={name} />}
                    </>
                  );
                },
              },
              {
                key: 'peer',
                headerRenderer: () => t('col.peer'),
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  const cfg = tunnels[name] as Record<string, unknown>;
                  return <>{String(cfg['remoteAddr'] ?? '')}</>;
                },
              },
              {
                key: 'proposal',
                headerRenderer: () => t('col.proposal'),
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  const cfg = tunnels[name] as Record<string, unknown>;
                  return <>{String(cfg['proposal'] ?? '')}</>;
                },
              },
              {
                key: 'ikeVersion',
                headerRenderer: () => t('col.ikeVersion'),
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  const cfg = tunnels[name] as Record<string, unknown>;
                  return <>{t('ikeV', { v: Number(cfg['ikeVersion'] ?? 2) })}</>;
                },
              },
              {
                key: 'actions',
                headerRenderer: () => null,
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  return (
                    <>
                      <Tooltip title={t('inspect')}>
                        <IconButton
                          size="small"
                          aria-label={t('inspect')}
                          onClick={() => setInspect(name)}
                        >
                          <InfoIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                      {perms.editConfig && (
                        <>
                          <IconButton
                            size="small"
                            aria-label={t('edit')}
                            onClick={() => setEdit({ section: TUNNELS, name, isNew: false })}
                          >
                            <EditIcon fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            aria-label={t('delete')}
                            onClick={() => setConfirm({ section: TUNNELS, name })}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </>
                      )}
                    </>
                  );
                },
              },
            ]}
            rowKey={({ name }) => name}
            rowProps={({ name }) => ({
              role: 'row',
              'data-testid': `ipsec-tunnel-${name}`,
            })}
            cellProps={(_item, column) => {
              if (column.key === 'name') return { ...LTR };
              if (column.key === 'peer') return { ...LTR };
              if (column.key === 'proposal') return { ...LTR };
              if (column.key === 'actions') return { sx: { whiteSpace: 'nowrap' } };
              return {};
            }}
            tableProps={{ size: 'small', 'aria-label': t('tunnels') }}

            emptyContent={null}
            noFormWrapper
            embedded
            memoEnable={false}
            hidePagination
            hideToolbar
            footer={false}
            search={false}
            sortable={false}
            exportToCSV={false}
          />
        )}
      </Paper>

      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Stack direction="row" sx={{ alignItems: 'center', mb: 1 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 600, flexGrow: 1 }}>
            {t('proposals')}
          </Typography>
          {perms.editConfig && (
            <Button
              size="small"
              startIcon={<AddIcon />}
              onClick={() => setEdit({ section: PROPOSALS, name: '', isNew: true })}
            >
              {t('addProposal')}
            </Button>
          )}
        </Stack>
        {candidate.isSuccess && proposalNames.length === 0 && (
          <Alert severity="info">{t('emptyProposals')}</Alert>
        )}
        {proposalNames.length > 0 && (
          <ReactGeereedMaterial
            hideFilterBuilderComponent
            items={proposalNames.map((name) => ({ name }))}
            columns={[
              {
                key: 'name',
                headerRenderer: () => null,
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  return <>{name}</>;
                },
              },
              {
                key: 'actions',
                headerRenderer: () => null,
                sortable: false,
                filterable: false,
                renderer: (_value, { name }: { name: string }) => {
                  return (
                    <>
                      {perms.editConfig && (
                        <>
                          <IconButton
                            size="small"
                            aria-label={t('edit')}
                            onClick={() =>
                              setEdit({
                                section: PROPOSALS,
                                name,
                                isNew: false,
                              })
                            }
                          >
                            <EditIcon fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            aria-label={t('delete')}
                            onClick={() => setConfirm({ section: PROPOSALS, name })}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </>
                      )}
                    </>
                  );
                },
              },
            ]}
            rowKey={({ name }) => name}
            rowProps={({ name }) => ({
              role: 'row',
              'data-testid': `ipsec-proposal-${name}`,
            })}
            cellProps={(_item, column) => {
              if (column.key === 'name') return { ...LTR };
              if (column.key === 'actions')
                return { sx: { whiteSpace: 'nowrap', textAlign: 'end' } };
              return {};
            }}
            tableProps={{ size: 'small', 'aria-label': t('proposals') }}
            headerRows={null}
            emptyContent={null}
            noFormWrapper
            embedded
            memoEnable={false}
            hidePagination
            hideToolbar
            footer={false}
            search={false}
            sortable={false}
            exportToCSV={false}
          />
        )}
      </Paper>

      {edit && (
        <SectionDialog
          key={`${edit.section}/${edit.name || 'new'}`}
          section={edit.section}
          target={edit}
          existing={edit.section === TUNNELS ? tunnels : proposals}
          onClose={() => setEdit(null)}
        />
      )}
      {confirm && (
        <Dialog open onClose={() => setConfirm(null)} maxWidth="xs" fullWidth>
          <DialogTitle>{t('confirmDelete.title')}</DialogTitle>
          <DialogContent>
            <DialogContentText>
              {t(`confirmDelete.${confirm.section}`, { name: confirm.name })}
            </DialogContentText>
            {patch.error !== null && <ProblemAlert error={patch.error} sx={{ mt: 2 }} />}
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setConfirm(null)}>{t('cancel')}</Button>
            <Button
              color="error"
              variant="contained"
              disabled={patch.isPending}
              onClick={() => void remove()}
            >
              {t('delete')}
            </Button>
          </DialogActions>
        </Dialog>
      )}
      {inspect !== null && <SaDrawer tunnel={inspect} onClose={() => setInspect(null)} />}
    </Box>
  );
}

/** Create or edit `vpn.ipsec.<section>.<name>` with the one schema's form (tunnels or proposals). */
function SectionDialog({
  section,
  target,
  existing,
  onClose,
}: {
  section: Section;
  target: { name: string; isNew: boolean };
  existing: Record<string, unknown>;
  onClose: () => void;
}) {
  const { t } = useTranslation('ipsec');
  const perms = usePermissions();
  const patch = usePatchIpsec();
  const fresh = useFreshIpsec();
  const schema = useMemo<JsonSchema>(
    () =>
      localizeIpsecSchema(section === TUNNELS ? tunnelFormSchema() : proposalFormSchema(), (k, o) =>
        t(k, o ?? {}),
      ),
    [section, t],
  );
  const [name, setName] = useState(target.name);
  const [value] = useState<Record<string, unknown> | undefined>(
    existing[target.name] as Record<string, unknown> | undefined,
  );
  const duplicate = target.isNew && name in existing;
  const nameOk = nameValid(section, name) && !duplicate;

  const save = async (v: unknown) => {
    const current = ((await fresh())[section] ?? {})[name] as Record<string, unknown> | undefined;
    const body =
      current === undefined ? v : createMergePatch(current, v as Record<string, unknown>);
    try {
      await patch.mutateAsync({ [section]: { [name]: body } });
      onClose();
    } catch {
      // rendered from patch.error
    }
  };
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>
        {target.isNew
          ? t(section === TUNNELS ? 'addTunnel' : 'addProposal')
          : t('editTitle', { name })}
      </DialogTitle>
      <DialogContent>
        {target.isNew && (
          <TextField
            label={t('name')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={name !== '' && !nameOk}
            helperText={
              duplicate
                ? t('nameTaken')
                : t(section === TUNNELS ? 'nameHintTunnel' : 'nameHintProposal')
            }
            fullWidth
            sx={{ my: 1 }}
            slotProps={{ htmlInput: LTR }}
          />
        )}
        <SchemaForm
          schema={schema}
          value={value}
          readOnly={!perms.editConfig || !nameOk}
          problem={problemFor(patch.error, `/vpn/ipsec/${section}/${esc(name)}`)}
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

/** SA inspection drawer: the IKE_SA(s) of a tunnel and their CHILD_SAs (SPIs, algorithms, counters). */
function SaDrawer({ tunnel, onClose }: { tunnel: string; onClose: () => void }) {
  const { t } = useTranslation('ipsec');
  const fmt = useFormatters();
  const sas = useIpsecSas(tunnel);
  const mine = (sas.data?.sas ?? []).filter((s) => s.tunnel === tunnel);
  const state = (s: string) => t(`sa.state.${s}`, { defaultValue: s });
  return (
    <Drawer anchor="right" open onClose={onClose} data-testid={`ipsec-sa-${tunnel}`}>
      <Box sx={{ inlineSize: { xs: '100vw', sm: 420 }, p: 2 }}>
        <Stack direction="row" sx={{ alignItems: 'center', mb: 1 }}>
          <Typography variant="h6" sx={{ flexGrow: 1 }}>
            {t('sa.title', { name: tunnel })}
          </Typography>
          <IconButton aria-label={t('close')} onClick={onClose}>
            <CloseIcon />
          </IconButton>
        </Stack>
        {sas.isPending && <LinearProgress sx={{ mb: 2 }} />}
        {sas.error !== null && <ProblemAlert error={sas.error} sx={{ mb: 2 }} />}
        {sas.isSuccess && mine.length === 0 && <Alert severity="info">{t('sa.none')}</Alert>}
        {mine.map((sa) => {
          const crypto = [
            sa.encrAlg + (sa.encrKeysize ? `-${sa.encrKeysize}` : ''),
            sa.integAlg,
            sa.prfAlg,
            sa.dhGroup,
          ]
            .filter(Boolean)
            .join(' ');
          return (
            <Paper key={sa.uniqueId} variant="outlined" sx={{ p: 1.5, mb: 2 }}>
              <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 1 }}>
                <Typography variant="subtitle2">{t('sa.ike', { id: sa.uniqueId })}</Typography>
                <Chip size="small" label={state(sa.state)} />
                <Typography variant="body2" color="text.secondary">
                  {t('ikeV', { v: Number(sa.version) })}
                </Typography>
              </Stack>
              <Typography variant="body2" dir="ltr">
                {`${sa.localHost}:${sa.localPort} ↔ ${sa.remoteHost}:${sa.remotePort}`}
              </Typography>
              <Typography variant="body2">{t('sa.crypto', { crypto })}</Typography>
              <Typography variant="body2">
                {t('sa.rekeyIn', { s: fmt.integer(sa.rekeySec) })}
              </Typography>
              {sas.data?.daemonVersion === 'vpp-ikev2' && (
                <NativeActions tunnel={tunnel} ikeSpi={sa.uniqueId} />
              )}
              <Divider sx={{ my: 1 }} />
              {sa.children.map((ch) => (
                <Box key={ch.uniqueId} sx={{ mb: 1 }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      {t('sa.child', { proto: ch.protocol, id: ch.uniqueId })}
                    </Typography>
                    <Chip size="small" label={state(ch.state)} />
                    {ch.encap && <Chip size="small" variant="outlined" label={t('sa.natt')} />}
                  </Stack>
                  <Typography variant="body2" dir="ltr">
                    {t('sa.spi', { in: ch.spiIn, out: ch.spiOut })}
                  </Typography>
                  <Typography variant="body2" dir="ltr">
                    {`${(ch.localTs ?? []).join(' ')} ↔ ${(ch.remoteTs ?? []).join(' ')}`}
                  </Typography>
                  <Typography variant="body2">
                    {t('sa.traffic', {
                      rx: fmt.integer(ch.bytesIn),
                      tx: fmt.integer(ch.bytesOut),
                    })}
                  </Typography>
                  {sas.data?.daemonVersion === 'vpp-ikev2' && (
                    <NativeActions
                      tunnel={tunnel}
                      ikeSpi={sa.uniqueId}
                      childSpi={ch.spiIn}
                      initiator={sa.initiator}
                    />
                  )}
                </Box>
              ))}
            </Paper>
          );
        })}
      </Box>
    </Drawer>
  );
}
