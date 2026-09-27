import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { domainSchemas } from '../../../schema/registry';
import { PageHeader } from '../../../shell/PageHeader';
import { NS } from './locale';
import {
  useCandidateHa,
  usePatchHa,
  useRunningHa,
  vrrpPatch,
  type HaCluster,
  type VrrpRouter,
} from './queries';


const VRRP = 'vrrp' as const;
const CLUSTER = 'cluster' as const;
const VRRP_BASE = '/ha/vrrp';
const CLUSTER_BASE = '/ha/cluster';
const DEFAULT_FAMILY = 'ipv4';
const DEFAULT_ENGINE = 'vpp';

/** A sub-schema of the `ha` domain — the one schema, never a hand-written form. */
export function haSubSchema(key: 'vrrp' | 'cluster'): JsonSchema {
  const ha = domainSchemas.ha as { properties?: Record<string, JsonSchema> };
  const s = ha.properties?.[key];
  if (!s) throw new Error(`ha.${key} schema not found`);
  return s;
}

/** Server problem with pointers made relative to a sub-form (`/ha/cluster/port` → `/port`). */
export function problemUnder(error: unknown, base: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(base) ? e.pointer.slice(base.length) || '/' : e.pointer,
    })),
  };
}

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

function VrrpTab() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const cand = useCandidateHa();
  const running = useRunningHa();
  const patch = usePatchHa();
  const vrrp = cand.data?.vrrp ?? {};
  const runVrrp = running.data?.vrrp ?? {};
  const entries = Object.entries(vrrp) as [string, VrrpRouter][];
  return (
    <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
      {cand.isSuccess && (
        <Paper variant="outlined" sx={{ p: 2, flex: 1, maxWidth: 720, width: '100%' }}>
          <SchemaForm
            id="ha-vrrp-form"
            schema={haSubSchema(VRRP)}
            value={vrrp}
            readOnly={!perms.editConfig}
            onSubmit={async (v) => {
              const body = { vrrp: vrrpPatch(vrrp, (v ?? {}) as Record<string, unknown>) };
              await patch.mutateAsync(body).catch(() => undefined);
            }}
            problem={patch.error ? problemUnder(patch.error, VRRP_BASE) : null}
            submitLabel={t('save')}
          />
        </Paper>
      )}
      <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
        <Typography component="h3" variant="h6" gutterBottom>
          {t('vrrp.title')}
        </Typography>
        {entries.length === 0 ? (
          <Typography color="text.secondary">{t('vrrp.empty')}</Typography>
        ) : (
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small" aria-label={t('vrrp.title')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('vrrp.name')}</TableCell>
                  <TableCell>{t('vrrp.interface')}</TableCell>
                  <TableCell>{t('vrrp.vrId')}</TableCell>
                  <TableCell>{t('vrrp.family')}</TableCell>
                  <TableCell>{t('vrrp.priority')}</TableCell>
                  <TableCell>{t('vrrp.addresses')}</TableCell>
                  <TableCell>{t('vrrp.engine')}</TableCell>
                  <TableCell>{t('vrrp.status')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {entries.map(([name, r]) => (
                  <TableRow key={name} data-testid={`vrrp-${name}`}>
                    <TableCell dir="ltr">{name}</TableCell>
                    <TableCell dir="ltr">{r.interface}</TableCell>
                    <TableCell dir="ltr">{r.vrId}</TableCell>
                    <TableCell dir="ltr">{r.addressFamily ?? DEFAULT_FAMILY}</TableCell>
                    <TableCell dir="ltr">{r.priority ?? 100}</TableCell>
                    <TableCell dir="ltr">{r.addresses.join(', ')}</TableCell>
                    <TableCell dir="ltr">{r.engine ?? DEFAULT_ENGINE}</TableCell>
                    <TableCell>
                      <Stack direction="row" spacing={1}>
                        {r.enabled === false && <Chip size="small" label={t('vrrp.disabled')} />}
                        {running.isSuccess && (
                          <Chip
                            size="small"
                            color={same(r, runVrrp[name]) ? 'success' : 'warning'}
                            label={same(r, runVrrp[name]) ? t('vrrp.committed') : t('vrrp.pending')}
                          />
                        )}
                      </Stack>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Box>
        )}
      </Paper>
    </Stack>
  );
}

function ClusterTab() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const cand = useCandidateHa();
  const running = useRunningHa();
  const patch = usePatchHa();
  const c: HaCluster | undefined = cand.data?.cluster;
  const onOff = (v: boolean | undefined) => (v ? t('cluster.on') : t('cluster.off'));
  return (
    <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
      {cand.isSuccess && (
        <Paper variant="outlined" sx={{ p: 2, flex: 1, maxWidth: 720, width: '100%' }}>
          <SchemaForm
            id="ha-cluster-form"
            schema={haSubSchema(CLUSTER)}
            value={c ?? {}}
            readOnly={!perms.editConfig}
            onSubmit={async (v) => {
              await patch.mutateAsync({ cluster: v }).catch(() => undefined);
            }}
            problem={patch.error ? problemUnder(patch.error, CLUSTER_BASE) : null}
            submitLabel={t('save')}
          />
        </Paper>
      )}
      <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }} data-testid="ha-cluster">
        <Typography component="h3" variant="h6" gutterBottom>
          {t('cluster.title')}
          {running.isSuccess && !same(c, running.data?.cluster) && (
            <Chip size="small" color="warning" label={t('cluster.pending')} sx={{ ms: 1 }} />
          )}
        </Typography>
        {!c ? (
          <Typography color="text.secondary">{t('cluster.notConfigured')}</Typography>
        ) : (
          <Table size="small" aria-label={t('cluster.title')}>
            <TableBody>
              <TableRow>
                <TableCell>{t('cluster.node')}</TableCell>
                <TableCell dir="ltr">{c.nodeName}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t('cluster.peers')}</TableCell>
                <TableCell dir="ltr">
                  {c.peers.map((p) => `${p.name} (${p.address})`).join(', ') || t('none')}
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t('cluster.port')}</TableCell>
                <TableCell dir="ltr">{c.port ?? 4370}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t('cluster.configSync')}</TableCell>
                <TableCell>{onOff(c.configSync ?? true)}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t('cluster.stateSync')}</TableCell>
                <TableCell dir="ltr">
                  {`NAT: ${onOff(c.stateSync?.nat)}, IPsec: ${onOff(c.stateSync?.ipsec)}, ACL: ${onOff(c.stateSync?.acl)}`}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        )}
      </Paper>
    </Stack>
  );
}

/**
 * System › High availability (WEB-4b, D-123): VRRPv3 virtual routers and cluster membership, schema-driven from
 * the `ha` domain. Merged UNROUTED — F-vrrp-config-sync adds the route/nav entry and the live VRRP state.
 */
export function HaPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateHa();
  const running = useRunningHa();
  const [tab, setTab] = useState<'vrrp' | 'cluster'>('vrrp');
  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs value={tab} onChange={(_, v: 'vrrp' | 'cluster') => setTab(v)} sx={{ mb: 2 }}>
        <Tab value={VRRP} label={t('tabs.vrrp')} />
        <Tab value={CLUSTER} label={t('tabs.cluster')} />
      </Tabs>
      {tab === 'vrrp' ? <VrrpTab /> : <ClusterTab />}
    </PageHeader>
  );
}
