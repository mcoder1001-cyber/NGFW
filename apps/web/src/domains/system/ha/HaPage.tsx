import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTabs as Tabs,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { domainSchemas } from '../../../schema/registry';
import { PageHeader } from '../../../shell/PageHeader';
import { clusterPanels } from './clusterPanels';
import { NS } from './locale';
import {
  useCandidateHa,
  useClusterRuntime,
  useForceClusterSync,
  usePatchHa,
  useRunningHa,
  useVrrpRuntime,
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
  const live = useVrrpRuntime();
  const roles = live.isError ? [] : (live.data?.routers ?? []);
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
        {live.isError && <ProblemAlert error={live.error} />}
        {entries.length === 0 ? (
          <Typography color="text.secondary">{t('vrrp.empty')}</Typography>
        ) : (
          <Box sx={{ overflowX: 'auto' }}>
            <ReactGeereedMaterial
              tableProps={{ size: 'small', 'aria-label': t('vrrp.title') }}
              columns={[
                {
                  key: 'column0',
                  headerRenderer: () => t('vrrp.name'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column1',
                  headerRenderer: () => t('vrrp.interface'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column2',
                  headerRenderer: () => t('vrrp.vrId'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column3',
                  headerRenderer: () => t('vrrp.family'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column4',
                  headerRenderer: () => t('vrrp.priority'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column5',
                  headerRenderer: () => t('vrrp.currentPriority'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column6',
                  headerRenderer: () => t('vrrp.masterInterval'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column7',
                  headerRenderer: () => t('vrrp.addresses'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column8',
                  headerRenderer: () => t('vrrp.engine'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column9',
                  headerRenderer: () => t('vrrp.status'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
              ]}
              items={[
                ...(entries.map(([name, r]) => ({
                  id: name,
                  column0: name,
                  column1: r.interface,
                  column2: r.vrId,
                  column3: r.addressFamily ?? DEFAULT_FAMILY,
                  column4: r.priority ?? 100,
                  column5: roles.find((v) => v.name === name)?.currentPriority || t('none'),
                  column6:
                    roles.find((v) => v.name === name)?.masterAdvertisementIntervalMs || t('none'),
                  column7: r.addresses.join(', '),
                  column8: t(`vrrp.engineLabels.${r.engine ?? DEFAULT_ENGINE}`),
                  column9: (
                    <>
                      <Stack direction="row" spacing={1}>
                        <Chip
                          size="small"
                          color={
                            roles.find((v) => v.name === name)?.state === 'master'
                              ? 'success'
                              : 'default'
                          }
                          label={t(
                            `vrrp.roles.${roles.find((v) => v.name === name)?.error ? 'unknown' : (roles.find((v) => v.name === name)?.state ?? 'unknown')}`,
                            { defaultValue: t('vrrp.roles.unknown') },
                          )}
                        />
                        {r.enabled === false && <Chip size="small" label={t('vrrp.disabled')} />}
                        {running.isSuccess && (
                          <Chip
                            size="small"
                            color={same(r, runVrrp[name]) ? 'success' : 'warning'}
                            label={same(r, runVrrp[name]) ? t('vrrp.committed') : t('vrrp.pending')}
                          />
                        )}
                      </Stack>
                    </>
                  ),
                  rowProps: { 'data-testid': `vrrp-${name}` } satisfies ReturnType<
                    NonNullable<GeereedTablePassthrough['rowProps']>
                  > &
                    Record<`data-${string}`, unknown>,
                  cellProps: {
                    column0: { dir: 'ltr' },
                    column1: { dir: 'ltr' },
                    column2: { dir: 'ltr' },
                    column3: { dir: 'ltr' },
                    column4: { dir: 'ltr' },
                    column7: { dir: 'ltr' },
                  } satisfies Record<
                    string,
                    NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                  >,
                })) ?? []),
              ]}
              rowKey={(item) => item.id}
              rowProps={(item) => item.rowProps ?? {}}
              cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
              emptyContent={null}
              noFormWrapper
              embedded
              hideToolbar
              hidePagination
              hideFilterBuilderComponent
              search={false}
              sortable={false}
              exportToCSV={false}
              footer={false}
              memoEnable={false}
            />
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
  const live = useClusterRuntime();
  const force = useForceClusterSync();
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
        {live.isError && <ProblemAlert error={live.error} />}
        {force.isError && <ProblemAlert error={force.error} />}
        <Button
          disabled={!perms.commit || force.isPending || live.isError || !live.data?.enabled}
          onClick={() => void force.mutateAsync().catch(() => undefined)}
        >
          {t('cluster.force')}
        </Button>
        <Typography>
          {t('cluster.revision')}: {live.isError ? t('none') : (live.data?.revision ?? t('none'))}
        </Typography>
        {!live.isError && live.data?.members && (
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('cluster.live') }}
            columns={[
              {
                key: 'column0',
                headerRenderer: () => t('cluster.peers'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column1',
                headerRenderer: () => t('cluster.role'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column2',
                headerRenderer: () => t('cluster.revision'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column3',
                headerRenderer: () => t('cluster.lag'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column4',
                headerRenderer: () => t('cluster.syncStatus'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(live.data.members.map((peer) => ({
                id: peer.name,
                column0: peer.name,
                column1: t(`vrrp.roles.${peer.role ?? 'unknown'}`),
                column2: peer.revision ?? t('none'),
                column3: peer.lag ?? t('none'),
                column4: peer.error || (peer.syncedAt ? t('cluster.synced') : t('cluster.waiting')),
                cellProps: { column0: { dir: 'ltr' } } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              })) ?? []),
            ]}
            rowKey={(item) => item.id}
            rowProps={(item) => item.rowProps ?? {}}
            cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
            emptyContent={null}
            noFormWrapper
            embedded
            hideToolbar
            hidePagination
            hideFilterBuilderComponent
            search={false}
            sortable={false}
            exportToCSV={false}
            footer={false}
            memoEnable={false}
          />
        )}
        {clusterPanels.map((Panel, i) => (
          <Panel key={i} />
        ))}
        {!c ? (
          <Typography color="text.secondary">{t('cluster.notConfigured')}</Typography>
        ) : (
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('cluster.title') }}
            columns={[
              { key: 'label', filterable: false, renderer: (value) => <>{value}</> },
              { key: 'value', filterable: false, renderer: (value) => <>{value}</> },
            ]}
            items={[
              {
                id: 0,
                label: t('cluster.node'),
                value: c.nodeName,
                cellProps: { value: { dir: 'ltr' } } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              },
              {
                id: 1,
                label: t('cluster.peers'),
                value: c.peers.map((p) => `${p.name} (${p.address})`).join(', ') || t('none'),
                cellProps: { value: { dir: 'ltr' } } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              },
              {
                id: 2,
                label: t('cluster.port'),
                value: c.port ?? 4370,
                cellProps: { value: { dir: 'ltr' } } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              },
              { id: 3, label: t('cluster.configSync'), value: onOff(c.configSync ?? true) },
              {
                id: 4,
                label: t('cluster.stateSync'),
                value: `NAT: ${onOff(c.stateSync?.nat)}, IPsec: ${onOff(c.stateSync?.ipsec)}, ACL: ${onOff(c.stateSync?.acl)}`,
                cellProps: { value: { dir: 'ltr' } } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              },
            ]}
            rowKey={(item) => item.id}
            rowProps={(item) => item.rowProps ?? {}}
            cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
            headerRows={<></>}
            emptyContent={null}
            noFormWrapper
            embedded
            hideToolbar
            hidePagination
            hideFilterBuilderComponent
            search={false}
            sortable={false}
            exportToCSV={false}
            footer={false}
            memoEnable={false}
          />
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
